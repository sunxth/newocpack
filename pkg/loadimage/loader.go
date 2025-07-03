package loadimage

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"ocpack/pkg/config"
)

// --- Constants ---
const (
	imagesDirName      = "images"
	registryDirName    = "registry"
	pullSecretFilename = "pull-secret.txt"
	mergedAuthFilename = "merged-auth.json"
	rootCACertFilename = "rootCA.pem"
	ocMirrorCmd        = "oc-mirror"
	podmanCmd          = "podman"
	dockerCmd          = "docker"
	registryPassword   = "ztesoft123" // Centralized fixed password
)

// ImageLoader is responsible for loading images from disk to a registry.
type ImageLoader struct {
	Config      *config.ClusterConfig
	ClusterName string
	ProjectRoot string
	ClusterDir  string
	DownloadDir string
}

// NewImageLoader creates a new ImageLoader instance.
func NewImageLoader(clusterName, projectRoot string) (*ImageLoader, error) {
	clusterDir := filepath.Join(projectRoot, clusterName)
	configPath := filepath.Join(clusterDir, "config.toml")

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		// 优化: 使用 %w 进行错误包装
		return nil, fmt.Errorf("加载配置文件失败: %w", err)
	}

	return &ImageLoader{
		Config:      cfg,
		ClusterName: clusterName,
		ProjectRoot: projectRoot,
		ClusterDir:  clusterDir,
		DownloadDir: filepath.Join(clusterDir, cfg.Download.LocalPath),
	}, nil
}

// LoadToRegistry orchestrates loading images from disk to the Quay registry.
func (l *ImageLoader) LoadToRegistry() error {
	fmt.Println("▶️  开始从磁盘加载镜像到 Quay registry")
	steps := 4

	imagesDir := filepath.Join(l.ClusterDir, imagesDirName)
	if _, err := os.Stat(imagesDir); os.IsNotExist(err) {
		return fmt.Errorf("镜像目录不存在: %s\n请先运行 'ocpack save-image' 命令保存镜像", imagesDir)
	}

	// 1. Configure CA certificates
	fmt.Printf("➡️  步骤 1/%d: 配置 CA 证书...\n", steps)
	if err := l.setupCACertificates(); err != nil {
		// This is often a manual step, so we warn instead of exiting.
		fmt.Printf("⚠️  CA 证书自动配置失败: %v\n", err)
		fmt.Println("   请根据提示手动完成证书信任配置。")
	} else {
		fmt.Println("✅ CA 证书配置完成。")
	}

	// 2. Validate registry connection
	fmt.Printf("➡️  步骤 2/%d: 验证 registry 连接...\n", steps)
	if err := l.validateRegistry(); err != nil {
		return fmt.Errorf("registry 连接验证失败: %w", err)
	}
	fmt.Println("✅ Registry 连接验证成功。")

	// 3. Configure authentication
	fmt.Printf("➡️  步骤 3/%d: 配置认证信息...\n", steps)
	if err := l.createOrUpdateAuthConfig(); err != nil {
		return fmt.Errorf("配置 registry 认证失败: %w", err)
	}
	fmt.Println("✅ Registry 认证配置完成。")

	// 4. Execute image loading
	fmt.Printf("➡️  步骤 4/%d: 执行镜像加载...\n", steps)
	if err := l.runOcMirrorLoad(); err != nil {
		return fmt.Errorf("oc-mirror 加载镜像失败: %w", err)
	}

	fmt.Println("\n🎉 镜像加载到 Quay registry 完成！")
	registryHostname := fmt.Sprintf("registry.%s.%s", l.Config.ClusterInfo.Name, l.Config.ClusterInfo.Domain)
	fmt.Printf("   Registry URL: https://%s:8443\n", registryHostname)
	fmt.Printf("   用户名: %s\n", l.Config.Registry.RegistryUser)
	fmt.Printf("   密码: %s\n", registryPassword)
	return nil
}

