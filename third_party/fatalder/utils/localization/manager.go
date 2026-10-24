package l10n

import (
	"bufio"
	"embed"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
)

//go:embed locales/*.properties
var embeddedLocales embed.FS

// Manager 语言包管理器
type Manager struct {
	locales     map[string]map[string]string
	currentLang string
	localesDir  string
	mu          sync.RWMutex
}

// 全局管理器实例
var globalManager *Manager
var managerOnce sync.Once

// InitManager 初始化语言包管理器
func InitManager(localesDir string) error {
	var err error
	managerOnce.Do(func() {
		globalManager = &Manager{
			locales:    make(map[string]map[string]string),
			localesDir: localesDir,
		}
		err = globalManager.init()
	})
	return err
}

// ReinitManager 重新初始化语言包管理器（用于更改存储路径）
func ReinitManager(localesDir string) error {
	globalManager = &Manager{
		locales:    make(map[string]map[string]string),
		localesDir: localesDir,
	}
	return globalManager.init()
}

// GetManager 获取全局管理器实例
func GetManager() *Manager {
	if globalManager == nil {
		// 使用默认路径初始化
		InitManager(filepath.Join(".", "locales"))
	}
	return globalManager
}

// init 初始化管理器
func (m *Manager) init() error {
	// 确保语言包目录存在
	if err := os.MkdirAll(m.localesDir, 0755); err != nil {
		return fmt.Errorf("failed to create locales directory: %w", err)
	}

	// 首次运行时解压默认语言包
	if err := m.extractDefaultLocales(); err != nil {
		return fmt.Errorf("failed to extract default locales: %w", err)
	}

	// 加载所有语言包
	if err := m.loadAllLocales(); err != nil {
		return fmt.Errorf("failed to load locales: %w", err)
	}

	return nil
}

// extractDefaultLocales 从嵌入文件解压默认语言包
func (m *Manager) extractDefaultLocales() error {
	// 检查是否已经解压过
	markerFile := filepath.Join(m.localesDir, ".extracted")
	if _, err := os.Stat(markerFile); err == nil {
		// return nil // 已经解压过
	}

	// 读取嵌入的语言包文件
	entries, err := embeddedLocales.ReadDir("locales")
	if err != nil {
		return fmt.Errorf("failed to read embedded locales: %w", err)
	}

	// 解压每个语言包文件
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".properties") {
			continue
		}

		// 读取嵌入的文件内容 (使用正斜杠路径，embed.FS总是期望Unix风格路径)
		embeddedPath := path.Join("locales", entry.Name())
		data, err := embeddedLocales.ReadFile(embeddedPath)
		if err != nil {
			return fmt.Errorf("failed to read embedded file %s: %w", embeddedPath, err)
		}

		// 写入到用户配置目录
		outputPath := filepath.Join(m.localesDir, entry.Name())
		if err := os.WriteFile(outputPath, data, 0644); err != nil {
			return fmt.Errorf("failed to write locale file %s: %w", outputPath, err)
		}

		fmt.Printf("Extracted language pack: %s\n", entry.Name())
	}

	// 创建标记文件
	if err := os.WriteFile(markerFile, []byte("extracted"), 0644); err != nil {
		return fmt.Errorf("failed to create marker file: %w", err)
	}

	return nil
}

// loadAllLocales 加载所有语言包
func (m *Manager) loadAllLocales() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 清空现有语言包
	m.locales = make(map[string]map[string]string)

	// 读取语言包目录
	entries, err := os.ReadDir(m.localesDir)
	if err != nil {
		return fmt.Errorf("failed to read locales directory: %w", err)
	}

	// 加载每个语言包文件
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".properties") {
			continue
		}

		lang := strings.TrimSuffix(entry.Name(), ".properties")
		filePath := filepath.Join(m.localesDir, entry.Name())

		if err := m.loadLocale(lang, filePath); err != nil {
			fmt.Printf("Warning: failed to load locale %s: %v\n", lang, err)
			continue
		}
	}

	return nil
}

// loadLocale 加载单个语言包
func (m *Manager) loadLocale(lang, filePath string) error {
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("failed to open locale file: %w", err)
	}
	defer file.Close()

	translations := make(map[string]string)
	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// 跳过空行和注释
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// 解析 key=value 格式
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue // 跳过格式不正确的行
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		translations[key] = value
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("failed to scan locale file: %w", err)
	}

	m.locales[lang] = translations
	return nil
}

// SetLanguage 设置当前语言
func (m *Manager) SetLanguage(lang string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.currentLang = lang
}

// GetLanguage 获取当前语言
func (m *Manager) GetLanguage() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.currentLang
}

// T 翻译函数
func (m *Manager) T(key string) string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// 尝试当前语言
	if translations, exists := m.locales[m.currentLang]; exists {
		if text, exists := translations[key]; exists {
			return text
		}
	}

	// 回退到中文
	if translations, exists := m.locales["zh"]; exists {
		if text, exists := translations[key]; exists {
			return text
		}
	}

	// 如果都找不到，返回键名
	return key
}

// GetAvailableLanguages 获取可用语言列表
func (m *Manager) GetAvailableLanguages() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var langs []string
	for lang := range m.locales {
		langs = append(langs, lang)
	}
	return langs
}

// GetLanguageDisplayName 获取语言包的显示名称
func (m *Manager) GetLanguageDisplayName(lang string) string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if translations, exists := m.locales[lang]; exists {
		if displayName, exists := translations["pack.name"]; exists {
			return displayName
		}
	}

	// 如果没有找到 pack.name，返回语言代码作为后备
	return lang
}

// GetAvailableLanguagesWithNames 获取可用语言列表及其显示名称
func (m *Manager) GetAvailableLanguagesWithNames() map[string]string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[string]string)
	for lang := range m.locales {
		result[lang] = m.GetLanguageDisplayName(lang)
	}
	return result
}

// ReloadLocales 重新加载语言包
func (m *Manager) ReloadLocales() error {
	return m.loadAllLocales()
}

// 全局便捷函数
func T(key string) string {
	return GetManager().T(key)
}

func SetCurrentLanguage(lang string) {
	GetManager().SetLanguage(lang)
}

func GetCurrentLanguage() string {
	return GetManager().GetLanguage()
}

func GetAvailableLanguages() []string {
	return GetManager().GetAvailableLanguages()
}

func GetLanguageDisplayName(lang string) string {
	return GetManager().GetLanguageDisplayName(lang)
}

func GetAvailableLanguagesWithNames() map[string]string {
	return GetManager().GetAvailableLanguagesWithNames()
}
