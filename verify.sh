#!/bin/bash

# 验证部署脚本配置

echo "🔍 验证部署脚本配置..."

# 检查默认用户设置
echo ""
echo "📋 检查脚本中的默认用户设置:"

for script in deploy.sh update.sh uninstall.sh; do
    echo "检查 $script:"
    if grep -q 'USER="admin"' "$script"; then
        echo "  ✅ 默认用户设置为 admin"
    else
        echo "  ❌ 默认用户设置不正确"
    fi

done

# 检查用户创建逻辑
echo ""
echo "📋 检查用户创建逻辑:"

if grep -q "请确保admin用户存在" deploy.sh; then
    echo "  ✅ deploy.sh: 正确检查现有用户"
else
    echo "  ❌ deploy.sh: 用户检查逻辑不正确"
fi

if grep -q "保留admin用户" uninstall.sh; then
    echo "  ✅ uninstall.sh: 正确保留admin用户"
else
    echo "  ❌ uninstall.sh: 用户处理逻辑不正确"
fi

echo ""
echo "🎯 验证完成!"
echo "现在你可以使用admin用户部署MiMo2API服务了。"
echo ""
echo "使用方法:"
echo "  sudo bash deploy.sh"
echo "  sudo bash deploy.sh --user admin --port 8080"