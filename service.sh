#!/bin/bash

# MiMo2API 服务管理脚本
# 使用方法: sudo bash service.sh [命令]
# 命令:
#   start     启动服务
#   stop      停止服务
#   restart   重启服务
#   status    查看服务状态
#   logs      查看服务日志
#   enable    启用开机自启
#   disable   禁用开机自启

set -e

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

# 检查是否为root用户
if [[ $EUID -ne 0 ]]; then
    echo_red "错误: 此脚本需要root权限运行"
    echo "请使用: sudo bash service.sh [命令]"
    exit 1
fi

# 检查命令参数
if [[ $# -eq 0 ]]; then
    echo "MiMo2API 服务管理脚本"
    echo "使用方法: sudo bash service.sh [命令]"
    echo ""
    echo "命令:"
    echo "  start     启动服务"
    echo "  stop      停止服务"
    echo "  restart   重启服务"
    echo "  status    查看服务状态"
    echo "  logs      查看服务日志"
    echo "  enable    启用开机自启"
    echo "  disable   禁用开机自启"
    exit 1
fi

COMMAND="$1"

case "$COMMAND" in
    start)
        echo_yellow "🚀 启动 MiMo2API 服务..."
        if systemctl is-active --quiet mimo2api; then
            echo_green "✅ 服务已在运行中"
        else
            systemctl start mimo2api
            sleep 2
            if systemctl is-active --quiet mimo2api; then
                echo_green "✅ 服务启动成功"
                echo "管理界面: http://localhost:$(grep -oP '(?<=-port=)[0-9]+' /etc/systemd/system/mimo2api.service 2>/dev/null || echo '8080')"
            else
                echo_red "❌ 服务启动失败"
                echo "查看日志: sudo journalctl -u mimo2api -f"
                exit 1
            fi
        fi
        ;;

    stop)
        echo_yellow "🛑 停止 MiMo2API 服务..."
        if systemctl is-active --quiet mimo2api; then
            systemctl stop mimo2api
            echo_green "✅ 服务已停止"
        else
            echo_green "✅ 服务未运行"
        fi
        ;;

    restart)
        echo_yellow "🔄 重启 MiMo2API 服务..."
        systemctl restart mimo2api
        sleep 2
        if systemctl is-active --quiet mimo2api; then
            echo_green "✅ 服务重启成功"
            echo "管理界面: http://localhost:$(grep -oP '(?<=-port=)[0-9]+' /etc/systemd/system/mimo2api.service 2>/dev/null || echo '8080')"
        else
            echo_red "❌ 服务重启失败"
            echo "查看日志: sudo journalctl -u mimo2api -f"
            exit 1
        fi
        ;;

    status)
        echo "📊 MiMo2API 服务状态:"
        echo ""

        # 服务运行状态
        if systemctl is-active --quiet mimo2api; then
            echo_green "✅ 服务状态: 运行中"
        else
            echo_red "❌ 服务状态: 已停止"
        fi

        # 开机自启状态
        if systemctl is-enabled --quiet mimo2api; then
            echo_green "✅ 开机自启: 已启用"
        else
            echo_yellow "⚠️  开机自启: 未启用"
        fi

        # 服务详细信息
        echo ""
        echo "详细信息:"
        systemctl status mimo2api --no-pager

        # 端口信息
        PORT=$(grep -oP '(?<=-port=)[0-9]+' /etc/systemd/system/mimo2api.service 2>/dev/null || echo '8080')
        echo ""
        echo "端口信息:"
        echo "  管理界面: http://localhost:$PORT"
        echo "  API端点: http://localhost:$PORT/v1/chat/completions"

        # 进程信息
        if systemctl is-active --quiet mimo2api; then
            echo ""
            echo "进程信息:"
            ps aux | grep mimo2api | grep -v grep
        fi
        ;;

    logs)
        echo "📋 MiMo2API 服务日志:"
        echo "按 Ctrl+C 退出日志查看"
        echo ""
        journalctl -u mimo2api -f
        ;;

    enable)
        echo_yellow "🔄 启用开机自启..."
        if systemctl is-enabled --quiet mimo2api; then
            echo_green "✅ 开机自启已启用"
        else
            systemctl enable mimo2api
            echo_green "✅ 开机自启启用成功"
        fi
        ;;

    disable)
        echo_yellow "🔄 禁用开机自启..."
        if systemctl is-enabled --quiet mimo2api; then
            systemctl disable mimo2api
            echo_green "✅ 开机自启禁用成功"
        else
            echo_green "✅ 开机自启已禁用"
        fi
        ;;

    *)
        echo_red "未知命令: $COMMAND"
        echo "可用命令: start, stop, restart, status, logs, enable, disable"
        exit 1
        ;;
esac