package config

import (
	"fmt"
	"io/ioutil"
	"encoding/json"
)

type Config struct {
	AuthServerAddress    string `json:"auth_server_address"`
	AuthServerToken      string `json:"auth_server_token"`
	RentalServerCode     string `json:"rental_server_code"`
	RentalServerPasscode string `json:"rental_server_passcode"`
	ListenAddress        string `json:"listen_address"`
	QQ                   string `json:"qq"`
}

// 加载或创建配置
func LoadConfig(path string) (*Config, error) {
	cfg := &Config{}

	// 尝试读取现有配置文件
	if data, err := ioutil.ReadFile(path); err == nil {
		if err := json.Unmarshal(data, cfg); err != nil {
			return nil, fmt.Errorf("解析配置文件失败: %v", err)
		}
	}

	// 检查并补全缺失的配置项
	cfg.completeMissingConfig()

	// 保存更新后的配置
	if err := cfg.Save(path); err != nil {
		return nil, err
	}

	return cfg, nil
}

// 补全缺失的配置项
func (c *Config) completeMissingConfig() {
	if c.AuthServerAddress == "" {
		fmt.Print("请输入验证服务器地址: ")
		fmt.Scanln(&c.AuthServerAddress)
	}

	if c.AuthServerToken == "" {
		fmt.Print("请输入验证服务器Token: ")
		fmt.Scanln(&c.AuthServerToken)
	}

	if c.RentalServerCode == "" {
		fmt.Print("请输入租赁服号码: ")
		fmt.Scanln(&c.RentalServerCode)
		if c.RentalServerCode == "" {
			c.RentalServerCode = "000000"
		}
	}

	if c.RentalServerPasscode == "" {
		fmt.Print("请输入租赁服密码: ")
		fmt.Scanln(&c.RentalServerPasscode)
	}

	if c.QQ == "" {
		fmt.Print("请输入你的QQ号: ")
		fmt.Scanln(&c.QQ)
	}

	if c.ListenAddress == "" {
		fmt.Print("请输入国际基岩版服务器监听地址: ")
		fmt.Scanln(&c.ListenAddress)
		if c.ListenAddress == "" {
			c.ListenAddress = "127.0.0.1:19132"
		}
	}
}

// 保存配置到文件
func (c *Config) Save(path string) error {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("JSON编码失败: %v", err)
	}

	if err := ioutil.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("写入配置文件失败: %v", err)
	}

	return nil
}

var ShuttlerConfig *Config

func init() {
	cfg, err := LoadConfig("config.json")
	if err != nil {
		panic(err)
	}
	ShuttlerConfig = cfg
}