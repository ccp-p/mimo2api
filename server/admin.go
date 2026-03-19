package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime"
	"mimo2api/account"
	"mimo2api/mimo"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
	"time"
)

// AdminHandler 管理界面处理器
type AdminHandler struct {
	mgr    *account.Manager
	client *mimo.Client
	cfg    *Config // API 配置

	// 登录代理：临时存储本次登录抓到的 Cookie
	loginMu           sync.Mutex
	capturedCookies   map[string]string // requestID -> cookie string
}

// NewAdminHandler 创建管理界面处理器
func NewAdminHandler(mgr *account.Manager, client *mimo.Client, cfg *Config) *AdminHandler {
	return &AdminHandler{
		mgr:             mgr,
		client:          client,
		cfg:             cfg,
		capturedCookies: make(map[string]string),
	}
}

// Register 注册管理路由
func (a *AdminHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("/admin/accounts", a.accounts)
	mux.HandleFunc("/admin/accounts/add", a.addAccount)
	mux.HandleFunc("/admin/accounts/delete", a.deleteAccount)
	mux.HandleFunc("/admin/accounts/test", a.testAccount)
	mux.HandleFunc("/admin/parse-curl", a.parseCurl)
	mux.HandleFunc("/admin/reload", a.reload)
	mux.HandleFunc("/admin/check-auth", a.checkAuth)
	// API Key 管理
	mux.HandleFunc("/admin/config/api-keys", a.getAPIKeys)
	mux.HandleFunc("/admin/config/api-keys/set", a.setAPIKeys)
	// 调试聊天接口（SSE 流式）
	mux.HandleFunc("/admin/debug/chat", a.debugChat)
	// 登录代理：将小米登录页面内嵌到管理面板，自动抓取 Cookie
	mux.HandleFunc("/admin/login/", a.loginProxy)
	mux.HandleFunc("/admin/login-result", a.loginResult)
}

// ——————————————————————————————————————————
// GET /admin/accounts — 列出所有账号
// ——————————————————————————————————————————

func (a *AdminHandler) accounts(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"accounts": a.mgr.List(),
	})
}

// ——————————————————————————————————————————
// POST /admin/accounts/add
// Body: {"name":"xxx","cookie":"serviceToken=...;userId=...","ph":"..."}
// ——————————————————————————————————————————

func (a *AdminHandler) addAccount(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Name   string `json:"name"`
		Cookie string `json:"cookie"`
		Ph     string `json:"ph"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "bad request: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.Cookie == "" {
		jsonError(w, "cookie is required", http.StatusBadRequest)
		return
	}

	// ph 为空时尝试从 Cookie 字符串中自动提取
	if req.Ph == "" {
		// 直接从 cookie 键值对里找
		for _, part := range strings.Split(req.Cookie, ";") {
			part = strings.TrimSpace(part)
			if strings.HasPrefix(part, "xiaomichatbot_ph=") {
				req.Ph = strings.Trim(strings.TrimPrefix(part, "xiaomichatbot_ph="), "\"'")
				break
			}
		}
	}

	// 无论来源如何，统一去掉 ph 两端的引号
	req.Ph = strings.Trim(req.Ph, "\"'")

	if req.Ph == "" {
		jsonError(w, "xiaomichatbot_ph not found in cookie, please fill it manually", http.StatusBadRequest)
		return
	}

	acc := &account.Account{
		Name:   req.Name,
		Cookie: req.Cookie,
		Ph:     req.Ph,
	}
	if err := a.mgr.Add(acc); err != nil {
		jsonError(w, "save account: "+err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "name": acc.Name})
}

// ——————————————————————————————————————————
// POST /admin/accounts/delete
// Body: {"name":"xxx"}
// ——————————————————————————————————————————

func (a *AdminHandler) deleteAccount(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "bad request: "+err.Error(), http.StatusBadRequest)
		return
	}
	_ = a.mgr.Delete(req.Name)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// ——————————————————————————————————————————
// POST /admin/accounts/test
// Body: {"name":"xxx"} — 测试账号是否有效
// ——————————————————————————————————————————

func (a *AdminHandler) testAccount(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "bad request: "+err.Error(), http.StatusBadRequest)
		return
	}

	// 临时使用该账号做一个极短的测试请求
	acc, err := findAccount(a.mgr, req.Name)
	if err != nil {
		jsonError(w, err.Error(), http.StatusNotFound)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	testReq := mimo.BuildChatRequest([]mimo.Message{
		{Role: "user", Content: "hi"},
	}, "mimo-v2-flash", false, false)

	_, content, _, err := a.client.CallSync(ctx, acc, testReq)
	if err != nil || content == "" {
		a.mgr.MarkInvalid(acc)
		writeJSON(w, http.StatusOK, map[string]any{"valid": false, "error": fmt.Sprintf("%v", err)})
		return
	}

	a.mgr.MarkValid(acc)
	writeJSON(w, http.StatusOK, map[string]any{"valid": true})
}

// ——————————————————————————————————————————
// POST /admin/parse-curl
// Body: {"curl":"curl 'https://...' -H 'Cookie: ...' ..."}
// 从 cURL 命令中提取 Cookie 和 ph 参数
// ——————————————————————————————————————————

func (a *AdminHandler) parseCurl(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// 支持 JSON 和 plain text
	ct, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
	var curlCmd string

	if ct == "application/json" {
		var req struct {
			Curl string `json:"curl"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			jsonError(w, "bad request", http.StatusBadRequest)
			return
		}
		curlCmd = req.Curl
	} else {
		var sb strings.Builder
		buf := make([]byte, 4096)
		for {
			n, err := r.Body.Read(buf)
			sb.Write(buf[:n])
			if err != nil {
				break
			}
		}
		curlCmd = sb.String()
	}

	cookie, ph := extractFromCurl(curlCmd)
	if cookie == "" {
		jsonError(w, "could not extract cookie from curl command", http.StatusBadRequest)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"cookie": cookie,
		"ph":     ph,
	})
}

