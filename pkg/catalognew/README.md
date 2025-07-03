# CatalogNew - 基于 pkg/mirror 的 Catalog 管理功能

这是一个新的 catalog 管理功能，基于 `pkg/mirror` 实现，不依赖外部 `oc-mirror` 工具。

## 功能特性

- ✅ **直接镜像操作**: 无需依赖外部 `oc-mirror` 工具
- ✅ **自动缓存**: 支持本地缓存，避免重复下载
- ✅ **多种格式支持**: 支持 `docker://` 和 `oci://` 协议
- ✅ **完整集成**: 与现有 `pkg/mirror` 功能完美集成
- ✅ **命令行接口**: 提供友好的 CLI 接口

## 使用方法

### 1. 列出 Catalog 中的所有 Operators

```bash
ocpack catalognew list --catalog registry.redhat.io/redhat/redhat-operator-index:v4.14
```

### 2. 获取指定 Operator 的详细信息

```bash
ocpack catalognew info my-operator --catalog registry.redhat.io/redhat/redhat-operator-index:v4.14
```

### 3. 自定义缓存和工作目录

```bash
ocpack catalognew list \
  --catalog registry.redhat.io/redhat/redhat-operator-index:v4.14 \
  --cache-dir /custom/cache \
  --working-dir /custom/working
```

## 参数说明

| 参数 | 描述 | 默认值 | 必需 |
|------|------|--------|------|
| `--catalog` | Catalog 镜像地址 | - | ✅ |
| `--cache-dir` | 缓存目录 | `/tmp/ocpack-catalog-cache` | ❌ |
| `--working-dir` | 工作目录 | `/tmp/ocpack-working` | ❌ |

## 架构设计

```
┌─────────────────┐    ┌──────────────────┐    ┌────────────────┐
│   CatalogNew    │────▶│   pkg/mirror     │────▶│  Container     │
│   Manager       │    │   Operator       │    │  Registry      │
└─────────────────┘    └──────────────────┘    └────────────────┘
         │                       │
         ▼                       ▼
┌─────────────────┐    ┌──────────────────┐
│  Local Cache    │    │  Working Dir     │
│  (JSON)         │    │  (OCI Format)    │
└─────────────────┘    └──────────────────┘
```

## 实现原理

1. **下载阶段**: 使用 `pkg/mirror` 的 `OperatorImageCollector` 下载 catalog 镜像
2. **解析阶段**: 将 OCI 格式的 catalog 解析为结构化数据
3. **缓存阶段**: 将解析结果以 JSON 格式缓存到本地
4. **查询阶段**: 从缓存中快速检索 operator 信息

## 缓存机制

- **缓存时效**: 24小时
- **缓存位置**: 默认 `/tmp/ocpack-catalog-cache`
- **缓存格式**: JSON
- **文件命名**: 基于 catalog 镜像地址生成唯一名称

## 错误处理

- 网络连接失败时会显示详细错误信息
- 镜像不存在时会提示相关错误
- 权限不足时会给出明确的解决建议

## 与现有功能的关系

这个新功能与现有的 `pkg/catalog` 功能是互补的：

- **现有功能**: 依赖外部 `oc-mirror` 工具，适用于已有环境
- **新功能**: 纯 Go 实现，更好的集成性，为将来的 `save`/`load` 功能打基础

## 后续计划

1. **完善 FBC 解析**: 目前只是基础实现，需要完整解析 File-Based Catalog 格式
2. **集成到 save/load**: 将此功能集成到计划中的 `ocpack save` 和 `ocpack load` 命令
3. **性能优化**: 并发下载和解析优化
4. **更多元数据**: 支持更详细的 operator 信息（版本、依赖等）

## 注意事项

⚠️ **当前限制**: 
- FBC 解析逻辑还在完善中，目前返回的是示例数据
- 需要网络连接来下载 catalog 镜像
- 首次下载可能需要较长时间

💡 **建议**:
- 在稳定的网络环境中使用
- 定期清理缓存目录以释放空间
- 可以配合现有的 catalog 功能进行对比测试 