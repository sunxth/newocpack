package catalog

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

// OperatorInfo 表示 Operator 信息
type OperatorInfo struct {
	Name           string `json:"name"`
	DisplayName    string `json:"displayName"`
	DefaultChannel string `json:"defaultChannel"`
}

// CatalogManager 管理 Operator 目录
type CatalogManager struct {
	CatalogImage    string
	CacheDir        string
	OCMirrorPath    string
	LockFile        string
	CacheFile       string
	TempFile        string
	DownloadTimeout time.Duration
}

// NewCatalogManager 创建新的目录管理器
func NewCatalogManager(catalogImage, cacheDir, ocMirrorPath string) *CatalogManager {
	// 基于目录镜像生成唯一的缓存文件名
	safeName := strings.ReplaceAll(strings.ReplaceAll(catalogImage, "/", "_"), ":", "_")

	return &CatalogManager{
		CatalogImage:    catalogImage,
		CacheDir:        cacheDir,
		OCMirrorPath:    ocMirrorPath,
		LockFile:        filepath.Join(cacheDir, fmt.Sprintf(".%s.lock", safeName)),
		CacheFile:       filepath.Join(cacheDir, fmt.Sprintf("%s.json", safeName)),
		TempFile:        filepath.Join(cacheDir, fmt.Sprintf("%s.tmp", safeName)),
		DownloadTimeout: 10 * time.Minute, // 10分钟超时
	}
}

// GetOperatorInfo 获取 Operator 信息，如果缓存不存在则下载
func (cm *CatalogManager) GetOperatorInfo(operatorName string) (*OperatorInfo, error) {
	// 确保缓存目录存在
	if err := os.MkdirAll(cm.CacheDir, 0755); err != nil {
		return nil, fmt.Errorf("创建缓存目录失败: %w", err)
	}

	// 获取文件锁
	lockFile, err := cm.acquireLock()
	if err != nil {
		return nil, fmt.Errorf("获取文件锁失败: %w", err)
	}
	defer cm.releaseLock(lockFile)

	// 检查缓存是否存在且有效
	if cm.isCacheValid() {
		fmt.Printf("ℹ️  使用缓存的 Operator 索引: %s\n", cm.CacheFile)
	} else {
		fmt.Printf("ℹ️  下载 Operator 索引: %s\n", cm.CatalogImage)
		if err := cm.downloadCatalog(); err != nil {
			return nil, fmt.Errorf("下载目录索引失败: %w", err)
		}
	}

	// 从缓存中读取 Operator 信息
	return cm.readOperatorFromCache(operatorName)
}

// GetAllOperators 获取所有 Operator 信息
func (cm *CatalogManager) GetAllOperators() ([]OperatorInfo, error) {
	// 确保缓存目录存在
	if err := os.MkdirAll(cm.CacheDir, 0755); err != nil {
		return nil, fmt.Errorf("创建缓存目录失败: %w", err)
	}

	// 获取文件锁
	lockFile, err := cm.acquireLock()
	if err != nil {
		return nil, fmt.Errorf("获取文件锁失败: %w", err)
	}
	defer cm.releaseLock(lockFile)

	// 检查缓存是否存在且有效
	if cm.isCacheValid() {
		fmt.Printf("ℹ️  使用缓存的 Operator 索引: %s\n", cm.CacheFile)
	} else {
		fmt.Printf("ℹ️  下载 Operator 索引: %s\n", cm.CatalogImage)
		if err := cm.downloadCatalog(); err != nil {
			return nil, fmt.Errorf("下载目录索引失败: %w", err)
		}
	}

	// 从缓存中读取所有 Operator 信息
	return cm.readAllOperatorsFromCache()
}

// acquireLock 获取文件锁
func (cm *CatalogManager) acquireLock() (*os.File, error) {
	lockFile, err := os.OpenFile(cm.LockFile, os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, fmt.Errorf("打开锁文件失败: %w", err)
	}

	// 尝试获取独占锁
	err = syscall.Flock(int(lockFile.Fd()), syscall.LOCK_EX)
	if err != nil {
		lockFile.Close()
		return nil, fmt.Errorf("获取文件锁失败: %w", err)
	}

	return lockFile, nil
}

// releaseLock 释放文件锁
func (cm *CatalogManager) releaseLock(lockFile *os.File) {
	if lockFile != nil {
		syscall.Flock(int(lockFile.Fd()), syscall.LOCK_UN)
		lockFile.Close()
		os.Remove(cm.LockFile)
	}
}

// isCacheValid 检查缓存是否有效（24小时内）
func (cm *CatalogManager) isCacheValid() bool {
	info, err := os.Stat(cm.CacheFile)
	if err != nil {
		return false
	}

	// 缓存有效期为24小时
	return time.Since(info.ModTime()) < 24*time.Hour
}