// validateRegistry checks the connection to the private registry.
func (l *ImageLoader) validateRegistry() error {
	registryHostname := fmt.Sprintf("registry.%s.%s", l.Config.ClusterInfo.Name, l.Config.ClusterInfo.Domain)
	registryURL := fmt.Sprintf("%s:8443", registryHostname)
	fmt.Printf("ℹ️  正在验证 Quay registry 连接: %s\n", registryURL)

	containerTool := l.getContainerTool()
	cmd := exec.Command(containerTool, "login",
		"--username", l.Config.Registry.RegistryUser,
		"--password", registryPassword, // 优化: 使用常量
		registryURL)

	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("使用 '%s' 登录失败: %w, 输出: %s", containerTool, err, string(output))
	}

	return nil
}

// getContainerTool finds an available container management tool.
func (l *ImageLoader) getContainerTool() string {
	if _, err := exec.LookPath(podmanCmd); err == nil {
		return podmanCmd
	}
	if _, err := exec.LookPath(dockerCmd); err == nil {
		return dockerCmd
	}
	return podmanCmd // Default to podman
}

// createOrUpdateAuthConfig merges the Red Hat pull secret with the local registry credentials.
func (l *ImageLoader) createOrUpdateAuthConfig() error {
	pullSecretPath := filepath.Join(l.ClusterDir, pullSecretFilename)
	pullSecretContent, err := os.ReadFile(pullSecretPath)
	if err != nil {
		return fmt.Errorf("读取 pull-secret 失败: %w", err)
	}

	var pullSecretData map[string]interface{}
	if err := json.Unmarshal(pullSecretContent, &pullSecretData); err != nil {
		return fmt.Errorf("解析 pull-secret JSON 失败: %w", err)
	}

	auths, ok := pullSecretData["auths"].(map[string]interface{})
	if !ok {
		return errors.New("pull-secret 格式无效: 缺少 'auths' 字段")
	}

	registryHostname := fmt.Sprintf("registry.%s.%s", l.Config.ClusterInfo.Name, l.Config.ClusterInfo.Domain)
	registryURL := fmt.Sprintf("%s:8443", registryHostname)
	authString := fmt.Sprintf("%s:%s", l.Config.Registry.RegistryUser, registryPassword)
	authBase64 := base64.StdEncoding.EncodeToString([]byte(authString))

	auths[registryURL] = map[string]interface{}{
		"auth":  authBase64,
		"email": "user@example.com", // email is a required field for some tools
	}

	mergedAuthContent, err := json.MarshalIndent(pullSecretData, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化合并后的认证配置失败: %w", err)
	}

	// Save to multiple conventional locations
	authPaths := []string{
		filepath.Join(l.ClusterDir, registryDirName, mergedAuthFilename),
		filepath.Join(os.Getenv("HOME"), ".docker", "config.json"),
	}

	for _, authPath := range authPaths {
		if err := os.MkdirAll(filepath.Dir(authPath), 0755); err != nil {
			return fmt.Errorf("创建认证配置目录失败: %w", err)
		}
		if err := os.WriteFile(authPath, mergedAuthContent, 0600); err != nil {
			return fmt.Errorf("保存合并后的认证配置失败: %w", err)
		}
		fmt.Printf("ℹ️  认证配置已更新/创建于: %s\n", authPath)
	}

	return nil
}

// runOcMirrorLoad executes the 'oc-mirror' command to load images.
func (l *ImageLoader) runOcMirrorLoad() error {
	ocMirrorPath := filepath.Join(l.DownloadDir, "bin", ocMirrorCmd)
	if _, err := os.Stat(ocMirrorPath); os.IsNotExist(err) {
		return fmt.Errorf("oc-mirror 工具不存在: %s", ocMirrorPath)
	}

	registryHostname := fmt.Sprintf("registry.%s.%s", l.Config.ClusterInfo.Name, l.Config.ClusterInfo.Domain)
	registryURL := fmt.Sprintf("docker://%s:8443", registryHostname)
	imagesDir := filepath.Join(l.ClusterDir, imagesDirName)

	args := []string{
		fmt.Sprintf("--from=%s", imagesDir),
		registryURL,
	}

	fmt.Printf("ℹ️  执行命令: %s %s\n", ocMirrorPath, strings.Join(args, " "))
	cmd := exec.Command(ocMirrorPath, args...)
	cmd.Dir = l.ClusterDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = append(os.Environ(),
		"REGISTRY_AUTH_FILE="+filepath.Join(l.ClusterDir, registryDirName, mergedAuthFilename),
	)

	if err := cmd.Run(); err != nil {
		if strings.Contains(err.Error(), "exec format error") {
			fmt.Println("⚠️  错误: oc-mirror 工具架构与当前系统不兼容。")
			l.printManualInstructions(ocMirrorPath, args)
			// Return a specific error to indicate a manual step is needed.
			return errors.New("oc-mirror 架构不兼容，请手动执行")
		}
		return fmt.Errorf("oc-mirror 命令执行失败: %w", err)
	}
	return nil
}

