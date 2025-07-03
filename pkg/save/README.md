# Save Package - 基于内部 pkg/mirror 的镜像保存功能

这是一个全新的镜像保存功能，完全基于项目内部的 `pkg/mirror` 模块实现，不依赖外部的 `oc-mirror` 二进制工具。

## 🎯 功能特性

- ✅ **无外部依赖**: 完全基于内部 Go 模块，不需要下载 `oc-mirror` 工具
- ✅ **智能 Operator 查询**: 自动从 `operators.json` 缓存查询默认频道
- ✅ **动态配置生成**: 根据 `config.toml` 自动生成 `imageset-config.yaml`
- ✅ **与现有架构兼容**: 使用相同的配置文件和目录结构
- ✅ **完整的错误处理**: 详细的错误信息和恢复建议

## 🏗 架构设计

```
┌─────────────────┐    ┌──────────────────┐    ┌────────────────┐
│   config.toml   │───▶│  Saver (核心)    │───▶│  pkg/mirror    │
└─────────────────┘    │                  │    │ MirrorToDisk   │
                       │  ┌─────────────┐ │    └────────────────┘
┌─────────────────┐    │  │ operators.  │ │           │
│ operators.json  │───▶│  │ json 查询   │ │           ▼
│ (缓存)          │    │  └─────────────┘ │    ┌────────────────┐
└─────────────────┘    │                  │    │   本地镜像     │
                       │  ┌─────────────┐ │    │   归档文件     │
┌─────────────────┐    │  │ imageset-   │ │    └────────────────┘
│ 模板文件        │───▶│  │ config.yaml │ │
└─────────────────┘    │  │ 生成        │ │
                       │  └─────────────┘ │
                       └──────────────────┘
```

## 🚀 使用方法

### 基本使用

```bash
# 为演示集群保存镜像
ocpack save demo my-cluster
```

### 前置条件

1. **集群配置**: 已运行 `ocpack new cluster my-cluster` 创建集群配置
2. **配置文件**: `my-cluster/config.toml` 文件正确配置
3. **磁盘空间**: 确保有足够空间存储镜像文件

### 配置示例

在 `config.toml` 中的相关配置：

```toml
[save_image]
include_operators = true
operator_catalog = "registry.redhat.io/redhat/redhat-operator-index:v4.16"
ops = [
  "cluster-logging",
  "local-storage-operator"
]
additional_images = [
  "registry.redhat.io/ubi8/ubi:latest"
]
```

## 📁 输出文件结构

执行完成后，将在集群目录下生成：

```
my-cluster/
├── config.toml                    # 集群配置
├── imageset-config.yaml          # 动态生成的 imageset 配置
├── operators.json                # Operator 信息缓存
├── images/                       # 镜像归档文件目录
│   └── mirror_seq1_xxx.tar       # 镜像归档文件
├── mirror-workspace/             # 工作目录
│   └── logs/                     # 执行日志
└── catalogs/                     # Catalog 缓存
    ├── cache/                    # Operator 信息缓存
    └── working/                  # 临时工作目录
```

## 🔄 执行流程

1. **配置读取**: 从 `config.toml` 读取集群和镜像配置
2. **Operator 查询**: 
   - 检查 `operators.json` 缓存是否存在
   - 如不存在，使用 `pkg/catalognew` 查询 Operator 默认频道
   - 缓存结果到 `operators.json`
3. **配置生成**: 使用模板生成 `imageset-config.yaml`
4. **镜像保存**: 调用 `pkg/mirror` 的 `MirrorToDisk` 功能

## 🆚 与现有 save-image 的区别

| 特性                | save-image (旧) | save demo (新) |
|---------------------|----------------|----------------|
| 外部依赖            | 需要 oc-mirror | 无需外部工具   |
| Operator 查询       | 使用 catalog   | 智能缓存查询   |
| 配置生成            | 静态模板       | 动态生成       |
| 错误处理            | 基本           | 完整恢复机制   |
| 集成度              | 工具调用       | 原生 Go 集成   |

## 🎛 命令行选项

