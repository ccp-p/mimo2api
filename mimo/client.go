package mimo

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mimo2api/account"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	BaseURL    = "https://aistudio.xiaomimimo.com"
	ChatAPI    = "/open-apis/bot/chat"
	AuthAPI    = "/open-apis/user/mi/get"
	PhParam    = "xiaomichatbot_ph"
	Timeout    = 120 * time.Second
)

// ChatAPIURL 带 ph 参数的聊天接口 URL
func ChatAPIURL(ph string) string {
	// 防御：去掉 ph 两端可能的引号（历史数据兼容）
	ph = strings.Trim(ph, "\"'")
	return fmt.Sprintf("%s%s?%s=%s", BaseURL, ChatAPI, PhParam, ph)
}

// Client 是与 aistudio.xiaomimimo.com 通信的客户端
type Client struct {
	httpClient *http.Client
}

// NewClient 创建一个新的 Mimo 客户端
func NewClient() *Client {
	return &Client{
		httpClient: &http.Client{
			Timeout: Timeout,
		},
	}
}

// ——————————————————————————————————————————
// 请求 / 响应结构体
// ——————————————————————————————————————————

// ChatRequest 向 aistudio 发送的请求体
type ChatRequest struct {
	MsgID          string      `json:"msgId"`
	ConversationID string      `json:"conversationId"`
	Query          string      `json:"query"`
	ModelConfig    ModelConfig `json:"modelConfig"`
	MultiMedias    []any       `json:"multiMedias"`
}

// ModelConfig 模型配置
type ModelConfig struct {
	EnableThinking  bool    `json:"enableThinking"`
	Temperature     float64 `json:"temperature"`
	TopP            float64 `json:"topP"`
	WebSearchStatus string  `json:"webSearchStatus"` // "disabled" | "enabled"
	Model           string  `json:"model"`
}

// SSEEvent 解析出的 SSE 事件数据
type SSEEvent struct {
	Event string
	Data  string
}

// MimoSSEData aistudio 流式返回的 JSON 数据（type+content 格式）
type MimoSSEData struct {
	Type    string `json:"type"`    // "text" | "thinking" | "finish" | "usage" 等
	Content string `json:"content"` // 具体内容
	// finish 事件字段
	FinishInfo *struct {
		Reason string `json:"finishReason"`
	} `json:"finishInfo"`
	// usage 事件字段
	Usage *struct {
		PromptTokens     int `json:"promptTokens"`
		CompletionTokens int `json:"completionTokens"`
		TotalTokens      int `json:"totalTokens"`
	} `json:"usage"`
}

// ——————————————————————————————————————————
// 构建请求
// ——————————————————————————————————————————

func buildHeaders(acc *account.Account) http.Header {
	h := http.Header{}
	h.Set("Accept", "*/*")
	h.Set("Content-Type", "application/json")
	h.Set("Origin", BaseURL)
	h.Set("Referer", BaseURL+"/")
	h.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36")
	h.Set("x-timezone", "Asia/Shanghai")
	// ph 已改为 URL 参数，Cookie 中不再包含 ph
	h.Set("Cookie", acc.Cookie)
	return h
}

// BuildChatRequest 根据 OpenAI 消息列表构建 Mimo 请求体
func BuildChatRequest(messages []Message, model string, enableThinking, enableSearch bool) ChatRequest {
	query := mergeMessages(messages)

	webSearch := "disabled"
	if enableSearch {
		webSearch = "enabled"
	}

	baseModel := "clawl-alpha"

	return ChatRequest{
		MsgID:          uuid.New().String(),
		ConversationID: uuid.New().String(),
		Query:          query,
		ModelConfig: ModelConfig{
			EnableThinking:  enableThinking,
			Temperature:     0.8,
			TopP:            0.95,
			WebSearchStatus: webSearch,
			Model:           baseModel,
		},
		MultiMedias: []any{},
	}
}

// mergeMessages 将 OpenAI 多轮消息合并成一段文本
func mergeMessages(messages []Message) string {
	var sb strings.Builder
	for _, msg := range messages {
		switch msg.Role {
		case "system":
			sb.WriteString("[System]: ")
			sb.WriteString(contentString(msg.Content))
			sb.WriteString("\n\n")
		case "user":
			sb.WriteString("[User]: ")
			sb.WriteString(contentString(msg.Content))
			sb.WriteString("\n\n")
		case "assistant":
			sb.WriteString("[Assistant]: ")
			sb.WriteString(contentString(msg.Content))
			sb.WriteString("\n\n")
		}
	}
	return strings.TrimSpace(sb.String())
}

// Message 兼容 OpenAI 消息格式
type Message struct {
	Role    string `json:"role"`
	Content any    `json:"content"` // string 或 []ContentPart
}

type ContentPart struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

func contentString(c any) string {
	switch v := c.(type) {
	case string:
		return v
	case []any:
		var sb strings.Builder
		for _, part := range v {
			if m, ok := part.(map[string]any); ok {
				if t, ok := m["text"].(string); ok {
					sb.WriteString(t)
				}
			}
		}
		return sb.String()
	default:
		b, _ := json.Marshal(c)
		return string(b)
	}
}

// ——————————————————————————————————————————
// 流式请求（核心）
// ——————————————————————————————————————————

