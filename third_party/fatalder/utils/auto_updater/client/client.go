package client

import (
	"crypto/sha512"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	l10n "github.com/Yeah114/Fatalder/utils/localization"
)

// NewUpdateClient 创建新的更新客户端
func NewUpdateClient(serverURL, prefix string) *UpdateClient {
	return &UpdateClient{
		ServerURL:   strings.TrimSuffix(serverURL, "/"),
		CurrentFile: getCurrentExecutablePath(),
		Timeout:     30 * time.Second,
		UserAgent:   "Fatalder-AutoUpdater/1.0",
		Prefix:      prefix,
	}
}

// getCurrentExecutablePath 获取当前可执行文件路径
func getCurrentExecutablePath() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}

	// 检测是否在开发模式下运行 (go run)
	// 如果路径包含 go-build，说明是从构建缓存运行，不应该被更新
	if strings.Contains(exe, "go-build") {
		return "" // 返回空路径，阻止自动更新
	}

	return exe
}

// getTargetFilename 根据当前平台和前缀获取目标文件名
func getTargetFilename(prefix string) string {
	// 如果前缀为空，使用默认前缀
	if prefix == "" {
		prefix = "Fatalder-cli"
	}

	switch runtime.GOOS {
	case "windows":
		return fmt.Sprintf("%s_windows_amd64.exe", prefix)
	case "linux":
		return fmt.Sprintf("%s_linux_amd64", prefix)
	case "android":
		return fmt.Sprintf("%s_android_aarch64", prefix)
	default:
		return fmt.Sprintf("%s_linux_amd64", prefix) // 默认
	}
}

// calculateSHA512 计算文件的SHA512
func (c *UpdateClient) calculateSHA512(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	h := sha512.New()
	if _, err := io.Copy(h, file); err != nil {
		return "", err
	}

	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

// CheckUpdate 检查是否有更新
func (c *UpdateClient) CheckUpdate() (*UpdateInfo, error) {
	if c.CurrentFile == "" {
		// 开发模式下直接返回无更新，避免错误
		return &UpdateInfo{
			HasUpdate:    false,
			RemoteSHA512: "",
			LocalSHA512:  "dev-mode",
			Filename:     "",
		}, nil
	}

	// 获取目标文件名
	filename := getTargetFilename(c.Prefix)

	// 计算本地文件SHA512
	localSHA512, err := c.calculateSHA512(c.CurrentFile)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", l10n.T(l10n.KeyAutoUpdateCalcSHAFailed), err)
	}

	// 从 GitHub Releases 获取远程文件SHA512
	remoteSHA512, err := c.getRemoteSHA512FromGitHub(filename)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", l10n.T(l10n.KeyAutoUpdateGetSHAFailed), err)
	}

	// 获取目标二进制的下载链接（使用 assets 中的 browser_download_url 并替换镜像）
	downloadURL, err := c.getAssetDownloadURL(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to get asset url: %w", err)
	}
	info := &UpdateInfo{
		HasUpdate:    localSHA512 != remoteSHA512,
		RemoteSHA512: remoteSHA512,
		LocalSHA512:  localSHA512,
		DownloadURL:  downloadURL,
		Filename:     filename,
	}

	return info, nil
}

// getRemoteSHA512 获取远程文件的SHA512
// getGitHubLatestRelease 获取最新 release 信息
func (c *UpdateClient) getGitHubLatestRelease() (map[string]any, error) {
	// 固定使用 GitHub API 获取最新 release
	url := "https://api.github.com/repos/Yeah114/Fatalder-Release/releases/latest"
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", c.UserAgent)
	client := &http.Client{Timeout: c.Timeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: %d", l10n.T(l10n.KeyAutoUpdateServerError), resp.StatusCode)
	}
	var obj map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&obj); err != nil {
		return nil, err
	}
	return obj, nil
}

func (c *UpdateClient) getRemoteSHA512FromGitHub(filename string) (string, error) {
	release, err := c.getGitHubLatestRelease()
	if err != nil {
		return "", err
	}
	assetsAny, ok := release["assets"].([]any)
	if !ok {
		return "", errors.New("invalid release assets")
	}
	target := filename + ".sha"
	for _, a := range assetsAny {
		m, _ := a.(map[string]any)
		if m == nil {
			continue
		}
		name, _ := m["name"].(string)
		if name == target {
			// 下载 .sha 内容（文本）
			browserURL, _ := m["browser_download_url"].(string)
			if browserURL == "" {
				return "", errors.New("empty download url")
			}
			// 替换为自定义 base（若 ServerURL 非默认）
			if !strings.HasPrefix(browserURL, c.ServerURL) && c.ServerURL != "" {
				// browserURL 示例: https://github.com/Yeah114/Fatalder-Release/releases/download/vX/file.sha
				// 将 https://github.com 前缀替换为自定义 base
				browserURL = strings.Replace(browserURL, "https://github.com", strings.TrimSuffix(c.ServerURL, "/"), 1)
			}
			req, err := http.NewRequest("GET", browserURL, nil)
			if err != nil {
				return "", err
			}
			req.Header.Set("User-Agent", c.UserAgent)
			httpClient := &http.Client{Timeout: c.Timeout}
			resp, err := httpClient.Do(req)
			if err != nil {
				return "", err
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				return "", fmt.Errorf("%s: %d", l10n.T(l10n.KeyAutoUpdateServerError), resp.StatusCode)
			}
			bytes, err := io.ReadAll(resp.Body)
			if err != nil {
				return "", err
			}
			return strings.TrimSpace(string(bytes)), nil
		}
	}
	return "", fmt.Errorf("%s", l10n.T(l10n.KeyAutoUpdateGetSHAFailed))
}

