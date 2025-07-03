package deploy

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"text/template"

	"ocpack/pkg/config"
)

//go:embed ansible/bastion/*
var bastionAnsibleFiles embed.FS

//go:embed ansible/registry/*
var registryAnsibleFiles embed.FS

//go:embed ansible/pxe/*
var pxeAnsibleFiles embed.FS

// AnsibleExecutor 处理 Ansible playbook 的执行
type AnsibleExecutor struct {
	config         *config.ClusterConfig
	workDir        string
	inventory      string
	ConfigFilePath string // 配置文件路径
}

// NewAnsibleExecutor 创建新的 Ansible 执行器
func NewAnsibleExecutor(cfg *config.ClusterConfig, configFilePath string) (*AnsibleExecutor, error) {
	// 创建临时工作目录
	workDir, err := os.MkdirTemp("", "ocpack-ansible-*")
	if err != nil {
		return nil, fmt.Errorf("创建临时目录失败: %w", err)
	}

	return &AnsibleExecutor{
		config:         cfg,
		workDir:        workDir,
		ConfigFilePath: configFilePath,
	}, nil
}

// getAnsibleEnv 获取 Ansible 执行环境变量
func (ae *AnsibleExecutor) getAnsibleEnv() []string {
	env := os.Environ()

	// 设置基本的 Ansible 环境变量以获得清洁的输出
	env = append(env, "ANSIBLE_STDOUT_CALLBACK=default")     // 使用默认回调插件
	env = append(env, "ANSIBLE_HOST_KEY_CHECKING=false")     // 禁用主机密钥检查
	env = append(env, "ANSIBLE_DISPLAY_SKIPPED_HOSTS=false") // 不显示跳过的主机
	env = append(env, "ANSIBLE_VERBOSITY=0")                 // 设置最小详细程度

	return env
}

// ExtractBastionFiles 提取 bastion 相关的 Ansible 文件到临时目录
func (ae *AnsibleExecutor) ExtractBastionFiles() error {
	// 提取所有嵌入的文件
	err := fs.WalkDir(bastionAnsibleFiles, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// 跳过根目录
		if path == "." {
			return nil
		}

		// 创建目标路径
		targetPath := filepath.Join(ae.workDir, path)

		if d.IsDir() {
			// 创建目录
			return os.MkdirAll(targetPath, 0755)
		}

		// 读取文件内容
		content, err := bastionAnsibleFiles.ReadFile(path)
		if err != nil {
			return fmt.Errorf("读取嵌入文件 %s 失败: %w", path, err)
		}

		// 确保目标目录存在
		if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
			return fmt.Errorf("创建目录 %s 失败: %w", filepath.Dir(targetPath), err)
		}

		// 写入文件
		if err := os.WriteFile(targetPath, content, 0644); err != nil {
			return fmt.Errorf("写入文件 %s 失败: %w", targetPath, err)
		}

		return nil
	})

	if err != nil {
		return fmt.Errorf("提取 Ansible 文件失败: %w", err)
	}

	return nil
}

// GenerateInventory 生成 Ansible inventory 文件
func (ae *AnsibleExecutor) GenerateInventory() error {
	inventoryTemplatePath := filepath.Join(ae.workDir, "ansible/bastion/inventory.ini")

	// 读取模板文件
	templateContent, err := os.ReadFile(inventoryTemplatePath)
	if err != nil {
		return fmt.Errorf("读取 inventory 模板失败: %w", err)
	}

	// 解析模板
	tmpl, err := template.New("inventory").Parse(string(templateContent))
	if err != nil {
		return fmt.Errorf("解析 inventory 模板失败: %w", err)
	}

	// 生成 inventory 文件
	inventoryPath := filepath.Join(ae.workDir, "inventory")
	inventoryFile, err := os.Create(inventoryPath)
	if err != nil {
		return fmt.Errorf("创建 inventory 文件失败: %w", err)
	}
	defer inventoryFile.Close()

	// 执行模板
	if err := tmpl.Execute(inventoryFile, ae.config); err != nil {
		return fmt.Errorf("生成 inventory 文件失败: %w", err)
	}

	ae.inventory = inventoryPath
	return nil
}

