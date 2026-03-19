#!/bin/bash

# MiMo2API 部署脚本
# 使用方法: sudo bash deploy.sh [选项]
# 选项:
#   --port PORT        服务端口 (默认: 8080)
#   --data-dir DIR     数据目录 (默认: /opt/mimo2api/data)
#   --install-dir DIR  安装目录 (默认: /opt/mimo2api)
#   --user USER        运行用户 (默认: admin)
#   --api-key KEY      初始API Key (可选)
#   --force           强制重新部署

set -e

# 默认配置
PORT=8080
DATA_DIR="/home/admin/mimo2api/data"
INSTALL_DIR="/home/admin/mimo2api"
USER="admin"
API_KEY=""
FORCE=false

# 颜色输出函数
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo_red() {
    echo -e "${RED}$1${NC}"
}

echo_green() {
    echo -e "${GREEN}$1${NC}"
}

echo_yellow() {
    echo -e "${YELLOW}$1${NC}"
}

# 解析命令行参数
while [[ $# -gt 0 ]]; do
    case $1 in
        --port)
            PORT="$2"
            shift 2
            ;;
        --data-dir)
            DATA_DIR="$2"
            shift 2
            ;;
        --install-dir)
            INSTALL_DIR="$2"
            shift 2
            ;;
        --user)
            USER="$2"
            shift 2
            ;;
        --api-key)
            API_KEY="$2"
            shift 2
            ;;
        --force)
            FORCE=true
            shift
            ;;
        --help)
            echo "MiMo2API 部署脚本"
            echo "使用方法: sudo bash deploy.sh [选项]"
            echo ""
            echo "选项:"
            echo "  --port PORT        服务端口 (默认: 8080)"
            echo "  --data-dir DIR     数据目录 (默认: /opt/mimo2api/data)"
            echo "  --install-dir DIR  安装目录 (默认: /opt/mimo2api)"
            echo "  --user USER        运行用户 (默认: admin)"
            echo "  --api-key KEY      初始API Key (可选)"
            echo "  --force           强制重新部署"
            exit 0
            ;;
        *)
            echo_red "未知参数: $1"
            echo "使用 --help 查看帮助"
            exit 1
            ;;
    esac
done

# 检查是否为root用户
if [[ $EUID -ne 0 ]]; then
    echo_red "错误: 此脚本需要root权限运行"
    echo "请使用: sudo bash deploy.sh"
    exit 1
fi

echo_green "🚀 开始部署 MiMo2API"
echo "配置信息:"
echo "  端口: $PORT"
echo "  数据目录: $DATA_DIR"
echo "  安装目录: $INSTALL_DIR"
echo "  运行用户: $USER"
echo "  API Key: ${API_KEY:-'未设置'}"

# 检查Go环境
if ! command -v go &> /dev/null; then
    echo_red "错误: Go 未安装或未在PATH中"
    echo "请先安装 Go 1.24.1 或更高版本"
    exit 1
fi

GO_VERSION=$(go version | awk '{print $3}' | sed 's/go//')
if [[ "$GO_VERSION" < "1.24" ]]; then
    echo_red "错误: Go 版本过低，需要 1.24.1 或更高版本"
    echo "当前版本: $GO_VERSION"
    exit 1
fi

echo_green "✅ Go 环境检查通过 ($GO_VERSION)"

# 检查项目文件
if [[ ! -f "main.go" ]]; then
    echo_red "错误: 当前目录下未找到 main.go 文件"
    echo "请确保在项目根目录运行此脚本"
    exit 1
fi

echo_green "✅ 项目文件检查通过"

# 停止现有服务（如果存在）
if systemctl is-active --quiet mimo2api; then
    if [[ "$FORCE" == true ]]; then
        echo_yellow "⚠️  停止现有服务..."
        systemctl stop mimo2api
        echo_green "✅ 服务已停止"
    else
        echo_red "错误: MiMo2API 服务正在运行"
        echo "使用 --force 参数强制重新部署"
        exit 1
    fi
fi

# 检查运行用户
if id "$USER" &> /dev/null; then
    echo_green "✅ 使用现有用户: $USER"
else
    echo_red "错误: 用户 $USER 不存在"
    echo "请确保admin用户存在或使用 --user 指定其他用户"
    exit 1
fi

# 创建目录
echo_yellow "📁 创建目录结构..."
mkdir -p "$INSTALL_DIR"
mkdir -p "$DATA_DIR"
echo_green "✅ 目录创建成功"

# 构建项目
echo_yellow "🔨 构建项目..."
if ! go build -o "$INSTALL_DIR/mimo2api" main.go; then
    echo_red "错误: 构建失败"
    exit 1
fi
echo_green "✅ 构建成功"

# 复制web资源（如果存在）
if [[ -d "web" ]]; then
    echo_yellow "📂 复制web资源..."
    cp -r web "$INSTALL_DIR/"
    echo_green "✅ web资源复制成功"
fi

# 设置权限
echo_yellow "🔐 设置权限..."
chown -R "$USER:$USER" "$INSTALL_DIR"
chown -R "$USER:$USER" "$DATA_DIR"
chmod +x "$INSTALL_DIR/mimo2api"
echo_green "✅ 权限设置成功"

# 创建systemd服务文件
echo_yellow "⚙️  创建systemd服务..."

# 构建ExecStart命令
EXEC_START="$INSTALL_DIR/mimo2api -port=$PORT -data=$DATA_DIR"
if [[ -n "$API_KEY" ]]; then
    EXEC_START="$EXEC_START -key=$API_KEY"
fi

cat > /etc/systemd/system/mimo2api.service << EOF
[Unit]
Description=MiMo2API Service
After=network.target

[Service]
Type=simple
User=$USER
Group=$USER
WorkingDirectory=$INSTALL_DIR
ExecStart=$EXEC_START
Restart=always
RestartSec=10

# 安全设置
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=full
ProtectHome=true

[Install]
WantedBy=multi-user.target
EOF

echo_green "✅ systemd服务创建成功"

# 重载systemd配置
echo_yellow "🔄 重载systemd配置..."
systemctl daemon-reexec
systemctl daemon-reload
echo_green "✅ systemd配置重载成功"

# 启用服务
echo_yellow "🚀 启用并启动服务..."
systemctl enable mimo2api
systemctl start mimo2api
echo_green "✅ 服务启动成功"

# 检查服务状态
sleep 2
if systemctl is-active --quiet mimo2api; then
    echo_green "🎉 MiMo2API 部署成功!"
    echo ""
    echo "服务信息:"
    echo "  管理界面: http://localhost:$PORT"
    echo "  API端点: http://localhost:$PORT/v1/chat/completions"
    echo "  数据目录: $DATA_DIR"
    echo "  日志查看: sudo journalctl -u mimo2api -f"
    echo ""
    echo "Claude Code 配置:"
    echo "  \$env:ANTHROPIC_BASE_URL = \"http://localhost:$PORT/v1\""
    echo "  \$env:ANTHROPIC_API_KEY = \"<在管理界面设置的Key>\""
    echo ""
    echo_yellow "⚠️  请确保防火墙开放端口 $PORT"
else
    echo_red "❌ 服务启动失败"
    echo "查看日志: sudo journalctl -u mimo2api -f"
    exit 1
fi