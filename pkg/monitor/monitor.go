package monitor

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// MonitorCluster 监控集群安装进度
func MonitorCluster(clusterName string) error {
	// 构建安装目录路径
	installDir := filepath.Join("installation", "ignition")

	// 检查安装目录是否存在
	if _, err := os.Stat(installDir); os.IsNotExist(err) {
		return fmt.Errorf("安装目录不存在: %s", installDir)
	}

	// 查找 openshift-install 工具
	openshiftInstallPath, err := findOpenshiftInstall()
	if err != nil {
		return fmt.Errorf("找不到 openshift-install 工具: %v", err)
	}

	// 直接执行 openshift-install 命令并透传输出
	cmd := exec.Command(openshiftInstallPath, "agent", "wait-for", "install-complete", "--dir", installDir)

	// 设置输出直接到标准输出和标准错误
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// 执行命令
	return cmd.Run()
}

// findOpenshiftInstall 查找 openshift-install 工具
func findOpenshiftInstall() (string, error) {
	// 首先检查当前目录中的标准名称
	if _, err := os.Stat("./openshift-install"); err == nil {
		// 获取绝对路径
		if absPath, err := filepath.Abs("./openshift-install"); err == nil {
			return absPath, nil
		}
		return "./openshift-install", nil
	}

	// 查找当前目录中以 openshift-install 开头的文件
	files, err := filepath.Glob("./openshift-install*")
	if err == nil && len(files) > 0 {
		// 获取绝对路径
		if absPath, err := filepath.Abs(files[0]); err == nil {
			return absPath, nil
		}
		return files[0], nil
	}

	// 然后检查 PATH 中的 openshift-install
	if path, err := exec.LookPath("openshift-install"); err == nil {
		return path, nil
	}

	return "", fmt.Errorf("openshift-install 工具未找到")
}
