# 变更日志

## [未发布] - 2025-01-06

### 新增功能
- 🚀 **一键部署全流程命令** (`ocpack all <cluster-name>`)
  - 自动执行完整的 OpenShift 集群部署流程
  - 包括：下载、部署基础设施、镜像管理、生成 ISO
  - 提供详细的进度显示和错误处理
  - 支持前置检查和配置验证

- 📊 **集群安装监控功能** (`ocpack mon <cluster-name>`)
  - 实时监控 Agent-based 安装的 OpenShift 集群进度
  - 支持持续监控模式 (`--follow`)
  - 可配置检查间隔和超时时间
  - 显示详细安装日志 (`--logs`)
  - 自动获取集群凭据信息 (`--credentials`)
  - 友好的进度显示和状态更新
  - 简洁的命令名称 `mon`，更易于使用

- 🎛️  **Ansible 输出优化**
  - 自动配置 Ansible 环境变量以获得清洁的输出
  - 禁用主机密钥检查，提高自动化部署体验
  - 隐藏跳过的任务，减少输出噪音
  - 设置最小详细程度，专注于重要信息
  - 无需用户手动配置，开箱即用

### 修复问题
- 🐛 **修复 SHA 提取失败问题**
  - 修复了 `ExtractSHAFromOutput` 函数无法从 `openshift-install version` 输出中提取 release SHA 的问题
  - 支持不区分大小写的 "release image" 匹配
  - 添加了完整的单元测试覆盖

### 改进
- 📚 **文档更新**
  - 更新 README.md 添加一键部署和监控功能说明
  - 新增 USAGE.md 详细使用指南，包含监控功能
  - 更新 Makefile 帮助信息
  - 改进命令行帮助文档

- 🔧 **ISO 生成改进**
  - 改进 ignition 文件复制过程
  - 增加详细的文件复制状态显示
  - 确保监控所需的文件正确生成

- 📊 **监控功能增强**
  - 默认显示重要的安装日志（INFO、WARNING、ERROR）
  - 增加 "验证中" 状态，更好地反映安装前验证阶段
  - 改进日志格式化，使用图标区分不同级别的信息
  - 优化输出布局，提高可读性
  - 简化监控命令，去掉 `--follow`、`--logs`、`--interval`、`--timeout` 参数，默认持续监控
  - 改进监控功能，能够读取安装日志文件显示历史信息，解决集群后期阶段无新日志输出的问题
  - 支持 debug 级别日志的解析，能够显示集群初始化的详细进度信息
  - **简化监控功能**：去掉复杂的状态分析和格式化，直接透传 `openshift-install agent wait-for install-complete` 命令的原始输出
  - **修复 openshift-install 工具查找**：支持查找带版本号和registry信息的 openshift-install 文件（如 `openshift-install-4.17.0-registry.dr.example.com`）
  - **改进目录切换逻辑**：支持在集群目录内直接执行监控命令，无需切换目录
  - **去掉 --credentials 参数**：监控命令现在只有一个功能，直接显示安装进度和完成后的访问信息

### 技术细节
- 🔧 **代码质量提升**
  - 增强了版本提取函数的健壮性
  - 添加了更多的错误处理和用户友好的提示
  - 完善了单元测试覆盖率

## 使用说明

### 一键部署
```bash
# 创建集群项目
ocpack new cluster my-cluster

# 编辑配置文件
vim my-cluster/config.toml

# 准备 pull-secret.txt
cp pull-secret.txt my-cluster/

# 一键部署 (默认 ISO 模式)
ocpack all my-cluster

# 指定部署模式
ocpack all my-cluster --mode=iso    # ISO 模式
ocpack all my-cluster --mode=pxe    # PXE 模式

# 使用 ISO 启动虚拟机后，监控安装进度
ocpack mon my-cluster

# 获取集群凭据
ocpack mon my-cluster --credentials
```

### 分步部署（如需要）
```bash
ocpack download my-cluster
ocpack deploy-bastion my-cluster
ocpack deploy-registry my-cluster
ocpack save-image my-cluster
ocpack load-image my-cluster
ocpack generate-iso my-cluster     # 生成 ISO 文件
# 或
ocpack setup-pxe my-cluster        # 设置 PXE 启动环境
```

## 故障排除

如果遇到 "无法从 openshift-install 输出中提取 release SHA" 错误：
1. 确保使用最新版本的 ocpack
2. 检查 openshift-install 工具是否正常工作
3. 使用 `--skip-verify` 标志跳过验证（如果需要）

## 兼容性

- 支持 OpenShift 4.14.0+
- 支持 RHEL 8/9, CentOS 8/9
- 需要 Go 1.19+ 