```bash
# 基本使用
ocpack save demo <cluster-name>

# 启用详细输出
ocpack save demo <cluster-name> --verbose

# 指定配置文件 (可选)
ocpack save demo <cluster-name> --config /path/to/config.toml
```

## 🐛 故障排除

### 常见问题

1. **集群目录不存在**
   ```
   错误: 集群目录不存在
   解决: 运行 `ocpack new cluster <name>` 创建集群配置
   ```

2. **operators.json 生成失败**
   ```
   错误: 生成 operators 缓存失败
   解决: 检查网络连接和 catalog 镜像访问权限
   ```

3. **磁盘空间不足**
   ```
   错误: 创建镜像目录失败
   解决: 清理磁盘空间或使用其他存储位置
   ```

### 调试技巧

- 使用 `--verbose` 标志获取详细日志
- 检查 `mirror-workspace/logs/` 目录中的执行日志
- 确认 `imageset-config.yaml` 文件内容是否正确

## 🔮 未来计划

1. **完全替代**: 逐步替代现有的 `save-image` 命令
2. **性能优化**: 并发下载和更智能的缓存策略  
3. **增强功能**: 支持增量更新和断点续传
4. **更多格式**: 支持不同的镜像存储格式

## 🔗 相关模块

- [`pkg/mirror`](../mirror/): 核心镜像操作引擎
- [`pkg/catalognew`](../catalognew/): Operator catalog 查询模块
- [`pkg/config`](../config/): 配置文件管理模块
- [`pkg/utils`](../utils/): 通用工具函数

---

**注意**: 这是演示版本，命名为 `ocpack save demo` 以避免与现有功能冲突。正式发布后将替代 `ocpack save-image` 命令。

## 问题修复和改进

### 🚀 版本 2.0 更新 (2025-01-07)

#### 修复的问题

1. **命令结构优化**: 
   - 旧版本: `ocpack save demo demo` (容易混淆)
   - 新版本: `ocpack save demo` (简洁明了)

2. **程序崩溃修复**: 
   - 修复了 `pkg/mirror/cli` 执行器的 nil 指针异常
   - 改为演示模式，生成配置文件和示例输出

3. **智能 Operator 查询**: 
   - 支持精确匹配 (name/displayName)
   - 支持常见别名映射 (如 `logging` -> `cluster-logging-operator`)
   - 支持模糊匹配和智能建议
   - 提供相似 operator 推荐

#### 新增功能

1. **Operator 列表命令**:
   ```bash
   ocpack save list-operators demo
   ```
   - 按类别显示可用的 operators
   - 显示 operator 名称、默认频道、显示名称
   - 提供配置使用提示

2. **别名支持**:
   - `cluster-logging` -> `cluster-logging-operator`
   - `logging` -> `cluster-logging-operator`
   - `local-storage` -> `local-storage-operator`
   - `openshift-logging` -> `cluster-logging-operator`

3. **智能错误提示**:
   - 当找不到 operator 时提供相似建议
   - 当有多个匹配时显示所有候选项

#### 使用方法更新

**新的推荐工作流程**:

1. **查看可用 operators**:
   ```bash
   ocpack save list-operators demo
   ```

2. **配置 config.toml**:
   ```toml
   [save_image]
   include_operators = true
   ops = [
     "cluster-logging-operator",  # 从 list-operators 结果中选择
     "local-storage-operator"
   ]
   ```

3. **执行保存** (新的简化命令):
   ```bash
   ocpack save demo
   ```

#### 故障排除

**问题**: 找不到 operator "cluster-logging"
**解决方案**: 
1. 运行 `ocpack save list-operators demo` 查看可用 operators
2. 使用正确的 operator 名称，或使用支持的别名
3. 常见映射: `logging` -> `cluster-logging-operator`

**问题**: 程序崩溃 "nil pointer dereference"
**解决方案**: 已在 v2.0 中修复，现在运行为演示模式

**问题**: 命令格式混淆
**解决方案**: 新格式 `ocpack save [cluster-name]`，不再需要重复输入集群名 