// ——————————————————————————————————————————
// POST /admin/reload — 重新加载 data 目录中的账号
// ——————————————————————————————————————————

func (a *AdminHandler) reload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := a.mgr.Load(); err != nil {
		jsonError(w, "reload failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "accounts": a.mgr.List()})
}

// ——————————————————————————————————————————
// 工具函数
// ——————————————————————————————————————————

// findAccount 根据名称找到账号（包括已标记为 invalid 的账号）
func findAccount(mgr *account.Manager, name string) (*account.Account, error) {
	for _, acc := range mgr.ListAll() {
		if acc.Name == name {
			return acc, nil
		}
	}
	return nil, fmt.Errorf("account %q not found", name)
}

// extractFromCurl 从 cURL 命令字符串中提取 Cookie 和 xiaomichatbot_ph
func extractFromCurl(cmd string) (cookie, ph string) {
	// 查找 -H 'Cookie: ...' 或 --cookie '...' 模式
	lines := strings.Split(cmd, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		line = strings.TrimSuffix(line, "\\")
		line = strings.TrimSpace(line)
		lower := strings.ToLower(line)

		// 情况 1: -H 'cookie: ...'
		if strings.Contains(lower, "cookie:") {
			idx := strings.Index(lower, "cookie:")
			raw := line[idx+7:]
			raw = strings.Trim(raw, " '\"`")
			// 去掉末尾可能残留的单引号或双引号（cURL 复制时常见的）
			raw = strings.TrimRight(raw, "'\"`")
			cookie = raw
		}
		// 情况 2: -b '...' 或 --cookie '...'
		if strings.Contains(lower, "-b ") || strings.Contains(lower, "--cookie ") {
			parts := strings.SplitN(line, " ", 2)
			if len(parts) > 1 {
				val := strings.TrimSpace(parts[1])
				val = strings.Trim(val, " '\"`")
				cookie = val
			}
		}
	}

	// 提取 ph: 优先从 URL 参数中找，因为 Cookie 里的经常被截断或转义
	// 查找 xiaomichatbot_ph=XXX
	if ph == "" {
		re := []string{"xiaomichatbot_ph=", "xiaomichatbot_ph%3D"}
		for _, prefix := range re {
			if idx := strings.Index(cmd, prefix); idx != -1 {
				val := cmd[idx+len(prefix):]
				// 找到结束符：空格、引号、分号、&、反斜杠
				endIdx := strings.IndexAny(val, " '\";&\\")
				if endIdx != -1 {
					ph = val[:endIdx]
				} else {
					ph = val
				}
				break
			}
		}
	}

	// 如果 URL 没找到，再从 Cookie 找
	if ph == "" && cookie != "" {
		for _, part := range strings.Split(cookie, ";") {
			part = strings.TrimSpace(part)
			if strings.HasPrefix(part, "xiaomichatbot_ph=") {
				ph = strings.TrimPrefix(part, "xiaomichatbot_ph=")
				ph = strings.Trim(ph, "\"'")
				break
			}
		}
	}

	// 最终清理：去掉可能存在的 URL 编码
	if strings.Contains(ph, "%") {
		if decoded, err := url.QueryUnescape(ph); err == nil {
			ph = decoded
		}
	}

	return
}

// ——————————————————————————————————————————
// GET/POST /admin/login/* — 代理小米登录页，自动抓 Cookie
// ——————————————————————————————————————————

const mimoOrigin = "https://aistudio.xiaomimimo.com"

// loginProxy 将 /admin/login/* 的请求代理到小米 aistudio，
// 并监听响应头中的 Set-Cookie，当检测到有效 Cookie 时保存。
func (a *AdminHandler) loginProxy(w http.ResponseWriter, r *http.Request) {
	target, err := url.Parse(mimoOrigin)
	if err != nil {
		http.Error(w, "proxy target error", http.StatusInternalServerError)
		return
	}

	// 去掉 /admin/login 前缀，转发到目标路径
	// /admin/login/xxx -> /xxx
	path := strings.TrimPrefix(r.URL.Path, "/admin/login")
	if path == "" {
		path = "/"
	}

	proxy := &httputil.ReverseProxy{
		Director: func(req *http.Request) {
			req.URL.Scheme = target.Scheme
			req.URL.Host = target.Host
			req.URL.Path = path
			req.Host = target.Host
			// 移除本地鉴权头，避免干扰上游
			req.Header.Del("Authorization")
		},
		ModifyResponse: func(resp *http.Response) error {
			// 拦截 Set-Cookie，提取关键字段
			cookies := resp.Cookies()
			if len(cookies) > 0 {
				var cookieParts []string
				var ph string
				for _, c := range cookies {
					cookieParts = append(cookieParts, c.Name+"="+c.Value)
					if c.Name == "xiaomichatbot_ph" {
						ph = c.Value
					}
				}
				if len(cookieParts) > 0 {
					cookieStr := strings.Join(cookieParts, "; ")
					a.loginMu.Lock()
					a.capturedCookies["latest"] = cookieStr
					if ph != "" {
						a.capturedCookies["latest_ph"] = ph
					}
					a.loginMu.Unlock()
					log.Printf("[LoginProxy] 捕获到 Cookie（%d 个字段），ph=%s", len(cookieParts), ph)
				}
			}

			// 将重定向地址改写到本地代理路径
			if loc := resp.Header.Get("Location"); loc != "" {
				if strings.HasPrefix(loc, mimoOrigin) {
					newLoc := "/admin/login" + strings.TrimPrefix(loc, mimoOrigin)
					resp.Header.Set("Location", newLoc)
				}
			}
			return nil
		},
	}

	proxy.ServeHTTP(w, r)
}

// GET /admin/login-result — 前端轮询，获取登录后捕获的 Cookie
func (a *AdminHandler) loginResult(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	a.loginMu.Lock()
	cookie := a.capturedCookies["latest"]
	ph := a.capturedCookies["latest_ph"]
	// 读完后清空，避免重复消费
	if cookie != "" {
		delete(a.capturedCookies, "latest")
		delete(a.capturedCookies, "latest_ph")
	}
	a.loginMu.Unlock()

	writeJSON(w, http.StatusOK, map[string]any{
		"cookie": cookie,
		"ph":     ph,
		"ready":  cookie != "",
	})
}

// ——————————————————————————————————————————
// POST /admin/check-auth — 立即检查所有账号鉴权状态
// ——————————————————————————————————————————

func (a *AdminHandler) checkAuth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	results := a.runAuthCheck()
	writeJSON(w, http.StatusOK, map[string]any{
		"results": results,
	})
}

