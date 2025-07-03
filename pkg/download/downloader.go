package download

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"ocpack/pkg/config"
	"ocpack/pkg/utils"
)

// --- Constants ---
const (
	baseURL              = "https://mirror.openshift.com/pub/openshift-v4"
	ocpClientsURLPattern = baseURL + "/clients/ocp/%s/%s"
	ocMirrorURLPattern   = baseURL + "/%s/clients/ocp/%s/oc-mirror.tar.gz"
	butaneURLPattern     = baseURL + "/clients/butane/latest/butane-%s"
	quayReleaseURL       = "https://mirror.openshift.com/pub/cgw/mirror-registry/latest/mirror-registry-amd64.tar.gz"
	progressBarWidth     = 30
	progressUpdateFreq   = 100 * time.Millisecond
)

// --- Struct Definitions ---

// Downloader is responsible for downloading required files.
type Downloader struct {
	config      *config.ClusterConfig
	downloadDir string
}

// ProgressReader is an io.Reader that displays download progress.
type ProgressReader struct {
	io.Reader
	total      int64
	downloaded int64
	fileName   string
	startTime  time.Time
	lastUpdate time.Time
}

// DownloadTask defines a file to be downloaded.
type DownloadTask struct {
	Name       string
	URL        string
	FileName   string
	Required   bool
	VersionDep bool // Does this depend on a specific OCP version?
}

// --- Main Logic ---

// NewDownloader creates a new downloader instance.
func NewDownloader(cfg *config.ClusterConfig, downloadDir string) *Downloader {
	return &Downloader{
		config:      cfg,
		downloadDir: downloadDir,
	}
}

// DownloadAll orchestrates the download of all necessary files.
func (d *Downloader) DownloadAll() error {
	fmt.Println("▶️  开始下载所需工具和文件...")

	if err := os.MkdirAll(d.downloadDir, 0755); err != nil {
		return fmt.Errorf("创建下载目录失败: %w", err)
	}

	version := d.config.ClusterInfo.OpenShiftVersion
	tasks := d.buildDownloadTasks(version)

	for i, task := range tasks {
		fmt.Printf("\n➡️  任务 %d/%d: %s\n", i+1, len(tasks), task.Name)

		if task.VersionDep && !utils.SupportsOcMirror(version) {
			fmt.Printf("⚠️  跳过 %s: OpenShift 版本 %s 不支持 (需要 4.14.0 及以上版本)\n", task.Name, version)
		} else {
			filePath := filepath.Join(d.downloadDir, task.FileName)
			if err := d.downloadFile(task.URL, filePath); err != nil {
				if task.Required {
					return fmt.Errorf("下载必需文件 '%s' 失败: %w", task.Name, err)
				}
				fmt.Printf("⚠️  下载可选文件 '%s' 失败，已跳过: %v\n", task.Name, err)
			}
		}
	}

	fmt.Println("\n➡️  正在提取工具...")
	if err := d.extractTools(version); err != nil {
		return fmt.Errorf("提取工具失败: %w", err)
	}

	fmt.Println("\n🎉 所有下载和提取操作完成！")
	return nil
}

// --- Task Building ---

// buildDownloadTasks constructs the list of files to download.
func (d *Downloader) buildDownloadTasks(version string) []DownloadTask {
	arch := getSystemArch()
	butaneArch := getSystemArchForButane()

	return []DownloadTask{
		{
			Name:     "OpenShift 客户端 (oc, kubectl)",
			URL:      fmt.Sprintf(ocpClientsURLPattern, version, fmt.Sprintf("openshift-client-linux-%s.tar.gz", version)),
			FileName: fmt.Sprintf("openshift-client-linux-%s.tar.gz", version),
			Required: true,
		},
		{
			Name:     "OpenShift 安装程序 (openshift-install)",
			URL:      fmt.Sprintf(ocpClientsURLPattern, version, fmt.Sprintf("openshift-install-linux-%s.tar.gz", version)),
			FileName: fmt.Sprintf("openshift-install-linux-%s.tar.gz", version),
			Required: true,
		},
		{
			Name:       "oc-mirror 工具",
			URL:        fmt.Sprintf(ocMirrorURLPattern, arch, version),
			FileName:   fmt.Sprintf("oc-mirror-%s.tar.gz", version),
			Required:   false, // Not required if version is too old
			VersionDep: true,
		},
		{
			Name:     "Butane 工具",
			URL:      fmt.Sprintf(butaneURLPattern, butaneArch),
			FileName: fmt.Sprintf("butane-%s", butaneArch),
			Required: true,
		},
		{
			Name:     "Quay 镜像仓库安装包",
			URL:      quayReleaseURL,
			FileName: "mirror-registry-amd64.tar.gz",
			Required: true,
		},
	}
}

