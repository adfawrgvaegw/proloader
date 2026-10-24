package client

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"time"

	l10n "github.com/Yeah114/Fatalder/utils/localization"
)

// RestartApplication 重启应用程序
func (c *UpdateClient) RestartApplication() error {
	switch runtime.GOOS {
	case "windows":
		return c.restartWindows()
	case "linux", "darwin", "android":
		return c.restartUnix()
	default:
		return fmt.Errorf("%s: %s", l10n.T(l10n.KeyAutoUpdateUnsupportedPlatform), runtime.GOOS)
	}
}

// restartWindows Windows平台重启
func (c *UpdateClient) restartWindows() error {
	// 获取当前进程参数
	args := os.Args[1:] // 排除程序名

	// 创建批处理脚本来重启程序
	script := fmt.Sprintf(`@echo off
timeout /t 1 /nobreak >nul
start "" "%s" %s
del "%%~f0"`, c.CurrentFile, joinArgs(args))

	// 写入临时批处理文件
	scriptFile := c.CurrentFile + "_restart.bat"
	if err := os.WriteFile(scriptFile, []byte(script), 0644); err != nil {
		return fmt.Errorf("%s: %w", l10n.T(l10n.KeyAutoUpdateScriptCreateFailed), err)
	}

	// 执行批处理脚本
	cmd := exec.Command("cmd", "/C", scriptFile)
	// Windows特定属性在构建时会处理
	if err := cmd.Start(); err != nil {
		os.Remove(scriptFile)
		return fmt.Errorf("%s: %w", l10n.T(l10n.KeyAutoUpdateScriptStartFailed), err)
	}

	// 延迟退出，让脚本有时间启动
	time.Sleep(500 * time.Millisecond)
	os.Exit(0)
	return nil
}

// restartUnix Unix平台重启
func (c *UpdateClient) restartUnix() error {
	// 获取当前进程参数
	args := os.Args[1:] // 排除程序名

	// 创建shell脚本来重启程序
	script := fmt.Sprintf(`#!/bin/sh
sleep 1
"%s" %s &
rm -f "$0"`, c.CurrentFile, joinArgs(args))

	// 写入临时脚本文件
	scriptFile := c.CurrentFile + "_restart.sh"
	if err := os.WriteFile(scriptFile, []byte(script), 0755); err != nil {
		return fmt.Errorf("%s: %w", l10n.T(l10n.KeyAutoUpdateScriptCreateFailed), err)
	}

	// 执行脚本
	cmd := exec.Command("sh", scriptFile)
	setSysProcAttr(cmd)

	if err := cmd.Start(); err != nil {
		os.Remove(scriptFile)
		return fmt.Errorf("%s: %w", l10n.T(l10n.KeyAutoUpdateScriptStartFailed), err)
	}

	// 延迟退出，让脚本有时间启动
	time.Sleep(500 * time.Millisecond)
	os.Exit(0)
	return nil
}

// joinArgs 连接参数
func joinArgs(args []string) string {
	if len(args) == 0 {
		return ""
	}

	result := ""
	for i, arg := range args {
		if i > 0 {
			result += " "
		}

		// 如果参数包含空格，添加引号
		if containsSpace(arg) {
			result += "\"" + arg + "\""
		} else {
			result += arg
		}
	}
	return result
}

// containsSpace 检查字符串是否包含空格
func containsSpace(s string) bool {
	for _, r := range s {
		if r == ' ' {
			return true
		}
	}
	return false
}