// GenerateVarsFile 生成 Ansible 变量文件
func (ae *AnsibleExecutor) GenerateVarsFile() error {
	varsPath := filepath.Join(ae.workDir, "vars.yml")

	// 获取当前工作目录（项目根目录）
	currentDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("获取当前工作目录失败: %w", err)
	}

	// 获取配置文件所在的目录名
	configDir := filepath.Dir(filepath.Join(currentDir, ae.ConfigFilePath))
	clusterDir := filepath.Base(configDir)

	varsContent := fmt.Sprintf(`---
cluster_info:
  name: "%s"
  domain: "%s"
  cluster_id: "%s"

bastion:
  ip: "%s"

registry:
  ip: "%s"
  storage_path: "%s"
  registry_user: "%s"

project_root: "%s"
cluster_dir: "%s"

cluster:
  control_plane:
`, ae.config.ClusterInfo.Name, ae.config.ClusterInfo.Domain, ae.config.ClusterInfo.ClusterID, ae.config.Bastion.IP, ae.config.Registry.IP, ae.config.Registry.StoragePath, ae.config.Registry.RegistryUser, currentDir, clusterDir)

	// 添加 Control Plane 节点
	for _, cp := range ae.config.Cluster.ControlPlane {
		varsContent += fmt.Sprintf(`    - name: "%s"
      ip: "%s"
      mac: "%s"
`, cp.Name, cp.IP, cp.MAC)
	}

	varsContent += "  worker:\n"
	// 添加 Worker 节点
	for _, worker := range ae.config.Cluster.Worker {
		varsContent += fmt.Sprintf(`    - name: "%s"
      ip: "%s"
      mac: "%s"
`, worker.Name, worker.IP, worker.MAC)
	}

	// 添加网络配置
	varsContent += fmt.Sprintf(`  network:
    cluster_network: "%s"
    service_network: "%s"
    machine_network: "%s"
`, ae.config.Cluster.Network.ClusterNetwork, ae.config.Cluster.Network.ServiceNetwork, ae.config.Cluster.Network.MachineNetwork)

	if err := os.WriteFile(varsPath, []byte(varsContent), 0644); err != nil {
		return fmt.Errorf("创建变量文件失败: %w", err)
	}

	return nil
}

// CheckAnsibleInstalled 检查 Ansible 是否已安装
func (ae *AnsibleExecutor) CheckAnsibleInstalled() error {
	_, err := exec.LookPath("ansible-playbook")
	if err != nil {
		return fmt.Errorf("未找到 ansible-playbook 命令，请先安装 Ansible")
	}
	return nil
}

// RunBastionPlaybook 执行 bastion 部署 playbook
func (ae *AnsibleExecutor) RunBastionPlaybook() error {
	// 检查 Ansible 是否安装
	if err := ae.CheckAnsibleInstalled(); err != nil {
		return err
	}

	// 提取文件
	if err := ae.ExtractBastionFiles(); err != nil {
		return err
	}

	// 生成 inventory
	if err := ae.GenerateInventory(); err != nil {
		return err
	}

	// 生成变量文件
	if err := ae.GenerateVarsFile(); err != nil {
		return err
	}

	// 执行 playbook
	playbookPath := filepath.Join(ae.workDir, "ansible/bastion/playbook.yml")
	varsPath := filepath.Join(ae.workDir, "vars.yml")

	cmd := exec.Command("ansible-playbook",
		"-i", ae.inventory,
		"-e", fmt.Sprintf("@%s", varsPath),
		playbookPath,
	)

	// 设置工作目录
	cmd.Dir = ae.workDir

	// 设置环境变量（包括 Ansible 回调插件配置）
	cmd.Env = ae.getAnsibleEnv()

	// 设置输出
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	fmt.Printf("执行 Ansible playbook: %s\n", playbookPath)
	fmt.Printf("使用 inventory: %s\n", ae.inventory)
	fmt.Printf("工作目录: %s\n", ae.workDir)

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("执行 Ansible playbook 失败: %w", err)
	}

	return nil
}

