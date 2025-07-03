package saveimage

import (
	"bytes"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"

	"ocpack/pkg/catalog"
	"ocpack/pkg/config"
	"ocpack/pkg/utils"
)

//go:embed templates/*
var templates embed.FS

// --- Constants ---
// 优化: 将硬编码的字符串定义为常量
const (
	imagesDirName               = "images"
	registryDirName             = "registry"
	ocMirrorWorkspaceDir        = "oc-mirror-workspace"
	ocMirrorCmd                 = "oc-mirror"
	pullSecretFilename          = "pull-secret.txt"
	pullSecretFormattedFilename = "pull-secret-formatted.json"
	dockerConfigFilename        = "config.json"
	imagesetConfigSaveFilename  = "imageset-config-save.yaml"
	ocpDefaultChannel           = "stable"
)

// --- Struct Definitions ---

// ImageSaver is responsible for saving container images to disk using oc-mirror.
type ImageSaver struct {
	Config      *config.ClusterConfig
	ClusterName string
	ProjectRoot string
	ClusterDir  string
	DownloadDir string
}

// ImageSetConfig defines the structure for the imageset configuration.
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

// HelmRepository defines a Helm repository configuration.
type HelmRepository struct {
	Name   string
	URL    string
	Charts []HelmChart
}

// HelmChart defines a specific Helm chart to be mirrored.
type HelmChart struct {
	Name    string
	Version string
}

// --- Main Logic ---

// NewImageSaver creates a new ImageSaver instance.
func NewImageSaver(clusterName, projectRoot string) (*ImageSaver, error) {
	clusterDir := filepath.Join(projectRoot, clusterName)
	configPath := filepath.Join(clusterDir, "config.toml")

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		// 优化: 使用 %w 进行错误包装
		return nil, fmt.Errorf("加载配置文件失败: %w", err)
	}

	return &ImageSaver{
		Config:      cfg,
		ClusterName: clusterName,
		ProjectRoot: projectRoot,
		ClusterDir:  clusterDir,
		DownloadDir: filepath.Join(clusterDir, cfg.Download.LocalPath),
	}, nil
}

// SaveImages orchestrates the process of saving images to disk.
// 优化: 重构为主流程清晰的"编排器"函数
func (s *ImageSaver) SaveImages() error {
	fmt.Println("▶️  开始保存镜像到磁盘...")
	steps := 4

	imagesDir := filepath.Join(s.ClusterDir, imagesDirName)
	if err := os.MkdirAll(imagesDir, 0755); err != nil {
		return fmt.Errorf("创建镜像目录失败: %w", err)
	}

	// 1. 检查是否已存在镜像
	fmt.Printf("➡️  步骤 1/%d: 检查本地镜像缓存...\n", steps)
	if s.checkExistingMirrorFiles(imagesDir) {
		fmt.Println("🔄 检测到已存在的镜像文件，跳过重复下载。")
		s.printSuccessMessage(imagesDir)
		return nil
	}
	fmt.Println("ℹ️  未发现镜像缓存，将开始新的下载。")

	// 2. 处理 pull-secret
	fmt.Printf("➡️  步骤 2/%d: 处理 pull-secret...\n", steps)
	if err := s.handlePullSecret(); err != nil {
		return fmt.Errorf("处理 pull-secret 失败: %w", err)
	}
	fmt.Println("✅ pull-secret 处理完成。")

	// 3. 生成 imageset-config.yaml
	fmt.Printf("➡️  步骤 3/%d: 生成 imageset 配置...\n", steps)
	imagesetConfigPath := filepath.Join(s.ClusterDir, imagesetConfigSaveFilename)
	if err := s.generateImageSetConfig(imagesetConfigPath); err != nil {
		return fmt.Errorf("生成 ImageSet 配置文件失败: %w", err)
	}
	fmt.Printf("✅ ImageSet 配置文件已生成: %s\n", imagesetConfigPath)

	// 4. 执行 oc-mirror 保存镜像
	fmt.Printf("➡️  步骤 4/%d: 执行镜像保存 (此过程可能需要较长时间)...\n", steps)
	if err := s.runOcMirrorSave(imagesetConfigPath, imagesDir); err != nil {
		return fmt.Errorf("oc-mirror 保存镜像失败: %w", err)
	}

	s.printSuccessMessage(imagesDir)
	return nil
}

// --- Step Implementations ---

// checkExistingMirrorFiles checks if mirror archive files already exist in the target directory.
func (s *ImageSaver) checkExistingMirrorFiles(imagesDir string) bool {
	files, err := os.ReadDir(imagesDir)
	if err != nil {
		// Log the error but don't fail, just assume no files exist.
		fmt.Printf("⚠️  读取镜像目录失败: %v\n", err)
		return false
	}

	for _, file := range files {
		// A more robust check for oc-mirror's output artifact.
		if !file.IsDir() && strings.HasPrefix(file.Name(), "mirror_seq") && strings.HasSuffix(file.Name(), ".tar") {
			fmt.Printf("📦 发现已存在的镜像文件: %s\n", file.Name())
			return true
		}
	}
	return false
}

// handlePullSecret validates and distributes the pull secret to necessary locations.
// 优化: 拆分职责，此函数现在是协调者
func (s *ImageSaver) handlePullSecret() error {
	pullSecretPath := filepath.Join(s.ClusterDir, pullSecretFilename)
	if _, err := os.Stat(pullSecretPath); os.IsNotExist(err) {
		return fmt.Errorf(`%s 文件不存在

请按照以下步骤获取 pull-secret:
1. 访问 https://console.redhat.com/openshift/install/pull-secret
2. 登录您的 Red Hat 账户
3. 下载 pull-secret 文件
4. 将文件保存为: %s`, pullSecretFilename, pullSecretPath)
	}
	fmt.Printf("ℹ️  找到 pull-secret 文件: %s\n", pullSecretPath)

	formattedContent, err := s.validateAndFormatPullSecret(pullSecretPath)
	if err != nil {
		return fmt.Errorf("pull-secret 文件处理失败: %w", err)
	}

	return s.saveFormattedPullSecret(formattedContent)
}

