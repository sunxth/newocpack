package catalognew

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/pelletier/go-toml/v2"
	"github.com/spf13/cobra"

	"ocpack/pkg/mirror/api/v2alpha1"
	clog "ocpack/pkg/mirror/log"
	"ocpack/pkg/mirror/manifest"
	"ocpack/pkg/mirror/mirror"
	"ocpack/pkg/mirror/operator"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// OperatorInfo 表示 Operator 信息
type OperatorInfo struct {
	Name           string `json:"name"`
	DisplayName    string `json:"displayName"`
	DefaultChannel string `json:"defaultChannel"`
	Version        string `json:"version,omitempty"`
	Description    string `json:"description,omitempty"`
}

// CatalogManagerNew 基于 pkg/mirror 的新 catalog 管理器
type CatalogManagerNew struct {
	CatalogImage    string
	CacheDir        string
	WorkingDir      string
	LockFile        string
	CacheFile       string
	DownloadTimeout time.Duration
	Logger          clog.PluggableLoggerInterface
	AuthFile        string
}

// NewCatalogManagerNew 创建新的 catalog 管理器
func NewCatalogManagerNew(catalogImage, cacheDir, workingDir string) *CatalogManagerNew {
	// 基于目录镜像生成唯一的缓存文件名
	safeName := strings.ReplaceAll(strings.ReplaceAll(catalogImage, "/", "_"), ":", "_")

	logger := clog.New("info")

	return &CatalogManagerNew{
		CatalogImage:    catalogImage,
		CacheDir:        cacheDir,
		WorkingDir:      workingDir,
		LockFile:        filepath.Join(cacheDir, fmt.Sprintf(".%s.lock", safeName)),
		CacheFile:       filepath.Join(cacheDir, fmt.Sprintf("%s_new.json", safeName)),
		DownloadTimeout: 10 * time.Minute,
		Logger:          logger,
	}
}

// GetOperatorInfo 获取指定 Operator 信息
func (cm *CatalogManagerNew) GetOperatorInfo(operatorName string) (*OperatorInfo, error) {
	operators, err := cm.GetAllOperators()
	if err != nil {
		return nil, err
	}

	for _, op := range operators {
		if op.Name == operatorName {
			return &op, nil
		}
	}

	return nil, fmt.Errorf("未找到 Operator: %s", operatorName)
}

// GetAllOperators 获取所有 Operator 信息
func (cm *CatalogManagerNew) GetAllOperators() ([]OperatorInfo, error) {
	// 确保缓存目录存在
	if err := os.MkdirAll(cm.CacheDir, 0755); err != nil {
		return nil, fmt.Errorf("创建缓存目录失败: %w", err)
	}

	// 检查缓存是否存在且有效
	if cm.isCacheValid() {
		cm.Logger.Info("使用缓存的 Operator 索引: %s", cm.CacheFile)
		return cm.readAllOperatorsFromCache()
	}

	cm.Logger.Info("下载 Operator 索引: %s", cm.CatalogImage)
	operators, err := cm.downloadCatalogUsingMirror()
	if err != nil {
		return nil, fmt.Errorf("下载目录索引失败: %w", err)
	}

	// 写入缓存
	if err := cm.writeOperatorsToCache(operators); err != nil {
		return nil, fmt.Errorf("写入缓存失败: %w", err)
	}

	cm.Logger.Info("成功下载并缓存了 %d 个 Operator 信息", len(operators))
	return operators, nil
}

