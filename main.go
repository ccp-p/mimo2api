package main

import (
	_ "embed"
	"embed"
	"flag"
	"fmt"
	"log"
	"mimo2api/account"
	"mimo2api/mimo"
	"mimo2api/server"
	"net/http"
	"os"
	"strings"
)

//go:embed web
var webFS embed.FS

func main() {
	var (
		port    = flag.String("port", envOr("PORT", "8080"), "监听端口")
		dataDir = flag.String("data", envOr("DATA_DIR", "data"), "账号配置目录")
		apiKey  = flag.String("key", envOr("API_KEY", ""), "API Key（可选，启动后可在管理界面配置多个）")
	)
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, `MiMo2API - 将 aistudio.xiaomimimo.com 转换为 OpenAI 兼容 API

用法:
  mimo2api [选项]

选项:
`)
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, `
环境变量（优先级低于命令行参数）:
  PORT       监听端口，默认 8080
  DATA_DIR   账号配置目录，默认 data/
  API_KEY    API Key（可选，启动后可在管理界面配置多个）

配置账号:
  在 data/ 目录下创建 JSON 文件，格式如下:
  {
    "name": "账号1",
    "cookie": "serviceToken=xxx; userId=yyy",
    "ph": "xiaomichatbot_ph 的值"
  }

  或访问 http://localhost:<PORT> 使用 Web 管理面板添加账号

在 Claude Code 中使用:
  访问管理界面设置 API Key 后:
  $env:ANTHROPIC_BASE_URL = "http://localhost:%s/v1"
  $env:ANTHROPIC_API_KEY = "<你的API Key>"
  claude --model mimo-v2-flash-studio
`, *port)
	}
	flag.Parse()

	// 初始化配置管理器
	cfg := server.NewConfig(*dataDir)
	if err := cfg.Load(); err != nil {
		log.Printf("⚠️  加载配置文件失败: %v，使用默认配置", err)
	}
	// 如果命令行传入了 API Key，添加到配置中
	if *apiKey != "" {
		keys := cfg.GetAPIKeys()
		// 检查是否已存在
		found := false
		for _, k := range keys {
			if k == *apiKey {
				found = true
				break
			}
		}
		if !found {
			keys = append(keys, *apiKey)
			cfg.SetAPIKeys(keys)
		}
	}
	apiKeys := cfg.GetAPIKeys()
	if len(apiKeys) == 0 {
		apiKeys = []string{"sk-mimo"}
		cfg.SetAPIKeys(apiKeys)
	}

	// 初始化账号管理器
	mgr, err := account.NewManager(*dataDir)
	if err != nil {
		log.Fatalf("初始化账号管理器失败: %v", err)
	}

	accounts := mgr.List()
	if len(accounts) == 0 {
		log.Printf("⚠️  未找到账号配置，请访问 http://localhost:%s 添加账号", *port)
	} else {
		log.Printf("✅ 加载了 %d 个账号", len(accounts))
	}

	// 初始化 Mimo 客户端
	mimoClient := mimo.NewClient()

	// 注册路由
	mux := http.NewServeMux()

	// API 路由（传入配置以支持多 API Key）
	apiHandler := server.NewHandler(mgr, mimoClient, cfg)
	apiHandler.Register(mux)

	// 管理 API 路由
	adminHandler := server.NewAdminHandler(mgr, mimoClient, cfg)
	adminHandler.Register(mux)

	// 静态文件（管理界面）
	webHandler := server.NewWebHandler(webFS)
	webHandler.Register(mux)

	addr := "0.0.0.0:" + *port
	log.Printf("🚀 MiMo2API 启动成功")
	log.Printf("   管理界面: http://<你的IP>%s", ":"+*port)
	log.Printf("   API 端点: http://<你的IP>%s/v1/chat/completions", ":"+*port)
	log.Printf("   API Keys: %s", maskKeys(apiKeys))
	log.Printf("")
	log.Printf("Claude Code 配置:")
	log.Printf("   $env:ANTHROPIC_BASE_URL = \"http://localhost%s/v1\"", addr)
	log.Printf("   $env:ANTHROPIC_API_KEY = \"<在管理界面设置的Key>\"")

	if err := http.ListenAndServe(addr, corsMiddleware(mux)); err != nil {
		log.Fatalf("服务器错误: %v", err)
	}
}

// corsMiddleware 跨域中间件
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func maskKey(key string) string {
	if len(key) <= 6 {
		return "***"
	}
	return key[:4] + "****" + key[len(key)-2:]
}

func maskKeys(keys []string) string {
	if len(keys) == 0 {
		return "无"
	}
	var masked []string
	for _, k := range keys {
		masked = append(masked, maskKey(k))
	}
	return strings.Join(masked, ", ")
}
