# 工具函数包 (Utils)

这个包包含了项目中常用的工具函数，避免在不同包中重复实现相同的功能。

## 文件结构

- `file.go`: 文件操作相关函数
- `network.go`: 网络相关函数
- `string.go`: 字符串处理函数
- `version.go`: 版本比较和解析函数
- `ssh.go`: SSH 连接和操作函数

## 主要功能

### 文件操作

- `CopyFile`: 复制文件
- `MoveFile`: 移动文件
- `CopyDir`: 复制目录
- `ExtractTarGz`: 从 tar.gz 文件中提取指定文件
- `MakeExecutable`: 设置文件可执行权限

### 网络操作

- `ExtractNetworkBase`: 从 CIDR 中提取网络基地址
- `ExtractPrefixLength`: 从 CIDR 中提取前缀长度
- `ExtractGateway`: 从 CIDR 中提取网关地址

### 版本处理

- `CompareVersion`: 比较两个版本号
- `ParseVersion`: 解析版本号为整数数组
- `ExtractVersionFromOutput`: 从命令输出中提取版本号
- `ExtractSHAFromOutput`: 从命令输出中提取 SHA 值
- `SupportsOcMirror`: 检查版本是否支持 oc-mirror 工具

### 字符串处理

- `JoinStringSlice`: 连接字符串切片
- `SplitString`: 分割字符串
- `ContainsString`: 检查字符串是否包含子串
- `TrimString`: 去除字符串两端的空白字符
- `ReplaceString`: 替换字符串中的子串

## 使用示例

```go
import "ocpack/pkg/utils"

// 文件操作
utils.CopyFile("source.txt", "dest.txt")

// 版本比较
if utils.CompareVersion("4.14.0", "4.13.0") > 0 {
    // 版本 4.14.0 大于 4.13.0
}

// 网络操作
baseIP := utils.ExtractNetworkBase("192.168.1.0/24") // 返回 "192.168.1.0"
gateway := utils.ExtractGateway("192.168.1.0/24")    // 返回 "192.168.1.1"
```

## 测试

包含完整的单元测试，可以通过以下命令运行：

```bash
cd pkg/utils
go test -v
``` 