// downloadCatalogUsingMirror 使用 pkg/mirror 下载 catalog
func (cm *CatalogManagerNew) downloadCatalogUsingMirror() ([]OperatorInfo, error) {
	// 创建临时工作目录
	tempWorkingDir := filepath.Join(cm.WorkingDir, "temp-catalog", fmt.Sprintf("%d", time.Now().Unix()))
	if err := os.MkdirAll(tempWorkingDir, 0755); err != nil {
		return nil, fmt.Errorf("创建临时工作目录失败: %w", err)
	}
	defer os.RemoveAll(tempWorkingDir) // 清理临时目录

	// 创建 ImageSetConfiguration
	imageSetConfig := v2alpha1.ImageSetConfiguration{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "mirror.openshift.io/v2alpha1",
			Kind:       "ImageSetConfiguration",
		},
		ImageSetConfigurationSpec: v2alpha1.ImageSetConfigurationSpec{
			Mirror: v2alpha1.Mirror{
				Operators: []v2alpha1.Operator{
					{
						Catalog: cm.CatalogImage,
						Full:    true, // 获取完整 catalog
					},
				},
			},
		},
	}

	// 创建选项组件
	globalOpts := &mirror.GlobalOptions{
		WorkingDir:   tempWorkingDir,
		SecurePolicy: false,
		IsTerminal:   false,
	}

	// 创建共享选项
	_, sharedOpts := mirror.SharedImageFlags()
	// 设置认证文件
	if cm.AuthFile != "" {
		// 注意：这里需要直接访问字段，因为是私有字段
		// 我们需要另一种方式来设置认证
		// 可以通过设置环境变量或者使用其他方法
		os.Setenv("REGISTRY_AUTH_FILE", cm.AuthFile)
	}
	_, deprecatedTLSVerifyOpt := mirror.DeprecatedTLSVerifyFlags()
	_, srcOpts := mirror.ImageSrcFlags(globalOpts, sharedOpts, deprecatedTLSVerifyOpt, "", "")
	_, destOpts := mirror.ImageDestFlags(globalOpts, sharedOpts, deprecatedTLSVerifyOpt, "", "")
	_, retryOpts := mirror.RetryFlags()

	// 设置复制选项
	copyOpts := &mirror.CopyOptions{
		Global:              globalOpts,
		DeprecatedTLSVerify: deprecatedTLSVerifyOpt,
		SrcImage:            srcOpts,
		DestImage:           destOpts,
		RetryOpts:           retryOpts,
		Mode:                mirror.MirrorToDisk,
		LocalStorageFQDN:    "localhost:5000", // 临时本地存储
	}

	// 创建所需的组件
	manifestHandler := manifest.New(cm.Logger)
	mirrorCopy := mirror.NewMirrorCopy()
	mirrorHandler := mirror.New(mirrorCopy, nil)

	// 创建 operator collector
	operatorCollector := operator.New(
		cm.Logger,
		tempWorkingDir,
		imageSetConfig,
		*copyOpts,
		mirrorHandler,
		manifestHandler,
	)

	ctx, cancel := context.WithTimeout(context.Background(), cm.DownloadTimeout)
	defer cancel()

	// 收集 operator 信息
	collectorSchema, err := operatorCollector.OperatorImageCollector(ctx)
	if err != nil {
		return nil, fmt.Errorf("收集 operator 信息失败: %w", err)
	}

	// 转换为 OperatorInfo 格式
	return cm.convertToOperatorInfo(collectorSchema)
}

