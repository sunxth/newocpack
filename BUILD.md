# ocpack 构建说明

本文档说明如何使用 Makefile 构建 ocpack 项目。

## 前置要求

- Go 1.20 或更高版本
- Make 工具
- Git（用于版本信息）

## 快速开始

### 构建当前平台

```bash
make build
```

这将在 `build/` 目录下生成适用于当前平台的 `ocpack` 可执行文件。

### 构建所有支持的平台

```bash
make build-all
```

这将构建所有支持平台的可执行文件：
- Linux amd64/arm64
- macOS amd64/arm64 (Intel/Apple Silicon)
- Windows amd64/arm64

### 构建特定平台

```bash
# Linux amd64
make linux/amd64

# Linux arm64
make linux/arm64

# macOS amd64 (Intel)
make darwin/amd64

# macOS arm64 (Apple Silicon)
make darwin/arm64

# Windows amd64
make windows/amd64

# Windows arm64
make windows/arm64
```

## 可用的 Make 目标

### 构建相关

- `make build` - 构建当前平台的可执行文件
- `make build-all` - 构建所有支持平台的可执行文件
- `make clean` - 清理构建目录
- `make install` - 安装到本地 Go bin 目录

### 开发相关

- `make test` - 运行测试
- `make fmt` - 格式化代码
- `make vet` - 代码检查
- `make deps` - 下载和整理依赖
- `make dev` - 开发流程（deps + fmt + vet + test + build）

### 发布相关

- `make release` - 创建发布包（tar.gz 格式）
- `make info` - 显示构建信息
- `make help` - 显示帮助信息

### 环境检查

- `make check-env` - 检查 Go 环境

## 构建输出

构建的可执行文件将保存在 `build/` 目录下：

```
build/
├── ocpack                    # 当前平台
├── ocpack-linux-amd64       # Linux amd64
├── ocpack-linux-arm64       # Linux arm64
├── ocpack-darwin-amd64      # macOS amd64
├── ocpack-darwin-arm64      # macOS arm64
├── ocpack-windows-amd64.exe # Windows amd64
├── ocpack-windows-arm64.exe # Windows arm64
└── release/                 # 发布包目录
    ├── ocpack-linux-amd64.tar.gz
    ├── ocpack-linux-arm64.tar.gz
    ├── ocpack-darwin-amd64.tar.gz
    ├── ocpack-darwin-arm64.tar.gz
    ├── ocpack-windows-amd64.tar.gz
    └── ocpack-windows-arm64.tar.gz
```

## 版本信息

构建时会自动注入版本信息：

- **版本号**: 从 Git 标签获取，如果没有标签则使用提交哈希
- **提交哈希**: 当前 Git 提交的短哈希
- **构建时间**: 构建时的 UTC 时间

查看版本信息：

```bash
./build/ocpack version
# 或
./build/ocpack --version
```

## 示例

### 完整的开发流程

```bash
# 1. 下载依赖
make deps

# 2. 格式化代码
make fmt

# 3. 代码检查
make vet

# 4. 运行测试
make test

# 5. 构建
make build

# 6. 测试构建结果
./build/ocpack version
```

### 创建发布版本

```bash
# 清理并构建所有平台
make clean
make build-all

# 创建发布包
make release

# 查看发布包
ls -la build/release/
```

### 快速开发

```bash
# 一键执行开发流程
make dev
```

## 自定义构建

### 修改构建参数

可以通过修改 `Makefile` 中的变量来自定义构建：

```makefile
# 项目信息
PROJECT_NAME := ocpack
MAIN_PATH := cmd/ocpack/main.go
BUILD_DIR := build

# Go 构建参数
LDFLAGS := -ldflags "-X main.Version=$(VERSION) -X main.Commit=$(COMMIT) -X main.BuildTime=$(BUILD_TIME) -s -w"
```

### 添加新平台

在 `PLATFORMS` 变量中添加新的平台/架构组合：

```makefile
PLATFORMS := \
	linux/amd64 \
	linux/arm64 \
	darwin/amd64 \
	darwin/arm64 \
	windows/amd64 \
	windows/arm64 \
	freebsd/amd64    # 新增平台
```

## 故障排除

### 常见问题

1. **Make 命令不存在**
   ```bash
   # macOS
   xcode-select --install
   
   # Ubuntu/Debian
   sudo apt-get install build-essential
   
   # CentOS/RHEL
   sudo yum groupinstall "Development Tools"
   ```

2. **Go 版本过低**
   ```bash
   go version
   # 确保版本 >= 1.20
   ```

3. **Git 不存在**
   ```bash
   # 安装 Git 以获取版本信息
   git --version
   ```

4. **权限问题**
   ```bash
   # 确保有写入 build/ 目录的权限
   chmod 755 build/
   ```

### 调试构建

```bash
# 显示详细的构建信息
make info

# 检查 Go 环境
make check-env

# 手动构建（查看详细输出）
go build -v -o build/ocpack cmd/ocpack/main.go
```

## 持续集成

可以在 CI/CD 流水线中使用这些 Make 目标：

```yaml
# GitHub Actions 示例
- name: Build
  run: make build-all

- name: Test
  run: make test

- name: Create Release
  run: make release
``` 