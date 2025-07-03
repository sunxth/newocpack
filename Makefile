# ocpack Makefile
# 用于构建不同平台的可执行文件

# 项目信息
PROJECT_NAME := ocpack
MAIN_PATH := cmd/ocpack/main.go
BUILD_DIR := build
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_TIME := $(shell date -u '+%Y-%m-%d_%H:%M:%S')

# Go 构建参数
LDFLAGS := -ldflags "-X main.Version=$(VERSION) -X main.Commit=$(COMMIT) -X main.BuildTime=$(BUILD_TIME) -s -w"
GO_BUILD := go build $(LDFLAGS)

# 支持的平台和架构
PLATFORMS := \
	linux/amd64 \
	linux/arm64 \
	darwin/amd64 \
	darwin/arm64 \
	windows/amd64 \
	windows/arm64

# 默认目标
.PHONY: all
all: clean build-all

# 清理构建目录
.PHONY: clean
clean:
	@echo "🧹 清理构建目录..."
	@rm -rf $(BUILD_DIR)
	@mkdir -p $(BUILD_DIR)

# 构建当前平台
.PHONY: build
build:
	@echo "🔨 构建当前平台..."
	@$(GO_BUILD) -o $(BUILD_DIR)/$(PROJECT_NAME) $(MAIN_PATH)
	@echo "✅ 构建完成: $(BUILD_DIR)/$(PROJECT_NAME)"

# 构建所有平台
.PHONY: build-all
build-all: $(PLATFORMS)

# 构建 Linux amd64
.PHONY: linux/amd64
linux/amd64:
	@echo "🔨 构建 Linux amd64..."
	@CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -tags "containers_image_openpgp" -o $(BUILD_DIR)/$(PROJECT_NAME)-linux-amd64 $(MAIN_PATH)
	@echo "✅ 构建完成: $(BUILD_DIR)/$(PROJECT_NAME)-linux-amd64"

# 构建 Linux arm64
.PHONY: linux/arm64
linux/arm64:
	@echo "🔨 构建 Linux arm64..."
	@CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build $(LDFLAGS) -tags "containers_image_openpgp" -o $(BUILD_DIR)/$(PROJECT_NAME)-linux-arm64 $(MAIN_PATH)
	@echo "✅ 构建完成: $(BUILD_DIR)/$(PROJECT_NAME)-linux-arm64"

# 构建 macOS amd64
.PHONY: darwin/amd64
darwin/amd64:
	@echo "🔨 构建 macOS amd64..."
	@GOOS=darwin GOARCH=amd64 $(GO_BUILD) -o $(BUILD_DIR)/$(PROJECT_NAME)-darwin-amd64 $(MAIN_PATH)
	@echo "✅ 构建完成: $(BUILD_DIR)/$(PROJECT_NAME)-darwin-amd64"

# 构建 macOS arm64 (Apple Silicon)
.PHONY: darwin/arm64
darwin/arm64:
	@echo "🔨 构建 macOS arm64..."
	@GOOS=darwin GOARCH=arm64 $(GO_BUILD) -o $(BUILD_DIR)/$(PROJECT_NAME)-darwin-arm64 $(MAIN_PATH)
	@echo "✅ 构建完成: $(BUILD_DIR)/$(PROJECT_NAME)-darwin-arm64"

# 构建 Windows amd64
.PHONY: windows/amd64
windows/amd64:
	@echo "🔨 构建 Windows amd64..."
	@GOOS=windows GOARCH=amd64 $(GO_BUILD) -o $(BUILD_DIR)/$(PROJECT_NAME)-windows-amd64.exe $(MAIN_PATH)
	@echo "✅ 构建完成: $(BUILD_DIR)/$(PROJECT_NAME)-windows-amd64.exe"

# 构建 Windows arm64
.PHONY: windows/arm64
windows/arm64:
	@echo "🔨 构建 Windows arm64..."
	@GOOS=windows GOARCH=arm64 $(GO_BUILD) -o $(BUILD_DIR)/$(PROJECT_NAME)-windows-arm64.exe $(MAIN_PATH)
	@echo "✅ 构建完成: $(BUILD_DIR)/$(PROJECT_NAME)-windows-arm64.exe"