// Cleanup 清理临时文件
func (ae *AnsibleExecutor) Cleanup() error {
	if ae.workDir != "" {
		return os.RemoveAll(ae.workDir)
	}
	return nil
}

// GetWorkDir 获取工作目录路径（用于调试）
func (ae *AnsibleExecutor) GetWorkDir() string {
	return ae.workDir
}

// ExtractRegistryFiles 提取 registry 相关的 Ansible 文件到临时目录
func (ae *AnsibleExecutor) ExtractRegistryFiles() error {
	// 提取所有嵌入的文件
	err := fs.WalkDir(registryAnsibleFiles, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// 跳过根目录
		if path == "." {
			return nil
		}

		// 创建目标路径
		targetPath := filepath.Join(ae.workDir, path)

		if d.IsDir() {
			// 创建目录
			return os.MkdirAll(targetPath, 0755)
		}

		// 读取文件内容
		content, err := registryAnsibleFiles.ReadFile(path)
		if err != nil {
			return fmt.Errorf("读取嵌入文件 %s 失败: %w", path, err)
		}

		// 确保目标目录存在
		if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
			return fmt.Errorf("创建目录 %s 失败: %w", filepath.Dir(targetPath), err)
		}

		// 写入文件
		if err := os.WriteFile(targetPath, content, 0644); err != nil {
			return fmt.Errorf("写入文件 %s 失败: %w", targetPath, err)
		}

		return nil
	})

	if err != nil {
		return fmt.Errorf("提取 Ansible 文件失败: %w", err)
	}

	return nil
}

// GenerateRegistryInventory 生成 Registry Ansible inventory 文件
func (ae *AnsibleExecutor) GenerateRegistryInventory() error {
	inventoryTemplatePath := filepath.Join(ae.workDir, "ansible/registry/inventory.ini")

	// 读取模板文件
	templateContent, err := os.ReadFile(inventoryTemplatePath)
	if err != nil {
		return fmt.Errorf("读取 inventory 模板失败: %w", err)
	}

	// 解析模板
	tmpl, err := template.New("inventory").Parse(string(templateContent))
	if err != nil {
		return fmt.Errorf("解析 inventory 模板失败: %w", err)
	}

	// 生成 inventory 文件
	inventoryPath := filepath.Join(ae.workDir, "registry_inventory")
	inventoryFile, err := os.Create(inventoryPath)
	if err != nil {
		return fmt.Errorf("创建 inventory 文件失败: %w", err)
	}
	defer inventoryFile.Close()

	// 执行模板
	if err := tmpl.Execute(inventoryFile, ae.config); err != nil {
		return fmt.Errorf("生成 inventory 文件失败: %w", err)
	}

	ae.inventory = inventoryPath
	return nil
}

// RunRegistryPlaybook 执行 registry 部署 playbook
func (ae *AnsibleExecutor) RunRegistryPlaybook() error {
	// 检查 Ansible 是否安装
	if err := ae.CheckAnsibleInstalled(); err != nil {
		return err
	}

	// 提取文件
	if err := ae.ExtractRegistryFiles(); err != nil {
		return err
	}

	// 生成 inventory
	if err := ae.GenerateRegistryInventory(); err != nil {
		return err
	}

	// 生成变量文件
	if err := ae.GenerateVarsFile(); err != nil {
		return err
	}

	// 执行 playbook
	playbookPath := filepath.Join(ae.workDir, "ansible/registry/playbook.yml")
	varsPath := filepath.Join(ae.workDir, "vars.yml")

	cmd := exec.Command("ansible-playbook",
		"-i", ae.inventory,
		"-e", fmt.Sprintf("@%s", varsPath),
		playbookPath,
	)

	// 设置工作目录
	cmd.Dir = ae.workDir

	// 设置环境变量（包括 Ansible 回调插件配置）
	cmd.Env = ae.getAnsibleEnv()

	// 设置输出
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	fmt.Printf("执行 Ansible playbook: %s\n", playbookPath)
	fmt.Printf("使用 inventory: %s\n", ae.inventory)
	fmt.Printf("工作目录: %s\n", ae.workDir)

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("执行 Ansible playbook 失败: %w", err)
	}

	return nil
}