// --- Download and Extraction ---

// downloadFile downloads a single file to a destination path with progress.
func (d *Downloader) downloadFile(url, destPath string) error {
	if _, err := os.Stat(destPath); err == nil {
		fmt.Printf("✅ 文件已存在，跳过下载: %s\n", filepath.Base(destPath))
		return nil
	}

	fileName := filepath.Base(destPath)
	tmpPath := destPath + ".tmp"
	defer os.Remove(tmpPath)

	headResp, err := http.Head(url)
	var contentLength int64
	if err == nil {
		defer headResp.Body.Close()
		contentLength = headResp.ContentLength
	}

	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("HTTP GET 请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("下载失败，HTTP 状态码: %d", resp.StatusCode)
	}
	if contentLength <= 0 {
		contentLength = resp.ContentLength
	}

	out, err := os.Create(tmpPath)
	if err != nil {
		return fmt.Errorf("创建临时文件失败: %w", err)
	}
	defer out.Close()

	progressReader := &ProgressReader{
		Reader:    resp.Body,
		total:     contentLength,
		fileName:  fileName,
		startTime: time.Now(),
	}

	if _, err = io.Copy(out, progressReader); err != nil {
		fmt.Println()
		return fmt.Errorf("保存文件时出错: %w", err)
	}
	fmt.Println()

	if err := os.Rename(tmpPath, destPath); err != nil {
		return fmt.Errorf("重命名临时文件失败: %w", err)
	}

	return nil
}

// extractTools extracts binaries from downloaded tarballs.
func (d *Downloader) extractTools(version string) error {
	binDir := filepath.Join(d.downloadDir, "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		return fmt.Errorf("创建 bin 目录失败: %w", err)
	}

	if err := d.cleanupBinDir(binDir); err != nil {
		fmt.Printf("⚠️  清理 bin 目录时发出警告: %v\n", err)
	}

	extractTasks := []struct {
		tarName   string
		tarPath   string
		files     []string
		condition bool
	}{
		{"OpenShift 客户端", fmt.Sprintf("openshift-client-linux-%s.tar.gz", version), []string{"oc", "kubectl"}, true},
		{"OpenShift 安装程序", fmt.Sprintf("openshift-install-linux-%s.tar.gz", version), []string{"openshift-install"}, true},
		{"oc-mirror 工具", fmt.Sprintf("oc-mirror-%s.tar.gz", version), []string{"oc-mirror"}, utils.SupportsOcMirror(version)},
	}

	for _, task := range extractTasks {
		if !task.condition {
			continue
		}
		fullPath := filepath.Join(d.downloadDir, task.tarPath)
		if err := utils.ExtractTarGz(fullPath, binDir, task.files); err != nil {
			if os.IsNotExist(errors.Unwrap(err)) {
				fmt.Printf("ℹ️  归档文件 %s 不存在，跳过提取。\n", task.tarPath)
				continue
			}
			return fmt.Errorf("提取 '%s' 失败: %w", task.tarName, err)
		}
	}

	if err := d.copyButaneTool(binDir); err != nil {
		return fmt.Errorf("复制 butane 工具失败: %w", err)
	}
	if err := utils.MakeExecutable(binDir); err != nil {
		return fmt.Errorf("设置可执行权限失败: %w", err)
	}

	fmt.Println("✅ 工具提取完成。")
	return nil
}

