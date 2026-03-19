# MiMo2API 快速开始指南

## 🎯 3分钟快速部署

### 第一步：准备服务器

确保你的服务器满足以下要求：
- Linux 系统（Ubuntu/CentOS/Debian等）
- Go 1.24.1+ 已安装
- root 权限可用
- 开放 8080 端口（或你选择的端口）

### 第二步：上传并部署

```bash
# 1. 上传项目到服务器（或使用git克隆）
cd /tmp
# 将项目文件上传到当前目录，或:
# git clone <your-repo-url> mimo2api

# 2. 进入项目目录
cd mimo2api  # 替换为你的项目目录名

# 3. 一键部署
sudo bash deploy.sh
```

### 第三步：验证部署

```bash
# 查看服务状态
sudo bash service.sh status

# 访问管理界面
curl http://localhost:8080

# 在浏览器中访问
# http://你的服务器IP:8080
```

## 🚀 部署成功后的配置

### 1. 配置账号

访问管理界面 `http://你的服务器IP:8080` 并：
1. 点击