# 运行测试
.PHONY: test
test:
	@echo "🧪 运行测试..."
	@go test -v ./...

# 代码格式化
.PHONY: fmt
fmt:
	@echo "🎨 格式化代码..."
	@go fmt ./...

# 代码检查
.PHONY: vet
vet:
	@echo "🔍 代码检查..."
	@go vet ./...

# 下载依赖
.PHONY: deps
deps:
	@echo "📦 下载依赖..."
	@go mod download
	@go mod tidy

# 安装到本地
.PHONY: install
install:
	@echo "📦 安装到本地..."
	@go install $(LDFLAGS) $(MAIN_PATH)
	@echo "✅ 安装完成"

# 创建发布包
.PHONY: release
release: clean build-all
	@echo "📦 创建发布包..."
	@mkdir -p $(BUILD_DIR)/release
	@for platform in $(PLATFORMS); do \
		os=$$(echo $$platform | cut -d'/' -f1); \
		arch=$$(echo $$platform | cut -d'/' -f2); \
		if [ "$$os" = "windows" ]; then \
			binary="$(PROJECT_NAME)-$$os-$$arch.exe"; \
		else \
			binary="$(PROJECT_NAME)-$$os-$$arch"; \
		fi; \
		if [ -f "$(BUILD_DIR)/$$binary" ]; then \
			echo "📦 打包 $$binary..."; \
			tar -czf "$(BUILD_DIR)/release/$(PROJECT_NAME)-$$os-$$arch.tar.gz" -C $(BUILD_DIR) $$binary README.md; \
		fi; \
	done
	@echo "✅ 发布包创建完成: $(BUILD_DIR)/release/"

# 显示构建信息
.PHONY: info
info:
	@echo "📋 构建信息:"
	@echo "  项目名称: $(PROJECT_NAME)"
	@echo "  版本: $(VERSION)"
	@echo "  提交: $(COMMIT)"
	@echo "  构建时间: $(BUILD_TIME)"
	@echo "  支持平台: $(PLATFORMS)"

# 显示帮助信息
.PHONY: help
help:
	@echo "🚀 ocpack Makefile 使用说明"
	@echo ""
	@echo "可用目标:"
	@echo "  build        - 构建当前平台的可执行文件"
	@echo "  build-all    - 构建所有支持平台的可执行文件"
	@echo "  linux/amd64  - 构建 Linux amd64 版本"
	@echo "  linux/arm64  - 构建 Linux arm64 版本"
	@echo "  darwin/amd64 - 构建 macOS amd64 版本"
	@echo "  darwin/arm64 - 构建 macOS arm64 版本 (Apple Silicon)"
	@echo "  windows/amd64- 构建 Windows amd64 版本"
	@echo "  windows/arm64- 构建 Windows arm64 版本"
	@echo "  test         - 运行测试"
	@echo "  fmt          - 格式化代码"
	@echo "  vet          - 代码检查"
	@echo "  deps         - 下载和整理依赖"
	@echo "  install      - 安装到本地"
	@echo "  release      - 创建发布包"
	@echo "  clean        - 清理构建目录"
	@echo "  info         - 显示构建信息"
	@echo "  help         - 显示此帮助信息"
	@echo ""
	@echo "示例:"
	@echo "  make build           # 构建当前平台"
	@echo "  make build-all       # 构建所有平台"
	@echo "  make linux/amd64     # 只构建 Linux amd64"
	@echo "  make release         # 创建发布包"
	@echo ""
	@echo "ocpack 使用示例:"
	@echo "  ./build/ocpack new cluster demo     # 创建集群项目"
	@echo "  ./build/ocpack all demo             # 一键部署完整流程 (默认 ISO 模式)"
	@echo "  ./build/ocpack all demo --mode=pxe  # 一键部署完整流程 (PXE 模式)"

# 开发相关目标
.PHONY: dev
dev: deps fmt vet test build

# 检查 Go 环境
.PHONY: check-env
check-env:
	@echo "🔍 检查 Go 环境..."
	@go version
	@echo "GOPATH: $(GOPATH)"
	@echo "GOROOT: $(GOROOT)"
	@echo "GO111MODULE: $(GO111MODULE)" 