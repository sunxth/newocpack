package iso

import (
	"bytes"
	"embed"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"

	"ocpack/pkg/config"
	"ocpack/pkg/utils"

	"gopkg.in/yaml.v3"
)

//go:embed templates/*
var templates embed.FS

// --- Constants ---
const (
	installDirName        = "installation"
	ignitionDirName       = "ignition"
	isoDirName            = "iso"
	tempDirName           = "temp"
	registryDirName       = "registry"
	ocMirrorWorkspaceDir  = "oc-mirror-workspace"
	imagesDirName         = "images"
	installConfigFilename = "install-config.yaml"
	agentConfigFilename   = "agent-config.yaml"
	icspFilename          = "imageContentSourcePolicy.yaml"
	pullSecretFilename    = "pull-secret.txt"
	mergedAuthFilename    = "merged-auth.json"
	tempIcspFilename      = ".icsp.yaml"
	rootCACertFilename    = "rootCA.pem"
	openshiftInstallCmd   = "openshift-install"
	ocCmd                 = "oc"
	defaultInterface      = "ens3"
	defaultHostPrefix     = 23
)

// --- Struct Definitions ---

// ISOGenerator ISO 生成器
type ISOGenerator struct {
	Config      *config.ClusterConfig
	ClusterName string
	ProjectRoot string
	ClusterDir  string
	DownloadDir string
}

// GenerateOptions ISO 生成选项
type GenerateOptions struct {
	OutputPath  string
	BaseISOPath string
	SkipVerify  bool
	Force       bool // 新增: 用于接收 --force 标志
}

// InstallConfigData install-config.yaml 模板数据
type InstallConfigData struct {
	BaseDomain            string
	ClusterName           string
	NumWorkers            int
	NumMasters            int
	MachineNetwork        string
	PrefixLength          int
	HostPrefix            int
	PullSecret            string
	SSHKeyPub             string
	AdditionalTrustBundle string
	ImageContentSources   string
	ArchShort             string
	UseProxy              bool
	HTTPProxy             string
	HTTPSProxy            string
	NoProxy               string
}

// AgentConfigData agent-config.yaml 模板数据
type AgentConfigData struct {
	ClusterName    string
	RendezvousIP   string
	Hosts          []HostConfig
	Port0          string
	PrefixLength   int
	NextHopAddress string
	DNSServers     []string
}

// HostConfig 主机配置
type HostConfig struct {
	Hostname   string
	Role       string
	MACAddress string
	IPAddress  string
	Interface  string
}

// ICSP a minimal struct for parsing ImageContentSourcePolicy
type ICSP struct {
	Spec struct {
		RepositoryDigestMirrors []struct {
			Source  string   `yaml:"source"`
			Mirrors []string `yaml:"mirrors"`
		} `yaml:"repositoryDigestMirrors"`
	} `yaml:"spec"`
}

// --- Main Logic ---

// NewISOGenerator 创建新的 ISO 生成器
func NewISOGenerator(clusterName, projectRoot string) (*ISOGenerator, error) {
	clusterDir := filepath.Join(projectRoot, clusterName)
	configPath := filepath.Join(clusterDir, "config.toml")

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		return nil, fmt.Errorf("加载配置文件失败: %w", err)
	}

	return &ISOGenerator{
		Config:      cfg,
		ClusterName: clusterName,
		ProjectRoot: projectRoot,
		ClusterDir:  clusterDir,
		DownloadDir: filepath.Join(clusterDir, cfg.Download.LocalPath),
	}, nil
}

