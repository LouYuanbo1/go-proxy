# Go-Proxy

> 多协议代理服务 · Go 语言实现 · 支持 HTTP/HTTPS / SOCKS5 / QUIC

Go-Proxy 是一个用 Go 语言编写的多功能代理服务，集成了 **HTTP/HTTPS 正向代理**、**SOCKS5 代理**、**QUIC 协议代理**以及 **Web 管理网关**。支持 Windows 系统代理的自动配置与恢复。

---

## ✨ 功能特性

| 特性 | 说明 |
|------|------|
| **HTTP/HTTPS 正向代理** | 支持 `GET`/`POST` 等 HTTP 方法与 `CONNECT` 隧道（端口 `:8080`） |
| **SOCKS5 代理** | 完整 SOCKS5 协议实现，支持 IPv4/IPv6/域名（端口 `:1080`） |
| **Web 管理网关** | 浏览器访问的管理界面 + PAC 自动配置（端口 `:8090`） |
| **QUIC 协议支持** | 基于 quic-go 的 QUIC 客户端与服务端，支持 QUIC 优先 + TCP 降级策略 |
| **服务端抓取测试** | Web 网关内置 `/api/fetch` 接口，可用于调试远程资源 |
| **Windows 系统代理** | 启动时自动设置系统代理，退出时自动恢复（通过注册表 + WinINET API） |
| **PAC 自动配置** | Web 网关提供 `proxy.pac` 自动配置脚本，支持局域网地址直连 |

---

## 📦 快速开始

### 前置要求

- Go 1.23+（推荐 1.27+）

### 编译运行

```bash
# 克隆仓库
git clone https://github.com/LouYuanbo1/go-proxy.git
cd go-proxy

# 编译
go build -o go-proxy main.go

# 运行（Windows 系统代理自动启用）
go-proxy.exe
```

### 端口说明

| 端口 | 协议 | 用途 |
|------|------|------|
| `:8080` | HTTP | HTTP/HTTPS 正向代理 |
| `:8090` | HTTP | Web 管理网关 + PAC + API |
| `:1080` | TCP | SOCKS5 代理 |
| `:1443` | UDP | QUIC 代理（默认注释，需手动启用） |

---

## 🚀 使用方式

### 方式一：系统代理（推荐）

程序在 **Windows** 上启动时会自动将系统代理设置为 `127.0.0.1:8090`，退出时自动恢复。这是最简洁的使用方式：

```bash
go-proxy.exe
```

然后在浏览器中直接访问任意网站即可。

### 方式二：浏览器 PAC 配置

