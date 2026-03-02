# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This is a Go utility library (`github.com/qiaojun2016/basic`) — **not a standalone application**. It has no `main()` function and is meant to be imported by other projects.

**已知使用方：** `D:\go-projects\jifen-server`（积分管理服务），通过 `replace github.com/qiaojun2016/basic => ../basic` 本地引用。该项目注册了 155 个路由，修改 basic 库时需同步确认对 jifen-server 的兼容性。

## Commands

```bash
# Run all tests
go test ./...

# Run tests for a specific package
go test ./http/... -v
go test ./mysql/... -v

# Run a specific test function
go test -run TestFunctionName ./package/...

# Download dependencies
go mod tidy

# Build check (no binary produced, just verifies compilation)
go build ./...
```

## Architecture

### Module Structure

Each subdirectory is an independent package with its own concern. Packages follow the **singleton + `.Run()` pattern**: create a config struct, call `.Run()` to initialize a global instance.

```go
// Example initialization pattern used across packages
mysql.Server{DataSource: "user:pass@tcp(host:3306)/db", MaxOpen: 10}.Run()
id.Server{Node: 1}.Run()
redis.Server{Addr: "localhost:6379"}.Run()
```

### HTTP Framework (`http/`)

#### 文件职责

| 文件 | 职责 |
|---|---|
| `http.go` | Server 定义、IP 限流、路由绑定、核心 handler |
| `middlewares.go` | `authMiddleware` — token 解析、版本校验、HMAC 签名验证 |
| `bodyParse.go` | `BodyParsingMiddleware` — 读取请求体存入 context |
| `bodySign.go` | `BodySigningMiddleware` — handler 返回后对响应体签名写入 `Content-Sign` header |
| `cache.go` | `ResponseCacheMiddleware` — Redis 响应缓存读写 |
| `cors.go` | `CORSMiddleware` — 跨域，仅 `Server.Web=true` 时启用 |
| `responseWriter.go` | 缓冲型 `responseWriter` + `responseWrapperMiddleware` |
| `config.go` | `createConfigMiddleware` — 将 Config/RoutePattern 注入 context |
| `route/pattern.go` | Pattern 枚举常量定义 |
| `route/route.go` | 全局路由表（`map[string]Route`）、4 种 handle 签名、注册方法 |
| `contextx/context.go` | context 存取函数（Auth、RoutePattern、Config、RequestBody） |
| `model/auth.go` | 请求体认证字段结构：`t`(token) / `d`(deviceId) / `v`(version) |

#### 中间件执行顺序（由外到内）

`http.go:584-614` 按正序遍历逐层包装，最后添加的最先执行：

```
请求
 │
 ▼  1. CORSMiddleware            （Web=true 时，最外层）
 │
 ▼  2. responseWrapperMiddleware  （创建缓冲 responseWriter，最后 Flush() 统一写出）
 │
 ▼  3. configMiddleware           （注入 Config、RoutePattern、Logger 到 context）
 │
 ▼  4. BodyParsingMiddleware      （读取 body 存入 context；GET 转 URL 参数为 JSON）
 │
 ▼  5. authMiddleware             （解析 token → 校验版本 → 校验 HMAC → SetAuth 到 context）
 │
 ▼  6. BodySigningMiddleware      （next 返回后读 rw.body，计算 HMAC 写 Content-Sign header）
 │
 ▼  7. ResponseCacheMiddleware    （命中 Redis 直接返回；否则 next 后写入缓存）
 │
 ▼  8. h.Middlewares（用户自定义）
 │
 ▼  9. 核心 handler
```

#### 缓冲型 responseWriter

`responseWriter` 将所有写操作缓冲到内存 `bytes.Buffer`，`WriteHeader` 只记录状态码，由 `responseWrapperMiddleware` 在最外层调用 `Flush()` 统一写入连接。

**目的**：让 `BodySigningMiddleware` 能在 handler 执行完成后读取完整响应体再计算 HMAC，因为 HTTP header 必须在 body 之前发送。

#### 认证流程

请求格式：
```
Header: Content-Sign: <HMAC-SHA256>
Body:   { "t": "<token>", "d": "<deviceId>", "v": <version>, ...业务字段 }
```