// GenerateISO 作为“编排器”来协调整个 ISO 生成流程
func (g *ISOGenerator) GenerateISO(options *GenerateOptions) error {
	fmt.Printf("▶️  开始为集群 %s 生成 ISO 镜像\n", g.ClusterName)

	// --- 新增逻辑: 检查 ISO 是否已存在 ---
	installDir := filepath.Join(g.ClusterDir, installDirName)
	targetISOPath := filepath.Join(installDir, isoDirName, fmt.Sprintf("%s-agent.x86_64.iso", g.ClusterName))

	if !options.Force {
		if _, err := os.Stat(targetISOPath); err == nil {
			fmt.Printf("\n🟡 ISO 文件已存在: %s\n", targetISOPath)
			fmt.Println("   跳过生成。使用 --force 标志可强制重新生成。")
			return nil
		}
	}
	// --- 新增逻辑结束 ---

	steps := 5
	// 1. 验证配置和依赖
	fmt.Printf("➡️  步骤 1/%d: 验证配置和依赖...\n", steps)
	if err := g.ValidateConfig(); err != nil {
		return fmt.Errorf("配置验证失败: %w", err)
	}
	fmt.Println("✅ 配置验证通过")

	// 2. 创建安装目录结构
	fmt.Printf("➡️  步骤 2/%d: 创建安装目录结构...\n", steps)
	if err := g.createInstallationDirs(installDir); err != nil {
		return fmt.Errorf("创建安装目录失败: %w", err)
	}
	fmt.Println("✅ 目录结构已创建")

	// 3. 生成 install-config.yaml
	fmt.Printf("➡️  步骤 3/%d: 生成 install-config.yaml...\n", steps)
	if err := g.generateInstallConfig(installDir); err != nil {
		return fmt.Errorf("生成 install-config.yaml 失败: %w", err)
	}
	fmt.Println("✅ install-config.yaml 已生成")

	// 4. 生成 agent-config.yaml
	fmt.Printf("➡️  步骤 4/%d: 生成 agent-config.yaml...\n", steps)
	if err := g.generateAgentConfig(installDir); err != nil {
		return fmt.Errorf("生成 agent-config.yaml 失败: %w", err)
	}
	fmt.Println("✅ agent-config.yaml 已生成")

	// 5. 生成 ISO 文件
	fmt.Printf("➡️  步骤 5/%d: 生成 ISO 文件...\n", steps)
	generatedPath, err := g.generateISOFiles(installDir, targetISOPath)
	if err != nil {
		return fmt.Errorf("生成 ISO 文件失败: %w", err)
	}

	fmt.Printf("\n🎉 ISO 生成完成！\n   文件位置: %s\n", generatedPath)
	return nil
}

// --- Step Implementations ---

// ValidateConfig 验证所有前提条件
func (g *ISOGenerator) ValidateConfig() error {
	if err := config.ValidateConfig(g.Config); err != nil {
		return err
	}
	toolPath := filepath.Join(g.DownloadDir, "bin", openshiftInstallCmd)
	if _, err := os.Stat(toolPath); os.IsNotExist(err) {
		return fmt.Errorf("缺少必需的工具: %s，请先运行 'ocpack download' 命令", openshiftInstallCmd)
	}
	pullSecretPath := filepath.Join(g.ClusterDir, pullSecretFilename)
	if _, err := os.Stat(pullSecretPath); os.IsNotExist(err) {
		return fmt.Errorf("缺少 %s 文件，请先获取 Red Hat pull-secret", pullSecretFilename)
	}
	return nil
}

// createInstallationDirs 创建所需的工作目录
func (g *ISOGenerator) createInstallationDirs(installDir string) error {
	dirs := []string{
		installDir,
		filepath.Join(installDir, ignitionDirName),
		filepath.Join(installDir, isoDirName),
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("创建目录 %s 失败: %w", dir, err)
		}
	}
	return nil
}

// generateInstallConfig 协调 install-config.yaml 的生成
func (g *ISOGenerator) generateInstallConfig(installDir string) error {
	pullSecret, err := g.getPullSecret()
	if err != nil {
		return err
	}

	sshKey, _ := g.getSSHKey() // SSH key is optional

	trustBundle, err := g.getAdditionalTrustBundle()
	if err != nil {
		fmt.Printf("ℹ️  未找到 CA 证书，将跳过: %v\n", err)
	}

	imageContentSources, err := g.findAndParseICSP()
	if err != nil {
		fmt.Printf("ℹ️  未找到 ICSP 文件，将跳过: %v\n", err)
	}

	data := InstallConfigData{
		BaseDomain:            g.Config.ClusterInfo.Domain,
		ClusterName:           g.Config.ClusterInfo.Name,
		NumWorkers:            len(g.Config.Cluster.Worker),
		NumMasters:            len(g.Config.Cluster.ControlPlane),
		MachineNetwork:        utils.ExtractNetworkBase(g.Config.Cluster.Network.MachineNetwork),
		PrefixLength:          utils.ExtractPrefixLength(g.Config.Cluster.Network.MachineNetwork),
		HostPrefix:            defaultHostPrefix,
		PullSecret:            pullSecret,
		SSHKeyPub:             sshKey,
		AdditionalTrustBundle: trustBundle,
		ImageContentSources:   imageContentSources,
		ArchShort:             "amd64",
	}

	funcMap := template.FuncMap{
		"indent": func(spaces int, text string) string {
			if text == "" {
				return ""
			}
			indentStr := strings.Repeat(" ", spaces)
			lines := strings.Split(text, "\n")
			for i, line := range lines {
				if line != "" {
					lines[i] = indentStr + line
				}
			}
			return strings.Join(lines, "\n")
		},
	}

	configPath := filepath.Join(installDir, installConfigFilename)
	return g.executeTemplate("templates/install-config.yaml", configPath, data, funcMap)
}

