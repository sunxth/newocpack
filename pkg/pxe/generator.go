package pxe

import (
	"bytes"
	"embed"
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

	"github.com/mattn/go-runewidth"
	"gopkg.in/yaml.v3"
)

//go:embed templates/*
var templates embed.FS

// --- Constants for filenames, directories, and commands ---
const (
	configDirName           = "config"
	filesDirName            = "files"
	tempDirName             = "temp"
	pxeDirName              = "pxe"
	installConfigFilename   = "install-config.yaml"
	agentConfigFilename     = "agent-config.yaml"
	icspFilename            = "imageContentSourcePolicy.yaml"
	pullSecretFilename      = "pull-secret.txt"
	mergedAuthFilename      = "merged-auth.json"
	registryDirName         = "registry"
	ocMirrorWorkspaceDir    = "oc-mirror-workspace"
	imagesDirName           = "images"
	openshiftInstallCmd     = "openshift-install"
	defaultInterface        = "ens3"
	uploadScriptPath        = "/usr/local/bin/upload-pxe-files.sh"
	defaultPxeWebServerPort = 8080
)

// --- Struct Definitions ---

// PXEGenerator holds the configuration and paths for generating PXE assets.
type PXEGenerator struct {
	Config      *config.ClusterConfig
	ClusterName string
	ProjectRoot string
	ClusterDir  string
	DownloadDir string
}

// GenerateOptions defines options for the PXE generation process.
type GenerateOptions struct {
	AssetServerURL string
	SkipVerify     bool
}

// AgentConfigDataPXE is the template data for agent-config.yaml.
type AgentConfigDataPXE struct {
	ClusterName          string
	RendezvousIP         string
	Hosts                []HostConfig
	Port0                string
	PrefixLength         int
	NextHopAddress       string
	DNSServers           []string
	BootArtifactsBaseURL string
}

// HostConfig represents a single host's configuration.
type HostConfig struct {
	Hostname   string
	Role       string
	MACAddress string
	IPAddress  string
	Interface  string
}

// --- Main Logic ---

// NewPXEGenerator creates a new PXE generator instance.
func NewPXEGenerator(clusterName, projectRoot string) (*PXEGenerator, error) {
	clusterDir := filepath.Join(projectRoot, clusterName)
	configPath := filepath.Join(clusterDir, "config.toml")

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		return nil, fmt.Errorf("加载配置文件失败: %w", err)
	}

	return &PXEGenerator{
		Config:      cfg,
		ClusterName: clusterName,
		ProjectRoot: projectRoot,
		ClusterDir:  clusterDir,
		DownloadDir: filepath.Join(clusterDir, cfg.Download.LocalPath),
	}, nil
}

// GeneratePXE orchestrates the entire PXE file generation process.
func (g *PXEGenerator) GeneratePXE(options *GenerateOptions) error {
	g.printHeader("PXE 文件生成", g.ClusterName)
	steps := 6

	// 1. Validate configuration and dependencies
	g.printStep(1, steps, "验证配置和依赖")
	if err := g.ValidateConfig(); err != nil {
		g.printError("配置验证失败", err)
		return err
	}
	g.printSuccess("配置验证通过")

	// 2. Create PXE directory structure
	g.printStep(2, steps, "创建目录结构")
	pxeDir := filepath.Join(g.ClusterDir, pxeDirName)
	if err := g.createPXEDirs(pxeDir); err != nil {
		g.printError("创建目录失败", err)
		return err
	}
	g.printSuccess("目录结构已创建")

	// 3. Generate install-config.yaml
	g.printStep(3, steps, "生成 install-config.yaml")
	if err := g.generateInstallConfig(pxeDir); err != nil {
		g.printError("生成 install-config.yaml 失败", err)
		return err
	}
	g.printSuccess("install-config.yaml 已生成")

	// 4. Generate agent-config.yaml
	g.printStep(4, steps, "生成 agent-config.yaml")
	if err := g.generateAgentConfig(pxeDir, options.AssetServerURL); err != nil {
		g.printError("生成 agent-config.yaml 失败", err)
		return err
	}
	g.printSuccess("agent-config.yaml 已生成")

	// 5. Generate PXE boot files using openshift-install
	g.printStep(5, steps, "生成 PXE 启动文件")
	if err := g.generatePXEFiles(pxeDir, options.AssetServerURL); err != nil {
		g.printError("生成 PXE 文件失败", err)
		return err
	}

	// 6. Upload files to PXE server
	g.printStep(6, steps, "上传文件到 PXE 服务器")
	if err := g.uploadPXEFiles(pxeDir); err != nil {
		g.printWarning("自动上传失败", err)
		g.printManualUploadInstructions(pxeDir)
	} else {
		g.printSuccess("文件已自动上传到服务器")
	}

	g.printCompletion(pxeDir)
	return nil
}