// convertToOperatorInfo 将 mirror 数据转换为 OperatorInfo
func (cm *CatalogManagerNew) convertToOperatorInfo(schema v2alpha1.CollectorSchema) ([]OperatorInfo, error) {
	var operators []OperatorInfo

	cm.Logger.Debug("开始解析 CollectorSchema，AllImages 数量: %d, CatalogToFBCMap 数量: %d",
		len(schema.AllImages), len(schema.CatalogToFBCMap))

	// 方式1：从 CatalogToFBCMap 中解析 FBC 数据
	for catalogRef, catalogResult := range schema.CatalogToFBCMap {
		cm.Logger.Debug("处理 catalog: %s", catalogRef)

		if catalogResult.DeclConfig != nil {
			// 解析 FBC 中的 Package 信息
			for _, pkg := range catalogResult.DeclConfig.Packages {
				operator := OperatorInfo{
					Name:           pkg.Name,
					DisplayName:    pkg.Name, // FBC 中通常没有单独的 displayName
					DefaultChannel: pkg.DefaultChannel,
					Description:    pkg.Description,
				}
				operators = append(operators, operator)
				cm.Logger.Debug("从 FBC 解析到 operator: %s", pkg.Name)
			}
		}
	}

	// 方式2：如果从 CatalogToFBCMap 没有解析到数据，从 RelatedImages 中获取
	if len(operators) == 0 {
		cm.Logger.Debug("从 CatalogToFBCMap 未获取到数据，尝试从 AllImages 解析")

		// 使用 map 去重
		operatorMap := make(map[string]OperatorInfo)

		for _, image := range schema.AllImages {
			if image.Type == v2alpha1.TypeOperatorBundle {
				// 从 bundle 镜像中提取 operator 信息
				operatorName := extractOperatorNameFromBundle(image.Origin)
				if operatorName != "" {
					operatorMap[operatorName] = OperatorInfo{
						Name:           operatorName,
						DisplayName:    operatorName,
						DefaultChannel: "stable",
						Description:    fmt.Sprintf("Operator bundle: %s", image.Origin),
					}
				}
			} else if image.Type == v2alpha1.TypeOperatorCatalog {
				// 处理 catalog 镜像
				catalogName := extractCatalogName(image.Origin)
				operatorMap[catalogName+"-catalog"] = OperatorInfo{
					Name:           catalogName + "-catalog",
					DisplayName:    catalogName + " Catalog",
					DefaultChannel: "stable",
					Description:    fmt.Sprintf("Catalog image: %s", image.Origin),
				}
			}
		}

		// 转换 map 为 slice
		for _, operator := range operatorMap {
			operators = append(operators, operator)
		}
	}

	// 方式3：如果前两种方式都没有数据，创建基本的 catalog 条目
	if len(operators) == 0 {
		cm.Logger.Debug("未从任何源解析到 operator 数据，创建基本 catalog 条目")

		catalogName := extractCatalogName(cm.CatalogImage)
		operators = append(operators, OperatorInfo{
			Name:           catalogName + "-catalog",
			DisplayName:    catalogName + " Catalog",
			DefaultChannel: "stable",
			Description:    fmt.Sprintf("Catalog image: %s", cm.CatalogImage),
		})
	}

	cm.Logger.Debug("共解析到 %d 个 operator", len(operators))
	return operators, nil
}

// extractOperatorNameFromBundle 从 bundle 镜像路径中提取 operator 名称
func extractOperatorNameFromBundle(bundleImage string) string {
	// 移除协议前缀
	ref := strings.TrimPrefix(bundleImage, "docker://")
	ref = strings.TrimPrefix(ref, "oci://")

	// 提取路径部分，通常格式为: registry/namespace/operator-bundle:tag
	parts := strings.Split(ref, "/")
	if len(parts) >= 2 {
		// 获取最后一部分，通常是 operator-bundle:tag 格式
		bundlePart := parts[len(parts)-1]
		// 移除标签
		bundlePart = strings.Split(bundlePart, ":")[0]
		bundlePart = strings.Split(bundlePart, "@")[0]

		// 尝试移除常见的 bundle 后缀
		if strings.HasSuffix(bundlePart, "-bundle") {
			return strings.TrimSuffix(bundlePart, "-bundle")
		}
		return bundlePart
	}

	return ""
}

// extractCatalogName 从 catalog 引用中提取名称
func extractCatalogName(catalogRef string) string {
	// 移除协议前缀
	ref := strings.TrimPrefix(catalogRef, "docker://")
	ref = strings.TrimPrefix(ref, "oci://")

	// 提取最后一部分作为名称
	parts := strings.Split(ref, "/")
	if len(parts) > 0 {
		name := parts[len(parts)-1]
		// 移除标签或摘要
		name = strings.Split(name, ":")[0]
		name = strings.Split(name, "@")[0]
		return name
	}

	return "unknown"
}

// isCacheValid 检查缓存是否有效（24小时内）
func (cm *CatalogManagerNew) isCacheValid() bool {
	info, err := os.Stat(cm.CacheFile)
	if err != nil {
		return false
	}

	// 缓存有效期为24小时
	return time.Since(info.ModTime()) < 24*time.Hour
}

