package server

import (
	"encoding/json"
	"fmt"
	"mimo2api/account"
	"mimo2api/mimo"
	"net/http"
	"strings"
	"time"
)

// ——————————————————————————————————————————
// OpenAI 兼容数据结构
// ——————————————————————————————————————————

// ChatCompletionRequest OpenAI 格式的请求
type ChatCompletionRequest struct {
	Model    string         `json:"model"`
	Messages []mimo.Message `json:"messages"`
	Stream   bool           `json:"stream"`
	// 以下字段兼容接收但不强制使用
	Temperature *float64 `json:"temperature,omitempty"`
	MaxTokens   *int     `json:"max_tokens,omitempty"`
	// 直接传入 cookies 调用（绕过 API Key 鉴权）
	Cookie  string `json:"cookie,omitempty"`
	Ph      string `json:"ph,omitempty"`
	Account string `json:"account,omitempty"` // 指定使用哪个已配置的账号
}

// ChatCompletionResponse 非流式响应
type ChatCompletionResponse struct {
	ID      string   `json:"id"`
	Object  string   `json:"object"`
	Created int64    `json:"created"`
	Model   string   `json:"model"`
	Choices []Choice `json:"choices"`
	Usage   UsageObj `json:"usage"`
}

// Choice 单个选项
type Choice struct {
	Index        int         `json:"index"`
	Message      *RespMsg    `json:"message,omitempty"`
	Delta        *DeltaMsg   `json:"delta,omitempty"`
	FinishReason *string     `json:"finish_reason"`
	Logprobs     interface{} `json:"logprobs"`
}

// RespMsg 非流式消息
type RespMsg struct {
	Role             string `json:"role"`
	Content          string `json:"content"`
	ReasoningContent string `json:"reasoning_content,omitempty"`
}

// DeltaMsg 流式增量消息
type DeltaMsg struct {
	Role             string `json:"role,omitempty"`
	Content          string `json:"content,omitempty"`
	ReasoningContent string `json:"reasoning_content,omitempty"`
}

// UsageObj token 用量
type UsageObj struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// ModelsResponse /v1/models 响应
type ModelsResponse struct {
	Object string        `json:"object"`
	Data   []ModelObject `json:"data"`
}

// ModelObject 单个模型对象
type ModelObject struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	OwnedBy string `json:"owned_by"`
}

// SupportedModels 对外暴露的模型列表
var SupportedModels = []ModelObject{
	{ID: "mimo-v2-flash-studio", Object: "model", Created: 1700000000, OwnedBy: "xiaomi"},
	{ID: "mimo-v2-flash-studio-thinking", Object: "model", Created: 1700000000, OwnedBy: "xiaomi"},
	{ID: "mimo-v2-flash-studio-search", Object: "model", Created: 1700000000, OwnedBy: "xiaomi"},
	{ID: "mimo-v2-flash-studio-thinking-search", Object: "model", Created: 1700000000, OwnedBy: "xiaomi"},
}

// ——————————————————————————————————————————
// Handler
// ——————————————————————————————————————————

// Handler 处理 OpenAI 兼容 API 请求
type Handler struct {
	mgr    *account.Manager
	client *mimo.Client
	cfg    *Config // API 配置，包含多个 API Keys
}

// NewHandler 创建处理器
func NewHandler(mgr *account.Manager, client *mimo.Client, cfg *Config) *Handler {
	return &Handler{mgr: mgr, client: client, cfg: cfg}
}

// Register 注册路由到 ServeMux
func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("/v1/chat/completions", h.authMiddleware(h.chatCompletions))
	mux.HandleFunc("/v1/models", h.authMiddleware(h.models))
	// Anthropic 风格兼容端点（Claude Code 有时会使用）
	mux.HandleFunc("/v1/messages", h.authMiddleware(h.chatCompletions))
}

// authMiddleware API Key 鉴权中间件
// 如果传入了 cookie/ph 或 account 使用已配置的账号，则跳过 API Key 鉴权
func (h *Handler) authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// 直接放行所有请求，让 chatCompletions 处理账号选择
		// API Key 验证作为可选项
		next(w, r)
	}
}

// ——————————————————————————————————————————
// /v1/models
// ——————————————————————————————————————————

func (h *Handler) models(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, ModelsResponse{
		Object: "list",
		Data:   SupportedModels,
	})
}

// ——————————————————————————————————————————
// /v1/chat/completions
// ——————————————————————————————————————————

func (h *Handler) chatCompletions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req ChatCompletionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, fmt.Sprintf("bad request: %v", err), http.StatusBadRequest)
		return
	}

	var acc *account.Account

	// 如果传入了 cookie/ph，直接用传入的凭证
	if req.Cookie != "" || req.Ph != "" {
		cookie := req.Cookie
		ph := req.Ph

		// 如果没传 ph，尝试从 cookie 中提取
		if ph == "" {
			for _, part := range strings.Split(cookie, ";") {
				part = strings.TrimSpace(part)
				if strings.HasPrefix(part, "xiaomichatbot_ph=") {
					ph = strings.Trim(strings.TrimPrefix(part, "xiaomichatbot_ph="), "\"'")
					break
				}
			}
		}

		if ph == "" {
			jsonError(w, "ph is required", http.StatusBadRequest)
			return
		}

		acc = &account.Account{
			Name:   "remote",
			Cookie: cookie,
			Ph:     strings.Trim(ph, "\"'"),
		}
	} else if req.Account != "" {
		// 指定了账号名，使用已配置的账号
		var err error
		acc, err = findAccount(h.mgr, req.Account)
		if err != nil {
			jsonError(w, "account not found: "+err.Error(), http.StatusNotFound)
			return
		}
	} else {
		// 否则自动选择管理的账号（轮询）
		var err error
		acc, err = h.mgr.Next()
		if err != nil {
			jsonError(w, "no available accounts: "+err.Error(), http.StatusServiceUnavailable)
			return
		}
	}

	enableThinking, enableSearch := mimo.ParseModelFlags(req.Model)
	mimoReq := mimo.BuildChatRequest(req.Messages, req.Model, enableThinking, enableSearch)

	if req.Stream {
		h.handleStream(w, r, acc, mimoReq, req.Model)
	} else {
		h.handleSync(w, r, acc, mimoReq, req.Model)
	}
}