// --- Step Implementations ---

// ValidateConfig checks for required configurations and tools.
func (g *PXEGenerator) ValidateConfig() error {
	if err := config.ValidateConfig(g.Config); err != nil {
		return err
	}

	// Validate required tools
	toolPath := filepath.Join(g.DownloadDir, "bin", openshiftInstallCmd)
	if _, err := os.Stat(toolPath); os.IsNotExist(err) {
		return fmt.Errorf("缺少必需的工具: %s，请先运行 'ocpack download' 命令", openshiftInstallCmd)
	}

	// Validate pull-secret file
	pullSecretPath := filepath.Join(g.ClusterDir, pullSecretFilename)
	if _, err := os.Stat(pullSecretPath); errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("缺少 %s 文件，请先获取 Red Hat pull-secret", pullSecretFilename)
	}

	return nil
}

// createPXEDirs creates the necessary directory structure for PXE files.
func (g *PXEGenerator) createPXEDirs(pxeDir string) error {
	dirs := []string{
		pxeDir,
		filepath.Join(pxeDir, configDirName),
		filepath.Join(pxeDir, filesDirName),
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("创建目录 %s 失败: %w", dir, err)
		}
	}
	return nil
}

// generateInstallConfig generates the install-config.yaml from a template.
func (g *PXEGenerator) generateInstallConfig(pxeDir string) error {
	g.printInfo("从模板生成 install-config.yaml")
	configPath := filepath.Join(pxeDir, configDirName, installConfigFilename)
	return g.generateInstallConfigFromTemplate(configPath)
}

// generateAgentConfig generates the agent-config.yaml from a template.
func (g *PXEGenerator) generateAgentConfig(pxeDir, assetServerURL string) error {
	configPath := filepath.Join(pxeDir, configDirName, agentConfigFilename)
	return g.generateAgentConfigFromTemplate(configPath, assetServerURL)
}

// generatePXEFiles runs 'openshift-install' to create boot files.
func (g *PXEGenerator) generatePXEFiles(pxeDir, assetServerURL string) error {
	openshiftInstallPath, err := g.findOpenshiftInstall()
	if err != nil {
		return fmt.Errorf("查找 openshift-install 失败: %w", err)
	}
	g.printInfo(fmt.Sprintf("使用工具: %s", filepath.Base(openshiftInstallPath)))

	tempDir := filepath.Join(pxeDir, tempDirName)
	if err := os.MkdirAll(tempDir, 0755); err != nil {
		return fmt.Errorf("创建临时目录失败: %w", err)
	}
	defer os.RemoveAll(tempDir)

	// Copy configs to temp dir as openshift-install might modify them.
	for _, filename := range []string{installConfigFilename, agentConfigFilename} {
		src := filepath.Join(pxeDir, configDirName, filename)
		dst := filepath.Join(tempDir, filename)
		if err := utils.CopyFile(src, dst); err != nil {
			return fmt.Errorf("复制 %s 失败: %w", filename, err)
		}
	}

	g.printInfo("执行 openshift-install agent create pxe-files")
	cmd := exec.Command(openshiftInstallPath, "agent", "create", "pxe-files", "--dir", tempDir)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("生成 PXE 文件失败: %w", err)
	}

	filesDir := filepath.Join(pxeDir, filesDirName)
	bootArtifactsDir := filepath.Join(tempDir, "boot-artifacts")

	var fileCount int
	// openshift-install > 4.12 creates a boot-artifacts subdirectory.
	if _, err := os.Stat(bootArtifactsDir); err == nil {
		fileCount, err = g.moveAndCountFiles(bootArtifactsDir, filesDir, nil)
		if err != nil {
			return err
		}
		// URLs in iPXE scripts need to be updated.
		g.updateIPXEScript(filesDir, assetServerURL)
	} else {
		// Older versions place files in the root.
		ignore := map[string]bool{installConfigFilename: true, agentConfigFilename: true}
		fileCount, err = g.moveAndCountFiles(tempDir, filesDir, ignore)
		if err != nil {
			return err
		}
	}

	g.printInfo(fmt.Sprintf("已生成 %d 个 PXE 文件", fileCount))
	g.printSuccess("PXE 启动文件生成完成")
	return nil
}