// StreamResult 是流式调用返回的单个 token
type StreamResult struct {
	ThinkingContent string // thinking 类型内容
	Content         string // text 类型内容
	FinishReason    string // stop / length
	Usage           *Usage
	Err             error
}

// Usage token 用量
type Usage struct {
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
}

// Stream 向 aistudio 发起流式请求，通过 channel 返回每个 token
func (c *Client) Stream(ctx context.Context, acc *account.Account, req ChatRequest) (<-chan StreamResult, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	// ph 作为 URL 参数
	url := ChatAPIURL(acc.Ph)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header = buildHeaders(acc)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		return nil, fmt.Errorf("upstream status %d", resp.StatusCode)
	}

	ch := make(chan StreamResult, 64)
	go func() {
		defer close(ch)
		defer resp.Body.Close()
		parseSSE(resp.Body, ch)
	}()

	return ch, nil
}

// CallSync 非流式调用，返回完整结果
func (c *Client) CallSync(ctx context.Context, acc *account.Account, req ChatRequest) (thinking, content string, usage *Usage, err error) {
	ch, err := c.Stream(ctx, acc, req)
	if err != nil {
		return "", "", nil, err
	}
	var sbT, sbC strings.Builder
	var finUsage *Usage
	for r := range ch {
		if r.Err != nil {
			return "", "", nil, r.Err
		}
		sbT.WriteString(r.ThinkingContent)
		sbC.WriteString(r.Content)
		if r.Usage != nil {
			finUsage = r.Usage
		}
	}
	return sbT.String(), sbC.String(), finUsage, nil
}

// CheckAuth 检查账号是否有效，调用 /open-apis/user/mi/get
func (c *Client) CheckAuth(acc *account.Account) bool {
	url := BaseURL + AuthAPI
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return false
	}
	req.Header = buildHeaders(acc)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return false
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return false
	}

	var result map[string]any
	if err := json.Unmarshal(body, &result); err != nil {
		return false
	}

	// code == 0 表示鉴权成功
	if code, ok := result["code"].(float64); ok {
		return code == 0
	}
	return false
}

// ——————————————————————————————————————————
// SSE 解析（type+content 方式）
// ——————————————————————————————————————————

func parseSSE(r io.Reader, ch chan<- StreamResult) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	var eventType string
	var dataLines []string

	flush := func() {
		if len(dataLines) == 0 {
			return
		}
		dataStr := strings.Join(dataLines, "\n")
		handleSSEEvent(eventType, dataStr, ch)
		eventType = ""
		dataLines = nil
	}

	for scanner.Scan() {
		line := scanner.Text()

		if line == "" {
			flush()
			continue
		}

		if strings.HasPrefix(line, "event:") {
			eventType = strings.TrimSpace(line[6:])
		} else if strings.HasPrefix(line, "data:") {
			dataLines = append(dataLines, strings.TrimSpace(line[5:]))
		}
	}
	flush()

	if err := scanner.Err(); err != nil && err != io.EOF {
		ch <- StreamResult{Err: fmt.Errorf("read SSE: %w", err)}
	}
}

func handleSSEEvent(event, data string, ch chan<- StreamResult) {
	if data == "[DONE]" || data == "" {
		return
	}

	var d MimoSSEData
	if err := json.Unmarshal([]byte(data), &d); err != nil {
		// 非 JSON，忽略
		return
	}

	result := StreamResult{}

	// 按 type 字段分发处理
	switch d.Type {
	case "thinking":
		// 思考内容
		if d.Content != "" && d.Content != "thinking" {
			result.ThinkingContent = d.Content
		}
	case "text":
		// 正文内容
		if d.Content != "" && d.Content != "webSearch" && d.Content != "[DONE]" {
			result.Content = d.Content
		}
	case "finish":
		if d.FinishInfo != nil {
			result.FinishReason = d.FinishInfo.Reason
			if result.FinishReason == "STOP" || result.FinishReason == "" {
				result.FinishReason = "stop"
			}
		} else {
			result.FinishReason = "stop"
		}
	case "usage":
		if d.Usage != nil {
			result.Usage = &Usage{
				PromptTokens:     d.Usage.PromptTokens,
				CompletionTokens: d.Usage.CompletionTokens,
				TotalTokens:      d.Usage.TotalTokens,
			}
		}
	default:
		// 无 type 字段时，尝试兼容旧格式（直接读 content）
		if d.Type == "" && d.Content != "" && d.Content != "webSearch" && d.Content != "[DONE]" {
			result.Content = d.Content
		}
		// 检查旧格式 finishInfo
		if d.FinishInfo != nil {
			result.FinishReason = d.FinishInfo.Reason
			if result.FinishReason == "STOP" || result.FinishReason == "" {
				result.FinishReason = "stop"
			}
		}
		if d.Usage != nil {
			result.Usage = &Usage{
				PromptTokens:     d.Usage.PromptTokens,
				CompletionTokens: d.Usage.CompletionTokens,
				TotalTokens:      d.Usage.TotalTokens,
			}
		}
	}

	// 只有有内容时才发送
	if result.ThinkingContent != "" || result.Content != "" || result.FinishReason != "" || result.Usage != nil {
		ch <- result
	}
}

// ParseModelFlags 从模型名解析功能标志
func ParseModelFlags(model string) (enableThinking, enableSearch bool) {
	m := strings.ToLower(model)
	enableThinking = strings.Contains(m, "thinking")
	enableSearch = strings.Contains(m, "search")
	return
}