`authMiddleware` 处理步骤（`middlewares.go:16`）：
1. 检查 `Content-Sign` header 存在（Auth 路由必须）
2. 从 context 取 body，`json.Unmarshal` 提取 `t`/`d`/`v`
3. 校验客户端版本 ≥ 路由要求版本
4. `token.Decode(t)` 解出 Uid、Session、AccessKeyID
5. `cipher.CheckSign(sig, body, ak)` HMAC-SHA256 校验
6. `contextx.SetAuth` 将解析结果存入 context

#### 路由注册

4 种 handle 签名，优先级从高到低：

```go
// 优先级 1：携带 UserAgent + uid
route.Route{Url: "/x"}.UserAgentRegister(func(agent, uid string, body []byte) (interface{}, error) {...})
// 优先级 2：携带 session
route.Route{Url: "/x"}.SessionRegister(func(session string, body []byte) (interface{}, error) {...})
// 优先级 3：携带 IP + uid
route.Route{Url: "/x"}.IpRegister(func(ip, uid string, body []byte) (interface{}, error) {...})
// 优先级 4：标准
route.Route{Url: "/x"}.Register(func(uid string, body []byte) (interface{}, error) {...})
```

Pattern 默认值（`route.go:49-68`）：Auth=Enable、Cache=Disable、Encrypt=Disable、UserAgent=Enable、General=Disable。

`General=Enable` 时 handler 必须返回 `[]byte`，框架直接输出，跳过 `{"version","state","data"}` 包装。

#### 缓存键设计（`cache.go`）

```
Redis Hash:  Key=pattern路径，Field=body去掉token和deviceId后的原始JSON，Value=完整响应
```

去掉 token/deviceId 的目的：不同用户相同业务参数可命中同一缓存（需 `redis.Server` 已初始化）。

#### IP 限流（`http.go:86`）

两级防护，均在中间件链之外，最先执行：
- **计数封禁**：10 分钟内请求 > 2000 次 → 429
- **令牌桶**：`rate.Limiter.Allow()` 失败 → 429 无 body 丢弃

每 10 分钟清理：超时不活跃 IP 删除，活跃 IP 计数归零。

#### 已知问题

- **WriteTimeout bug**（`http.go:165`）：`h.WriteTimeout==0` 时错误地赋值给 `h.ReadTimeout`
- **body 大小限制**（`bodyParse.go:41`）：硬编码 `1<<20`，未使用 `Server.MaxPayloadBytes`
- **DEBUG 日志无条件输出**（`bodyParse.go:18` 等）：未检查 `config.Debug`，生产环境也打印
- **`http.go` 约 200 行注释代码**：中间件拆分前的旧版实现残留

### Authentication & Token (`token/`)

Tokens are 24-byte structures (8 bytes timestamp + 8 bytes user ID + 8 bytes session ID), Base64-encoded. HMAC-SHA256 signing uses `AccessKeyID()` derived from the token.

### Database (`mysql/`)

Uses `sqlx` (not a full ORM). Key design: transaction lifecycle managed via `TxBegin()`/`TxEnd()`/`TxAuto()`. Stored procedures are the primary query mechanism (`TxExecProc`, `TxQueryProc`). Use `defer RowsCloseAndTxEnd()` for cleanup.

### ID Generation (`id/`)

Snowflake algorithm with Base58 encoding. `id.SId.String()` returns Base58, `id.SId.Int()` returns int64. Requires unique node number per server instance.

### Key Global Singletons

| Package | Global | After `.Run()` |
|---------|--------|----------------|
| `mysql` | `mysql.Mysql` | Ready for queries |
| `redis` | `redis.Redis` | Ready for operations |
| `id`    | `id.SId`      | Ready for ID generation |
| `ws`    | `ws.WS`       | WebSocket server |

### Route Registration

Routes have flags that activate middleware behavior:
- `Auth` — validates token in header
- `Cache` — caches response in Redis
- `Encrypt` — encrypts response body
- `UserAgent` — enforces `Server.UserAgent` check
- `General` — raw handler, bypasses response wrapper

### WebSocket (`ws/`)

Token-authenticated connections. Callbacks: `OnConn(uid, data)`, `OnMessage(data)`, `OnClose(id)`. Uses gorilla/websocket with ping/pong keep-alive.

### External Integrations

- **Alibaba Cloud:** `oss/` (object storage), `dysms/` (SMS), `nlp/` (NLP), `mqtt/` (IoT)
- **Maps:** `amap/` (高德地图 Amap/Gaode)
- **Notifications:** `jiguang/` (极光推送 JPush)
- **Other:** `wechat/`, `logistic/` (courier tracking), `reptile/` (web scraping via chromedp)
