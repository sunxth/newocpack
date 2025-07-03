package utils

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// CopyFile 复制文件
func CopyFile(src, dst string) error {
	if _, err := os.Stat(src); os.IsNotExist(err) {
		return fmt.Errorf("源文件不存在: %s", src)
	}

	srcFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("打开源文件失败: %w", err)
	}
	defer srcFile.Close()

	dstFile, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("创建目标文件失败: %w", err)
	}
	defer dstFile.Close()

	if _, err = io.Copy(dstFile, srcFile); err != nil {
		return fmt.Errorf("复制文件失败: %w", err)
	}

	return nil
}

// MoveFile 移动文件
func MoveFile(src, dst string) error {
	return os.Rename(src, dst)
}

// CopyDir 复制目录
func CopyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}

		dstPath := filepath.Join(dst, relPath)

		if info.IsDir() {
			return os.MkdirAll(dstPath, info.Mode())
		}

		return CopyFile(path, dstPath)
	})
}

// CopyFileOrDir 复制文件或目录
func CopyFileOrDir(src, dst string) error {
	srcInfo, err := os.Stat(src)
	if err != nil {
		return err
	}

	if srcInfo.IsDir() {
		return CopyDir(src, dst)
	}
	return CopyFile(src, dst)
}

// ExtractFile 从tar.Reader提取单个文件
func ExtractFile(tr *tar.Reader, destPath string) error {
	destFile, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("创建目标文件失败: %w", err)
	}
	defer destFile.Close()

	_, err = io.Copy(destFile, tr)
	return err
}

// ExtractTarGz 从tar.gz文件中提取指定的文件
func ExtractTarGz(tarPath, destDir string, targetFiles []string) error {
	if _, err := os.Stat(tarPath); os.IsNotExist(err) {
		return fmt.Errorf("tar文件不存在: %s", tarPath)
	}

	file, err := os.Open(tarPath)
	if err != nil {
		return fmt.Errorf("打开文件失败: %w", err)
	}
	defer file.Close()

	gzr, err := gzip.NewReader(file)
	if err != nil {
		return fmt.Errorf("创建gzip读取器失败: %w", err)
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)
	hardLinks := make(map[string]string)
	targetSet := make(map[string]bool)
	
	// 构建目标文件集合
	for _, file := range targetFiles {
		targetSet[file] = true
	}

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("读取tar文件失败: %w", err)
		}

		fileName := filepath.Base(header.Name)
		if !targetSet[fileName] {
			continue
		}

		destPath := filepath.Join(destDir, fileName)

		if header.Typeflag == tar.TypeLink {
			// 处理硬链接
			linkTarget := filepath.Base(header.Linkname)
			if targetPath, exists := hardLinks[linkTarget]; exists {
				// 检查目标文件是否已存在，如果存在则删除
				if _, err := os.Stat(destPath); err == nil {
					if err := os.Remove(destPath); err != nil {
						return fmt.Errorf("删除已存在的文件失败: %w", err)
					}
				}
				
				if err := os.Link(targetPath, destPath); err != nil {
					return fmt.Errorf("创建硬链接失败: %w", err)
				}
			} else {
				return fmt.Errorf("硬链接目标%s不存在", linkTarget)
			}
		} else {
			// 处理普通文件
			if err := ExtractFile(tr, destPath); err != nil {
				return fmt.Errorf("提取文件%s失败: %w", fileName, err)
			}
			hardLinks[fileName] = destPath
		}
	}

	return nil
}

// MakeExecutable 为目录中的所有文件设置可执行权限
func MakeExecutable(dir string) error {
	return filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			if err := os.Chmod(path, 0755); err != nil {
				return fmt.Errorf("设置文件权限失败 %s: %w", path, err)
			}
		}
		return nil
	})
}

// EnsureDirExists 确保目录存在，如果不存在则创建
func EnsureDirExists(dir string) error {
	return os.MkdirAll(dir, 0755)
}

// FileExists 检查文件是否存在
func FileExists(path string) bool {
	_, err := os.Stat(path)
	return !os.IsNotExist(err)
} 