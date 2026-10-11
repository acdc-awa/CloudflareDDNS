# Cloudflare DDNS

一个轻量高效的 Cloudflare 动态 DNS (DDNS) 自动化更新工具，由 Go 编写，通过 systemd timer/oneshot 托管。

单一静态二进制，**零运行时依赖**（无需 bash / curl / python），交叉编译后可直接部署到 Linux（amd64 / arm64）。

## 特性

- **公网 IP 自动探测**：轮询多个 IP 源并校验 IPv4
- **Cloudflare API**：自动查询 Zone、A 记录增/改/查
- **智能更新**：IP 未变化跳过；记录不存在自动创建；有变化才更新
- **systemd 托管**：generate service + timer，`Persistent=true` 防漏跑
- **结构化日志**：`log/slog`，自动按行数轮转（保留最近 1000 行）
- **配置校验**：启动、修改时自动校验 Token / 域名 / TTL / 间隔
- **多命令行工具**：install / run-once / status / log / uninstall / config

## 目录结构

```
cmd/ddns/           主程序（CLI + 配置向导）
internal/
  config/           配置模型、TOML 解析/校验/存储
  cloudflare/       Cloudflare API 客户端 + IP 检测
  runner/           DDNS 单次更新流程
  systemd/          systemd 单元文件生成与安装
  logx/             文件日志（分级 + 轮转）
deploy.sh           Linux 一键部署与管理脚本
dist/               Linux 交叉编译产物
```

## 构建

本地构建（Windows / Linux / macOS）：

```bash
go build ./cmd/ddns
```

交叉编译 Linux 产物（需 Go 工具链，无需 CGO）：

```bash
mkdir -p dist
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o dist/ddns-linux-amd64 ./cmd/ddns
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags="-s -w" -o dist/ddns-linux-arm64 ./cmd/ddns
```

## 部署与安装

### 方式一：一键脚本在线安装（推荐）

在目标 Linux 服务器上执行：

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/acdc-awa/CloudflareDDNS/main/deploy.sh)
```

脚本会自动检测 CPU 架构（amd64 / arm64）、安装二进制文件、启动交互式配置向导并注册启动 systemd 定时服务。

### 方式二：手动部署

在目标 Linux 服务器上以 root 执行：

```bash
# 1. 复制二进制到系统路径
sudo cp dist/ddns-linux-amd64 /usr/local/bin/ddns
sudo chmod +x /usr/local/bin/ddns

# 2. 首次安装（自动进入配置向导 + 生成并启用 systemd timer）
sudo ddns install
```

`install` 会：

1. 若 `/etc/ddns/config.toml` 不存在，进入交互式配置向导并保存
2. 生成 `/etc/systemd/system/ddns.service` 和 `ddns.timer`
3. `systemctl daemon-reload && enable && start ddns.timer`

## 命令

| 命令 | 说明 |
|------|------|
| `ddns install` | 配置 + 安装并启用 systemd service/timer |
| `ddns run-once` | 静默执行一次 DDNS 更新（timer 调用） |
| `ddns config` | 交互式修改配置 |
| `ddns status` | 查看配置、定时器状态、最近日志 |
| `ddns log` | 查看最近 30 行日志 |
| `ddns uninstall` | 停止并卸载 systemd 服务与配置 |
| `ddns install-write-only` | 仅生成单元文件不启用（供审查） |
| `ddns version` | 显示版本 |

## 配置文件

默认 `/etc/ddns/config.toml`（权限 0600）：

```toml
cf_api_token = "你的_Cloudflare_API_Token"      # 需 DNS:Edit 权限
zone_name = "example.com"                       # 主域名
record_name = "ddns.example.com"                # 待更新的 A 记录
record_ttl = 120                                # 1=自动，或 >=120
check_interval = "5m"                           # [0-9]+[smhd]
ip_sources = ["https://api.ipify.org", "https://icanhazip.com"]
```

## 路径与环境变量

默认遵循 FHS，可用环境变量覆盖（便于自定义部署/测试）：

| 环境变量 | 默认值 |
|----------|--------|
| `DDNS_CONFIG_PATH` | `/etc/ddns/config.toml` |
| `DDNS_LOG_PATH` | `/var/log/ddns.log` |
| `DDNS_BIN_PATH` | `/usr/local/bin/ddns` |
| `DDNS_SERVICE_PATH` | `/etc/systemd/system/ddns.service` |
| `DDNS_TIMER_PATH` | `/etc/systemd/system/ddns.timer` |

## 运维

```bash
journalctl -u ddns        # 查看 systemd 日志
systemctl status ddns.timer
systemctl restart ddns.timer
```

## 测试

```bash
go test ./...
```