// generateAgentConfig 协调 agent-config.yaml 的生成
func (g *ISOGenerator) generateAgentConfig(installDir string) error {
	var hosts []HostConfig
	for _, cp := range g.Config.Cluster.ControlPlane {
		hosts = append(hosts, HostConfig{Hostname: cp.Name, Role: "master", MACAddress: cp.MAC, IPAddress: cp.IP, Interface: defaultInterface})
	}
	for _, worker := range g.Config.Cluster.Worker {
		hosts = append(hosts, HostConfig{Hostname: worker.Name, Role: "worker", MACAddress: worker.MAC, IPAddress: worker.IP, Interface: defaultInterface})
	}

	data := AgentConfigData{
		ClusterName:    g.Config.ClusterInfo.Name,
		RendezvousIP:   g.Config.Cluster.ControlPlane[0].IP,
		Hosts:          hosts,
		Port0:          defaultInterface,
		PrefixLength:   utils.ExtractPrefixLength(g.Config.Cluster.Network.MachineNetwork),
		NextHopAddress: utils.ExtractGateway(g.Config.Cluster.Network.MachineNetwork),
		DNSServers:     []string{g.Config.Bastion.IP},
	}

	configPath := filepath.Join(installDir, agentConfigFilename)
	return g.executeTemplate("templates/agent-config.yaml", configPath, data, nil)
}

// generateISOFiles 协调 ISO 文件的实际生成过程
func (g *ISOGenerator) generateISOFiles(installDir, targetISOPath string) (string, error) {
	openshiftInstallPath, err := g.findOpenshiftInstall()
	if err != nil {
		return "", fmt.Errorf("查找 openshift-install 失败: %w", err)
	}

	tempDir := filepath.Join(installDir, tempDirName)
	if err := os.MkdirAll(tempDir, 0755); err != nil {
		return "", fmt.Errorf("创建临时目录失败: %w", err)
	}
	defer os.RemoveAll(tempDir)

	for _, filename := range []string{installConfigFilename, agentConfigFilename} {
		src := filepath.Join(installDir, filename)
		dst := filepath.Join(tempDir, filename)
		if err := utils.CopyFile(src, dst); err != nil {
			return "", fmt.Errorf("复制 %s 失败: %w", filename, err)
		}
	}

	fmt.Printf("ℹ️  执行命令: %s agent create image --dir %s\n", openshiftInstallPath, tempDir)
	cmd := exec.Command(openshiftInstallPath, "agent", "create", "image", "--dir", tempDir)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("生成 agent ISO 失败: %w", err)
	}

	agentISOPath := filepath.Join(tempDir, "agent.x86_64.iso")
	if err := utils.MoveFile(agentISOPath, targetISOPath); err != nil {
		return "", fmt.Errorf("移动 ISO 文件失败: %w", err)
	}

	ignitionDir := filepath.Join(installDir, ignitionDirName)
	filesToCopy := []string{"auth", ".openshift_install.log", ".openshift_install_state.json"}
	for _, file := range filesToCopy {
		srcPath := filepath.Join(tempDir, file)
		if _, err := os.Stat(srcPath); err == nil {
			dstPath := filepath.Join(ignitionDir, file)
			if err := utils.CopyFileOrDir(srcPath, dstPath); err != nil {
				fmt.Printf("⚠️  复制 %s 失败: %v\n", file, err)
			}
		}
	}

	return targetISOPath, nil
}

// --- Helper Functions ---

// executeTemplate 通用的模板执行函数
func (g *ISOGenerator) executeTemplate(templatePath, outputPath string, data interface{}, funcMap template.FuncMap) error {
	tmplContent, err := templates.ReadFile(templatePath)
	if err != nil {
		return fmt.Errorf("读取模板 %s 失败: %w", templatePath, err)
	}

	tmpl := template.New(filepath.Base(templatePath))
	if funcMap != nil {
		tmpl = tmpl.Funcs(funcMap)
	}

	tmpl, err = tmpl.Parse(string(tmplContent))
	if err != nil {
		return fmt.Errorf("解析模板 %s 失败: %w", templatePath, err)
	}

	file, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("创建文件 %s 失败: %w", outputPath, err)
	}
	defer file.Close()

	if err := tmpl.Execute(file, data); err != nil {
		return fmt.Errorf("执行模板生成 %s 失败: %w", outputPath, err)
	}
	return nil
}