// printManualInstructions provides clear instructions for manual execution.
func (l *ImageLoader) printManualInstructions(cmdPath string, args []string) {
	fmt.Println("   请在与 oc-mirror 工具架构兼容的 Linux 系统上，手动执行以下命令:")
	fmt.Printf("   export REGISTRY_AUTH_FILE=%s\n", filepath.Join(l.ClusterDir, registryDirName, mergedAuthFilename))
	fmt.Printf("   %s %s\n", cmdPath, strings.Join(args, " "))
}

// setupCACertificates configures system trust for the registry's CA certificate.
func (l *ImageLoader) setupCACertificates() error {
	caCertPath := filepath.Join(l.ClusterDir, registryDirName, l.Config.Registry.IP, rootCACertFilename)

	if _, err := os.Stat(caCertPath); os.IsNotExist(err) {
		return fmt.Errorf("CA 证书文件不存在: %s", caCertPath)
	}
	fmt.Printf("ℹ️  找到 CA 证书: %s\n", caCertPath)

	switch runtime.GOOS {
	case "linux":
		return l.configureLinuxCertificateTrust(caCertPath)
	case "darwin":
		fmt.Printf("   macOS 用户请手动执行: sudo security add-trusted-cert -d -r trustRoot -k /Library/Keychains/System.keychain %s\n", caCertPath)
	case "windows":
		fmt.Printf("   Windows 用户请手动将证书添加到 '受信任的根证书颁发机构': %s\n", caCertPath)
	default:
		fmt.Printf("⚠️  不支持为操作系统 '%s' 自动配置证书，请手动完成。\n", runtime.GOOS)
	}
	return nil
}

// configureLinuxCertificateTrust handles certificate trust on Linux systems.
// 优化: This function now clarifies the need for sudo and provides copy-pasteable commands.
func (l *ImageLoader) configureLinuxCertificateTrust(caCertPath string) error {
	certDirs := map[string]string{
		"/etc/pki/ca-trust/source/anchors": "update-ca-trust",
		"/usr/local/share/ca-certificates": "update-ca-certificates",
	}

	var targetDir, updateCmd string
	for dir, cmd := range certDirs {
		if _, err := os.Stat(dir); err == nil {
			targetDir = dir
			updateCmd = cmd
			break
		}
	}
	if targetDir == "" {
		return errors.New("未找到系统证书目录 (如 /etc/pki/ca-trust/source/anchors 或 /usr/local/share/ca-certificates)")
	}

	certName := fmt.Sprintf("ocpack-registry-%s.crt", l.Config.ClusterInfo.Name)
	targetPath := filepath.Join(targetDir, certName)

	fmt.Println("   为了使系统信任 registry 证书，需要 root 权限执行以下命令。")
	fmt.Printf("   请复制并执行:\n")
	fmt.Printf("   sudo cp %s %s && sudo %s\n", caCertPath, targetPath, updateCmd)

	// Attempt to run with sudo, will prompt for password if not cached.
	// This might fail, but the user has the manual instructions.
	fmt.Println("   正在尝试自动执行...")
	cmd := exec.Command("sudo", "cp", caCertPath, targetPath)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("自动复制证书失败 (可能需要手动执行): %w, 输出: %s", err, string(output))
	}
	cmd = exec.Command("sudo", updateCmd)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("自动更新证书库失败 (可能需要手动执行): %w, 输出: %s", err, string(output))
	}
	return nil
}
