package utils

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/Yeah114/Fatalder/constants"
)

// BuildTaskConfig 构建任务配置
type BuildTaskConfig struct {
	MinOnceParseChunkNum int     `json:"min_once_parse_chunk_num"` // 单次解析区块数量最小值
	MinMatchingDegree    float64 `json:"min_matching_degree"`      // 匹配度最小值 (0-100)
	MaxFixDepth          int     `json:"max_fix_depth"`            // 最大修补深度 (区块修补循环次数)
}

// UpdateConfig 更新相关配置
type UpdateConfig struct {
	// GitHub 基础 URL，可改为镜像源，例如 https://github.com 或其他镜像加速
	GitHubBaseURL string `json:"github_base_url"`
}

// UserCenterConfig 用户中心加密客户端配置
type UserCenterConfig struct {
	// 后端根地址（不含 /api）
	BaseRoot string `json:"user_center_base_root"`
	// X-API-Key 值
	APIKey string `json:"user_center_api_key"`
}

// ExtraConfig 扩展配置
type ExtraConfig struct {
	// TanLobbyRoomOperateMode ...
	TanLobbyRoomOperateMode bool `json:"tan_lobby_room_operate_mode"`
}

// Config 应用配置结构
type Config struct {
	Language    string           `json:"language"`     // 用户界面语言
	Version     string           `json:"version"`      // 配置文件版本
	StoragePath string           `json:"storage_path"` // 存储路径
	BuildTask   BuildTaskConfig  `json:"build_task"`   // 构建任务配置
	Update      UpdateConfig     `json:"update"`       // 更新配置
	UserCenter  UserCenterConfig `json:"user_center"`  // 用户中心加密客户端配置
	Extra       ExtraConfig      `json:"extra_config"` // 扩展配置
	mu          sync.RWMutex     `json:"-"`
}

// 全局配置实例
var AppConfig *Config
var configOnce sync.Once

// 配置文件名
const ConfigFileName = "config.json"

// 支持的语言
const (
	LangChinese = "zh"
)

// InitConfig 初始化配置系统
func InitConfig() error {
	var err error
	configOnce.Do(func() {
		AppConfig = &Config{
			Language: LangChinese,
			Version:  constants.Version,
		}
		err = AppConfig.Load()
	})
	return err
}

// GetConfig 获取全局配置实例
func GetConfig() *Config {
	if AppConfig == nil {
		InitConfig()
	}
	return AppConfig
}

// Load 从文件加载配置
func (c *Config) Load() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	configPath := c.getConfigPath()

	// 如果配置文件不存在，创建默认配置
	if !FileExists(configPath) {
		c.setDefaults()
		return c.save()
	}

	// 读取配置文件
	data, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("failed to read config file: %w", err)
	}

	// 解析配置
	if err := json.Unmarshal(data, c); err != nil {
		return fmt.Errorf("failed to parse config file: %w", err)
	}

	// 兼容旧配置：如果未设置更新配置，填充默认值
	if strings.TrimSpace(c.Update.GitHubBaseURL) == "" {
		c.Update.GitHubBaseURL = "https://github.com"
		// 不强制写回磁盘，保持加载时兜底
	}

	// 兼容旧配置：如果未设置用户中心 BaseRoot，填充默认地址
	if strings.TrimSpace(c.UserCenter.BaseRoot) == "" {
		c.UserCenter.BaseRoot = "https://fatalder.yeah114.top"
	}

	return nil
}

// Save 保存配置到文件
func (c *Config) Save() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.save()
}

