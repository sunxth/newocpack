package save

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"
	"time"

	"github.com/spf13/cobra"

	"ocpack/pkg/catalognew"
	"ocpack/pkg/config"
	"ocpack/pkg/mirror/cli"
	clog "ocpack/pkg/mirror/log"
	"ocpack/pkg/mirror/mirror"
	"ocpack/pkg/utils"
)

//go:embed templates/*
var templates embed.FS

// --- Constants ---
const (
	imagesDirName          = "images"
	imagesetConfigFilename = "imageset-config.yaml"
	operatorsJsonFilename  = "operators.json"
	ocpDefaultChannel      = "stable"
)

// --- Struct Definitions ---

// Saver 负责使用内部 pkg/mirror 模块保存镜像到磁盘
type Saver struct {
	Config      *config.ClusterConfig
	ClusterName string
	ProjectRoot string
	ClusterDir  string
	Logger      clog.PluggableLoggerInterface
}

// ImageSetConfig 定义 imageset 配置的结构体
type ImageSetConfig struct {
	OCPChannel       string
	OCPVerMajor      string
	OCPVer           string
	IncludeOperators bool
	OperatorCatalog  string
	OperatorPackages []OperatorPackage
	AdditionalImages []string
	WorkspacePath    string
}

// OperatorPackage 表示要包含的 Operator 包
type OperatorPackage struct {
	Name    string
	Channel string
}

// OperatorInfo 表示从 operators.json 读取的 operator 信息
type OperatorInfo struct {
	Name           string `json:"name"`
	DisplayName    string `json:"displayName"`
	DefaultChannel string `json:"defaultChannel"`
	Version        string `json:"version,omitempty"`
	Description    string `json:"description,omitempty"`
}

// OperatorsCache 表示 operators.json 的结构
type OperatorsCache struct {
	UpdatedAt time.Time      `json:"updated_at"`
	Catalog   string         `json:"catalog_image"`
	Count     int            `json:"operator_count"`
	Operators []OperatorInfo `json:"operators"`
}

// --- Main Logic ---

// NewSaver 创建新的 Saver 实例
func NewSaver(clusterName, projectRoot string) (*Saver, error) {
	clusterDir := filepath.Join(projectRoot, clusterName)
	configPath := filepath.Join(clusterDir, "config.toml")

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		return nil, fmt.Errorf("加载配置文件失败: %w", err)
	}

	logger := clog.New("info")

	return &Saver{
		Config:      cfg,
		ClusterName: clusterName,
		ProjectRoot: projectRoot,
		ClusterDir:  clusterDir,
		Logger:      logger,
	}, nil
}

// SaveImages 执行镜像保存的主流程
func (s *Saver) SaveImages() error {
	s.Logger.Info("▶️  开始使用内部 mirror 引擎保存镜像到磁盘...")
	steps := 4

	imagesDir := filepath.Join(s.ClusterDir, imagesDirName)
	if err := os.MkdirAll(imagesDir, 0755); err != nil {
		return fmt.Errorf("创建镜像目录失败: %w", err)
	}

	// 1. 检查是否已存在镜像
	s.Logger.Info("➡️  步骤 1/%d: 检查本地镜像缓存...", steps)
	if s.checkExistingMirrorFiles(imagesDir) {
		s.Logger.Info("🔄 检测到已存在的镜像文件，跳过重复下载。")
		s.printSuccessMessage(imagesDir)
		return nil
	}
	s.Logger.Info("ℹ️  未发现镜像缓存，将开始新的下载。")

	// 2. 动态生成 imageset-config.yaml
	s.Logger.Info("➡️  步骤 2/%d: 动态生成 imageset 配置...", steps)
	imagesetConfigPath := filepath.Join(s.ClusterDir, imagesetConfigFilename)
	if err := s.generateImageSetConfig(imagesetConfigPath); err != nil {
		return fmt.Errorf("生成 ImageSet 配置文件失败: %w", err)
	}
	s.Logger.Info("✅ ImageSet 配置文件已生成: %s", imagesetConfigPath)

	// 3. 准备工作目录
	s.Logger.Info("➡️  步骤 3/%d: 准备工作环境...", steps)
	workingDir := filepath.Join(s.ClusterDir, "mirror-workspace")
	if err := os.MkdirAll(workingDir, 0755); err != nil {
		return fmt.Errorf("创建工作目录失败: %w", err)
	}

	// 4. 执行镜像保存
	s.Logger.Info("➡️  步骤 4/%d: 执行镜像保存 (此过程可能需要较长时间)...", steps)
	if err := s.runMirrorToDisk(imagesetConfigPath, imagesDir, workingDir); err != nil {
		return fmt.Errorf("镜像保存失败: %w", err)
	}

	s.printSuccessMessage(imagesDir)
	return nil
}

