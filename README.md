# ocpack

ocpack 是一个用于离线环境中部署 OpenShift 集群的 Go 语言命令行工具。

## 功能特点

- **项目管理**: 创建和管理集群配置
- **自动化部署**: 使用 Ansible 自动配置 Bastion 和 Registry 节点
- **离线支持**: 下载、保存和加载 OpenShift 安装介质和镜像
- **ISO 生成**: 生成包含 ignition 配置的安装 ISO 镜

## 快速开始

### 1. 安装

```bash
# 使用 Makefile 构建
make build

# 或从源码编译
go build -o ocpack cmd/ocpack/main.go
```

### 2. 创建集群项目

```bash
ocpack new cluster my-cluster
```

### 3. 编辑配置

编辑 `my-cluster/config.toml` 文件，配置节点信息和网络设置。

### 4. 部署流程

#### 一键部署（推荐）

```bash
# 一键执行完整部署流程 (默认 ISO 模式)
ocpack all my-cluster

# 指定部署模式
ocpack all my-cluster --mode=iso    # ISO 模式
ocpack all my-cluster --mode=pxe    # PXE 模式
```

#### 分步部署

```bash
# 下载安装介质
ocpack download my-cluster

# 部署基础设施
ocpack deploy-bastion my-cluster
ocpack deploy-registry my-cluster

# 镜像管理
ocpack save-image my-cluster    # 保存镜像到本地
ocpack load-image my-cluster    # 加载镜像到 registry

# 生成安装介质
ocpack generate-iso my-cluster     # 生成 ISO 文件
# 或
ocpack setup-pxe my-cluster        # 设置 PXE 启动环境

# 使用 ISO 启动虚拟机或通过 PXE 启动后，监控安装进度
ocpack mon my-cluster
```

## 配置文件示例

```toml
[cluster_info]
name = "my-cluster"
domain = "example.com"
openshift_version = "4.14.0"

[bastion]
ip = "192.168.1.10"
username = "root"
password = "password"

[registry]
ip = "192.168.1.11"
username = "root"
password = "password"

[[cluster.control_plane]]
name = "master-0"
ip = "192.168.1.21"
mac = "52:54:00:12:34:56"

[[cluster.worker]]
name = "worker-0"
ip = "192.168.1.31"
mac = "52:54:00:12:34:59"

[cluster.network]
cluster_network = "10.128.0.0/14"
service_network = "172.30.0.0/16"
machine_network = "192.168.1.0/24"
```

## 主要命令

| 命令 | 说明 |
|------|------|
| `new cluster <n>` | 创建新的集群项目 |
| `all <n> [--mode=iso\|pxe]` | **一键执行完整部署流程** |
| `download <n>` | 下载 OpenShift 安装工具 |
| `deploy-bastion <n>` | 部署 Bastion 节点 (DNS + HAProxy) |
| `deploy-registry <n>` | 部署 Registry 节点 |
| `save-image <n>` | 保存 OpenShift 镜像到本地 (使用外部 oc-mirror) |
| `save demo <n>` | **🆕 保存镜像 (使用内部 mirror 引擎)** |
| `load-image <n>` | 加载镜像到 Registry |
| `generate-iso <n>` | 生成安装 ISO 镜像 |
| `setup-pxe <n>` | 设置 PXE 启动环境 |
| `mon <n>` | **监控集群安装进度** |

## 镜像管理

### 保存镜像
```bash
# 传统方法 (使用外部 oc-mirror)
ocpack save-image my-cluster

# 新方法 (使用内部 mirror 引擎) - 演示版本
ocpack save demo my-cluster

# 包含 Operator 镜像
ocpack save-image my-cluster --include-operators
```

### 加载镜像
```bash
# 加载到 Registry
ocpack load-image my-cluster
```

## 前置条件

- **OpenShift 版本**: 4.14.0+ (支持 oc-mirror)
- **Pull Secret**: 从 [Red Hat Console](https://console.redhat.com/openshift/install/pull-secret) 获取
- **网络环境**: 确保 Bastion 和 Registry 节点可以通过 SSH 访问



## 部署架构

```
┌─────────────┐    ┌─────────────┐    ┌─────────────┐
│   ocpack    │───▶│   Bastion   │    │  Registry   │
│             │    │ DNS+HAProxy │    │   Quay      │
└─────────────┘    └─────────────┘    └─────────────┘
                          │
                          ▼
                   ┌─────────────┐
                   │ OpenShift   │
                   │   Cluster   │
                   └─────────────┘
```

## 开发

### 项目结构
```
ocpack/
├── cmd/ocpack/     # 命令行入口
├── pkg/
│   ├── config/     # 配置管理
│   ├── deploy/     # 部署功能 (嵌入式 Ansible)
│   ├── download/   # 工具下载
│   ├── saveimage/  # 镜像保存
│   ├── loadimage/  # 镜像加载
│   └── utils/      # 工具函数
└── README.md
```

### 构建
```bash
# 当前平台
make build

# 所有平台
make build-all

# 特定平台
make linux/amd64
make darwin/arm64
```

详细构建说明请参考 [BUILD.md](BUILD.md)

## 许可证

MIT License