// getAssetDownloadURL 从最新 release 的 assets 中获取指定文件的下载链接，并按需要替换镜像前缀
func (c *UpdateClient) getAssetDownloadURL(filename string) (string, error) {
	release, err := c.getGitHubLatestRelease()
	if err != nil {
		return "", err
	}
	assetsAny, ok := release["assets"].([]any)
	if !ok {
		return "", errors.New("invalid release assets")
	}
	for _, a := range assetsAny {
		m, _ := a.(map[string]any)
		if m == nil {
			continue
		}
		name, _ := m["name"].(string)
		if name == filename {
			browserURL, _ := m["browser_download_url"].(string)
			if browserURL == "" {
				return "", errors.New("empty download url")
			}
			// 将 https://github.com 前缀替换为配置的镜像（仅替换前缀一次）
			base := strings.TrimSuffix(c.ServerURL, "/")
			if base != "" && base != "https://github.com" && strings.HasPrefix(browserURL, "https://github.com") {
				browserURL = strings.Replace(browserURL, "https://github.com", base, 1)
			}
			return browserURL, nil
		}
	}
	return "", fmt.Errorf("asset not found: %s", filename)
}

// DownloadUpdate 下载更新文件
func (c *UpdateClient) DownloadUpdate(info *UpdateInfo, progressCallback ProgressCallback) (string, error) {
	client := &http.Client{Timeout: 5 * time.Minute} // 下载超时5分钟

	req, err := http.NewRequest("GET", info.DownloadURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", c.UserAgent)

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("%s: %d", l10n.T(l10n.KeyAutoUpdateDownloadError), resp.StatusCode)
	}

	// 创建临时文件
	tempDir := os.TempDir()
	tempFile := filepath.Join(tempDir, "fatalder_update_"+info.Filename)

	file, err := os.Create(tempFile)
	if err != nil {
		return "", err
	}
	defer file.Close()

	// 获取文件大小
	contentLength := resp.ContentLength

	// 下载文件并显示进度
	var downloaded int64
	buffer := make([]byte, 32*1024) // 32KB缓冲区

	for {
		n, err := resp.Body.Read(buffer)
		if n > 0 {
			if _, writeErr := file.Write(buffer[:n]); writeErr != nil {
				return "", writeErr
			}
			downloaded += int64(n)

			if progressCallback != nil && contentLength > 0 {
				percent := float64(downloaded) / float64(contentLength) * 100
				progressCallback(downloaded, contentLength, percent)
			}
		}

		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}
	}

	// 验证下载文件的SHA512
	downloadedSHA512, err := c.calculateSHA512(tempFile)
	if err != nil {
		os.Remove(tempFile)
		return "", fmt.Errorf("%s: %w", l10n.T(l10n.KeyAutoUpdateVerifyFailed), err)
	}

	if downloadedSHA512 != info.RemoteSHA512 {
		os.Remove(tempFile)
		return "", fmt.Errorf("%s", l10n.T(l10n.KeyAutoUpdateCheckSumFailed))
	}

	return tempFile, nil
}

// ReplaceExecutable 替换当前可执行文件
func (c *UpdateClient) ReplaceExecutable(newFile string) error {
	// 备份当前文件
	backupFile := c.CurrentFile + ".backup"
	if err := copyFile(c.CurrentFile, backupFile); err != nil {
		return fmt.Errorf("%s: %w", l10n.T(l10n.KeyAutoUpdateBackupFailed), err)
	}

	// 设置新文件的可执行权限 (Unix系统)
	if runtime.GOOS != "windows" {
		if err := os.Chmod(newFile, 0755); err != nil {
			return fmt.Errorf("%s: %w", l10n.T(l10n.KeyAutoUpdatePermissionFailed), err)
		}
	}

	// 使用原子操作替换文件，避免 "text file busy" 错误
	if err := os.Rename(newFile, c.CurrentFile); err != nil {
		// 恢复备份
		copyFile(backupFile, c.CurrentFile)
		return fmt.Errorf("%s: %w", l10n.T(l10n.KeyAutoUpdateReplaceFailed), err)
	}

	// 清理备份文件
	os.Remove(backupFile)

	return nil
}

// copyFile 复制文件
func copyFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	dstFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	_, err = io.Copy(dstFile, srcFile)
	return err
}