// --- File System Helpers ---

func (d *Downloader) cleanupBinDir(binDir string) error {
	filesToClean := []string{"oc", "kubectl", "openshift-install", "oc-mirror", "butane"}
	for _, fileName := range filesToClean {
		filePath := filepath.Join(binDir, fileName)
		if _, err := os.Stat(filePath); err == nil {
			if err := os.Remove(filePath); err != nil {
				return fmt.Errorf("删除文件 %s 失败: %w", fileName, err)
			}
		}
	}
	return nil
}

func (d *Downloader) copyButaneTool(binDir string) error {
	arch := getSystemArchForButane()
	srcPath := filepath.Join(d.downloadDir, fmt.Sprintf("butane-%s", arch))
	dstPath := filepath.Join(binDir, "butane")

	if _, err := os.Stat(srcPath); os.IsNotExist(err) {
		return fmt.Errorf("butane 源文件不存在: %s", srcPath)
	}
	return utils.CopyFile(srcPath, dstPath)
}

// --- Progress Reader Methods and Helpers ---

func (pr *ProgressReader) Read(p []byte) (int, error) {
	n, err := pr.Reader.Read(p)
	pr.downloaded += int64(n)
	now := time.Now()
	if now.Sub(pr.lastUpdate) >= progressUpdateFreq || err == io.EOF {
		pr.lastUpdate = now
		pr.printProgress()
	}
	return n, err
}

func (pr *ProgressReader) printProgress() {
	if pr.total <= 0 {
		fmt.Printf("\rDownloading %s: %s", pr.fileName, formatBytes(pr.downloaded))
		return
	}
	percent := float64(pr.downloaded) * 100 / float64(pr.total)
	filled := int(percent * float64(progressBarWidth) / 100)
	bar := strings.Repeat("█", filled) + strings.Repeat("░", progressBarWidth-filled)
	elapsed := time.Since(pr.startTime)
	speed := float64(pr.downloaded) / elapsed.Seconds()
	var eta time.Duration
	if speed > 0 && pr.downloaded < pr.total {
		eta = time.Duration(float64(pr.total-pr.downloaded)/speed) * time.Second
	}
	fmt.Printf("\r⬇️  %s [%s] %.1f%% (%s/%s) %s/s ETA: %s ",
		pr.fileName, bar, percent,
		formatBytes(pr.downloaded), formatBytes(pr.total),
		formatBytes(int64(speed)), formatDuration(eta))
}

func formatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %s", float64(b)/float64(div), []string{"KB", "MB", "GB", "TB"}[exp])
}

func formatDuration(d time.Duration) string {
	d = d.Round(time.Second)
	h := d / time.Hour
	d -= h * time.Hour
	m := d / time.Minute
	d -= m * time.Minute
	s := d / time.Second
	if h > 0 {
		return fmt.Sprintf("%02d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%02d:%02d", m, s)
}

// --- System Architecture Helpers ---

func getSystemArch() string {
	archMap := map[string]string{
		"amd64": "x86_64",
		"arm64": "aarch64",
	}
	if mapped, exists := archMap[runtime.GOARCH]; exists {
		return mapped
	}
	return "x86_64"
}

// **FIX:** Reverted this function to the original logic to match the mirror's specific naming scheme.
func getSystemArchForButane() string {
	osArchMap := map[string]map[string]string{
		"linux": {
			"amd64":   "amd64",
			"arm64":   "aarch64",
			"ppc64le": "ppc64le",
			"s390x":   "s390x",
		},
		"darwin": {
			"amd64": "darwin-amd64",
			"arm64": "darwin-amd64", // Fallback for M1/M2 macs
		},
		"windows": {
			"amd64": "windows-amd64.exe",
		},
	}
	if osMap, ok := osArchMap[runtime.GOOS]; ok {
		if arch, ok := osMap[runtime.GOARCH]; ok {
			return arch
		}
	}
	return "amd64" // Default to amd64 for linux
}
