package utils

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/Yeah114/Fatalder/constants"
)

var (
	StoragePath          string
	ErrNoValidStorageDir = errors.New("no valid storage directory found")
)

// FindStableUserDir 自动寻找稳定的用户目录
func FindStableUserDir() (string, error) {
	var candidates []string

	switch runtime.GOOS {
	case "windows":
		candidates = getWindowsStorageCandidates()
	case "darwin":
		candidates = getMacOSStorageCandidates()
	case "linux":
		candidates = getLinuxStorageCandidates()
	case "android":
		candidates = getLinuxStorageCandidates()
		candidates = append(candidates, "/sdcard", "/sdcard/Download")
	default:
		candidates = getUnixStorageCandidates()
	}

	for _, dir := range candidates {
		if isValidStorageDir(dir) {
			return filepath.Join(dir, constants.Name), nil
		}
	}

	return "", ErrNoValidStorageDir
}

// getWindowsStorageCandidates 获取 Windows 系统的存储目录候选列表
func getWindowsStorageCandidates() []string {
	var candidates []string

	// 1. APPDATA (推荐)
	if appdata := os.Getenv("APPDATA"); appdata != "" {
		candidates = append(candidates, appdata)
	}

	// 2. LOCALAPPDATA
	if localAppData := os.Getenv("LOCALAPPDATA"); localAppData != "" {
		candidates = append(candidates, localAppData)
	}

	// 3. USERPROFILE
	if userProfile := os.Getenv("USERPROFILE"); userProfile != "" {
		candidates = append(candidates, userProfile)
	}

	// 4. 当前目录
	if cwd, err := os.Getwd(); err == nil {
		candidates = append(candidates, cwd)
	}

	// 5. 临时目录
	candidates = append(candidates, os.TempDir())

	return candidates
}

// getMacOSStorageCandidates 获取 macOS 系统的存储目录候选列表
func getMacOSStorageCandidates() []string {
	var candidates []string

	// 1. ~/Library/Application Support (推荐)
	if home := os.Getenv("HOME"); home != "" {
		candidates = append(candidates, filepath.Join(home, "Library", "Application Support"))
	}

	// 2. ~/.config
	if home := os.Getenv("HOME"); home != "" {
		candidates = append(candidates, filepath.Join(home, ".config"))
	}

	// 3. 用户主目录
	if home := os.Getenv("HOME"); home != "" {
		candidates = append(candidates, home)
	}

	// 4. 当前目录
	if cwd, err := os.Getwd(); err == nil {
		candidates = append(candidates, cwd)
	}

	// 5. 临时目录
	candidates = append(candidates, os.TempDir())

	return candidates
}

// getLinuxStorageCandidates 获取 Linux 系统的存储目录候选列表
func getLinuxStorageCandidates() []string {
	var candidates []string

	// 1. XDG_DATA_HOME (推荐)
	if xdgDataHome := os.Getenv("XDG_DATA_HOME"); xdgDataHome != "" {
		candidates = append(candidates, xdgDataHome)
	}

	// 2. ~/.local/share (XDG 默认)
	if home := os.Getenv("HOME"); home != "" {
		candidates = append(candidates, filepath.Join(home, ".local", "share"))
	}

	// 3. XDG_CONFIG_HOME
	if xdgConfigHome := os.Getenv("XDG_CONFIG_HOME"); xdgConfigHome != "" {
		candidates = append(candidates, xdgConfigHome)
	}

	// 4. ~/.config (XDG 默认)
	if home := os.Getenv("HOME"); home != "" {
		candidates = append(candidates, filepath.Join(home, ".config"))
	}

	// 5. 用户主目录
	if home := os.Getenv("HOME"); home != "" {
		candidates = append(candidates, home)
	}

	// 6. 当前目录
	if cwd, err := os.Getwd(); err == nil {
		candidates = append(candidates, cwd)
	}

	// 7. 临时目录
	candidates = append(candidates, os.TempDir())

	return candidates
}

// getUnixStorageCandidates 获取其他 Unix 系统的存储目录候选列表
func getUnixStorageCandidates() []string {
	var candidates []string

	// 1. 用户主目录
	if home := os.Getenv("HOME"); home != "" {
		candidates = append(candidates, home)
	}

	// 2. 当前目录
	if cwd, err := os.Getwd(); err == nil {
		candidates = append(candidates, cwd)
	}

	// 3. 临时目录
	candidates = append(candidates, os.TempDir())

	return candidates
}

// isValidStorageDir 检查目录是否适合作为存储目录
func isValidStorageDir(dir string) bool {
	if dir == "" {
		return false
	}

	// 检查目录是否存在
	info, err := os.Stat(dir)
	if err != nil {
		// 如果目录不存在，尝试创建父目录
		if os.IsNotExist(err) {
			return canCreateDir(dir)
		}
		return false
	}

	// 检查是否为目录
	if !info.IsDir() {
		return false
	}

	// 检查是否有写权限
	return hasWritePermission(dir)
}

// canCreateDir 检查是否可以创建目录
func canCreateDir(dir string) bool {
	parent := filepath.Dir(dir)
	if parent == dir {
		return false // 到达根目录
	}

	// 检查父目录是否存在且有写权限
	info, err := os.Stat(parent)
	if err != nil {
		if os.IsNotExist(err) {
			// 递归检查父目录
			return canCreateDir(parent)
		}
		return false
	}

	if !info.IsDir() {
		return false
	}

	return hasWritePermission(parent)
}

// hasWritePermission 检查目录是否有写权限
func hasWritePermission(dir string) bool {
	// 尝试在目录中创建一个临时文件来测试写权限
	testFile := filepath.Join(dir, ".fatalder_write_test")
	file, err := os.Create(testFile)
	if err != nil {
		return false
	}
	file.Close()
	os.Remove(testFile)
	return true
}

// InitStorage 初始化存储目录
func InitStorage() error {
	if StoragePath != "" {
		// 如果已经设置了存储路径，检查是否有效
		if isValidStorageDir(filepath.Dir(StoragePath)) {
			return ensureStorageDir(StoragePath)
		}
	}

	// 自动寻找合适的存储目录
	dir, err := FindStableUserDir()
	if err != nil {
		return fmt.Errorf("failed to find stable user directory: %w", err)
	}

	StoragePath = dir
	return ensureStorageDir(StoragePath)
}

// ensureStorageDir 确保存储目录存在
func ensureStorageDir(dir string) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create storage directory %s: %w", dir, err)
	}
	return nil
}

// GetStoragePath 获取当前存储路径
func GetStoragePath() string {
	return StoragePath
}

// SetStoragePath 设置存储路径
func SetStoragePath(path string) error {
	if !isValidStorageDir(filepath.Dir(path)) {
		return fmt.Errorf("invalid storage directory: %s", filepath.Dir(path))
	}
	StoragePath = path
	return ensureStorageDir(StoragePath)
}

// FileExists 检查文件是否存在
func FileExists(filename string) bool {
	_, err := os.Stat(filename)
	return err == nil
}
