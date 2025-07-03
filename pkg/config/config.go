package config

import (
	"fmt"
	"os"

	"ocpack/pkg/utils"

	"github.com/pelletier/go-toml/v2"
)

// ClusterConfig 表示集群配置
type ClusterConfig struct {
	// 集群基本信息
	ClusterInfo struct {
		Name             string `toml:"name"`
		Domain           string `toml:"domain"`
		ClusterID        string `toml:"cluster_id"`
		OpenShiftVersion string `toml:"openshift_version"`
	} `toml:"cluster_info"`

	// Bastion 节点配置
	Bastion struct {
		IP         string `toml:"ip"`
		Username   string `toml:"username"`
		SSHKeyPath string `toml:"ssh_key_path"`
		Password   string `toml:"password"`
	} `toml:"bastion"`

	// Registry 节点配置
	Registry struct {
		IP           string `toml:"ip"`
		Username     string `toml:"username"`
		SSHKeyPath   string `toml:"ssh_key_path"`
		Password     string `toml:"password"`
		StoragePath  string `toml:"storage_path"`
		RegistryUser string `toml:"registry_user"`
	} `toml:"registry"`

	// 集群节点配置
	Cluster struct {
		// Control Plane 节点
		ControlPlane []struct {
			Name string `toml:"name"`
			IP   string `toml:"ip"`
			MAC  string `toml:"mac"`
		} `toml:"control_plane"`

		// Worker 节点
		Worker []struct {
			Name string `toml:"name"`
			IP   string `toml:"ip"`
			MAC  string `toml:"mac"`
		} `toml:"worker"`

		// 网络配置
		Network struct {
			ClusterNetwork string `toml:"cluster_network"`
			ServiceNetwork string `toml:"service_network"`
			MachineNetwork string `toml:"machine_network"`
		} `toml:"network"`
	} `toml:"cluster"`

	// 下载配置
	Download struct {
		LocalPath string `toml:"local_path"`
	} `toml:"download"`

	// 镜像保存配置
	SaveImage struct {
		IncludeOperators bool     `toml:"include_operators"`
		OperatorCatalog  string   `toml:"operator_catalog"`
		Ops              []string `toml:"ops"`
		AdditionalImages []string `toml:"additional_images"`
	} `toml:"save_image"`

	// Operator 配置列表
	Operators []struct {
		Name      string `toml:"name"`
		Namespace string `toml:"namespace"`
	} `toml:"operators"`
}

// NewDefaultConfig 创建默认配置
func NewDefaultConfig(clusterName string) *ClusterConfig {
	config := &ClusterConfig{}

	// 设置默认值
	config.ClusterInfo.Name = clusterName
	config.ClusterInfo.Domain = "example.com"
	config.ClusterInfo.ClusterID = clusterName
	config.ClusterInfo.OpenShiftVersion = "4.14.0"

	config.Bastion.Username = "root"

	config.Registry.Username = "root"
	config.Registry.StoragePath = "/var/lib/registry"
	config.Registry.RegistryUser = "ocp4"

	// 设置集群节点默认值
	config.Cluster.ControlPlane = []struct {
		Name string `toml:"name"`
		IP   string `toml:"ip"`
		MAC  string `toml:"mac"`
	}{
		{Name: "master-0", IP: "", MAC: ""},
		{Name: "master-1", IP: "", MAC: ""},
		{Name: "master-2", IP: "", MAC: ""},
	}

	config.Cluster.Worker = []struct {
		Name string `toml:"name"`
		IP   string `toml:"ip"`
		MAC  string `toml:"mac"`
	}{
		{Name: "worker-0", IP: "", MAC: ""},
		{Name: "worker-1", IP: "", MAC: ""},
	}

	// 设置网络默认值
	config.Cluster.Network.ClusterNetwork = "10.128.0.0/14"
	config.Cluster.Network.ServiceNetwork = "172.30.0.0/16"
	config.Cluster.Network.MachineNetwork = "192.168.1.0/24"

	config.Download.LocalPath = "downloads"

	// 设置镜像保存默认值
	config.SaveImage.IncludeOperators = false
	config.SaveImage.OperatorCatalog = fmt.Sprintf("registry.redhat.io/redhat/redhat-operator-index:v%s", utils.ExtractMajorVersion(config.ClusterInfo.OpenShiftVersion))
	config.SaveImage.Ops = []string{
		// 示例 Operator，用户可以根据需要修改
		"cluster-logging",
		"local-storage-operator",
	}
	config.SaveImage.AdditionalImages = []string{
		// 示例额外镜像，用户可以根据需要添加
	}

	return config
}