// writeOperatorsToCache 将 Operator 信息写入缓存
func (cm *CatalogManagerNew) writeOperatorsToCache(operators []OperatorInfo) error {
	// 先写入临时文件
	tempCacheFile := cm.CacheFile + ".tmp"
	file, err := os.Create(tempCacheFile)
	if err != nil {
		return fmt.Errorf("创建临时缓存文件失败: %w", err)
	}

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")

	if err := encoder.Encode(operators); err != nil {
		file.Close()
		os.Remove(tempCacheFile)
		return fmt.Errorf("编码 JSON 失败: %w", err)
	}

	file.Close()

	// 原子性重命名操作
	if err := os.Rename(tempCacheFile, cm.CacheFile); err != nil {
		os.Remove(tempCacheFile)
		return fmt.Errorf("重命名缓存文件失败: %w", err)
	}

	return nil
}

// readAllOperatorsFromCache 从缓存中读取所有 Operator 信息
func (cm *CatalogManagerNew) readAllOperatorsFromCache() ([]OperatorInfo, error) {
	file, err := os.Open(cm.CacheFile)
	if err != nil {
		return nil, fmt.Errorf("打开缓存文件失败: %w", err)
	}
	defer file.Close()

	var operators []OperatorInfo
	decoder := json.NewDecoder(file)

	if err := decoder.Decode(&operators); err != nil {
		return nil, fmt.Errorf("解码缓存文件失败: %w", err)
	}

	return operators, nil
}

// ListOperators 列出 catalog 中的所有 operators
func (cm *CatalogManagerNew) ListOperators() error {
	operators, err := cm.GetAllOperators()
	if err != nil {
		return err
	}

	if len(operators) == 0 {
		fmt.Println("未找到任何 Operator")
		return nil
	}

	fmt.Printf("找到 %d 个 Operator:\n\n", len(operators))
	for _, op := range operators {
		fmt.Printf("名称: %s\n", op.Name)
		fmt.Printf("显示名称: %s\n", op.DisplayName)
		fmt.Printf("默认通道: %s\n", op.DefaultChannel)
		if op.Description != "" {
			fmt.Printf("描述: %s\n", op.Description)
		}
		fmt.Println("---")
	}

	return nil
}