// --- Step Implementations ---

// checkExistingMirrorFiles 检查是否已存在镜像归档文件
func (s *Saver) checkExistingMirrorFiles(imagesDir string) bool {
	files, err := os.ReadDir(imagesDir)
	if err != nil {
		s.Logger.Warn("⚠️  读取镜像目录失败: %v", err)
		return false
	}

	for _, file := range files {
		// 检查 oc-mirror 的输出产物
		if !file.IsDir() && strings.HasPrefix(file.Name(), "mirror_seq") && strings.HasSuffix(file.Name(), ".tar") {
			s.Logger.Info("📦 发现已存在的镜像文件: %s", file.Name())
			return true
		}
	}
	return false
}

// generateImageSetConfig 从模板生成 ImageSet 配置文件
func (s *Saver) generateImageSetConfig(configPath string) error {
	version := s.Config.ClusterInfo.OpenShiftVersion
	majorVersion := utils.ExtractMajorVersion(version)

	// 从配置文件读取镜像保存配置
	saveImageConfig := s.Config.SaveImage

	// 构建 Operator 目录镜像地址
	catalogImage := saveImageConfig.OperatorCatalog
	if catalogImage == "" {
		catalogImage = fmt.Sprintf("registry.redhat.io/redhat/redhat-operator-index:v%s", majorVersion)
	}

	var operatorPackages []OperatorPackage

	// 如果需要包含 Operator，则获取它们的默认 channel
	if saveImageConfig.IncludeOperators && len(saveImageConfig.Ops) > 0 {
		s.Logger.Info("ℹ️  正在获取 Operator 信息...")

		// 为每个配置的 Operator 获取默认 channel
		for _, opName := range saveImageConfig.Ops {
			opInfo, err := s.getOperatorDefaultChannel(opName)
			if err != nil {
				s.Logger.Warn("⚠️  警告: 无法获取 Operator %s 的信息: %v", opName, err)
				s.Logger.Warn("   将使用 Operator 名称而不指定 channel")
				operatorPackages = append(operatorPackages, OperatorPackage{
					Name: opName,
				})
			} else {
				s.Logger.Info("✅ Operator %s 默认 channel: %s", opName, opInfo.DefaultChannel)
				operatorPackages = append(operatorPackages, OperatorPackage{
					Name:    opName,
					Channel: opInfo.DefaultChannel,
				})
			}
		}
	}

	imagesetConfig := ImageSetConfig{
		OCPChannel:       ocpDefaultChannel,
		OCPVerMajor:      majorVersion,
		OCPVer:           version,
		IncludeOperators: saveImageConfig.IncludeOperators,
		OperatorCatalog:  catalogImage,
		OperatorPackages: operatorPackages,
		AdditionalImages: saveImageConfig.AdditionalImages,
		WorkspacePath:    filepath.Join(s.ClusterDir, "mirror-workspace"),
	}

	// 生成配置文件
	tmplContent, err := templates.ReadFile("templates/imageset-config.yaml")
	if err != nil {
		return fmt.Errorf("读取模板文件失败: %w", err)
	}
	tmpl, err := template.New("imageset").Parse(string(tmplContent))
	if err != nil {
		return fmt.Errorf("解析模板失败: %w", err)
	}

	file, err := os.Create(configPath)
	if err != nil {
		return fmt.Errorf("创建配置文件失败: %w", err)
	}
	defer file.Close()

	return tmpl.Execute(file, imagesetConfig)
}