// uploadPXEFiles uploads the generated PXE files to the bastion server.
func (g *PXEGenerator) uploadPXEFiles(pxeDir string) error {
	filesDir := filepath.Join(pxeDir, filesDirName)
	if _, err := os.Stat(filesDir); errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("PXE 文件目录不存在: %s", filesDir)
	}

	uploadCmdStr := fmt.Sprintf("sudo %s %s", uploadScriptPath, filesDir)
	sshUserHost := fmt.Sprintf("%s@%s", g.Config.Bastion.Username, g.Config.Bastion.IP)

	var sshCmd *exec.Cmd
	if g.Config.Bastion.SSHKeyPath != "" {
		sshCmd = exec.Command("ssh", "-i", g.Config.Bastion.SSHKeyPath, "-o", "StrictHostKeyChecking=no", sshUserHost, uploadCmdStr)
	} else {
		sshCmd = exec.Command("sshpass", "-p", g.Config.Bastion.Password, "ssh", "-o", "StrictHostKeyChecking=no", sshUserHost, uploadCmdStr)
	}

	sshCmd.Stdout = os.Stdout
	sshCmd.Stderr = os.Stderr

	if err := sshCmd.Run(); err != nil {
		return fmt.Errorf("执行上传脚本失败: %w", err)
	}
	return nil
}

// --- Template Generation and Data Gathering ---

