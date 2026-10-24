package client

import (
	"bufio"
	"fmt"
	"os"

	l10n "github.com/Yeah114/Fatalder/utils/localization"
)

// 全局变量控制是否开启自动更新
var EnableAutoUpdate = true

// AutoUpdateConfig 自动更新配置
type AutoUpdateConfig struct {
	ServerURL string // GitHub 基础URL（可替换为镜像，例如 https://github.com）
	Silent    bool   // 静默模式，不显示进度条
	Prefix    string // 文件名前缀，例如 "Fatalder-cli"
}

// DefaultConfig 默认配置
var DefaultConfig = &AutoUpdateConfig{
	ServerURL: "https://github.com", // 默认 GitHub
	Silent:    false,
	Prefix:    "Fatalder-cli", // 默认文件前缀
}

// PerformAutoUpdate 执行自动更新的主函数
// 新逻辑：
// 1. 启动时删除.new和.old文件
// 2. 与远程服务器文件哈希对比
// 3. 如果不一样，下载新文件到.new
// 4. 验证下载文件与远程哈希一致
// 5. 将当前文件重命名为.old，将.new文件重命名为原名
// 6. 提示更新完成并等待用户按键退出
func PerformAutoUpdate(config *AutoUpdateConfig) error {
	// 检查是否启用自动更新
	if !EnableAutoUpdate {
		return nil // 静默返回，不进行更新
	}

	if config == nil {
		config = DefaultConfig
	}

	// 创建更新客户端
	client := NewUpdateClient(config.ServerURL, config.Prefix)

	// 步骤1：启动时删除.new和.old文件
	if client.CurrentFile != "" {
		oldFile := client.CurrentFile + ".old"
		newFile := client.CurrentFile + ".new"

		if _, err := os.Stat(oldFile); err == nil {
			os.Remove(oldFile)
			if !config.Silent {
				fmt.Printf("已删除旧的.old文件: %s\n", oldFile)
			}
		}

		if _, err := os.Stat(newFile); err == nil {
			os.Remove(newFile)
			if !config.Silent {
				fmt.Printf("已删除旧的.new文件: %s\n", newFile)
			}
		}
	}

	if !config.Silent {
		fmt.Println(l10n.T(l10n.KeyAutoUpdateChecking))
	}

	// 步骤2：检查更新（与远程服务器文件哈希对比）
	updateInfo, err := client.CheckUpdate()
	if err != nil {
		return fmt.Errorf("%s: %w", l10n.T(l10n.KeyAutoUpdateCheckFailed), err)
	}

	// 如果没有更新，直接返回
	if !updateInfo.HasUpdate {
		if !config.Silent {
			// 检查是否是开发模式
			if updateInfo.LocalSHA512 == "dev-mode" {
				fmt.Println(l10n.T(l10n.KeyAutoUpdateDevMode))
			} else {
				fmt.Println(l10n.T(l10n.KeyAutoUpdateLatest))
			}
		}
		return nil
	}

	if !config.Silent {
		fmt.Println(l10n.T(l10n.KeyAutoUpdateNewVersion))
		fmt.Printf("   %s: %s...\n", l10n.T(l10n.KeyAutoUpdateLocalVersion), updateInfo.LocalSHA512[:16])
		fmt.Printf("   %s: %s...\n", l10n.T(l10n.KeyAutoUpdateRemoteVersion), updateInfo.RemoteSHA512[:16])
	}

	// 创建进度回调
	var progressCallback ProgressCallback
	if !config.Silent {
		progressCallback = CreateProgressCallback()
	}

	// 步骤3：下载新文件
	tempFile, err := client.DownloadUpdate(updateInfo, progressCallback)
	if err != nil {
		return fmt.Errorf("%s: %w", l10n.T(l10n.KeyAutoUpdateDownloadFailed), err)
	}
	defer os.Remove(tempFile)

	// 步骤4：下载完成后验证哈希已在DownloadUpdate中完成

	// 步骤5：将当前文件重命名为.old，将下载的文件重命名为原名
	if client.CurrentFile != "" {
		oldFile := client.CurrentFile + ".old"
		newFile := client.CurrentFile + ".new"

		// 先将下载的文件复制到.new位置
		if err := copyFile(tempFile, newFile); err != nil {
			return fmt.Errorf("复制文件到.new失败: %w", err)
		}

		// 设置可执行权限
		if err := os.Chmod(newFile, 0755); err != nil {
			return fmt.Errorf("设置.new文件权限失败: %w", err)
		}

		// 将当前文件重命名为.old
		if err := os.Rename(client.CurrentFile, oldFile); err != nil {
			os.Remove(newFile) // 清理.new文件
			return fmt.Errorf("重命名当前文件为.old失败: %w", err)
		}

		// 将.new文件重命名为原文件名
		if err := os.Rename(newFile, client.CurrentFile); err != nil {
			// 如果失败，尝试恢复原文件
			os.Rename(oldFile, client.CurrentFile)
			return fmt.Errorf("重命名.new文件失败: %w", err)
		}

		if !config.Silent {
			fmt.Printf("更新成功！当前版本已备份为: %s\n", oldFile)
		}
	}

	// 步骤6：提示更新完成并等待用户按键退出
	if !config.Silent {
		fmt.Println("更新完成，请重新打开程序，按任意键后退出")
		reader := bufio.NewReader(os.Stdin)
		reader.ReadRune()
	}

	os.Exit(0)
	return nil
}

// CheckUpdateOnly 仅检查更新，不执行下载和安装
func CheckUpdateOnly(config *AutoUpdateConfig) (*UpdateInfo, error) {
	if !EnableAutoUpdate {
		return nil, fmt.Errorf("%s", l10n.T(l10n.KeyAutoUpdateDisabled))
	}

	if config == nil {
		config = DefaultConfig
	}

	client := NewUpdateClient(config.ServerURL, config.Prefix)
	return client.CheckUpdate()
}

// SetUpdateServer 设置更新服务器地址
func SetUpdateServer(serverURL string) {
	DefaultConfig.ServerURL = serverURL
}

// DisableAutoUpdate 禁用自动更新
func DisableAutoUpdate() {
	EnableAutoUpdate = false
}

// EnableAutoUpdateFunc 启用自动更新
func EnableAutoUpdateFunc() {
	EnableAutoUpdate = true
}