// getOperatorDefaultChannel 从 operators.json 缓存中获取 Operator 的默认频道
func (s *Saver) getOperatorDefaultChannel(operatorName string) (*OperatorInfo, error) {
	operatorsJsonPath := filepath.Join(s.ClusterDir, operatorsJsonFilename)

	// 检查 operators.json 是否存在
	if _, err := os.Stat(operatorsJsonPath); os.IsNotExist(err) {
		// 如果不存在，尝试使用 catalognew 生成
		s.Logger.Info("operators.json 不存在，尝试生成...")
		if err := s.generateOperatorsCache(); err != nil {
			return nil, fmt.Errorf("生成 operators 缓存失败: %w", err)
		}
	}

	// 读取 operators.json
	data, err := os.ReadFile(operatorsJsonPath)
	if err != nil {
		return nil, fmt.Errorf("读取 operators.json 失败: %w", err)
	}

	var operatorsCache OperatorsCache
	if err := json.Unmarshal(data, &operatorsCache); err != nil {
		return nil, fmt.Errorf("解析 operators.json 失败: %w", err)
	}

	// 1. 精确匹配 (name 和 displayName)
	for _, op := range operatorsCache.Operators {
		if op.Name == operatorName || op.DisplayName == operatorName {
			return &op, nil
		}
	}

	// 2. 已知别名映射
	aliasMap := map[string]string{
		"cluster-logging":        "cluster-logging-operator",
		"logging":                "cluster-logging-operator",
		"local-storage-operator": "local-storage-operator",
		"local-storage":          "local-storage-operator",
		"openshift-logging":      "cluster-logging-operator",
	}

	if alias, exists := aliasMap[operatorName]; exists {
		for _, op := range operatorsCache.Operators {
			if op.Name == alias || op.DisplayName == alias {
				s.Logger.Info("通过别名找到 operator: %s -> %s", operatorName, op.Name)
				return &op, nil
			}
		}
	}

	// 3. 模糊匹配 (包含关系)
	var candidates []OperatorInfo
	for _, op := range operatorsCache.Operators {
		if strings.Contains(op.Name, operatorName) || strings.Contains(op.DisplayName, operatorName) {
			candidates = append(candidates, op)
		}
	}

	// 如果只有一个候选，直接返回
	if len(candidates) == 1 {
		s.Logger.Info("找到模糊匹配的 operator: %s -> %s", operatorName, candidates[0].Name)
		return &candidates[0], nil
	}

	// 如果有多个候选，返回错误并提供建议
	if len(candidates) > 1 {
		var suggestions []string
		for _, candidate := range candidates {
			suggestions = append(suggestions, candidate.Name)
		}
		return nil, fmt.Errorf("未找到 operator '%s'，但找到了多个可能的匹配: %s",
			operatorName, strings.Join(suggestions, ", "))
	}

	// 4. 如果完全没找到，提供相似的建议
	suggestions := s.findSimilarOperators(operatorName, operatorsCache.Operators)
	if len(suggestions) > 0 {
		return nil, fmt.Errorf("未找到 operator '%s'，您是否想要: %s",
			operatorName, strings.Join(suggestions, ", "))
	}

	return nil, fmt.Errorf("未找到 operator: %s", operatorName)
}

// findSimilarOperators 查找相似的 operator 名称
func (s *Saver) findSimilarOperators(target string, operators []OperatorInfo) []string {
	var suggestions []string
	target = strings.ToLower(target)

	// 查找包含目标关键词的 operator
	keywords := []string{"logging", "storage", "monitoring", "network", "security", "backup"}

	for _, keyword := range keywords {
		if strings.Contains(target, keyword) {
			for _, op := range operators {
				opName := strings.ToLower(op.Name)
				if strings.Contains(opName, keyword) && len(suggestions) < 5 {
					suggestions = append(suggestions, op.Name)
				}
			}
			break // 只使用第一个匹配的关键词
		}
	}

	return suggestions
}