// GenerateDefaultConfig 生成默认配置文件
func GenerateDefaultConfig(filePath string, clusterName string) error {
	config := NewDefaultConfig(clusterName)

	// 使用自定义模板生成配置文件
	configContent := fmt.Sprintf(`# OpenShift 集群配置文件
# 请根据实际环境修改以下配置项

[cluster_info]
name = "%s"                    # 集群名称
domain = "%s"                  # 集群域名
cluster_id = "%s"              # 集群ID
openshift_version = "%s"       # OpenShift 版本

[bastion]
ip = ""                        # Bastion 节点 IP (必填)
username = "%s"                # SSH 用户名
ssh_key_path = ""              # SSH 私钥路径 (可选，与 password 二选一)
password = ""                  # SSH 密码 (可选，与 ssh_key_path 二选一)

[registry]
ip = ""                        # Registry 节点 IP (必填)
username = "%s"                # SSH 用户名
ssh_key_path = ""              # SSH 私钥路径 (可选，与 password 二选一)
password = ""                  # SSH 密码 (可选，与 ssh_key_path 二选一)
storage_path = "%s"            # 镜像存储路径
registry_user = "%s"           # Registry 用户名

# Control Plane 节点配置
[[cluster.control_plane]]
name = "master-0"
ip = ""                        # 节点 IP (必填)
mac = ""                       # 节点 MAC 地址 (必填)

[[cluster.control_plane]]
name = "master-1"
ip = ""
mac = ""

[[cluster.control_plane]]
name = "master-2"
ip = ""
mac = ""

# Worker 节点配置
[[cluster.worker]]
name = "worker-0"
ip = ""                        # 节点 IP (必填)
mac = ""                       # 节点 MAC 地址 (必填)

[[cluster.worker]]
name = "worker-1"
ip = ""
mac = ""

[cluster.network]
cluster_network = "%s"         # 集群网络 CIDR
service_network = "%s"         # 服务网络 CIDR
machine_network = "%s"         # 机器网络 CIDR

[download]
local_path = "%s"              # 下载文件存储路径

[save_image]
include_operators = %t         # 是否包含 Operator 镜像
operator_catalog = "%s"        # Operator 目录镜像 (自动设置)
ops = [                        # 需要的 Operator 列表
  "%s",
  "%s"
]
additional_images = []         # 额外的镜像列表
`,
		config.ClusterInfo.Name,
		config.ClusterInfo.Domain,
		config.ClusterInfo.ClusterID,
		config.ClusterInfo.OpenShiftVersion,
		config.Bastion.Username,
		config.Registry.Username,
		config.Registry.StoragePath,
		config.Registry.RegistryUser,
		config.Cluster.Network.ClusterNetwork,
		config.Cluster.Network.ServiceNetwork,
		config.Cluster.Network.MachineNetwork,
		config.Download.LocalPath,
		config.SaveImage.IncludeOperators,
		config.SaveImage.OperatorCatalog,
		config.SaveImage.Ops[0],
		config.SaveImage.Ops[1],
	)

	// 写入文件
	if err := os.WriteFile(filePath, []byte(configContent), 0644); err != nil {
		return fmt.Errorf("写入配置文件失败: %w", err)
	}

	return nil
}

// LoadConfig 从文件加载配置
func LoadConfig(filePath string) (*ClusterConfig, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("读取配置文件失败: %w", err)
	}

	config := &ClusterConfig{}
	if err := toml.Unmarshal(data, config); err != nil {
		return nil, fmt.Errorf("解析配置文件失败: %w", err)
	}

	return config, nil
}

// SaveConfig 保存配置到文件
func SaveConfig(config *ClusterConfig, filePath string) error {
	data, err := toml.Marshal(config)
	if err != nil {
		return fmt.Errorf("序列化配置失败: %w", err)
	}

	if err := os.WriteFile(filePath, data, 0644); err != nil {
		return fmt.Errorf("写入配置文件失败: %w", err)
	}

	return nil
}

