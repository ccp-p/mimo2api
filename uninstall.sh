#!/bin/bash

# MiMo2API 卸载脚本
# 使用方法: sudo bash uninstall.sh [选项]
# 选项:
#   --install-dir DIR  安装目录 (默认: /opt/admin)
#   --data-dir DIR     数据目录 (默认: /opt/admin/data)
#   --user USER        运行用户 (默认: admin)
#   --keep-data        保留数据目录

set -e

# 默认配置
INSTALL_DIR="/home/admin/mimo2api"
DATA_DIR="/home/admin/mimo2api/data"
USER="admin"
KEEP_DATA=false

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
        --data-dir)
            DATA_DIR="$2"
            shift 2
            ;;
        --user)
            USER="$2"
            shift 2
            ;;
        --keep-data)
            KEEP_DATA=true
            shift
            ;;
        --help)
            echo "MiMo2API 卸载脚本"
            echo "使用方法: sudo bash uninstall.sh [选项]"
            echo ""
            echo "选项:"
            echo "  --install-dir DIR  安装目录 (默认: /opt/admin)"
            echo "  --data-dir DIR     数据目录 (默认: /opt/admin/data)"
            echo "  --user USER        运行用户 (默认: admin)"
            echo "  --keep-data       保留数据目录"
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
    echo "请使用: sudo bash uninstall.sh"
    exit 1
fi

echo_green "🗑️  开始卸载 MiMo2API"
echo "配置信息:"
echo "  安装目录: $INSTALL_DIR"
echo "  数据目录: $DATA_DIR"
echo "  运行用户: $USER"
echo "  保留数据: $KEEP_DATA"

# 确认卸载
read -p "确定要卸载 MiMo2API 吗？(y/N): " confirm
if [[ ! "$confirm" =~ ^[Yy]$ ]]; then
    echo "取消卸载"
    exit 0
fi

# 停止服务
echo_yellow "🛑 停止服务..."
if systemctl is-active --quiet admin; then
    systemctl stop admin
    echo_green "✅ 服务已停止"
else
    echo_green "✅ 服务未运行"
fi

# 禁用服务
echo_yellow "🔄 禁用服务..."
if systemctl is-enabled --quiet admin; then
    systemctl disable admin
    echo_green "✅ 服务已禁用"
else
    echo_green "✅ 服务未启用"
fi

# 删除systemd服务文件
echo_yellow "📄 删除systemd服务文件..."
if [[ -f "/etc/systemd/system/admin.service" ]]; then
    rm -f /etc/systemd/system/admin.service
    systemctl daemon-reexec
    systemctl daemon-reload
    echo_green "✅ systemd服务文件已删除"
else
    echo_green "✅ systemd服务文件不存在"
fi

# 删除安装目录
echo_yellow "📁 删除安装目录..."
if [[ -d "$INSTALL_DIR" ]]; then
    rm -rf "$INSTALL_DIR"
    echo_green "✅ 安装目录已删除"
else
    echo_green "✅ 安装目录不存在"
fi

# 删除数据目录（如果未设置保留）
if [[ "$KEEP_DATA" == false ]]; then
    echo_yellow "📂 删除数据目录..."
    if [[ -d "$DATA_DIR" ]]; then
        rm -rf "$DATA_DIR"
        echo_green "✅ 数据目录已删除"
    else
        echo_green "✅ 数据目录不存在"
    fi
else
    echo_yellow "⚠️  保留数据目录: $DATA_DIR"
fi

# 注意：不删除admin用户
echo_yellow "⚠️  保留admin用户（系统用户）"

echo_green "🎉 MiMo2API 卸载完成!"

if [[ "$KEEP_DATA" == true ]]; then
    echo_yellow "⚠️  数据目录已保留: $DATA_DIR"
    echo "如需完全清理，请手动删除此目录"
fi