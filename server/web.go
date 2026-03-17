package server

import (
	"embed"
	"io/fs"
	"net/http"
)

// WebHandler 处理静态文件服务
type WebHandler struct {
	fs embed.FS
}

// NewWebHandler 创建静态文件处理器
func NewWebHandler(efs embed.FS) *WebHandler {
	return &WebHandler{fs: efs}
}

// Register 注册静态文件路由
func (wh *WebHandler) Register(mux *http.ServeMux) {
	sub, _ := fs.Sub(wh.fs, "web")
	mux.Handle("/", http.FileServer(http.FS(sub)))
}