// runAuthCheck 对所有账号执行鉴权检查，返回每个账号的状态
func (a *AdminHandler) runAuthCheck() []map[string]any {
	list := a.mgr.ListAll()
	var results []map[string]any

	for _, acc := range list {
		valid := a.client.CheckAuth(acc)
		if valid {
			a.mgr.MarkValid(acc)
		} else {
			a.mgr.MarkInvalid(acc)
			log.Printf("[AuthCheck] 账号 %s 鉴权失效", acc.Name)
		}
		results = append(results, map[string]any{
			"name":  acc.Name,
			"valid": valid,
		})
	}
	return results
}

// StartAuthChecker 启动定期鉴权检查协程
// interval: 检查间隔（秒），0 或负数使用默认值 300s
func (a *AdminHandler) StartAuthChecker(intervalSec int) {
	if intervalSec <= 0 {
		intervalSec = 300
	}
	go func() {
		ticker := time.NewTicker(time.Duration(intervalSec) * time.Second)
		defer ticker.Stop()

		// 启动时立即检查一次
		log.Printf("[AuthCheck] 首次鉴权检查开始...")
		a.runAuthCheck()
		log.Printf("[AuthCheck] 首次鉴权检查完成")

		for range ticker.C {
			log.Printf("[AuthCheck] 定期鉴权检查开始...")
			results := a.runAuthCheck()
			valid := 0
			for _, r := range results {
				if r["valid"].(bool) {
					valid++
				}
			}
			log.Printf("[AuthCheck] 完成：%d/%d 账号有效", valid, len(results))
		}
	}()
}