// getPullSecret 负责获取最终的 pull-secret 字符串
func (g *ISOGenerator) getPullSecret() (string, error) {
	mergedAuthPath := filepath.Join(g.ClusterDir, registryDirName, mergedAuthFilename)
	if _, err := os.Stat(mergedAuthPath); err == nil {
		fmt.Println("ℹ️  使用已合并的认证文件 " + mergedAuthFilename)
		secretBytes, err := os.ReadFile(mergedAuthPath)
		if err != nil {
			return "", fmt.Errorf("读取合并认证文件失败: %w", err)
		}
		return strings.TrimSpace(string(secretBytes)), nil
	}

	fmt.Println("ℹ️  合并认证文件不存在，将创建并使用它...")
	if err := g.createMergedAuthConfig(); err != nil {
		fmt.Printf("⚠️  创建合并认证文件失败: %v。将回退到原始 pull-secret。\n", err)
		pullSecretPath := filepath.Join(g.ClusterDir, pullSecretFilename)
		secretBytes, err := os.ReadFile(pullSecretPath)
		if err != nil {
			return "", fmt.Errorf("读取原始 pull-secret 失败: %w", err)
		}
		return strings.TrimSpace(string(secretBytes)), nil
	}
	return g.getPullSecret()
}

// getSSHKey 获取用户的公钥
func (g *ISOGenerator) getSSHKey() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("无法获取用户主目录: %w", err)
	}
	sshKeyPath := filepath.Join(home, ".ssh", "id_rsa.pub")
	sshKeyBytes, err := os.ReadFile(sshKeyPath)
	if err != nil {
		return "", fmt.Errorf("读取 SSH 公钥失败 (%s): %w", sshKeyPath, err)
	}
	return strings.TrimSpace(string(sshKeyBytes)), nil
}

// getAdditionalTrustBundle 查找并读取自定义 CA 证书
func (g *ISOGenerator) getAdditionalTrustBundle() (string, error) {
	possibleCertPaths := []string{
		filepath.Join(g.ClusterDir, registryDirName, g.Config.Registry.IP, rootCACertFilename),
		filepath.Join(g.ClusterDir, registryDirName, rootCACertFilename),
	}
	for _, certPath := range possibleCertPaths {
		if caCertBytes, err := os.ReadFile(certPath); err == nil {
			return string(caCertBytes), nil
		}
	}
	return "", errors.New("在任何预期位置都未找到 " + rootCACertFilename)
}

// findAndParseICSP 使用健壮的 YAML 解析器
func (g *ISOGenerator) findAndParseICSP() (string, error) {
	workspaceDir, err := g.findOcMirrorWorkspace()
	if err != nil {
		return "", err
	}
	latestResultsDir, err := g.findLatestResultsDir(workspaceDir)
	if err != nil {
		return "", fmt.Errorf("查找最新 results 目录失败: %w", err)
	}

	icspFile := filepath.Join(latestResultsDir, icspFilename)
	icspContent, err := os.ReadFile(icspFile)
	if err != nil {
		return "", fmt.Errorf("读取 ICSP 文件 %s 失败: %w", icspFile, err)
	}

	decoder := yaml.NewDecoder(bytes.NewReader(icspContent))
	var resultBuilder strings.Builder
	for {
		var icspDoc ICSP
		if err := decoder.Decode(&icspDoc); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return "", fmt.Errorf("解析 ICSP YAML 文档失败: %w", err)
		}

		for _, rdm := range icspDoc.Spec.RepositoryDigestMirrors {
			mirrorBlock := fmt.Sprintf("- mirrors:\n  - %s\n  source: %s", strings.Join(rdm.Mirrors, "\n  - "), rdm.Source)
			resultBuilder.WriteString(mirrorBlock)
			resultBuilder.WriteString("\n")
		}
	}

	if resultBuilder.Len() == 0 {
		return "", errors.New("ICSP 文件中未找到有效的镜像源配置")
	}
	return strings.TrimSpace(resultBuilder.String()), nil
}

// findOcMirrorWorkspace 查找 oc-mirror 的工作空间
func (g *ISOGenerator) findOcMirrorWorkspace() (string, error) {
	dirsToCheck := []string{
		filepath.Join(g.ClusterDir, ocMirrorWorkspaceDir),
		filepath.Join(g.ClusterDir, imagesDirName, ocMirrorWorkspaceDir),
	}
	for _, dir := range dirsToCheck {
		if _, err := os.Stat(dir); err == nil {
			return dir, nil
		}
	}
	return "", errors.New("oc-mirror workspace 目录不存在")
}