// NewCatalogCommand 创建 catalog 命令
func NewCatalogCommand() *cobra.Command {
	var catalogImage string
	var cacheDir string
	var workingDir string
	var authFile string

	cmd := &cobra.Command{
		Use:   "catalognew",
		Short: "基于 pkg/mirror 的新 catalog 管理功能",
		Long:  "使用 pkg/mirror 下载和管理 operator catalog 信息。可以指定集群名称自动读取配置，或使用 --catalog 手动指定镜像地址。",
	}

	listCmd := &cobra.Command{
		Use:   "list [cluster-name]",
		Short: "列出 catalog 中的所有 operators",
		Long: `列出 catalog 中的所有 operators

使用方法:
  ocpack catalognew list demo                    # 从集群配置读取
  ocpack catalognew list --catalog <image>      # 手动指定镜像`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// 确定工作模式：集群模式 vs 手动模式
			var clusterName string
			var finalCatalogImage string
			var finalCacheDir string
			var finalWorkingDir string

			if len(args) > 0 {
				// 集群模式：从集群配置读取
				clusterName = args[0]
				configPath := filepath.Join(clusterName, "config.toml")

				// 检查配置文件是否存在
				if _, err := os.Stat(configPath); err != nil {
					return fmt.Errorf("集群配置文件不存在: %s\n请先运行 'ocpack new cluster %s' 创建集群配置", configPath, clusterName)
				}

				// 读取集群配置
				config, err := loadClusterConfig(configPath)
				if err != nil {
					return fmt.Errorf("读取集群配置失败: %w", err)
				}

				// 从配置中获取 catalog 镜像地址
				finalCatalogImage = config.SaveImage.OperatorCatalog
				if finalCatalogImage == "" {
					// 如果配置中没有指定，根据版本自动生成
					finalCatalogImage = fmt.Sprintf("registry.redhat.io/redhat/redhat-operator-index:v%s",
						extractMajorMinorVersion(config.ClusterInfo.OpenShiftVersion))
				}

				// 设置集群专用的缓存和工作目录
				finalCacheDir = filepath.Join(clusterName, "catalogs", "cache")
				finalWorkingDir = filepath.Join(clusterName, "catalogs", "working")

				fmt.Printf("🔗 使用集群配置: %s\n", configPath)
				fmt.Printf("📋 OpenShift 版本: %s\n", config.ClusterInfo.OpenShiftVersion)
				fmt.Printf("📦 Catalog 镜像: %s\n", finalCatalogImage)
			} else {
				// 手动模式：使用 --catalog 参数
				if catalogImage == "" {
					return fmt.Errorf("必须指定集群名称或使用 --catalog 参数\n\n使用方法:\n  ocpack catalognew list <cluster-name>          # 从集群配置读取\n  ocpack catalognew list --catalog <image>       # 手动指定镜像")
				}

				finalCatalogImage = catalogImage
				finalCacheDir = cacheDir
				if finalCacheDir == "" {
					finalCacheDir = "/tmp/ocpack-catalog-cache"
				}
				finalWorkingDir = workingDir
				if finalWorkingDir == "" {
					finalWorkingDir = "/tmp/ocpack-working"
				}
			}

			// 创建目录
			if err := os.MkdirAll(finalCacheDir, 0755); err != nil {
				return fmt.Errorf("创建缓存目录失败: %w", err)
			}
			if err := os.MkdirAll(finalWorkingDir, 0755); err != nil {
				return fmt.Errorf("创建工作目录失败: %w", err)
			}

			manager := NewCatalogManagerNew(finalCatalogImage, finalCacheDir, finalWorkingDir)
			// 设置认证文件
			if authFile != "" {
				manager.AuthFile = authFile
			}

			err := manager.ListOperators()
			if err != nil {
				return err
			}

			// 如果是集群模式，额外保存一份 JSON 缓存到集群目录
			if clusterName != "" {
				operators, err := manager.GetAllOperators()
				if err != nil {
					fmt.Printf("⚠️  获取 operator 列表失败: %v\n", err)
					return nil
				}

				// 保存到集群目录
				clusterCatalogFile := filepath.Join(clusterName, "operators.json")
				if err := manager.saveOperatorsToClusterDir(operators, clusterCatalogFile); err != nil {
					fmt.Printf("⚠️  保存 operator 列表到集群目录失败: %v\n", err)
				} else {
					fmt.Printf("💾 已保存 operator 列表到: %s\n", clusterCatalogFile)
				}
			}

			return nil
		},
	}

	infoCmd := &cobra.Command{
		Use:   "info <operator-name> [cluster-name]",
		Short: "获取指定 operator 的详细信息",
		Long: `获取指定 operator 的详细信息

使用方法:
  ocpack catalognew info operator-name demo      # 从集群配置读取
  ocpack catalognew info operator-name --catalog <image>  # 手动指定镜像`,
		Args: cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			operatorName := args[0]
			var clusterName string
			var finalCatalogImage string
			var finalCacheDir string
			var finalWorkingDir string

			if len(args) > 1 {
				// 集群模式
				clusterName = args[1]
				configPath := filepath.Join(clusterName, "config.toml")

				if _, err := os.Stat(configPath); err != nil {
					return fmt.Errorf("集群配置文件不存在: %s", configPath)
				}

				config, err := loadClusterConfig(configPath)
				if err != nil {
					return fmt.Errorf("读取集群配置失败: %w", err)
				}

				finalCatalogImage = config.SaveImage.OperatorCatalog
				if finalCatalogImage == "" {
					finalCatalogImage = fmt.Sprintf("registry.redhat.io/redhat/redhat-operator-index:v%s",
						extractMajorMinorVersion(config.ClusterInfo.OpenShiftVersion))
				}

				finalCacheDir = filepath.Join(clusterName, "catalogs", "cache")
				finalWorkingDir = filepath.Join(clusterName, "catalogs", "working")
			} else {
				// 手动模式
				if catalogImage == "" {
					return fmt.Errorf("必须指定集群名称或使用 --catalog 参数")
				}

				finalCatalogImage = catalogImage
				finalCacheDir = cacheDir
				if finalCacheDir == "" {
					finalCacheDir = "/tmp/ocpack-catalog-cache"
				}
				finalWorkingDir = workingDir
				if finalWorkingDir == "" {
					finalWorkingDir = "/tmp/ocpack-working"
				}
			}

			manager := NewCatalogManagerNew(finalCatalogImage, finalCacheDir, finalWorkingDir)
			if authFile != "" {
				manager.AuthFile = authFile
			}

			operator, err := manager.GetOperatorInfo(operatorName)
			if err != nil {
				return err
			}

			if operator == nil {
				fmt.Printf("未找到 operator: %s\n", operatorName)
				return nil
			}

			fmt.Printf("Operator 信息:\n")
			fmt.Printf("名称: %s\n", operator.Name)
			fmt.Printf("显示名称: %s\n", operator.DisplayName)
			fmt.Printf("默认通道: %s\n", operator.DefaultChannel)
			if operator.Version != "" {
				fmt.Printf("版本: %s\n", operator.Version)
			}
			if operator.Description != "" {
				fmt.Printf("描述: %s\n", operator.Description)
			}

			return nil
		},
	}

	// 添加命令标志
	cmd.PersistentFlags().StringVar(&catalogImage, "catalog", "", "Catalog 镜像地址（手动模式必需）")
	cmd.PersistentFlags().StringVar(&cacheDir, "cache-dir", "", "缓存目录（默认: 集群模式使用 <cluster>/catalogs/cache，手动模式使用 /tmp/ocpack-catalog-cache）")
	cmd.PersistentFlags().StringVar(&workingDir, "working-dir", "", "工作目录（默认: 集群模式使用 <cluster>/catalogs/working，手动模式使用 /tmp/ocpack-working）")
	cmd.PersistentFlags().StringVar(&authFile, "authfile", "", "认证文件路径")

	cmd.AddCommand(listCmd)
	cmd.AddCommand(infoCmd)

	return cmd
}