// ValidateConfig 验证配置是否有效
func ValidateConfig(config *ClusterConfig) error {
	// 验证集群基本信息
	if config.ClusterInfo.Name == "" {
		return fmt.Errorf("集群名称不能为空")
	}
	if config.ClusterInfo.Domain == "" {
		return fmt.Errorf("集群域名不能为空")
	}
	if config.ClusterInfo.ClusterID == "" {
		return fmt.Errorf("集群ID不能为空")
	}
	if config.ClusterInfo.OpenShiftVersion == "" {
		return fmt.Errorf("OpenShift版本不能为空")
	}

	// 验证Bastion节点配置
	if config.Bastion.IP == "" {
		return fmt.Errorf("Bastion节点IP不能为空")
	}
	if config.Bastion.Username == "" {
		return fmt.Errorf("Bastion节点用户名不能为空")
	}
	if config.Bastion.SSHKeyPath == "" && config.Bastion.Password == "" {
		return fmt.Errorf("Bastion节点必须提供SSH密钥或密码")
	}

	// 验证Registry节点配置
	if config.Registry.IP == "" {
		return fmt.Errorf("Registry节点IP不能为空")
	}
	if config.Registry.Username == "" {
		return fmt.Errorf("Registry节点用户名不能为空")
	}
	if config.Registry.SSHKeyPath == "" && config.Registry.Password == "" {
		return fmt.Errorf("Registry节点必须提供SSH密钥或密码")
	}
	if config.Registry.StoragePath == "" {
		return fmt.Errorf("Registry节点存储路径不能为空")
	}

	// 验证集群节点配置
	if len(config.Cluster.ControlPlane) == 0 {
		return fmt.Errorf("至少需要配置一个Control Plane节点")
	}

	for i, cp := range config.Cluster.ControlPlane {
		if cp.Name == "" {
			return fmt.Errorf("Control Plane节点[%d]名称不能为空", i)
		}
		if cp.IP == "" {
			return fmt.Errorf("Control Plane节点[%d] %s 的IP不能为空", i, cp.Name)
		}
		if cp.MAC == "" {
			return fmt.Errorf("Control Plane节点[%d] %s 的MAC地址不能为空", i, cp.Name)
		}
	}

	for i, worker := range config.Cluster.Worker {
		if worker.Name == "" {
			return fmt.Errorf("Worker节点[%d]名称不能为空", i)
		}
		if worker.IP == "" {
			return fmt.Errorf("Worker节点[%d] %s 的IP不能为空", i, worker.Name)
		}
		if worker.MAC == "" {
			return fmt.Errorf("Worker节点[%d] %s 的MAC地址不能为空", i, worker.Name)
		}
	}

	// 验证网络配置
	if config.Cluster.Network.ClusterNetwork == "" {
		return fmt.Errorf("集群网络CIDR不能为空")
	}
	if config.Cluster.Network.ServiceNetwork == "" {
		return fmt.Errorf("服务网络CIDR不能为空")
	}
	if config.Cluster.Network.MachineNetwork == "" {
		return fmt.Errorf("机器网络CIDR不能为空")
	}

	return nil
}