// downloadCatalog 下载目录索引
func (cm *CatalogManager) downloadCatalog() error {
	// 检查 oc-mirror 工具是否存在
	if _, err := os.Stat(cm.OCMirrorPath); os.IsNotExist(err) {
		return fmt.Errorf("oc-mirror 工具不存在: %s", cm.OCMirrorPath)
	}

	// 创建带超时的上下文
	ctx, cancel := context.WithTimeout(context.Background(), cm.DownloadTimeout)
	defer cancel()

	// 构建命令
	args := []string{
		"list",
		"operators",
		"--catalog",
		cm.CatalogImage,
	}

	fmt.Printf("ℹ️  执行命令: %s %s\n", cm.OCMirrorPath, strings.Join(args, " "))

	// 执行命令
	cmd := exec.CommandContext(ctx, cm.OCMirrorPath, args...)

	// 创建临时文件
	tempFile, err := os.Create(cm.TempFile)
	if err != nil {
		return fmt.Errorf("创建临时文件失败: %w", err)
	}
	defer os.Remove(cm.TempFile) // 确保清理临时文件

	// 分离输出和错误
	cmd.Stdout = tempFile
	var errorBuffer strings.Builder
	cmd.Stderr = &errorBuffer

	// 执行命令
	if err := cmd.Run(); err != nil {
		tempFile.Close()
		if ctx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("下载超时 (%v): %s", cm.DownloadTimeout, errorBuffer.String())
		}
		return fmt.Errorf("命令执行失败: %w\n错误输出: %s", err, errorBuffer.String())
	}

	tempFile.Close()

	// 解析并验证下载的数据
	operators, err := cm.parseCatalogOutput(cm.TempFile)
	if err != nil {
		return fmt.Errorf("解析目录数据失败: %w", err)
	}

	// 将解析后的数据写入缓存文件
	if err := cm.writeOperatorsToCache(operators); err != nil {
		return fmt.Errorf("写入缓存失败: %w", err)
	}

	fmt.Printf("✅ 成功下载并缓存了 %d 个 Operator 信息\n", len(operators))
	return nil
}

// parseCatalogOutput 解析 oc-mirror 输出
func (cm *CatalogManager) parseCatalogOutput(filePath string) ([]OperatorInfo, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("打开输出文件失败: %w", err)
	}
	defer file.Close()

	var operators []OperatorInfo
	scanner := bufio.NewScanner(file)

	// 跳过头部信息，找到表格开始
	headerFound := false
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.Contains(line, "NAME") && strings.Contains(line, "DISPLAY NAME") && strings.Contains(line, "DEFAULT CHANNEL") {
			headerFound = true
			break
		}
	}

	if !headerFound {
		return nil, fmt.Errorf("未找到有效的 Operator 列表")
	}

	// 解析 Operator 信息
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		// 解析表格行
		parts := strings.Fields(line)
		if len(parts) < 3 {
			continue
		}

		name := parts[0]
		defaultChannel := parts[len(parts)-1]

		// 显示名称是中间的部分，可能包含空格
		displayName := strings.Join(parts[1:len(parts)-1], " ")

		operators = append(operators, OperatorInfo{
			Name:           name,
			DisplayName:    displayName,
			DefaultChannel: defaultChannel,
		})
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("读取文件失败: %w", err)
	}

	if len(operators) == 0 {
		return nil, fmt.Errorf("未找到任何 Operator 信息")
	}

	return operators, nil
}

// writeOperatorsToCache 将 Operator 信息写入缓存
func (cm *CatalogManager) writeOperatorsToCache(operators []OperatorInfo) error {
	// 先写入临时文件
	tempCacheFile := cm.CacheFile + ".tmp"
	file, err := os.Create(tempCacheFile)
	if err != nil {
		return fmt.Errorf("创建临时缓存文件失败: %w", err)
	}

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")

	if err := encoder.Encode(operators); err != nil {
		file.Close()
		os.Remove(tempCacheFile)
		return fmt.Errorf("编码 JSON 失败: %w", err)
	}

	file.Close()

	// 原子性重命名操作
	if err := os.Rename(tempCacheFile, cm.CacheFile); err != nil {
		os.Remove(tempCacheFile)
		return fmt.Errorf("重命名缓存文件失败: %w", err)
	}

	return nil
}

// readOperatorFromCache 从缓存中读取指定 Operator 信息
func (cm *CatalogManager) readOperatorFromCache(operatorName string) (*OperatorInfo, error) {
	operators, err := cm.readAllOperatorsFromCache()
	if err != nil {
		return nil, err
	}

	for _, op := range operators {
		if op.Name == operatorName {
			return &op, nil
		}
	}

	return nil, fmt.Errorf("未找到 Operator: %s", operatorName)
}

// readAllOperatorsFromCache 从缓存中读取所有 Operator 信息
func (cm *CatalogManager) readAllOperatorsFromCache() ([]OperatorInfo, error) {
	file, err := os.Open(cm.CacheFile)
	if err != nil {
		return nil, fmt.Errorf("打开缓存文件失败: %w", err)
	}
	defer file.Close()

	var operators []OperatorInfo
	decoder := json.NewDecoder(file)

	if err := decoder.Decode(&operators); err != nil {
		return nil, fmt.Errorf("解码缓存文件失败: %w", err)
	}

	return operators, nil
}
