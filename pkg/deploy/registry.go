package deploy

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"time"

	"ocpack/pkg/config"
)

// --- Constants ---
// 优化: 将硬编码的值定义为常量
const (
	registryPort            = "8443"
	registryHealthEndpoint  = "/health/instance"
	registryDefaultPassword = "ztesoft123"
)

// DeployRegistry 部署 Registry 节点，如果它尚未部署。
func DeployRegistry(cfg *config.ClusterConfig, configFilePath string) error {
	fmt.Println("▶️  开始部署 Registry 节点...")

	// 1. 验证配置
	if err := config.ValidateRegistryConfig(cfg); err != nil {
		return fmt.Errorf("配置验证失败: %w", err)
	}

	// 2. 检查 Registry 是否已经部署
	registryHostPort := fmt.Sprintf("%s:%s", cfg.Registry.IP, registryPort)
	fmt.Printf("➡️  正在检查 Registry 在 %s 的状态...\n", registryHostPort)

	deployed, err := checkRegistryDeployed(cfg)
	if err == nil && deployed {
		fmt.Println("🔄 Registry 节点已经部署并运行。跳过重复部署。")
		printSuccessMessage(cfg) // 优化: 调用统一的成功消息函数
		return nil
	}

	// 如果检查出错，打印信息但继续执行部署，因为错误通常意味着服务不可用
	if err != nil {
		fmt.Printf("ℹ️  检查失败 (这通常意味着 Registry 未部署): %v\n", err)
	}

	// 3. 执行部署
	fmt.Printf("🚀 Registry 未部署或不可访问，开始执行部署 playbook (%s)...\n", cfg.Registry.IP)

	// 创建 Ansible 执行器
	executor, err := NewAnsibleExecutor(cfg, configFilePath)
	if err != nil {
		return fmt.Errorf("创建 Ansible 执行器失败: %w", err)
	}
	defer executor.Cleanup()

	// 执行 Registry playbook
	if err := executor.RunRegistryPlaybook(); err != nil {
		return fmt.Errorf("Registry 节点部署失败: %w", err)
	}

	printSuccessMessage(cfg) // 优化: 调用统一的成功消息函数
	return nil
}

// checkRegistryDeployed 检查 Registry 是否已经部署并返回结果和错误。
// 优化: 返回 (bool, error) 以提供更丰富的上下文。
func checkRegistryDeployed(cfg *config.ClusterConfig) (bool, error) {
	registryURL := fmt.Sprintf("https://%s:%s%s", cfg.Registry.IP, registryPort, registryHealthEndpoint)

	// 创建一个可重用的 HTTP 客户端，并设置超时和不安全的 TLS
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	client := &http.Client{
		Timeout:   10 * time.Second,
		Transport: tr,
	}

	// 尝试访问 Registry 健康检查端点
	resp, err := client.Get(registryURL)
	if err != nil {
		return false, fmt.Errorf("无法访问 health check 端点 '%s': %w", registryURL, err)
	}
	defer resp.Body.Close()

	// 如果返回 200 OK 状态码，说明 Registry 已经部署并运行
	return resp.StatusCode == http.StatusOK, nil
}

// printSuccessMessage 打印部署成功后的信息。
// 优化: 提取重复代码到此函数中。
func printSuccessMessage(cfg *config.ClusterConfig) {
	registryURL := fmt.Sprintf("https://%s:%s", cfg.Registry.IP, registryPort)
	fmt.Println("✅ Registry 部署完成！")
	fmt.Printf("   Quay 镜像仓库: %s\n", registryURL)
	fmt.Printf("   用户名: %s\n", cfg.Registry.RegistryUser)
	fmt.Printf("   密码: %s\n", registryDefaultPassword)
}

/*
// NewAnsibleExecutor and other related functions are assumed to exist in this package.
// For example:
type AnsibleExecutor struct {
    // fields...
}
func NewAnsibleExecutor(cfg *config.ClusterConfig, configFilePath string) (*AnsibleExecutor, error) {
    // implementation...
    return &AnsibleExecutor{}, nil
}
func (e *AnsibleExecutor) RunRegistryPlaybook() error {
    // implementation...
    return nil
}
func (e *AnsibleExecutor) Cleanup() {
    // implementation...
}
*/
