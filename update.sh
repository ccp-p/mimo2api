#!/bin/bash

# MiMo2API 更新脚本
# 使用方法: sudo bash update.sh [选项]
# 选项:
#   --install-dir DIR  安装目录 (默认: /opt/admin)
#   --user USER        运行用户 (默认: admin)
#   --no-restart       不重启服务（仅更新文件）

set -e

# 默认配置
INSTALL_DIR="/home/admin/mimo2api"
USER="admin"
NO_RESTART=false

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
        --install-dir)
            INSTALL_DIR="$2"
            shift 2
            ;;
        --user)
            USER="$2"
            shift 2
            ;;
        --no-restart)
            NO_RESTART=true
            shift
            ;;
        --help)
            echo "MiMo2API 更新脚本"
            echo "使用方法: sudo bash update.sh [选项]"
            echo ""
            echo "选项:"
            echo "  --install-dir DIR  安装目录 (默认: /opt/admin)"
            echo "  --user USER        运行用户 (默认: admin)"
            echo "  --no-restart      不重启服务（仅更新文件）"
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
    echo "请使用: sudo bash update.sh"
    exit 1
fi

echo_green "🔄 开始更新 MiMo2API"
echo "配置信息:"
echo "  安装目录: $INSTALL_DIR"
echo "  运行用户: $USER"
echo "  重启服务: ${NO_RESTART:-true}"

# 检查项目文件
if [[ ! -f "main.go" ]]; then
    echo_red "错误: 当前目录下未找到 main.go 文件"
    echo "请确保在项目根目录运行此脚本"
    exit 1
fi

# 检查安装目录
if [[ ! -d "$INSTALL_DIR" ]]; then
    echo_red "错误: 安装目录不存在: $INSTALL_DIR"
    echo "请先运行部署脚本"
    exit 1
fi

# 检查服务状态
SERVICE_WAS_RUNNING=false
if systemctl is-active --quiet admin; then
    SERVICE_WAS_RUNNING=true
    if [[ "$NO_RESTART" == false ]]; then
        echo_yellow "🛑 停止服务..."
        systemctl stop admin
        echo_green "✅ 服务已停止"
    fi
fi

# 备份当前版本
echo_yellow "💾 备份当前版本..."
BACKUP_DIR="/tmp/admin_backup_$(date +%Y%m%d_%H%M%S)"
mkdir -p "$BACKUP_DIR"
if [[ -f "$INSTALL_DIR/admin" ]]; then
    cp "$INSTALL_DIR/admin" "$BACKUP_DIR/"
fi
if [[ -d "$INSTALL_DIR/web" ]]; then
    cp -r "$INSTALL_DIR/web" "$BACKUP_DIR/"
fi
echo_green "✅ 备份完成: $BACKUP_DIR"

# 构建新版本
echo_yellow "🔨 构建新版本..."
if ! go build -o "$INSTALL_DIR/admin" main.go; then
    echo_red "错误: 构建失败，恢复备份..."
    if [[ -f "$BACKUP_DIR/admin" ]]; then
        cp "$BACKUP_DIR/admin" "$INSTALL_DIR/"
    fi
    if [[ -d "$BACKUP_DIR/web" ]]; then
        rm -rf "$INSTALL_DIR/web"
        cp -r "$BACKUP_DIR/web" "$INSTALL_DIR/"
    fi
    if [[ "$SERVICE_WAS_RUNNING" == true ]] && [[ "$NO_RESTART" == false ]]; then
        systemctl start admin
    fi
    rmdir "$BACKUP_DIR"
    exit 1
fi

# 复制web资源（如果存在）
if [[ -d "web" ]]; then
    echo_yellow "📂 更新web资源..."
    rm -rf "$INSTALL_DIR/web"
    cp -r web "$INSTALL_DIR/"
    echo_green "✅ web资源更新成功"
fi

# 设置权限
echo_yellow "🔐 设置权限..."
chown -R "$USER:$USER" "$INSTALL_DIR"
chmod +x "$INSTALL_DIR/admin"
echo_green "✅ 权限设置成功"

# 重启服务
if [[ "$SERVICE_WAS_RUNNING" == true ]] && [[ "$NO_RESTART" == false ]]; then
    echo_yellow "🚀 重启服务..."
    systemctl start admin

    # 等待服务启动
    sleep 3

    if systemctl is-active --quiet admin; then
        echo_green "✅ 服务重启成功"
    else
        echo_red "❌ 服务启动失败，查看日志: sudo journalctl -u admin -f"
        exit 1
    fi
elif [[ "$NO_RESTART" == true ]]; then
    echo_yellow "⚠️  跳过服务重启，需要手动重启: sudo systemctl restart admin"
else
    echo_green "✅ 文件更新完成"
fi

# 清理备份
echo_yellow "🧹 清理备份..."
rm -rf "$BACKUP_DIR"
echo_green "✅ 备份已清理"

echo_green "🎉 MiMo2API 更新完成!"

echo ""
echo "更新信息:"
echo "  安装目录: $INSTALL_DIR"
echo "  服务状态: $(systemctl is-active admin 2>/dev/null || echo 'stopped')"
echo "  查看日志: sudo journalctl -u admin -f"