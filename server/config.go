package server

import (
	"encoding/json"
	"os"
	"strings"
	"sync"
)

// Config API 配置管理
type Config struct {
	mu         sync.RWMutex
	filePath   string
	APIKeys    []string `json:"api_keys"` // 支持多个 Key，用逗号分隔
}

type ConfigData struct {
	APIKeys []string `json:"api_keys"`
}

// NewConfig 创建配置管理器
func NewConfig(dataDir string) *Config {
	return &Config{
		filePath: dataDir + "/config.json",
		APIKeys:  []string{"sk-mimo"}, // 默认 API Key
	}
}

// Load 从文件加载配置
func (c *Config) Load() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	data, err := os.ReadFile(c.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			// 文件不存在，使用默认值
			return c.save()
		}
		return err
	}

	var cfg ConfigData
	if err := json.Unmarshal(data, &cfg); err != nil {
		return err
	}

	// 解析逗号分隔的 Key
	if len(cfg.APIKeys) == 1 && strings.Contains(cfg.APIKeys[0], ",") {
		// 兼容旧格式：单个字符串用逗号分隔
		c.APIKeys = strings.Split(cfg.APIKeys[0], ",")
		for i := range c.APIKeys {
			c.APIKeys[i] = strings.TrimSpace(c.APIKeys[i])
		}
	} else {
		c.APIKeys = cfg.APIKeys
	}

	// 确保至少有一个 Key
	if len(c.APIKeys) == 0 {
		c.APIKeys = []string{"sk-mimo"}
	}

	return nil
}

// save 保存配置到文件
func (c *Config) save() error {
	data := ConfigData{
		APIKeys: c.APIKeys,
	}
	b, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(c.filePath, b, 0644)
}

// GetAPIKeys 获取所有有效的 API Key
func (c *Config) GetAPIKeys() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	keys := make([]string, len(c.APIKeys))
	copy(keys, c.APIKeys)
	return keys
}

// SetAPIKeys 设置 API Keys
func (c *Config) SetAPIKeys(keys []string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// 过滤空值并去重
	seen := make(map[string]bool)
	var validKeys []string
	for _, k := range keys {
		k = strings.TrimSpace(k)
		if k == "" || seen[k] {
			continue
		}
		seen[k] = true
		validKeys = append(validKeys, k)
	}

	if len(validKeys) == 0 {
		validKeys = []string{"sk-mimo"}
	}

	c.APIKeys = validKeys
	return c.save()
}

// ValidateAPIKey 验证 API Key 是否有效
func (c *Config) ValidateAPIKey(key string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	key = strings.TrimSpace(key)
	for _, k := range c.APIKeys {
		if k == key {
			return true
		}
	}
	return false
}
