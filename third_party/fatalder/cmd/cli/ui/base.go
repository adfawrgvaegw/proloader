package ui

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	l10n "github.com/Yeah114/Fatalder/utils/localization"
)

var stdinReader = bufio.NewReader(os.Stdin)

func sanitizePrompt(hint string) string {
	if hint == "" {
		return ""
	}
	trimmed := strings.TrimRight(hint, "\r\n")
	trimmed = strings.TrimSpace(trimmed)
	for {
		switch {
		case strings.HasSuffix(trimmed, ":"):
			trimmed = strings.TrimSpace(strings.TrimSuffix(trimmed, ":"))
		case strings.HasSuffix(trimmed, "："):
			trimmed = strings.TrimSpace(strings.TrimSuffix(trimmed, "："))
		default:
			return trimmed
		}
	}
}

// Input 获取用户输入（使用 pterm 交互式输入）
func Input(hint string) string {
	prompt := sanitizePrompt(hint)
	Info.Print(prompt + ": ")
	line, err := stdinReader.ReadString('\n')
	if err != nil {
		if errors.Is(err, io.EOF) {
			return strings.TrimSpace(line)
		}
		return ""
	}
	return strings.TrimSpace(line)
}

// InputWithDefault 获取用户输入，支持默认值
func InputWithDefault(hint, defaultValue string) string {
	basePrompt := sanitizePrompt(hint)
	displayPrompt := basePrompt
	if defaultValue != "" {
		displayPrompt = fmt.Sprintf("%s [%s]", basePrompt, defaultValue)
	}
	input := Input(displayPrompt)
	if strings.TrimSpace(input) == "" {
		return defaultValue
	}
	return input
}

// Confirm 获取用户确认
func Confirm(hint string) bool {
	return ConfirmWithDefault(hint, false)
}

// ConfirmWithDefault 获取用户确认，支持默认值
func ConfirmWithDefault(hint string, defaultValue bool) bool {
	defaultText := "n"
	if defaultValue {
		defaultText = "y"
	}
	text := InputWithDefault(hint, defaultText)
	result := strings.Contains(strings.ToLower(text), "y")
	return result
}

// Password 获取密码输入
func Password(hint string) string {
	text := Input(hint)
	return text
}

// Select 从选项中选择
func Select(hint string, options []string) (int, string) {
	fmt.Println(hint)
	for i, option := range options {
		fmt.Printf("  %d. %s\n", i+1, option)
	}

	prompt := fmt.Sprintf("%s (1-%d): ", l10n.T(l10n.KeyUISelectPrompt), len(options))

	for {
		choice := InputInt(prompt)

		if choice < 1 || choice > len(options) {
			fmt.Printf(l10n.T(l10n.KeyUISelectRange)+"\n", len(options))
			continue
		}

		return choice - 1, options[choice-1]
	}
}

// MultiInput 获取多行输入，直到用户输入空行
func MultiInput(hint string) []string {
	fmt.Printf("%s (%s):\n", hint, l10n.T(l10n.KeyUIMultiInputEnd))

	var lines []string
	lineNum := 1

	for {
		prompt := fmt.Sprintf("%d> ", lineNum)
		input := strings.TrimSpace(Input(prompt))

		if input == "" {
			break
		}

		lines = append(lines, input)
		lineNum++
	}

	return lines
}

// InputInt 获取整数输入
func InputInt(hint string) int {
	for {
		input := Input(hint)
		if input == "" {
			continue
		}

		var value int
		if _, err := fmt.Sscanf(input, "%d", &value); err != nil {
			Error.Println(l10n.T(l10n.KeyUIInputValidInteger))
			continue
		}

		return value
	}
}

// InputIntWithDefault 获取整数输入，支持默认值
func InputIntWithDefault(hint string, defaultValue int) int {
	input := InputWithDefault(hint, fmt.Sprintf("%d", defaultValue))
	if input == fmt.Sprintf("%d", defaultValue) {
		return defaultValue
	}

	var value int
	if _, err := fmt.Sscanf(input, "%d", &value); err != nil {
		return defaultValue
	}

	return value
}

// InputFloat 获取浮点数输入
func InputFloat(hint string) float64 {
	for {
		input := Input(hint)
		if input == "" {
			continue
		}

		var value float64
		if _, err := fmt.Sscanf(input, "%f", &value); err != nil {
			Error.Println(l10n.T(l10n.KeyUIInputValidNumber))
			continue
		}

		return value
	}
}

// ValidateInput 验证输入
func ValidateInput(hint string, validator func(string) bool, errorMsg string) string {
	for {
		input := Input(hint)
		if validator(input) {
			return input
		}
		Error.Println(errorMsg)
	}
}
