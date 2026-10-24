package args

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/Yeah114/Fatalder/cmd/common"
	"github.com/Yeah114/Fatalder/constants"
	"github.com/Yeah114/Fatalder/utils"
	l10n "github.com/Yeah114/Fatalder/utils/localization"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// 全局变量（保持单例模式）
var (
	argsOnce    sync.Once
	rootCmd     *cobra.Command
	initialized bool // 标记是否已初始化
)

// 自动初始化基础系统（不解析参数）
func init() {
	initSystems()
}

// initSystems 初始化存储、配置、i18n 系统（复用原有逻辑）
func initSystems() {
	argsOnce.Do(func() {
		// 初始化存储系统
		if err := utils.InitStorage(); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to initialize storage: %v\n", err)
			os.Exit(1)
		}

		// 初始化配置系统
		if err := utils.InitConfig(); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to initialize config: %v\n", err)
			os.Exit(1)
		}

		// 初始化语言包管理器
		localesDir := filepath.Join(utils.GetStoragePath(), "locales")
		if err := l10n.InitManager(localesDir); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to initialize localization manager: %v\n", err)
			os.Exit(1)
		}
		// 设置初始语言
		currentLang := utils.GetLanguage()
		l10n.SetCurrentLanguage(currentLang)

		// 初始化 cobra root 命令
		initCobraCmd()
	})
}

// initCobraCmd 初始化 cobra 命令结构（核心重构点）
func initCobraCmd() {
	rootCmd = &cobra.Command{
		Use:   constants.Name,
		Short: fmt.Sprintf(l10n.T(l10n.KeyAppDescription), constants.Name),
		Long:  fmt.Sprintf(l10n.T(l10n.KeyAppDescription), constants.Name),
		Run: func(cmd *cobra.Command, args []string) {
			// 主命令逻辑
			initialized = true
		},
	}

	rootCmd.Flags().BoolP(
		"help",
		"h",
		false,
		l10n.T(l10n.KeyArgsHelpUsage),
	)

	rootCmd.Flags().BoolP(
		"version",
		"v",
		false,
		l10n.T(l10n.KeyArgsVersionUsage),
	)

	rootCmd.Flags().StringP(
		"storage-path",
		"s",
		"",
		l10n.T(l10n.KeyArgsStoragePathUsage),
	)

	rootCmd.Flags().StringP(
		"language",
		"l",
		"",
		l10n.T(l10n.KeyArgsLanguageUsage),
	)
}

// Parse 解析命令行参数（开放返回值：error=初始化是否成功，bool=是否继续运行）
func Parse() (error error, shouldContinue bool) {
	if initialized {
		return nil, true // 已解析过，直接返回
	}

	// 预处理语言参数（确保 flag 描述使用正确语言）
	preprocessLanguageArg()

	// 解析命令行参数
	if err := rootCmd.Execute(); err != nil {
		return fmt.Errorf("failed to parse args: %w", err), false
	}

	shouldContinue = initialized
	storagePath, _ := rootCmd.Flags().GetString("storage-path")
	language, _ := rootCmd.Flags().GetString("language")
	showVersion, _ := rootCmd.Flags().GetBool("version")

	// 处理存储路径参数（复用原有逻辑）
	if storagePath != "" {
		if err := utils.SetStoragePath(storagePath); err != nil {
			return fmt.Errorf("invalid storage path: %w", err), false
		}
		// 更新配置中的存储路径
		if err := utils.GetConfig().SetStoragePathInConfig(storagePath); err != nil {
			return fmt.Errorf("failed to update storage path in config: %w", err), false
		}
		// 重新初始化 l10n
		localesDir := filepath.Join(utils.GetStoragePath(), "locales")
		if err := l10n.ReinitManager(localesDir); err != nil {
			return fmt.Errorf("failed to reinit localization: %w", err), false
		}
		l10n.SetCurrentLanguage(utils.GetLanguage())
	}

	// 处理语言参数（复用原有逻辑）
	if language != "" {
		if err := utils.SetLanguage(language); err != nil {
			return fmt.Errorf("invalid language: %w", err), false
		}
		l10n.SetCurrentLanguage(language)
	}

	if showVersion {
		common.ShowProgramInfo()
		shouldContinue = false
	}

	// 标记初始化完成，返回继续运行
	return nil, shouldContinue
}

// Flags 返回 cobra 命令的 Flags 实例（开放 Flags 供外部使用）
func Flags() *pflag.FlagSet {
	return rootCmd.Flags()
}

// preprocessLanguageArg 预处理语言参数（确保 flag 描述翻译正确）
func preprocessLanguageArg() {
	for i, arg := range os.Args {
		arg = strings.TrimPrefix(arg, "-")
		if arg == "language" && i+1 < len(os.Args) {
			lang := os.Args[i+1]
			if err := utils.SetLanguage(lang); err == nil {
				l10n.SetCurrentLanguage(lang)
				// 重新初始化 cobra 命令（更新 flag 描述翻译）
				initCobraCmd()
			}
			break
		}
	}
}

// GetRootCmd 暴露 rootCmd 供外部扩展（如添加子命令）
func GetRootCmd() *cobra.Command {
	return rootCmd
}