// generateOperatorsCache 使用 catalognew 生成 operators 缓存
func (s *Saver) generateOperatorsCache() error {
	// 构建 catalog 镜像地址
	majorVersion := utils.ExtractMajorVersion(s.Config.ClusterInfo.OpenShiftVersion)
	catalogImage := s.Config.SaveImage.OperatorCatalog
	if catalogImage == "" {
		catalogImage = fmt.Sprintf("registry.redhat.io/redhat/redhat-operator-index:v%s", majorVersion)
	}

	// 设置缓存和工作目录
	cacheDir := filepath.Join(s.ClusterDir, "catalogs", "cache")
	workingDir := filepath.Join(s.ClusterDir, "catalogs", "working")

	// 确保目录存在
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return fmt.Errorf("创建缓存目录失败: %w", err)
	}
	if err := os.MkdirAll(workingDir, 0755); err != nil {
		return fmt.Errorf("创建工作目录失败: %w", err)
	}

	// 创建 catalog 管理器
	manager := catalognew.NewCatalogManagerNew(catalogImage, cacheDir, workingDir)

	// 获取所有 operators
	operators, err := manager.GetAllOperators()
	if err != nil {
		return fmt.Errorf("获取 operators 失败: %w", err)
	}

	// 转换为本地 OperatorInfo 类型
	localOperators := make([]OperatorInfo, len(operators))
	for i, op := range operators {
		localOperators[i] = OperatorInfo{
			Name:           op.Name,
			DisplayName:    op.DisplayName,
			DefaultChannel: op.DefaultChannel,
			Version:        op.Version,
			Description:    op.Description,
		}
	}

	// 保存到集群目录
	operatorsJsonPath := filepath.Join(s.ClusterDir, operatorsJsonFilename)
	return s.saveOperatorsToClusterDir(localOperators, operatorsJsonPath, catalogImage)
}

// runMirrorToDisk 使用 pkg/mirror 执行实际的镜像保存到磁盘
func (s *Saver) runMirrorToDisk(configPath, imagesDir, workingDir string) error {
	s.Logger.Info("使用内部 mirror 引擎执行镜像保存...")

	// 创建全局选项
	globalOpts := &mirror.GlobalOptions{
		SecurePolicy:    false,
		Force:           true,
		WorkingDir:      workingDir,
		ConfigPath:      configPath,
		LogLevel:        "info",
		IsTerminal:      true,
		StrictArchiving: false,
		CacheDir:        filepath.Join(workingDir, ".cache"),
		Port:            5000, // 本地 registry 端口
	}

	// 创建镜像选项
	_, sharedOpts := mirror.SharedImageFlags()
	_, deprecatedTLSVerifyOpt := mirror.DeprecatedTLSVerifyFlags()
	_, srcOpts := mirror.ImageSrcFlags(globalOpts, sharedOpts, deprecatedTLSVerifyOpt, "src-", "screds")
	_, destOpts := mirror.ImageDestFlags(globalOpts, sharedOpts, deprecatedTLSVerifyOpt, "dest-", "dcreds")
	_, retryOpts := mirror.RetryFlags()

	// 设置目标为 file:// 协议
	destination := fmt.Sprintf("file://%s", imagesDir)

	// 创建复制选项
	copyOpts := &mirror.CopyOptions{
		Global:              globalOpts,
		DeprecatedTLSVerify: deprecatedTLSVerifyOpt,
		SrcImage:            srcOpts,
		DestImage:           destOpts,
		RetryOpts:           retryOpts,
		Dev:                 false,
		Mode:                mirror.MirrorToDisk,
		Destination:         destination,
		LocalStorageFQDN:    fmt.Sprintf("localhost:%d", globalOpts.Port),
	}

	// 确保必要的目录存在
	if err := os.MkdirAll(globalOpts.CacheDir, 0755); err != nil {
		return fmt.Errorf("创建缓存目录失败: %w", err)
	}

	// 创建日志目录
	logsDir := filepath.Join(workingDir, "logs")
	if err := os.MkdirAll(logsDir, 0755); err != nil {
		return fmt.Errorf("创建日志目录失败: %w", err)
	}

	// 创建执行器
	executor := &cli.ExecutorSchema{
		Log:     s.Logger,
		Opts:    copyOpts,
		LogsDir: logsDir,
		MakeDir: cli.MakeDir{},
	}

	// 创建命令上下文
	cmd := &cobra.Command{}
	ctx := context.Background()
	cmd.SetContext(ctx)

	// 执行验证
	if err := executor.Validate([]string{destination}); err != nil {
		return fmt.Errorf("验证配置失败: %w", err)
	}

	// 执行初始化
	if err := executor.Complete([]string{destination}); err != nil {
		return fmt.Errorf("初始化执行器失败: %w", err)
	}

	s.Logger.Info("🚀 开始实际下载镜像...")

	// 执行镜像保存
	if err := executor.Run(cmd, []string{destination}); err != nil {
		return fmt.Errorf("执行镜像保存失败: %w", err)
	}

	s.Logger.Info("✅ 镜像保存完成")
	return nil
}