// findLatestResultsDir 查找最新的 results-* 目录
func (g *ISOGenerator) findLatestResultsDir(workspaceDir string) (string, error) {
	entries, err := os.ReadDir(workspaceDir)
	if err != nil {
		return "", fmt.Errorf("读取 workspace 目录失败: %w", err)
	}

	var latestDir string
	var latestTime int64

	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), "results-") {
			continue
		}
		dirPath := filepath.Join(workspaceDir, entry.Name())
		if entries, _ := os.ReadDir(dirPath); len(entries) == 0 {
			continue // Skip empty dirs
		}

		if timeValue, err := utils.ParseTimestamp(strings.TrimPrefix(entry.Name(), "results-")); err == nil {
			if timeValue > latestTime {
				latestTime = timeValue
				latestDir = dirPath
			}
		}
	}

	if latestDir == "" {
		return "", errors.New("未找到有效的 results 目录")
	}
	return latestDir, nil
}

// findOpenshiftInstall 查找可用的 openshift-install 二进制文件
func (g *ISOGenerator) findOpenshiftInstall() (string, error) {
	registryHost := fmt.Sprintf("registry.%s.%s", g.Config.ClusterInfo.Name, g.Config.ClusterInfo.Domain)
	extractedBinary := filepath.Join(g.ClusterDir, fmt.Sprintf("%s-%s-%s", openshiftInstallCmd, g.Config.ClusterInfo.OpenShiftVersion, registryHost))
	if _, err := os.Stat(extractedBinary); err == nil {
		fmt.Printf("ℹ️  使用从 Registry 提取的 openshift-install: %s\n", extractedBinary)
		return extractedBinary, nil
	}

	downloadedBinary := filepath.Join(g.DownloadDir, "bin", openshiftInstallCmd)
	if _, err := os.Stat(downloadedBinary); err == nil {
		fmt.Printf("ℹ️  使用下载的 openshift-install: %s\n", downloadedBinary)
		return downloadedBinary, nil
	}

	return "", fmt.Errorf("在 %s 或 %s 中均未找到 %s 工具", extractedBinary, downloadedBinary, openshiftInstallCmd)
}

// createMergedAuthConfig 创建包含私有仓库认证的 pull-secret 文件
func (g *ISOGenerator) createMergedAuthConfig() error {
	fmt.Println("🔐 创建合并的认证配置文件...")

	pullSecretPath := filepath.Join(g.ClusterDir, pullSecretFilename)
	pullSecretContent, err := os.ReadFile(pullSecretPath)
	if err != nil {
		return fmt.Errorf("读取 %s 失败: %w", pullSecretFilename, err)
	}

	var pullSecretData map[string]interface{}
	if err := json.Unmarshal(pullSecretContent, &pullSecretData); err != nil {
		return fmt.Errorf("解析 %s JSON 失败: %w", pullSecretFilename, err)
	}

	auths, ok := pullSecretData["auths"].(map[string]interface{})
	if !ok {
		return errors.New("pull-secret.txt 格式无效: 缺少 'auths' 字段")
	}

	registryHostname := fmt.Sprintf("registry.%s.%s", g.Config.ClusterInfo.Name, g.Config.ClusterInfo.Domain)
	registryURL := fmt.Sprintf("%s:8443", registryHostname)

	authString := fmt.Sprintf("%s:ztesoft123", g.Config.Registry.RegistryUser)
	authBase64 := base64.StdEncoding.EncodeToString([]byte(authString))

	auths[registryURL] = map[string]interface{}{
		"auth":  authBase64,
		"email": "user@example.com",
	}

	mergedAuthContent, err := json.MarshalIndent(pullSecretData, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化合并后的认证配置失败: %w", err)
	}

	registryDir := filepath.Join(g.ClusterDir, registryDirName)
	if err := os.MkdirAll(registryDir, 0755); err != nil {
		return fmt.Errorf("创建 registry 目录失败: %w", err)
	}

	mergedAuthPath := filepath.Join(registryDir, mergedAuthFilename)
	if err := os.WriteFile(mergedAuthPath, mergedAuthContent, 0600); err != nil {
		return fmt.Errorf("保存合并后的认证配置失败: %w", err)
	}

	fmt.Printf("✅ 认证配置已保存到: %s\n", mergedAuthPath)
	return nil
}