// ClusterConfig 表示集群配置（简化版，只包含我们需要的字段）
type ClusterConfig struct {
	ClusterInfo struct {
		Name             string `toml:"name"`
		OpenShiftVersion string `toml:"openshift_version"`
	} `toml:"cluster_info"`

	SaveImage struct {
		OperatorCatalog string `toml:"operator_catalog"`
	} `toml:"save_image"`
}

// loadClusterConfig 从文件加载集群配置
func loadClusterConfig(filePath string) (*ClusterConfig, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("读取配置文件失败: %w", err)
	}

	config := &ClusterConfig{}
	if err := toml.Unmarshal(data, config); err != nil {
		return nil, fmt.Errorf("解析配置文件失败: %w", err)
	}

	return config, nil
}

// extractMajorMinorVersion 从版本字符串中提取主版本号和次版本号
// 例如：4.14.0 -> 4.14，4.16.1 -> 4.16
func extractMajorMinorVersion(version string) string {
	// 移除可能的 "v" 前缀
	version = strings.TrimPrefix(version, "v")

	// 按点分割版本号
	parts := strings.Split(version, ".")
	if len(parts) >= 2 {
		return fmt.Sprintf("%s.%s", parts[0], parts[1])
	}

	// 如果格式不正确，返回原版本
	return version
}

// saveOperatorsToClusterDir 将 operator 列表保存到集群目录
func (cm *CatalogManagerNew) saveOperatorsToClusterDir(operators []OperatorInfo, filePath string) error {
	// 确保目录存在
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("创建目录失败: %w", err)
	}

	// 创建带时间戳的数据结构
	data := struct {
		UpdatedAt time.Time      `json:"updated_at"`
		Catalog   string         `json:"catalog_image"`
		Count     int            `json:"operator_count"`
		Operators []OperatorInfo `json:"operators"`
	}{
		UpdatedAt: time.Now(),
		Catalog:   cm.CatalogImage,
		Count:     len(operators),
		Operators: operators,
	}

	// 先写入临时文件
	tempFile := filePath + ".tmp"
	file, err := os.Create(tempFile)
	if err != nil {
		return fmt.Errorf("创建临时文件失败: %w", err)
	}

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")

	if err := encoder.Encode(data); err != nil {
		file.Close()
		os.Remove(tempFile)
		return fmt.Errorf("编码 JSON 失败: %w", err)
	}

	file.Close()

	// 原子性重命名操作
	if err := os.Rename(tempFile, filePath); err != nil {
		os.Remove(tempFile)
		return fmt.Errorf("重命名文件失败: %w", err)
	}

	return nil
}
