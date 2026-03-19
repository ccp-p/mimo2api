# MiMo2API 部署指南

本指南提供了 MiMo2API 的完整部署说明，包括自动化脚本的使用。

## 📋 系统要求

- Linux 服务器（推荐 Ubuntu 20.04+ 或 CentOS 7+）
- Go 1.24.1 或更高版本
- root 权限（用于部署和管理）
- 开放所需端口（默认 8080）

## 🚀 快速部署

### 1. 上传项目到服务器

```bash
# 克隆或上传项目到服务器
cd /tmp
git clone <your-repo-url> mimo2api-deploy
cd mimo2api-deploy
```

### 2. 执行部署脚本

```bash
# 基本部署（使用默认配置）
sudo bash deploy.sh

# 自定义配置部署
sudo bash deploy.sh \
  --port 8080 \
  --data-dir /home/admin/mimo2api/data \
  --install-dir /home/admin/mimo2api \
  --user admin \
  --api-key your-api-key
```

### 3. 验证部署

```bash
# 查看服务状态
sudo bash service.sh status

# 查看服务日志
sudo bash service.sh logs

# 访问管理界面
curl http://localhost:8080
```

## 📝 部署脚本参数说明

### deploy.sh

| 参数 | 说明 | 默认值 |
|------|------|--------|
| `--port PORT` | 服务监听端口 | 8080 |
| `--data-dir DIR` | 数据目录路径 | /home/admin/mimo2api/data |
| `--install-dir DIR` | 安装目录路径 | /home/admin/mimo2api |
| `--user USER` | 运行服务用户 | admin |
| `--api-key KEY` | 初始API Key | 空 |
| `--force` | 强制重新部署 | false |

### update.sh

| 参数 | 说明 | 默认值 |
|------|------|--------|
| `--install-dir DIR` | 安装目录路径 | /home/admin/mimo2api |
| `--user USER` | 运行服务用户 | admin |
| `--no-restart` | 不重启服务 | false |

### uninstall.sh

| 参数 | 说明 | 默认值 |
|------|------|--------|
| `--install-dir DIR` | 安装目录路径 | /home/admin/mimo2api |
| `--data-dir DIR` | 数据目录路径 | /home/admin/mimo2api/data |
| `--user USER` | 运行服务用户 | admin |
| `--keep-data` | 保留数据目录 | false |

## 🔧 常用管理命令

### 服务管理

```bash
# 启动服务
sudo bash service.sh start

# 停止服务
sudo bash service.sh stop

# 重启服务
sudo bash service.sh restart

# 查看状态
sudo bash service.sh status

# 查看日志
sudo bash service.sh logs

# 启用开机自启
sudo bash service.sh enable

# 禁用开机自启
sudo bash service.sh disable
```

### 手动管理

```bash
# 使用systemctl直接管理
sudo systemctl start mimo2api
sudo systemctl stop mimo2api
sudo systemctl restart mimo2api
sudo systemctl status mimo2api
sudo journalctl -u mimo2api -f

# 查看服务配置
cat /etc/systemd/system/mimo2api.service

# 重新加载配置
sudo systemctl daemon-reload
```

## 🔄 更新应用

### 方法1：使用更新脚本

```bash
# 进入项目目录
cd /path/to/mimo2api

# 拉取最新代码
git pull

# 执行更新
sudo bash update.sh
```

### 方法2：手动更新

```bash
# 停止服务
sudo systemctl stop mimo2api

# 构建新版本
cd /path/to/mimo2api
go build -o /home/admin/mimo2api/mimo2api main.go

# 设置权限
sudo chown mimo2api:mimo2api /home/admin/mimo2api/mimo2api
sudo chmod +x /home/admin/mimo2api/mimo2api

# 启动服务
sudo systemctl start mimo2api
```

## 🗑️ 卸载应用

### 方法1：使用卸载脚本

```bash
# 保留数据卸载
sudo bash uninstall.sh --keep-data

# 完全卸载（包括数据）
sudo bash uninstall.sh
```

### 方法2：手动卸载

```bash
# 停止并禁用服务
sudo systemctl stop mimo2api
sudo systemctl disable mimo2api

# 删除服务文件
sudo rm -f /etc/systemd/system/mimo2api.service
sudo systemctl daemon-reload

# 删除应用和数据
sudo rm -rf /home/admin/mimo2api
# 注意：不删除admin用户
```

## 🔐 安全配置

### 防火墙配置

```bash
# Ubuntu/Debian (ufw)
sudo ufw allow 8080/tcp
sudo ufw reload

# CentOS/RHEL (firewalld)
sudo firewall-cmd --permanent --add-port=8080/tcp
sudo firewall-cmd --reload

# 或者使用iptables
sudo iptables -A INPUT -p tcp --dport 8080 -j ACCEPT
```

### Nginx 反向代理

```nginx
server {
    listen 80;
    server_name api.yourdomain.com;

    location / {
        proxy_pass http://localhost:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
```

### SSL 配置（推荐）

```nginx
server {
    listen 443 ssl http2;
    server_name api.yourdomain.com;

    ssl_certificate /path/to/cert.pem;
    ssl_certificate_key /path/to/key.pem;

    location / {
        proxy_pass http://localhost:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
```

## 📊 监控和维护

### 日志查看

```bash
# 实时查看日志
sudo journalctl -u mimo2api -f

# 查看最近100条日志
sudo journalctl -u mimo2api -n 100

# 查看错误日志
sudo journalctl -u mimo2api -p err
```

### 性能监控

```bash
# 查看服务资源使用
top -p $(pgrep mimo2api)

# 查看端口监听
ss -tulpn | grep mimo2api

# 查看磁盘使用
df -h /home/admin/mimo2api/data
```

### 定期维护

```bash
# 清理旧日志
sudo journalctl --vacuum-time=7d

# 备份数据目录
sudo tar -czf mimo2api-backup-$(date +%Y%m%d).tar.gz /home/admin/mimo2api/data

# 检查服务健康状态
curl -f http://localhost:8080/health || echo "服务异常"
```

## 🚨 故障排除

### 服务无法启动

1. **检查端口占用**
   ```bash
   sudo netstat -tulpn | grep :8080
   ```

2. **检查权限**
   ```bash
   sudo chown -R mimo2api:mimo2api /home/admin/mimo2api
   sudo chmod +x /home/admin/mimo2api/mimo2api
   ```

3. **查看日志**
   ```bash
   sudo journalctl -u mimo2api -n 50
   ```

### 构建失败

1. **检查Go版本**
   ```bash
   go version
   ```

2. **检查依赖**
   ```bash
   go mod tidy
   ```

### 权限问题

```bash
# 修复目录权限
sudo chown -R mimo2api:mimo2api /home/admin/mimo2api
sudo chmod -R 755 /home/admin/mimo2api

# 修复systemd权限
sudo systemctl daemon-reexec
sudo systemctl daemon-reload
```

## 📞 支持

如果遇到问题，请：

1. 查看日志：`sudo journalctl -u mimo2api -f`
2. 检查服务状态：`sudo systemctl status mimo2api`
3. 确保端口开放：`sudo ufw status` 或 `sudo firewall-cmd --list-all`

## 📄 许可证

本项目遵循原始许可证条款。