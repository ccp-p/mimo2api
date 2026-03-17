package account

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Account 代表一个小米账号的认证信息
type Account struct {
	Name     string    `json:"name"`
	Cookie   string    `json:"cookie"` // serviceToken=xxx; userId=xxx 格式的完整Cookie
	Ph       string    `json:"ph"`     // xiaomichatbot_ph 参数
	Valid    bool      `json:"valid"`
	LastCheck time.Time `json:"-"`
}

// Manager 多账号管理器
type Manager struct {
	mu       sync.RWMutex
	accounts []*Account
	current  int
	dataDir  string
}

// NewManager 创建账号管理器，dataDir 为存放账号 JSON 文件的目录
func NewManager(dataDir string) (*Manager, error) {
	m := &Manager{
		dataDir: dataDir,
	}
	if err := m.Load(); err != nil {
		return nil, err
	}
	return m, nil
}

// Load 从 dataDir 读取所有账号配置
func (m *Manager) Load() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if err := os.MkdirAll(m.dataDir, 0755); err != nil {
		return fmt.Errorf("create data dir: %w", err)
	}

	entries, err := os.ReadDir(m.dataDir)
	if err != nil {
		return fmt.Errorf("read data dir: %w", err)
	}

	var accounts []*Account
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(m.dataDir, e.Name()))
		if err != nil {
			continue
		}
		var acc Account
		if err := json.Unmarshal(data, &acc); err != nil {
			continue
		}
		acc.Valid = true
		accounts = append(accounts, &acc)
	}
	m.accounts = accounts
	m.current = 0
	return nil
}

// Save 保存账号信息到 dataDir
func (m *Manager) Save(acc *Account) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	name := acc.Name
	if name == "" {
		name = fmt.Sprintf("account_%d", time.Now().UnixNano())
		acc.Name = name
	}
	data, err := json.MarshalIndent(acc, "", "  ")
	if err != nil {
		return err
	}
	filename := filepath.Join(m.dataDir, name+".json")
	return os.WriteFile(filename, data, 0600)
}

// Add 动态添加一个账号
func (m *Manager) Add(acc *Account) error {
	if err := m.Save(acc); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	acc.Valid = true
	m.accounts = append(m.accounts, acc)
	return nil
}

// Delete 删除指定名称的账号
func (m *Manager) Delete(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	filename := filepath.Join(m.dataDir, name+".json")
	_ = os.Remove(filename)

	newAccounts := m.accounts[:0]
	for _, a := range m.accounts {
		if a.Name != name {
			newAccounts = append(newAccounts, a)
		}
	}
	m.accounts = newAccounts
	return nil
}

// Next 轮询获取下一个可用账号
func (m *Manager) Next() (*Account, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if len(m.accounts) == 0 {
		return nil, fmt.Errorf("no accounts configured")
	}

	total := len(m.accounts)
	for i := 0; i < total; i++ {
		idx := (m.current + i) % total
		acc := m.accounts[idx]
		if acc.Valid {
			m.mu.RUnlock()
			m.mu.Lock()
			m.current = (idx + 1) % total
			m.mu.Unlock()
			m.mu.RLock()
			return acc, nil
		}
	}
	return nil, fmt.Errorf("all accounts are invalid")
}

// MarkInvalid 将账号标记为不可用
func (m *Manager) MarkInvalid(acc *Account) {
	m.mu.Lock()
	defer m.mu.Unlock()
	acc.Valid = false
}

// MarkValid 将账号标记为可用
func (m *Manager) MarkValid(acc *Account) {
	m.mu.Lock()
	defer m.mu.Unlock()
	acc.Valid = true
}

// ListAll 返回所有账号的内部指针（慎用）
func (m *Manager) ListAll() []*Account {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.accounts
}

// List 返回所有账号的副本（不含敏感信息用于展示）
func (m *Manager) List() []AccountInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []AccountInfo
	for _, a := range m.accounts {
		result = append(result, AccountInfo{
			Name:  a.Name,
			Valid: a.Valid,
		})
	}
	return result
}

// AccountInfo 用于展示的账号信息（不含凭证）
type AccountInfo struct {
	Name  string `json:"name"`
	Valid bool   `json:"valid"`
}

// ParseCookie 从完整 Cookie 字符串中提取 serviceToken 和 userId
func ParseCookieFields(cookie string) (serviceToken, userId string) {
	// 格式：serviceToken=xxx; userId=xxx; ...
	for _, part := range splitKV(cookie, ';') {
		k, v := parseKV(part, '=')
		switch k {
		case "serviceToken":
			serviceToken = v
		case "userId":
			userId = v
		}
	}
	return
}

func splitKV(s string, sep rune) []string {
	var parts []string
	start := 0
	for i, c := range s {
		if c == sep {
			parts = append(parts, s[start:i])
			start = i + 1
		}
	}
	parts = append(parts, s[start:])
	return parts
}

func parseKV(s string, sep rune) (string, string) {
	for i, c := range s {
		if c == sep {
			k := trimSpace(s[:i])
			v := trimSpace(s[i+1:])
			return k, v
		}
	}
	return trimSpace(s), ""
}

func trimSpace(s string) string {
	start := 0
	end := len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t') {
		end--
	}
	return s[start:end]
}