// ValidateBastionConfig 验证 Bastion 部署所需的配置
func ValidateBastionConfig(config *ClusterConfig) error {
	// 验证集群基本信息
	if config.ClusterInfo.Name == "" {
		return fmt.Errorf("集群名称不能为空")
	}
	if config.ClusterInfo.Domain == "" {
		return fmt.Errorf("集群域名不能为空")
	}
	if config.ClusterInfo.ClusterID == "" {
		return fmt.Errorf("集群ID不能为空")
	}
	if config.ClusterInfo.OpenShiftVersion == "" {
		return fmt.Errorf("OpenShift版本不能为空")
	}

	// 验证Bastion节点配置
	if config.Bastion.IP == "" {
		return fmt.Errorf("Bastion节点IP不能为空")
	}
	if config.Bastion.Username == "" {
		return fmt.Errorf("Bastion节点用户名不能为空")
	}
	if config.Bastion.SSHKeyPath == "" && config.Bastion.Password == "" {
		return fmt.Errorf("Bastion节点必须提供SSH密钥或密码")
	}

	// 验证Registry节点IP（Bastion需要配置Registry的DNS解析）
	if config.Registry.IP == "" {
		return fmt.Errorf("Registry节点IP不能为空（Bastion需要配置Registry的DNS解析）")
	}

	// 验证集群节点配置（Bastion 需要这些信息来配置 DNS 和 HAProxy）
	if len(config.Cluster.ControlPlane) == 0 {
		return fmt.Errorf("至少需要配置一个Control Plane节点")
	}

	for i, cp := range config.Cluster.ControlPlane {
		if cp.Name == "" {
			return fmt.Errorf("Control Plane节点[%d]名称不能为空", i)
		}
		if cp.IP == "" {
			return fmt.Errorf("Control Plane节点[%d] %s 的IP不能为空", i, cp.Name)
		}
		// MAC 地址对于 Bastion 部署不是必需的
	}

	for i, worker := range config.Cluster.Worker {
		if worker.Name == "" {
			return fmt.Errorf("Worker节点[%d]名称不能为空", i)
		}
		if worker.IP == "" {
			return fmt.Errorf("Worker节点[%d] %s 的IP不能为空", i, worker.Name)
		}
		// MAC 地址对于 Bastion 部署不是必需的
	}

	// 验证网络配置
	if config.Cluster.Network.ClusterNetwork == "" {
		return fmt.Errorf("集群网络CIDR不能为空")
	}
	if config.Cluster.Network.ServiceNetwork == "" {
		return fmt.Errorf("服务网络CIDR不能为空")
	}
	if config.Cluster.Network.MachineNetwork == "" {
		return fmt.Errorf("机器网络CIDR不能为空")
	}

	return nil
}

// ValidateRegistryConfig 验证 Registry 部署所需的配置
func ValidateRegistryConfig(config *ClusterConfig) error {
	// 验证集群基本信息
	if config.ClusterInfo.Name == "" {
		return fmt.Errorf("集群名称不能为空")
	}
	if config.ClusterInfo.OpenShiftVersion == "" {
		return fmt.Errorf("OpenShift版本不能为空")
	}

	// 验证Registry节点配置
	if config.Registry.IP == "" {
		return fmt.Errorf("Registry节点IP不能为空")
	}
	if config.Registry.Username == "" {
		return fmt.Errorf("Registry节点用户名不能为空")
	}
	if config.Registry.SSHKeyPath == "" && config.Registry.Password == "" {
		return fmt.Errorf("Registry节点必须提供SSH密钥或密码")
	}
	if config.Registry.StoragePath == "" {
		return fmt.Errorf("Registry节点存储路径不能为空")
	}

	return nil
}

// ValidateRegistryConfigWithDownloads 验证 Registry 部署所需的配置和下载文件
func ValidateRegistryConfigWithDownloads(config *ClusterConfig, downloadDir string) error {
	// 先验证基本配置
	if err := ValidateRegistryConfig(config); err != nil {
		return err
	}

	// 验证必需的下载文件是否存在
	requiredFiles := []struct {
		path        string
		description string
	}{
		{
			path:        downloadDir + "/mirror-registry-amd64.tar.gz",
			description: "Quay 镜像仓库安装包",
		},
		{
			path:        downloadDir + "/bin/oc",
			description: "OpenShift 客户端工具",
		},
		{
			path:        downloadDir + "/bin/kubectl",
			description: "Kubernetes 客户端工具",
		},
		{
			path:        downloadDir + "/bin/oc-mirror",
			description: "OpenShift 镜像同步工具",
		},
	}

	for _, file := range requiredFiles {
		if _, err := os.Stat(file.path); os.IsNotExist(err) {
			return fmt.Errorf("缺少必需的文件: %s (%s)\n请先运行 'ocpack download' 命令下载所需文件", file.path, file.description)
		}
	}

	return nil
}

// ValidateDownloadConfig 验证下载功能所需的配置
func ValidateDownloadConfig(config *ClusterConfig) error {
	// 验证集群基本信息
	if config.ClusterInfo.OpenShiftVersion == "" {
		return fmt.Errorf("OpenShift版本不能为空")
	}

	return nil
}