// ExtractPXEFiles 提取 PXE 相关的 Ansible 文件到临时目录
func (ae *AnsibleExecutor) ExtractPXEFiles() error {
	// 提取所有嵌入的文件
	err := fs.WalkDir(pxeAnsibleFiles, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// 跳过根目录
		if path == "." {
			return nil
		}

		// 创建目标路径
		targetPath := filepath.Join(ae.workDir, path)

		if d.IsDir() {
			// 创建目录
			return os.MkdirAll(targetPath, 0755)
		}

		// 读取文件内容
		content, err := pxeAnsibleFiles.ReadFile(path)
		if err != nil {
			return fmt.Errorf("读取嵌入文件 %s 失败: %w", path, err)
		}

		// 确保目标目录存在
		if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
			return fmt.Errorf("创建目录 %s 失败: %w", filepath.Dir(targetPath), err)
		}

		// 写入文件
		if err := os.WriteFile(targetPath, content, 0644); err != nil {
			return fmt.Errorf("写入文件 %s 失败: %w", targetPath, err)
		}

		return nil
	})

	if err != nil {
		return fmt.Errorf("提取 Ansible 文件失败: %w", err)
	}

	return nil
}

// GeneratePXEInventory 生成 PXE Ansible inventory 文件
func (ae *AnsibleExecutor) GeneratePXEInventory() error {
	inventoryTemplatePath := filepath.Join(ae.workDir, "ansible/pxe/inventory.ini")

	// 读取模板文件
	templateContent, err := os.ReadFile(inventoryTemplatePath)
	if err != nil {
		return fmt.Errorf("读取 inventory 模板失败: %w", err)
	}

	// 解析模板
	tmpl, err := template.New("inventory").Parse(string(templateContent))
	if err != nil {
		return fmt.Errorf("解析 inventory 模板失败: %w", err)
	}

	// 生成 inventory 文件
	inventoryPath := filepath.Join(ae.workDir, "pxe_inventory")
	inventoryFile, err := os.Create(inventoryPath)
	if err != nil {
		return fmt.Errorf("创建 inventory 文件失败: %w", err)
	}
	defer inventoryFile.Close()

	// 执行模板
	if err := tmpl.Execute(inventoryFile, ae.config); err != nil {
		return fmt.Errorf("生成 inventory 文件失败: %w", err)
	}

	ae.inventory = inventoryPath
	return nil
}

// RunPXEPlaybook 执行 PXE 部署 playbook
func (ae *AnsibleExecutor) RunPXEPlaybook() error {
	// 检查 Ansible 是否安装
	if err := ae.CheckAnsibleInstalled(); err != nil {
		return err
	}

	// 提取文件
	if err := ae.ExtractPXEFiles(); err != nil {
		return err
	}

	// 生成 inventory
	if err := ae.GeneratePXEInventory(); err != nil {
		return err
	}

	// 生成变量文件
	if err := ae.GenerateVarsFile(); err != nil {
		return err
	}

	// 执行 playbook
	playbookPath := filepath.Join(ae.workDir, "ansible/pxe/playbook.yml")
	varsPath := filepath.Join(ae.workDir, "vars.yml")

	cmd := exec.Command("ansible-playbook",
		"-i", ae.inventory,
		"-e", fmt.Sprintf("@%s", varsPath),
		playbookPath,
	)

	// 设置工作目录
	cmd.Dir = ae.workDir

	// 设置环境变量（包括 Ansible 回调插件配置）
	cmd.Env = ae.getAnsibleEnv()

	// 设置输出
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	fmt.Printf("执行 Ansible playbook: %s\n", playbookPath)
	fmt.Printf("使用 inventory: %s\n", ae.inventory)
	fmt.Printf("工作目录: %s\n", ae.workDir)

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("执行 Ansible playbook 失败: %w", err)
	}

	return nil
}