// ——————————————————————————————————————————
// GET /admin/config/api-keys — 获取 API Keys
// ——————————————————————————————————————————

func (a *AdminHandler) getAPIKeys(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"api_keys": a.cfg.GetAPIKeys(),
	})
}

// POST /admin/config/api-keys/set — 设置 API Keys
// Body: {"keys":"sk-key1,sk-key2,sk-key3"} 或 {"keys":["sk-key1","sk-key2"]}
// ——————————————————————————————————————————

func (a *AdminHandler) setAPIKeys(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Keys interface{} `json:"keys"` // 支持字符串或数组
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "bad request: "+err.Error(), http.StatusBadRequest)
		return
	}

	var keys []string
	switch v := req.Keys.(type) {
	case string:
		// 逗号分隔的字符串
		keys = strings.Split(v, ",")
		for i := range keys {
			keys[i] = strings.TrimSpace(keys[i])
		}
	case []interface{}:
		// 数组格式
		for _, item := range v {
			if s, ok := item.(string); ok {
				keys = append(keys, strings.TrimSpace(s))
			}
		}
	}

	if err := a.cfg.SetAPIKeys(keys); err != nil {
		jsonError(w, "save failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"api_keys": a.cfg.GetAPIKeys(),
	})
}

// ——————————————————————————————————————————
// POST /admin/debug/chat — 调试对话（SSE 流式）
// Body: {"account":"xxx","model":"mimo-v2-flash-studio","message":"你好"}
// ——————————————————————————————————————————

func (a *AdminHandler) debugChat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Account string `json:"account"` // 账号名，空则自动选
		Model   string `json:"model"`
		Message string `json:"message"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "bad request: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.Message == "" {
		jsonError(w, "message is required", http.StatusBadRequest)
		return
	}
	if req.Model == "" {
		req.Model = "mimo-v2-flash-studio"
	}

	// 选择账号
	var selectedAcc *account.Account
	var err error
	if req.Account != "" {
		selectedAcc, err = findAccount(a.mgr, req.Account)
		if err != nil {
			jsonError(w, err.Error(), http.StatusNotFound)
			return
		}
	} else {
		selectedAcc, err = a.mgr.Next()
		if err != nil {
			jsonError(w, "no available accounts: "+err.Error(), http.StatusServiceUnavailable)
			return
		}
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		jsonError(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	enableThinking, enableSearch := mimo.ParseModelFlags(req.Model)
	mimoReq := mimo.BuildChatRequest([]mimo.Message{
		{Role: "user", Content: req.Message},
	}, req.Model, enableThinking, enableSearch)

	ch, err := a.client.Stream(r.Context(), selectedAcc, mimoReq)
	if err != nil {
		data, _ := json.Marshal(map[string]string{"type": "error", "content": err.Error()})
		fmt.Fprintf(w, "data: %s\n\n", data)
		flusher.Flush()
		return
	}

	for result := range ch {
		if result.Err != nil {
			data, _ := json.Marshal(map[string]string{"type": "error", "content": result.Err.Error()})
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
			return
		}
		if result.ThinkingContent != "" {
			data, _ := json.Marshal(map[string]string{"type": "thinking", "content": result.ThinkingContent})
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		}
		if result.Content != "" {
			data, _ := json.Marshal(map[string]string{"type": "text", "content": result.Content})
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		}
		if result.FinishReason != "" {
			var finData map[string]any
			if result.Usage != nil {
				finData = map[string]any{
					"type":             "finish",
					"finishReason":     result.FinishReason,
					"promptTokens":     result.Usage.PromptTokens,
					"completionTokens": result.Usage.CompletionTokens,
					"totalTokens":      result.Usage.TotalTokens,
				}
			} else {
				finData = map[string]any{"type": "finish", "finishReason": result.FinishReason}
			}
			data, _ := json.Marshal(finData)
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		}
	}

	fmt.Fprintf(w, "data: [DONE]\n\n")
	flusher.Flush()
}

// ——————————————————————————————————————————
// 内部辅助：读取 Body（避免重复）
// ——————————————————————————————————————————

func readBody(r *http.Request) ([]byte, error) {
	defer r.Body.Close()
	return io.ReadAll(r.Body)
}