// --- Helper Functions ---

// saveOperatorsToClusterDir 将 operator 列表保存到集群目录
func (s *Saver) saveOperatorsToClusterDir(operators []OperatorInfo, filePath, catalogImage string) error {
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
		Catalog:   catalogImage,
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

// printSuccessMessage 打印成功消息
func (s *Saver) printSuccessMessage(imagesDir string) {
	s.Logger.Info("\n🎉 镜像保存完成！")
	s.Logger.Info("   镜像已保存到: %s", imagesDir)
	s.Logger.Info("   下一步: 使用 'ocpack load-image' 命令将镜像加载到 registry。")
}

// ListOperators 列出可用的 operators
func (s *Saver) ListOperators() error {
	operatorsJsonPath := filepath.Join(s.ClusterDir, operatorsJsonFilename)

	// 检查 operators.json 是否存在
	if _, err := os.Stat(operatorsJsonPath); os.IsNotExist(err) {
		fmt.Printf("📥 operators.json 不存在，正在下载 operator 目录...\n")
		if err := s.generateOperatorsCache(); err != nil {
			return fmt.Errorf("生成 operators 缓存失败: %w", err)
		}
		fmt.Printf("✅ operator 目录下载完成\n\n")
	}

	// 读取 operators.json
	data, err := os.ReadFile(operatorsJsonPath)
	if err != nil {
		return fmt.Errorf("读取 operators.json 失败: %w", err)
	}

	var operatorsCache OperatorsCache
	if err := json.Unmarshal(data, &operatorsCache); err != nil {
		return fmt.Errorf("解析 operators.json 失败: %w", err)
	}

	fmt.Printf("📋 可用的 Operator 列表 (共 %d 个):\n", len(operatorsCache.Operators))
	fmt.Printf("🏷️  目录镜像: %s\n", operatorsCache.Catalog)
	fmt.Printf("🕒 更新时间: %s\n\n", operatorsCache.UpdatedAt.Format("2006-01-02 15:04:05"))

	// 按类别分组显示（简化版本）
	categories := map[string][]OperatorInfo{
		"日志记录 (Logging)":  {},
		"存储 (Storage)":    {},
		"监控 (Monitoring)": {},
		"网络 (Network)":    {},
		"安全 (Security)":   {},
		"其他 (Others)":     {},
	}

	// 简单的关键词分类
	for _, op := range operatorsCache.Operators {
		name := strings.ToLower(op.Name)
		displayName := strings.ToLower(op.DisplayName)

		if strings.Contains(name, "log") || strings.Contains(displayName, "log") {
			categories["日志记录 (Logging)"] = append(categories["日志记录 (Logging)"], op)
		} else if strings.Contains(name, "storage") || strings.Contains(displayName, "storage") {
			categories["存储 (Storage)"] = append(categories["存储 (Storage)"], op)
		} else if strings.Contains(name, "monitor") || strings.Contains(displayName, "monitor") {
			categories["监控 (Monitoring)"] = append(categories["监控 (Monitoring)"], op)
		} else if strings.Contains(name, "network") || strings.Contains(displayName, "network") {
			categories["网络 (Network)"] = append(categories["网络 (Network)"], op)
		} else if strings.Contains(name, "security") || strings.Contains(displayName, "security") {
			categories["安全 (Security)"] = append(categories["安全 (Security)"], op)
		} else {
			categories["其他 (Others)"] = append(categories["其他 (Others)"], op)
		}
	}

	// 显示分类结果
	for category, operators := range categories {
		if len(operators) > 0 {
			fmt.Printf("### %s\n", category)
			for _, op := range operators {
				fmt.Printf("  %-30s | %-15s | %s\n", op.Name, op.DefaultChannel, op.DisplayName)
			}
			fmt.Println()
		}
	}

	fmt.Printf("💡 使用提示:\n")
	fmt.Printf("   - 在 config.toml 的 [save_image] ops 列表中使用 Operator 名称 (第一列)\n")
	fmt.Printf("   - 例如: ops = [\"cluster-logging\", \"local-storage-operator\"]\n")
	fmt.Printf("   - 支持常见别名，如 \"logging\" 会自动映射到 \"cluster-logging-operator\"\n\n")

	return nil
}