访问 [http://localhost:8090/proxy.pac](http://localhost:8090/proxy.pac) 获取 PAC 脚本地址，将其配置到浏览器的自动代理设置中。PAC 脚本会自动绕过内网地址（`10.*`、`192.168.*`、`localhost` 等）。

### 方式三：手动配置

| 代理类型 | 代理地址 |
|----------|----------|
| HTTP 代理 | `http://127.0.0.1:8080` |
| HTTPS 代理 | `http://127.0.0.1:8080`（CONNECT 隧道） |
| SOCKS5 代理 | `socks5://127.0.0.1:1080` |
| Web 网关 | `http://127.0.0.1:8090` |

### 方式四：Web 管理界面

浏览器打开 [http://localhost:8090](http://localhost:8090)，可以看到：

- 代理地址与状态一目了然
- 一键跳转测试（百度、哔哩哔哩、IP 查询等）
- 自定义 URL 跳转输入框
- PAC 地址快速复制

### 方式五：API 测试

Web 网关提供 `/api/fetch` 接口，可服务端抓取目标 URL 并返回状态码、响应头和内容：

```
GET http://localhost:8090/api/fetch?url=https://httpbin.org/ip
```

---

## 🧠 架构设计

```
┌─────────────┐     ┌─────────────────────────────────────┐
│   客户端     │     │          Go-Proxy 服务               │
│  (浏览器/App) │────▶                                      │
└─────────────┘     │  ┌────────────┐  ┌──────────────┐   │
                    │  │ :8080      │  │ :1080        │   │
                    │  │ HTTP/HTTPS │  │ SOCKS5       │   │
                    │  │  代理      │  │   代理       │   │
                    │  └─────┬──────┘  └──────┬───────┘   │
                    │        │                │           │
                    │  ┌─────▼──────────────────▼───────┐  │
                    │  │      传输层 (Transport)         │  │
                    │  │  · TCP 直连 (DefaultTransport) │  │
                    │  │  · QUIC 优先降级 (可选)        │  │
                    │  └──────────────────────────────┘  │
                    │                                      │
                    │  ┌──────────────────────────────┐   │
                    │  │ :8090                        │   │
                    │  │ Web 管理网关                  │   │
                    │  │  ├── GET /        → 管理首页  │   │
                    │  │  ├── /proxy.pac  → PAC 脚本   │   │
                    │  │  ├── /api/fetch  → 抓取 API   │   │
                    │  │  ├── /api/diag   → 诊断信息   │   │
                    │  │  └── CONNECT → QUIC优先+TCP  │   │
                    │  └──────────────────────────────┘   │
                    │                                      │
                    │  ┌──────────────────────────────┐   │
                    │  │ :1443 (可选)                  │   │
                    │  │ QUIC 代理 (服务端模式)        │   │
                    │  └──────────────────────────────┘   │
                    │                                      │
                    │  ┌──────────────────────────────┐   │
                    │  │ ProxyCtl (Windows代理管理)    │   │
                    │  │  · 启动时保存并启用系统代理   │   │
                    │  │  · 退出时恢复原始代理配置     │   │
                    │  │  · 通过注册表 + WinINET API   │   │
                    │  └──────────────────────────────┘   │
                    └─────────────────────────────────────┘
```

### 核心模块

| 包路径 | 说明 |
|--------|------|
| `main.go` | 入口，协调各服务启动与优雅关闭 |
| `internal/proxy/http.go` | HTTP/HTTPS 正向代理实现 |
| `internal/proxy/socks5.go` | SOCKS5 协议完整实现 |
| `internal/proxy/quic.go` | QUIC 出站客户端 |
| `internal/proxy/quic_server.go` | QUIC 入站服务端 |
| `internal/proxy/transport.go` | 默认 HTTP 传输层（连接池） |
| `internal/gateway/web.go` | Web 管理网关（策略层） |
| `internal/proxyctl/` | 系统代理控制（Windows/跨平台） |
| `internal/utils/relay.go` | 双向数据转发 |
| `internal/utils/copyheaders.go` | HTTP 头复制（过滤逐跳头） |

---

## 🔌 协议详解

### HTTP/HTTPS 代理

- **普通 HTTP 请求**：通过 `RoundTrip` 将请求转发到目标服务器，响应透传回客户端
- **CONNECT 隧道**：通过 `Hijack` 劫持连接，建立客户端到目标的 TCP 双向转发
- 自动过滤 `Proxy-Connection`、`Transfer-Encoding` 等逐跳头

### SOCKS5 代理

完整的 SOCKS5 协议实现：

1. **认证协商**：支持无认证方式（`0x00`）
2. **请求解析**：支持 `CONNECT` 命令（`0x01`）
3. **地址类型**：IPv4（`0x01`）、域名（`0x03`）、IPv6（`0x04`）
4. **数据转发**：认证成功后建立双向 TCP 隧道

### QUIC 代理

基于 [quic-go](https://github.com/quic-go/quic-go) 库实现：

- **QUIC 客户端**（`QUIC`）：作为出站连接的后端传输层，通过 UDP 连接目标服务器，跳过 TLS 证书验证
- **QUIC 服务端**（`QUICServer`）：监听 UDP 端口，接收 QUIC 流，解析目标地址后转发到 TCP 目标
- **QUIC 优先策略**：在 Web 网关中，CONNECT 请求先尝试 1 秒内建立 QUIC 连接，失败则自动降级到 TCP

---

## 🛡 策略说明

### Web 网关策略路由

Web 网关是代理的**策略层**，根据请求类型和 QUIC 可用性智能路由：

```
请求进入 Web 网关
  │
  ├── GET /           → serveHome (管理首页)
  ├── GET /proxy.pac  → servePAC (PAC 脚本)
  ├── GET /api/diag   → serveDiag (诊断信息)
  ├── GET /api/fetch  → serveAPIFetch (服务端抓取)
  │
  ├── CONNECT + QUIC 可用 → handleTunnelQUIC (QUIC 优先)
  │     ├── QUIC 连接成功 ✅ → RelayConns
  │     └── QUIC 连接失败 ❌ → 降级到 TCP
  │
  └── 其他 (HTTP/非QUIC CONNECT) → HTTP 代理 (TCP)
```

### Windows 系统代理管理

通过修改注册表 `HKCU\Software\Microsoft\Windows\CurrentVersion\Internet Settings` 实现：

- **启动时**：保存当前代理设置 → 启用代理并广播变更通知
- **退出时**：恢复原始代理设置 → 广播变更通知
- **跨平台**：非 Windows 系统自动跳过代理设置，仅打印日志

---

## 🧪 测试

```bash
# 运行所有测试
go test ./...

# 运行特定包测试
go test ./internal/proxy/...
go test ./internal/utils/...

# 查看测试覆盖率
go test -cover ./...
```

测试覆盖：

- `internal/proxy/`：HTTP 转发、CONNECT 隧道、SOCKS5 全流程（认证/请求/地址解析/回复）
- `internal/utils/`：双向数据转发、Context 取消传播、超时处理
- `main_test.go`：端到端集成测试

---

## 📂 项目结构

```
go-proxy/
├── main.go                          # 入口，服务协调与优雅关闭
├── main_test.go                     # 集成测试
├── go.mod / go.sum                  # Go 模块依赖
├── Dockerfile                       # 多阶段 Docker 构建
├── docker-compose.yml               # Docker Compose (Go + Nginx)
├── .gitignore
├── nginx/
│   └── nginx.conf                   # Nginx 反向代理配置
└── internal/
    ├── gateway/
    │   └── web.go                   # Web 管理网关（策略层）
    ├── proxy/
    │   ├── http.go                  # HTTP/HTTPS 正向代理
    │   ├── http_test.go             # HTTP 代理测试
    │   ├── socks5.go                # SOCKS5 代理
    │   ├── socks5_test.go           # SOCKS5 测试
    │   ├── quic.go                  # QUIC 客户端
    │   ├── quic_server.go           # QUIC 服务端
    │   └── transport.go             # 默认 HTTP 传输层
    ├── proxyctl/
    │   ├── proxyctl.go              # 代理控制接口
    │   ├── proxyctl_windows.go      # Windows 实现（注册表）
    │   └── proxyctl_other.go        # 非 Windows 空实现
    └── utils/
        ├── relay.go                 # 双向数据转发
        ├── relay_test.go            # 转发测试
        └── copyheaders.go           # HTTP 头复制工具
```

---

## 📜 依赖

| 依赖 | 用途 |
|------|------|
| [quic-go/quic-go](https://github.com/quic-go/quic-go) | QUIC 协议实现 |
| [golang.org/x/sys](https://golang.org/x/sys) | Windows 注册表操作 |
| [stretchr/testify](https://github.com/stretchr/testify) | 测试断言库 |

---

## ⚠️ 注意事项

1. **Windows 防火墙**：首次运行可能弹出防火墙允许提示，请允许 `:8080`、`:8090`、`:1080` 端口的入站连接
2. **浏览器缓存**：如果修改了代理配置但浏览器未生效，请重启浏览器
3. **QUIC 代理**：QUIC 功能默认注释（`main.go` 中 `quicProxy.Serve` 被注释），如需启用需取消注释
4. **生产部署**：建议使用 Docker + Nginx 方案，配置 HTTPS 证书和 Basic Auth 认证
5. **内网直连**：PAC 脚本和系统代理配置均包含内网地址旁路规则，避免内网流量走代理

---

## 📄 许可

本项目基于 MIT 许可证开源。