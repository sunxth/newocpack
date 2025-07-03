package deploy

import (
	"fmt"

	"ocpack/pkg/config"
)

// PXEDeployer 用于部署 PXE 服务
type PXEDeployer struct {
	config      *config.ClusterConfig
	downloadDir string
}

// NewPXEDeployer 创建一个新的 PXE 部署器
func NewPXEDeployer(cfg *config.ClusterConfig, downloadDir string) *PXEDeployer {
	return &PXEDeployer{
		config:      cfg,
		downloadDir: downloadDir,
	}
}

// Deploy 执行 PXE 服务部署
func (d *PXEDeployer) Deploy(configFilePath string) error {
	fmt.Printf("开始在 Bastion 节点 (%s) 上部署 PXE 服务...\n", d.config.Bastion.IP)

	// 使用 Ansible 执行器进行部署
	executor, err := NewAnsibleExecutor(d.config, configFilePath)
	if err != nil {
		return fmt.Errorf("创建 Ansible 执行器失败: %w", err)
	}
	defer executor.Cleanup()

	// 执行 PXE playbook
	if err := executor.RunPXEPlaybook(); err != nil {
		return fmt.Errorf("PXE 服务部署失败: %w", err)
	}

	fmt.Println("PXE 服务部署完成！")
	fmt.Printf("PXE 服务器: %s\n", d.config.Bastion.IP)
	fmt.Printf("TFTP 服务: tftp://%s\n", d.config.Bastion.IP)
	fmt.Printf("HTTP 服务: http://%s:8080/pxe (端口8080避免与HAProxy冲突)\n", d.config.Bastion.IP)
	fmt.Printf("DHCP 服务: %s (已配置MAC-IP映射)\n", d.config.Bastion.IP)
	fmt.Printf("PXE 文件目录: /var/lib/tftpboot\n")
	fmt.Printf("HTTP 文件目录: /var/www/html/pxe\n")

	return nil
}