// generateInstallConfigFromTemplate fills and writes the install-config.yaml template.
func (g *PXEGenerator) generateInstallConfigFromTemplate(configPath string) error {
	pullSecret, err := g.getPullSecret()
	if err != nil {
		return err
	}

	sshKey, _ := g.getSSHKey() // SSH key is optional

	trustBundle, err := g.getAdditionalTrustBundle()
	if err != nil {
		g.printInfo(fmt.Sprintf("未找到证书文件，将跳过: %v", err))
	} else {
		g.printInfo("已找到并加载 CA 证书")
	}

	icsp, err := g.findAndParseICSP()
	if err != nil {
		g.printInfo(fmt.Sprintf("未找到 ICSP 文件，将跳过: %v", err))
	} else {
		g.printInfo("已找到并解析 ICSP 文件")
	}

	data := struct {
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
	}{
		BaseDomain:            g.Config.ClusterInfo.Domain,
		ClusterName:           g.Config.ClusterInfo.Name,
		NumWorkers:            len(g.Config.Cluster.Worker),
		NumMasters:            len(g.Config.Cluster.ControlPlane),
		MachineNetwork:        utils.ExtractNetworkBase(g.Config.Cluster.Network.MachineNetwork),
		PrefixLength:          utils.ExtractPrefixLength(g.Config.Cluster.Network.MachineNetwork),
		HostPrefix:            23, // Default host prefix
		PullSecret:            pullSecret,
		SSHKeyPub:             sshKey,
		AdditionalTrustBundle: trustBundle,
		ImageContentSources:   icsp,
		ArchShort:             "amd64",
		UseProxy:              false, // Proxy settings can be added here
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

	return g.executeTemplate("templates/install-config.yaml", configPath, data, funcMap)
}

// generateAgentConfigFromTemplate fills and writes the agent-config.yaml template.
func (g *PXEGenerator) generateAgentConfigFromTemplate(configPath, assetServerURL string) error {
	var hosts []HostConfig
	for _, cp := range g.Config.Cluster.ControlPlane {
		hosts = append(hosts, HostConfig{
			Hostname:   cp.Name,
			Role:       "master",
			MACAddress: cp.MAC,
			IPAddress:  cp.IP,
			Interface:  defaultInterface,
		})
	}
	for _, worker := range g.Config.Cluster.Worker {
		hosts = append(hosts, HostConfig{
			Hostname:   worker.Name,
			Role:       "worker",
			MACAddress: worker.MAC,
			IPAddress:  worker.IP,
			Interface:  defaultInterface,
		})
	}

	if assetServerURL == "" {
		assetServerURL = fmt.Sprintf("http://%s:%d/%s", g.Config.Bastion.IP, defaultPxeWebServerPort, pxeDirName)
	}

	data := AgentConfigDataPXE{
		ClusterName:          g.Config.ClusterInfo.Name,
		RendezvousIP:         g.Config.Cluster.ControlPlane[0].IP,
		Hosts:                hosts,
		Port0:                defaultInterface,
		PrefixLength:         utils.ExtractPrefixLength(g.Config.Cluster.Network.MachineNetwork),
		NextHopAddress:       utils.ExtractGateway(g.Config.Cluster.Network.MachineNetwork),
		DNSServers:           []string{g.Config.Bastion.IP},
		BootArtifactsBaseURL: assetServerURL,
	}

	err := g.executeTemplate("templates/agent-config-pxe.yaml", configPath, data, nil)
	if err == nil {
		g.printInfo(fmt.Sprintf("bootArtifactsBaseURL: %s", assetServerURL))
	}
	return err
}

// getPullSecret reads the pull secret from either merged-auth.json or pull-secret.txt.
func (g *PXEGenerator) getPullSecret() (string, error) {
	mergedAuthPath := filepath.Join(g.ClusterDir, registryDirName, mergedAuthFilename)
	if _, err := os.Stat(mergedAuthPath); err == nil {
		g.printInfo("使用合并后的认证文件 " + mergedAuthFilename)
		secretBytes, err := os.ReadFile(mergedAuthPath)
		if err != nil {
			return "", fmt.Errorf("读取合并认证文件失败: %w", err)
		}
		return strings.TrimSpace(string(secretBytes)), nil
	}

	pullSecretPath := filepath.Join(g.ClusterDir, pullSecretFilename)
	g.printInfo("使用 " + pullSecretFilename)
	secretBytes, err := os.ReadFile(pullSecretPath)
	if err != nil {
		return "", fmt.Errorf("读取 pull-secret 失败: %w", err)
	}
	return strings.TrimSpace(string(secretBytes)), nil
}

// getSSHKey reads the user's public SSH key.
func (g *PXEGenerator) getSSHKey() (string, error) {
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

// getAdditionalTrustBundle finds and reads the custom CA certificate.
func (g *PXEGenerator) getAdditionalTrustBundle() (string, error) {
	possibleCertPaths := []string{
		filepath.Join(g.ClusterDir, registryDirName, g.Config.Registry.IP, "rootCA.pem"),
		filepath.Join(g.ClusterDir, registryDirName, fmt.Sprintf("registry.%s.%s", g.Config.ClusterInfo.Name, g.Config.ClusterInfo.Domain), "rootCA.pem"),
		filepath.Join(g.ClusterDir, registryDirName, "rootCA.pem"),
	}

	for _, certPath := range possibleCertPaths {
		if caCertBytes, err := os.ReadFile(certPath); err == nil {
			return string(caCertBytes), nil
		}
	}
	return "", errors.New("未在任何预期位置找到 rootCA.pem")
}

// --- ICSP Parsing (Robust Version) ---

// ICSP represents the structure of an ImageContentSourcePolicy YAML file.
type ICSP struct {
	Spec struct {
		RepositoryDigestMirrors []struct {
			Source  string   `yaml:"source"`
			Mirrors []string `yaml:"mirrors"`
		} `yaml:"repositoryDigestMirrors"`
	} `yaml:"spec"`
}

// findAndParseICSP finds the latest ICSP file and parses it robustly using a YAML library.
func (g *PXEGenerator) findAndParseICSP() (string, error) {
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
			// Using a simple template for consistent formatting.
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

// findOcMirrorWorkspace locates the oc-mirror workspace directory.
func (g *PXEGenerator) findOcMirrorWorkspace() (string, error) {
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

// findLatestResultsDir finds the most recent non-empty 'results-*' directory.
func (g *PXEGenerator) findLatestResultsDir(workspaceDir string) (string, error) {
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

		timestampStr := strings.TrimPrefix(entry.Name(), "results-")
		if timeValue, err := utils.ParseTimestamp(timestampStr); err == nil {
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

// --- Utility and Helper Functions ---

// executeTemplate parses a template, executes it with data, and writes to a file.
func (g *PXEGenerator) executeTemplate(templatePath, outputPath string, data interface{}, funcMap template.FuncMap) error {
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
		return fmt.Errorf("生成文件 %s 失败: %w", outputPath, err)
	}
	return nil
}

// updateIPXEScript replaces hardcoded URLs in iPXE scripts with the correct asset server URL.
func (g *PXEGenerator) updateIPXEScript(filesDir, assetServerURL string) error {
	ipxeFiles, err := filepath.Glob(filepath.Join(filesDir, "*.ipxe"))
	if err != nil || len(ipxeFiles) == 0 {
		return err // No files found is not an error here
	}

	if assetServerURL == "" {
		assetServerURL = fmt.Sprintf("http://%s:%d/%s", g.Config.Bastion.IP, defaultPxeWebServerPort, g.ClusterName)
	}

	// This is the default URL structure generated by openshift-install
	oldBaseURL := fmt.Sprintf("http://%s:8080/pxe", g.Config.Bastion.IP)

	for _, ipxeFile := range ipxeFiles {
		content, err := os.ReadFile(ipxeFile)
		if err != nil {
			g.printWarning(fmt.Sprintf("读取 iPXE 文件 %s 失败", ipxeFile), err)
			continue
		}
		// Replace the base URL prefix for all assets
		updatedContent := strings.ReplaceAll(string(content), oldBaseURL, assetServerURL)

		if err := os.WriteFile(ipxeFile, []byte(updatedContent), 0644); err != nil {
			return fmt.Errorf("更新 iPXE 文件 %s 失败: %w", ipxeFile, err)
		}
	}
	return nil
}

// findOpenshiftInstall locates the openshift-install binary.
func (g *PXEGenerator) findOpenshiftInstall() (string, error) {
	// Prioritize the version extracted from the local registry
	registryHost := fmt.Sprintf("registry.%s.%s", g.Config.ClusterInfo.Name, g.Config.ClusterInfo.Domain)
	extractedBinary := filepath.Join(g.ClusterDir, fmt.Sprintf("%s-%s-%s", openshiftInstallCmd, g.Config.ClusterInfo.OpenShiftVersion, registryHost))
	if _, err := os.Stat(extractedBinary); err == nil {
		return extractedBinary, nil
	}

	// Fallback to the one in the download directory
	downloadedBinary := filepath.Join(g.DownloadDir, "bin", openshiftInstallCmd)
	if _, err := os.Stat(downloadedBinary); err == nil {
		return downloadedBinary, nil
	}

	return "", fmt.Errorf("在 %s 或 %s 中未找到 %s 工具", extractedBinary, downloadedBinary, openshiftInstallCmd)
}

// moveAndCountFiles moves files from src to dst, ignoring specified files, and returns the count.
func (g *PXEGenerator) moveAndCountFiles(srcDir, dstDir string, ignore map[string]bool) (int, error) {
	entries, err := os.ReadDir(srcDir)
	if err != nil {
		return 0, fmt.Errorf("读取目录 %s 失败: %w", srcDir, err)
	}

	count := 0
	for _, entry := range entries {
		if entry.IsDir() || (ignore != nil && ignore[entry.Name()]) {
			continue
		}
		srcPath := filepath.Join(srcDir, entry.Name())
		dstPath := filepath.Join(dstDir, entry.Name())
		if err := utils.MoveFile(srcPath, dstPath); err == nil {
			count++
		} else {
			g.printWarning(fmt.Sprintf("移动文件 %s 失败", entry.Name()), err)
		}
	}
	return count, nil
}

// --- Console Output Formatting (Alignment Fixed) ---

// padRight pads a string with spaces on the right, respecting multi-width characters.
func padRight(s string, totalWidth int) string {
	return s + strings.Repeat(" ", totalWidth-runewidth.StringWidth(s))
}

func (g *PXEGenerator) printHeader(title, clusterName string) {
	fullTitle := fmt.Sprintf("%s - %s", title, clusterName)
	fmt.Printf("\n╔══════════════════════════════════════════════════════════════╗\n")
	fmt.Printf("║ %s ║\n", padRight(fullTitle, 60))
	fmt.Printf("╚══════════════════════════════════════════════════════════════╝\n\n")
}

func (g *PXEGenerator) printStep(current, total int, description string) {
	fmt.Printf("📋 [%d/%d] %s\n", current, total, description)
}

func (g *PXEGenerator) printSuccess(message string) {
	fmt.Printf("   ✅ %s\n\n", message)
}

func (g *PXEGenerator) printError(title string, err error) {
	fmt.Printf("   ❌ %s: %v\n\n", title, err)
}

func (g *PXEGenerator) printWarning(title string, err error) {
	fmt.Printf("   ⚠️  %s: %v\n", title, err)
}

func (g *PXEGenerator) printInfo(message string) {
	fmt.Printf("   ℹ️  %s\n", message)
}

func (g *PXEGenerator) printManualUploadInstructions(pxeDir string) {
	filesPath := filepath.Join(pxeDir, filesDirName)
	sshUser := g.Config.Bastion.Username
	sshIP := g.Config.Bastion.IP

	fmt.Printf("\n📋 手动上传步骤:\n")
	fmt.Printf("┌─────────────────────────────────────────────────────────────┐\n")
	fmt.Printf("│ 1. 使用上传脚本:                                            │\n")
	fmt.Printf("│    ssh %s@%s 'sudo %s %s' │\n", sshUser, sshIP, uploadScriptPath, filesPath)
	fmt.Printf("│                                                             │\n")
	fmt.Printf("│ 2. 或手动复制文件:                                          │\n")
	fmt.Printf("│    scp %s/* %s@%s:/tmp/                │\n", filesPath, sshUser, sshIP)
	fmt.Printf("│    ssh %s@%s 'sudo cp /tmp/agent.x86_64-* /var/www/html/pxe/%s/' │\n", sshUser, sshIP, g.ClusterName)
	fmt.Printf("│                                                             │\n")
	fmt.Printf("│ 3. 验证文件访问:                                            │\n")
	fmt.Printf("│    curl http://%s:%d/pxe/%s/                         │\n", sshIP, defaultPxeWebServerPort, g.ClusterName)
	fmt.Printf("└─────────────────────────────────────────────────────────────┘\n\n")
}

func (g *PXEGenerator) printCompletion(pxeDir string) {
	pxeURL := fmt.Sprintf("http://%s:%d/pxe/%s", g.Config.Bastion.IP, defaultPxeWebServerPort, g.ClusterName)
	fmt.Printf("╔══════════════════════════════════════════════════════════════╗\n")
	fmt.Printf("║ %s ║\n", padRight("✅ PXE 文件生成完成！", 60))
	fmt.Printf("║ %s ║\n", padRight("", 60))
	fmt.Printf("║ %s ║\n", padRight(fmt.Sprintf("📁 文件位置: %s", pxeDir), 60))
	fmt.Printf("║ %s ║\n", padRight(fmt.Sprintf("🌐 PXE 服务器: %s", pxeURL), 60))
	fmt.Printf("║ %s ║\n", padRight("", 60))
	fmt.Printf("║ %s ║\n", padRight("🚀 下一步: 配置目标机器从 PXE 启动", 60))
	fmt.Printf("╚══════════════════════════════════════════════════════════════╝\n")
}