// generateImageSetConfig generates the ImageSet configuration file from a template.
func (s *ImageSaver) generateImageSetConfig(configPath string) error {
	version := s.Config.ClusterInfo.OpenShiftVersion
	majorVersion := utils.ExtractMajorVersion(version)

	workspacePath := filepath.Join(s.ClusterDir, imagesDirName, ocMirrorWorkspaceDir)
	if err := os.MkdirAll(workspacePath, 0755); err != nil {
		return fmt.Errorf("创建 oc-mirror workspace 目录失败: %w", err)
	}

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
		fmt.Printf("ℹ️  正在获取 Operator 信息...\n")

		// 创建目录管理器
		cacheDir := filepath.Join(s.ClusterDir, ".catalog-cache")
		ocMirrorPath := filepath.Join(s.DownloadDir, "bin", ocMirrorCmd)
		catalogManager := catalog.NewCatalogManager(catalogImage, cacheDir, ocMirrorPath)

		// 为每个配置的 Operator 获取默认 channel
		for _, opName := range saveImageConfig.Ops {
			opInfo, err := catalogManager.GetOperatorInfo(opName)
			if err != nil {
				fmt.Printf("⚠️  警告: 无法获取 Operator %s 的信息: %v\n", opName, err)
				fmt.Printf("   将使用 Operator 名称而不指定 channel\n")
				operatorPackages = append(operatorPackages, OperatorPackage{
					Name: opName,
				})
			} else {
				fmt.Printf("✅ Operator %s 默认 channel: %s\n", opName, opInfo.DefaultChannel)
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
		WorkspacePath:    workspacePath,
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

// runOcMirrorSave executes the 'oc-mirror' command to save images to disk.
func (s *ImageSaver) runOcMirrorSave(configPath, imagesDir string) error {
	ocMirrorPath := filepath.Join(s.DownloadDir, "bin", ocMirrorCmd)
	if _, err := os.Stat(ocMirrorPath); os.IsNotExist(err) {
		return fmt.Errorf("oc-mirror 工具不存在: %s", ocMirrorPath)
	}

	args := []string{
		fmt.Sprintf("--config=%s", configPath),
		fmt.Sprintf("file://%s", imagesDir), // target directory
	}

	fmt.Printf("ℹ️  执行命令: %s %s\n", ocMirrorPath, strings.Join(args, " "))

	cmd := exec.Command(ocMirrorPath, args...)
	cmd.Dir = s.ClusterDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		if strings.Contains(err.Error(), "exec format error") {
			fmt.Println("⚠️  错误: oc-mirror 工具架构与当前系统不兼容。")
			s.printManualInstructions(ocMirrorPath, args)
			return errors.New("oc-mirror 架构不兼容，请手动执行")
		}
		return fmt.Errorf("oc-mirror 命令执行失败: %w", err)
	}

	return nil
}

// --- Helper Functions ---

// validateAndFormatPullSecret reads, validates, and formats the pull secret JSON.
// 优化: 职责更单一的辅助函数
func (s *ImageSaver) validateAndFormatPullSecret(filePath string) ([]byte, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("读取文件失败: %w", err)
	}

	var pullSecret map[string]interface{}
	if err := json.Unmarshal(bytes.TrimSpace(content), &pullSecret); err != nil {
		return nil, fmt.Errorf("pull-secret 不是有效的 JSON 格式: %w", err)
	}

	if _, exists := pullSecret["auths"]; !exists {
		return nil, errors.New("pull-secret 缺少 'auths' 字段")
	}

	// Simple validation passed, format it for saving.
	return json.MarshalIndent(pullSecret, "", "  ")
}

// saveFormattedPullSecret saves the formatted pull secret to multiple conventional locations.
func (s *ImageSaver) saveFormattedPullSecret(content []byte) error {
	savePaths := map[string]string{
		"registry config":  filepath.Join(s.ClusterDir, registryDirName, "pull-secret.json"),
		"docker config":    filepath.Join(os.Getenv("HOME"), ".docker", dockerConfigFilename),
		"formatted backup": filepath.Join(s.ClusterDir, pullSecretFormattedFilename),
	}

	for name, path := range savePaths {
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return fmt.Errorf("为 %s 创建目录失败: %w", name, err)
		}
		if err := os.WriteFile(path, content, 0600); err != nil {
			// Don't fail the whole process for non-critical save locations
			if name == "formatted backup" {
				fmt.Printf("⚠️  警告: 无法保存格式化的备份文件: %v\n", err)
				continue
			}
			return fmt.Errorf("保存 %s 文件失败: %w", name, err)
		}
		fmt.Printf("ℹ️  格式化的 pull-secret 已保存到 (%s): %s\n", name, path)
	}
	return nil
}

// printManualInstructions provides clear instructions for manual execution.
func (s *ImageSaver) printManualInstructions(cmdPath string, args []string) {
	fmt.Println("   请在与 oc-mirror 工具架构兼容的 Linux 系统上，手动执行以下命令:")
	fmt.Printf("   %s %s\n", cmdPath, strings.Join(args, " "))
}

// printSuccessMessage prints the final success message.
func (s *ImageSaver) printSuccessMessage(imagesDir string) {
	fmt.Println("\n🎉 镜像保存完成！")
	fmt.Printf("   镜像已保存到: %s\n", imagesDir)
	fmt.Println("   下一步: 使用 'ocpack load-image' 命令将镜像加载到 registry。")
}