// save 内部保存方法（不加锁）
func (c *Config) save() error {
	configPath := c.getConfigPath()

	// 确保配置目录存在
	configDir := filepath.Dir(configPath)
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// 序列化配置
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	// 写入文件
	if err := os.WriteFile(configPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// getConfigPath 获取配置文件路径
func (c *Config) getConfigPath() string {
	return filepath.Join(GetStoragePath(), ConfigFileName)
}

// setDefaults 设置默认配置
func (c *Config) setDefaults() {
	c.Language = c.detectSystemLanguage()
	c.Version = constants.Version
	c.StoragePath = GetStoragePath()
	c.BuildTask = BuildTaskConfig{
		MinOnceParseChunkNum: 1,    // 默认单次解析1个区块
		MinMatchingDegree:    60.0, // 默认匹配度60%
		MaxFixDepth:          3,    // 默认最大修补深度3次
	}
	c.Update = UpdateConfig{
		GitHubBaseURL: "https://github.com",
	}
	c.UserCenter = UserCenterConfig{
		BaseRoot: "https://fatalder.yeah114.top",
	}
	c.Extra = ExtraConfig{
		TanLobbyRoomOperateMode: false,
	}
}

// detectSystemLanguage 检测系统语言
func (c *Config) detectSystemLanguage() string {
	// 检查环境变量
	envVars := []string{"LANG", "LC_ALL", "LC_MESSAGES"}

	for _, envVar := range envVars {
		if lang := os.Getenv(envVar); lang != "" {
			if strings.Contains(strings.ToLower(lang), "zh") {
				return LangChinese
			}
		}
	}

	// 默认返回中文
	return LangChinese
}

// GetLanguage 获取当前语言设置
func (c *Config) GetLanguage() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.Language
}

// SetLanguage 设置语言
func (c *Config) SetLanguage(lang string) error {
	c.mu.Lock()
	c.Language = lang
	c.mu.Unlock()
	return c.Save()
}

// GetStoragePathFromConfig 从配置获取存储路径
func (c *Config) GetStoragePathFromConfig() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.StoragePath
}

// SetStoragePathInConfig 在配置中设置存储路径
func (c *Config) SetStoragePathInConfig(path string) error {
	c.mu.Lock()
	c.StoragePath = path
	c.mu.Unlock()
	return c.Save()
}

// GetVersion 获取配置版本
func (c *Config) GetVersion() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.Version
}

// GetBuildTaskConfig 获取构建任务配置
func (c *Config) GetBuildTaskConfig() BuildTaskConfig {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.BuildTask
}

// SetBuildTaskConfig 设置构建任务配置
func (c *Config) SetBuildTaskConfig(config BuildTaskConfig) error {
	c.mu.Lock()
	c.BuildTask = config
	c.mu.Unlock()
	return c.Save()
}

// GetUpdateConfig 获取更新配置
func (c *Config) GetUpdateConfig() UpdateConfig {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.Update
}

// SetUpdateConfig 设置更新配置
func (c *Config) SetUpdateConfig(update UpdateConfig) error {
	c.mu.Lock()
	c.Update = update
	c.mu.Unlock()
	return c.Save()
}

// GetUserCenterConfig 获取用户中心客户端配置
func (c *Config) GetUserCenterConfig() UserCenterConfig {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.UserCenter
}

// SetUserCenterConfig 设置用户中心客户端配置
func (c *Config) SetUserCenterConfig(uc UserCenterConfig) error {
	c.mu.Lock()
	c.UserCenter = uc
	c.mu.Unlock()
	return c.Save()
}

// GetGitHubBaseURL 获取 GitHub 基础 URL（若为空则返回默认值）
func (c *Config) GetGitHubBaseURL() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if strings.TrimSpace(c.Update.GitHubBaseURL) == "" {
		return "https://github.com"
	}
	return c.Update.GitHubBaseURL
}

// SetGitHubBaseURL 设置 GitHub 基础 URL
func (c *Config) SetGitHubBaseURL(url string) error {
	c.mu.Lock()
	c.Update.GitHubBaseURL = url
	c.mu.Unlock()
	return c.Save()
}

// GetExtraConfig 获取扩展配置
func (c *Config) GetExtraConfig() ExtraConfig {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.Extra
}

// SetExtraConfig 设置扩展配置
func (c *Config) SetExtraConfig(extra ExtraConfig) error {
	c.mu.Lock()
	c.Extra = extra
	c.mu.Unlock()
	return c.Save()
}

// 便捷函数
func GetLanguage() string {
	return GetConfig().GetLanguage()
}

func SetLanguage(lang string) error {
	return GetConfig().SetLanguage(lang)
}
