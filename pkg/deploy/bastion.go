package deploy

import (
	"fmt"

	"ocpack/pkg/config"
)

// --- Constants ---
// 优化：将硬编码的端口号定义为常量，便于管理
const (
	dnsPort       = 53
	haproxyPort   = 9000
	apiServerPort = 6443
)

// BastionDeployer 用于部署 Bastion 节点
type BastionDeployer struct {
	config      *config.ClusterConfig
	downloadDir string
}

// NewBastionDeployer 创建一个新的 Bastion 部署器
func NewBastionDeployer(cfg *config.ClusterConfig, downloadDir string) *BastionDeployer {
	return &BastionDeployer{
		config:      cfg,
		downloadDir: downloadDir,
	}
}

// Deploy 执行 Bastion 节点部署
// 优化：重构为职责更单一的“编排器”函数
func (d *BastionDeployer) Deploy(configFilePath string) error {
	fmt.Printf("▶️  开始部署 Bastion 节点 (%s)...\n", d.config.Bastion.IP)

	// 1. 创建 Ansible 执行器
	fmt.Println("➡️  正在初始化部署环境...")
	executor, err := NewAnsibleExecutor(d.config, configFilePath)
	if err != nil {
		return fmt.Errorf("创建 Ansible 执行器失败: %w", err)
	}
	defer executor.Cleanup()

	// 2. 执行 Bastion playbook
	fmt.Println("🚀 正在执行 Bastion 部署 playbook (此过程可能需要几分钟)...")
	if err := executor.RunBastionPlaybook(); err != nil {
		return fmt.Errorf("Bastion 节点部署失败: %w", err)
	}

	// 3. 打印成功信息
	// 优化：调用独立的函数来打印最终的成功信息
	d.printSuccessMessage()

	return nil
}

// printSuccessMessage 打印部署成功后的信息
// 优化：提取重复的打印逻辑到此函数中
func (d *BastionDeployer) printSuccessMessage() {
	fmt.Println("\n✅ Bastion 节点部署完成！")
	fmt.Printf("   DNS 服务器: %s:%d\n", d.config.Bastion.IP, dnsPort)
	fmt.Printf("   HAProxy 统计页面: http://%s:%d/stats\n", d.config.Bastion.IP, haproxyPort)
}

/*
// NewAnsibleExecutor 和其他相关函数被假定存在于此包中
// 例如：
type AnsibleExecutor struct {
    // fields...
}
func NewAnsibleExecutor(cfg *config.ClusterConfig, configFilePath string) (*AnsibleExecutor, error) {
    // implementation...
    return &AnsibleExecutor{}, nil
}
func (e *AnsibleExecutor) RunBastionPlaybook() error {
    // implementation...
    return nil
}
func (e *AnsibleExecutor) Cleanup() {
    // implementation...
}
*/