// ——————————————————————————————————————————
// 非流式处理
// ——————————————————————————————————————————

func (h *Handler) handleSync(w http.ResponseWriter, r *http.Request, acc *account.Account, mimoReq mimo.ChatRequest, model string) {
	thinking, content, usage, err := h.client.CallSync(r.Context(), acc, mimoReq)
	if err != nil {
		h.mgr.MarkInvalid(acc)
		jsonError(w, "upstream error: "+err.Error(), http.StatusBadGateway)
		return
	}

	// 将思考内容合并到 reasoning_content 字段
	stop := "stop"
	resp := ChatCompletionResponse{
		ID:      "chatcmpl-" + mimoReq.MsgID[:8],
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   model,
		Choices: []Choice{
			{
				Index: 0,
				Message: &RespMsg{
					Role:             "assistant",
					Content:          content,
					ReasoningContent: thinking,
				},
				FinishReason: &stop,
			},
		},
	}
	if usage != nil {
		resp.Usage = UsageObj{
			PromptTokens:     usage.PromptTokens,
			CompletionTokens: usage.CompletionTokens,
			TotalTokens:      usage.TotalTokens,
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

// ——————————————————————————————————————————
// 流式处理（SSE）
// ——————————————————————————————————————————

func (h *Handler) handleStream(w http.ResponseWriter, r *http.Request, acc *account.Account, mimoReq mimo.ChatRequest, model string) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		jsonError(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	ch, err := h.client.Stream(r.Context(), acc, mimoReq)
	if err != nil {
		h.mgr.MarkInvalid(acc)
		// 已设置了 SSE header，用 SSE 格式返回错误
		writeSSEError(w, flusher, err.Error())
		return
	}

	respID := "chatcmpl-" + mimoReq.MsgID[:8]
	created := time.Now().Unix()

	// 发送 role delta
	sendSSEChunk(w, flusher, respID, model, created, DeltaMsg{Role: "assistant"}, nil, nil)

	var hasThinking bool
	var hasContent bool

	for result := range ch {
		if result.Err != nil {
			writeSSEError(w, flusher, result.Err.Error())
			return
		}

		if result.ThinkingContent != "" {
			if !hasThinking {
				hasThinking = true
			}
			sendSSEChunk(w, flusher, respID, model, created,
				DeltaMsg{ReasoningContent: result.ThinkingContent}, nil, nil)
		}

		if result.Content != "" {
			if !hasContent && hasThinking {
				// 思考阶段结束，发送一个空的 content 标记切换
			}
			hasContent = true
			sendSSEChunk(w, flusher, respID, model, created,
				DeltaMsg{Content: result.Content}, nil, nil)
		}

		if result.FinishReason != "" {
			fr := result.FinishReason
			if fr == "STOP" || fr == "" {
				fr = "stop"
			}
			sendSSEChunk(w, flusher, respID, model, created, DeltaMsg{}, &fr, result.Usage)
		}
	}

	// 确保发送 [DONE]
	fmt.Fprintf(w, "data: [DONE]\n\n")
	flusher.Flush()
}

// sendSSEChunk 发送一个 SSE chunk
func sendSSEChunk(w http.ResponseWriter, f http.Flusher,
	id, model string, created int64,
	delta DeltaMsg, finishReason *string, usage *mimo.Usage) {

	chunk := map[string]any{
		"id":      id,
		"object":  "chat.completion.chunk",
		"created": created,
		"model":   model,
		"choices": []map[string]any{
			{
				"index":         0,
				"delta":         delta,
				"finish_reason": finishReason,
				"logprobs":      nil,
			},
		},
	}
	if usage != nil {
		chunk["usage"] = UsageObj{
			PromptTokens:     usage.PromptTokens,
			CompletionTokens: usage.CompletionTokens,
			TotalTokens:      usage.TotalTokens,
		}
	}

	data, _ := json.Marshal(chunk)
	fmt.Fprintf(w, "data: %s\n\n", data)
	f.Flush()
}

func writeSSEError(w http.ResponseWriter, f http.Flusher, msg string) {
	errChunk := map[string]any{
		"error": map[string]string{"message": msg, "type": "upstream_error"},
	}
	data, _ := json.Marshal(errChunk)
	fmt.Fprintf(w, "data: %s\n\n", data)
	fmt.Fprintf(w, "data: [DONE]\n\n")
	f.Flush()
}

// ——————————————————————————————————————————
// 工具函数
// ——————————————————————————————————————————

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func jsonError(w http.ResponseWriter, msg string, status int) {
	writeJSON(w, status, map[string]any{
		"error": map[string]string{
			"message": msg,
			"type":    "api_error",
		},
	})
}
