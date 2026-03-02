# basic 库使用文档

`github.com/qiaojun2016/basic` 是一个 Go 后端开发工具库，提供 HTTP 框架、数据库、缓存、ID 生成、定时任务等常用能力。

---

## 目录

- [安装](#安装)
- [初始化模式](#初始化模式)
- [HTTP 框架](#http-框架)
  - [Server 配置字段](#server-配置字段)
  - [启动服务](#启动服务)
  - [路由注册](#路由注册)
  - [Pattern 配置](#pattern-配置)
  - [请求 / 响应格式](#请求--响应格式)
  - [请求耗时日志](#请求耗时日志logliming)
  - [Debug 模式](#debug-模式)
  - [IP 限流](#ip-限流)
  - [CORS 跨域](#cors-跨域)
  - [User-Agent 校验](#user-agent-校验)
  - [响应缓存](#响应缓存)
  - [自定义全局中间件](#自定义全局中间件)
  - [从 Context 获取请求信息](#从-context-获取请求信息)
  - [中间件执行顺序](#中间件执行顺序)
- [verify — 参数解析与校验](#verify--参数解析与校验)
- [MySQL](#mysql)
  - [初始化](#初始化-1)
  - [DBExec 接口](#dbexec-接口)
  - [WithTransaction 事务](#withtransaction-事务)
  - [查询耗时日志](#查询耗时日志)
  - [存储过程（旧方式）](#存储过程旧方式)
- [Redis](#redis)
- [ID 生成](#id-生成)
- [Token](#token)
- [定时任务](#定时任务)
- [WebSocket](#websocket)
- [OSS 对象存储](#oss-对象存储)
- [短信 dysms](#短信-dysms)

---

## 安装

```bash
go get github.com/qiaojun2016/basic
```

本地开发时可在 `go.mod` 中使用 replace 指向本地路径：

```
replace github.com/qiaojun2016/basic => ../basic
```

---

## 初始化模式

库中每个模块统一使用 **配置结构体 + `.Run()`** 的初始化方式，调用后生成一个全局单例可直接使用。所有 `.Run()` 均应在 `main()` 中按依赖顺序调用。

```go
// ID 服务必须最先初始化，Token 依赖它生成 session
id.Server{Node: 1}.Run()

mysql.Server{DataSource: "..."}.Run()
redis.Server{Addr: "localhost:6379"}.Run()
```

> `.Run()` 内部有重复初始化保护，多次调用只生效一次。

---

## HTTP 框架

### Server 配置字段

```go
basicHttp.Server{
    Addr            string      // 监听地址，默认 ":80"
    MaxPayloadBytes int         // 请求体最大字节数，默认 1MB（1<<20）
    MaxHeaderBytes  int         // 请求头最大字节数，默认 1MB
    Rate            rate.Limit  // IP 限流：每秒向令牌桶放入的令牌数，默认 10，-1 关闭限流
    Burst           int         // IP 限流：令牌桶容量，默认 15，-1 关闭限流
    ReadTimeout     int         // 读超时（秒），默认 5
    WriteTimeout    int         // 写超时（秒），默认 5
    Web             bool        // 是否开启 CORS，配合 CorsCfg 使用
    CorsCfg         *CORSConfig // CORS 白名单配置，Web=true 时有效
    UserAgent       string      // 允许的 User-Agent，支持 "prefix-*" 通配符
    Middlewares     []Middleware // 全局自定义中间件，通过 UseGlobal() 添加
    LogTiming       bool        // 是否打印每个请求的耗时日志
    Debug           bool        // 是否打印中间件 DEBUG 日志
}
```

### 启动服务

```go
import (
    basicHttp "github.com/qiaojun2016/basic/http"
    _ "yourproject/api_http" // 通过空白导入触发路由注册的 init()
    _ "yourproject/task"
)

func main() {
    id.Server{Node: 1}.Run()
    mysql.Server{DataSource: "..."}.Run()
    redis.Server{Addr: ":6379"}.Run()

    s := &basicHttp.Server{
        Addr:      ":8080",
        Web:       true,
        CorsCfg: &basicHttp.CORSConfig{
            AllowedOrigins: []string{
                "https://yourdomain.com",
                "http://localhost:3000",
            },
        },
        UserAgent:  "myapp-*",
        Rate:       10,
        Burst:      15,
        LogTiming:  true,
    }
    s.Run()
}
```

---

### 路由注册

路由在各业务模块的 `init()` 中注册，main.go 通过空白导入触发。同一 URL 重复注册会 panic。

#### 4 种 handle 签名

根据业务需要选择一种，同一路由只能注册一种。当一个路由同时需要多种信息时优先使用 `IpRegister`（含 uid）。

```go
// 1. 标准：uid + body（最常用）
route.Route{Url: "/api/user/info"}.Register(
    func(uid string, body []byte) (interface{}, error) { ... },
)

// 2. 携带客户端真实 IP：ip + uid + body
route.Route{Url: "/api/log/visit"}.IpRegister(
    func(ip, uid string, body []byte) (interface{}, error) { ... },
)

// 3. 携带 session：session + body
route.Route{Url: "/api/session/check"}.SessionRegister(
    func(session string, body []byte) (interface{}, error) { ... },
)

// 4. 携带 User-Agent：agent + uid + body
route.Route{Url: "/api/user/login"}.UserAgentRegister(
    func(agent, uid string, body []byte) (interface{}, error) { ... },
)
```

> `uid`、`session` 均为 Base58 编码的字符串（非 int64）。Auth 关闭时 `uid` 为空字符串。

#### 完整注册示例

```go
// api_http/user.go
package api_http

import (
    "github.com/qiaojun2016/basic/http/route"
    "github.com/qiaojun2016/basic/verify"
    "yourproject/service/db"
)

func init() {
    // 需要认证的接口（默认）
    route.Route{Url: "/api/user/info"}.Register(
        func(uid string, body []byte) (interface{}, error) {
            req := &db.UserInfoReq{}
            if err := verify.Unmarshal(body, req); err != nil {
                return nil, err
            }
            return db.GetUserInfo(uid)
        },
    )

    // 不需要认证的公开接口
    route.Route{
        Url: "/api/public/config",
        Pattern: route.Pattern{
            Auth:      route.AuthDisable,
            UserAgent: route.UserAgentDisable,
        },
    }.Register(
        func(_ string, body []byte) (interface{}, error) {
            return db.GetPublicConfig()
        },
    )
}
```

---

### Pattern 配置

每个路由通过 `Pattern` 字段控制中间件行为，所有字段均有默认值：

| 字段 | 类型 | 默认值 | 说明 |
|---|---|---|---|
| `Auth` | PatternType | `Enable`（开启） | token 认证 + HMAC 签名校验 |
| `Cache` | PatternType | `CacheDisable`（关闭） | 响应结果缓存到 Redis |
| `CacheExpire` | int64 | 0（不过期） | 缓存秒数，Cache 开启时有效 |
| `Encrypt` | PatternType | `EncryptDisable`（关闭） | 响应加密（保留字段）|
| `UserAgent` | PatternType | `Enable`（开启） | 校验请求的 User-Agent |
| `General` | PatternType | `GeneralDisable`（关闭） | 通用模式，handler 返回 `[]byte` |
| `Version` | int64 | 0（不限制） | 要求客户端版本号 >= 此值 |

```go
route.Route{
    Url: "/api/rank/list",
    Pattern: route.Pattern{
        Auth:        route.Enable,         // 需要认证（可省略，是默认值）
        Cache:       route.Enable,         // 开启缓存
        CacheExpire: 60,                   // 缓存 60 秒
        UserAgent:   route.UserAgentDisable, // 不校验 UA（如 H5 页面访问）
        Version:     3,                    // 要求客户端版本 >= 3
    },
}.Register(handler)
```

---

### 请求 / 响应格式

#### 认证路由（Auth=Enable）

**请求：**
```
Header:
  Content-Sign: <HMAC-SHA256(body字节, accessKeyID)>

Body JSON:
  {
    "t": "<token>",       // 登录后获得的 token
    "d": "<deviceId>",    // 设备 ID
    "v": 1,               // 客户端版本号
    ...业务字段
  }
```

**响应：**
```
Header:
  Content-Sign: <HMAC-SHA256(响应body字节, accessKeyID)>

Body JSON（成功）:
  {"version": 1, "state": "OK", "data": {...}}

Body JSON（失败，handler 返回 error）:
  {"version": 1, "state": "错误原因", "data": null}
```

#### 非认证路由（Auth=AuthDisable）

无需 `Content-Sign` header，body 中也无需 `t`/`d`/`v` 字段，可以为空 body 或纯业务 JSON：

```
Body JSON:
  {"phone": "138xxxx", "code": "123456"}
```

#### General 模式（General=Enable）

handler 必须返回 `[]byte`，框架直接写入响应体，不包装 JSON 格式，适用于文件下载、图片输出等：

```go
route.Route{
    Url:         "/api/file/download",
    ContentType: "application/octet-stream",
    Pattern: route.Pattern{
        General: route.Enable,
    },
}.Register(func(uid string, body []byte) (interface{}, error) {
    fileBytes, err := readFile()
    if err != nil {
        return nil, err
    }
    return fileBytes, nil // 必须是 []byte
})
```

---

### 请求耗时日志（LogTiming）

开启 `LogTiming: true` 后，每个请求完成时自动打印：

```go
s := &basicHttp.Server{
    Addr:      ":8080",
    LogTiming: true,
}
```

输出格式：
```
[HTTP] /api/user/login 200 43ms
[HTTP] /api/score/list 200 128ms
[HTTP] /api/user/info 401 2ms
```

字段说明：路由 URL、HTTP 状态码、完整处理耗时（从接收请求到响应写出）。

---

### Debug 模式

开启 `Debug: true` 后，每个请求会打印各中间件的执行日志，用于排查中间件调用顺序问题：

```go
s := &basicHttp.Server{
    Addr:  ":8080",
    Debug: true,
}
```

输出示例：
```
DEBUG: BodyParsingMiddleware executed
DEBUG: BodySigningMiddleware executed
DEBUG: ResponseCacheMiddleware executed
DEBUG: ResponseCacheMiddleware completed
DEBUG: BodySigningMiddleware completed
```

> Debug 日志仅在开发环境开启，生产环境关闭以避免性能损耗和日志污染。

---

### IP 限流

框架内置基于令牌桶的 IP 级限流，两级防护：

| 级别 | 触发条件 | 响应 |
|---|---|---|
| 高频封禁 | 10 分钟内同一 IP 请求超过 2000 次 | 429，返回错误信息 |
| 令牌桶 | 请求速率超过 `Rate` 令牌/秒 | 429，无 body 直接丢弃 |

```go
s := &basicHttp.Server{
    Rate:  10,   // 每秒产生 10 个令牌
    Burst: 15,   // 桶最多存 15 个令牌，允许短暂突发
}
```

设置 `Rate: -1, Burst: -1` 可完全关闭限流（适合内网服务）。

每 10 分钟自动清理：超过 10 分钟未活跃的 IP 记录删除，活跃 IP 计数归零。

---

### CORS 跨域

```go
s := &basicHttp.Server{
    Web: true, // 必须开启
    CorsCfg: &basicHttp.CORSConfig{
        AllowedOrigins: []string{
            "https://app.example.com",
            "http://localhost:3000",
        },
    },
}
```

- 仅 `AllowedOrigins` 中的域名可获得 `Access-Control-Allow-Origin` 响应头
- 自动处理 OPTIONS 预检请求（返回 204）
- 自动暴露 `Content-Sign` 头（供客户端读取响应签名）

---

### User-Agent 校验

```go
s := &basicHttp.Server{
    UserAgent: "myapp-*", // 允许所有 "myapp-" 前缀的 UA
}
// 或精确匹配
s := &basicHttp.Server{
    UserAgent: "myapp/1.0",
}
```

- `"prefix-*"` 格式：匹配所有以 `prefix-` 开头的 UA
- 精确字符串：完全匹配
- UA 为 `"dev tool"` 时始终放行（便于开发调试）
- 单个路由可通过 `Pattern.UserAgent = route.UserAgentDisable` 跳过校验

---

### 响应缓存

开启缓存需要 Redis 已初始化（`redis.Server{...}.Run()`）：

```go
route.Route{
    Url: "/api/rank/top10",
    Pattern: route.Pattern{
        Cache:       route.Enable,
        CacheExpire: 300, // 5 分钟
    },
}.Register(handler)
```

**缓存 key 规则：** 以路由 URL 为 Hash key，以请求 body（去掉 `t` 和 `d` 字段后）为 Hash field。不同用户携带相同业务参数时命中同一缓存。

---

### 自定义全局中间件

`UseGlobal` 添加的中间件在**所有路由**上生效，在内置中间件链（认证、签名、缓存等）之外执行：

```go
s := &basicHttp.Server{Addr: ":8080"}

// 示例：请求日志中间件
s.UseGlobal(func(next http.HandlerFunc) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        log.Printf("-> %s %s", r.Method, r.URL.Path)
        next(w, r)
        log.Printf("<- %s %s done", r.Method, r.URL.Path)
    }
})

s.Run()
```

> 多次调用 `UseGlobal` 可添加多个中间件，执行顺序与添加顺序相同。

---

### 从 Context 获取请求信息

框架通过 `context` 在中间件和 handler 之间传递数据，在自定义中间件中可按需读取：

```go
import "github.com/qiaojun2016/basic/http/contextx"

func myMiddleware(next http.HandlerFunc) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        // 获取已认证的用户信息（Auth=Enable 且认证通过后才有值）
        auth := contextx.GetAuth(r)
        if auth != nil {
            auth.Uid     // int64 用户 ID
            auth.Session // int64 session ID
            auth.Ak      // []byte HMAC 密钥
            auth.Token   // string 原始 token 字符串
        }

        // 获取请求体（BodyParsingMiddleware 执行后才有值）
        body := contextx.GetRequestBody(r)

        // 获取路由 Pattern 配置
        rp := contextx.GetRoutePattern(r)
        rp.Pattern // 路由 URL，如 "/api/user/info"
        rp.Auth    // Auth 标志
        rp.Cache   // Cache 标志

        // 获取服务配置
        cfg := contextx.GetConfig(r)
        cfg.Debug  // bool

        next(w, r)
    }
}
```

---

### 中间件执行顺序

了解执行顺序有助于在正确位置读取 context 数据：

```
请求到达
  │
  ├─ IP 限流（高频封禁 + 令牌桶）          ← 最先，在所有中间件之外
  │
  ▼ 进入中间件链
  1. CORSMiddleware                        ← Web=true 时
  2. responseWrapperMiddleware             ← 创建缓冲 ResponseWriter；Flush() 写出响应；LogTiming 在此输出
  3. configMiddleware                      ← 注入 Config、RoutePattern 到 context
  4. BodyParsingMiddleware                 ← 读取并缓存请求体到 context
  5. authMiddleware                        ← 解析 token、校验版本、校验 HMAC（Auth=Enable 时）
  6. BodySigningMiddleware                 ← 调用 next 后对响应体签名（Auth=Enable 时）
  7. ResponseCacheMiddleware               ← 命中缓存直接返回；否则执行 next 后写入缓存
  8. UseGlobal 自定义中间件               ← 按添加顺序
  │
  ▼
  核心 handler（业务代码）
```

> 在自定义中间件中使用 `contextx.GetAuth(r)` 时，此时 `authMiddleware` 已执行完毕，可以安全读取。

---

## verify — 参数解析与校验

将 JSON body 解析为结构体，同时校验 `required` 标签：

```go
import "github.com/qiaojun2016/basic/verify"

type LoginReq struct {
    Phone    string `json:"phone" required:"true"`
    Password string `json:"pwd"   required:"true"`
    Version  int64  `json:"v"`
}

func handler(uid string, body []byte) (interface{}, error) {
    req := &LoginReq{}
    if err := verify.Unmarshal(body, req); err != nil {
        return nil, err // 返回字段名 + "不得为空"
    }
    return db.Login(req)
}
```

**规则：**
- `required:"true"` 的基本类型（string/int 等）：零值报错
- `required:"true"` 的 Slice/Map：长度为 0 报错
- 支持嵌套 struct、`[]struct`

---

## MySQL

### 初始化

```go
mysql.Server{
    DataSource:         "user:password@tcp(127.0.0.1:3306)/dbname",
    MaxOpen:            20,    // 最大连接数
    LogTiming:          true,  // 开启查询耗时日志
    LogTimingThreshold: 200,   // 只打印耗时 >= 200ms 的查询/事务，0 表示全部打印
}.Run()
```

连接字符串自动附加 `charset=utf8mb4&loc=Asia/Shanghai&parseTime=true&multiStatements=true`。

---

### DBExec 接口

`DBExec` 是对数据库执行能力的统一抽象，由 `*sqlx.DB` 和 `*sqlx.Tx` 共同实现。DAO 层函数接受 `mysql.DBExec` 作为参数，可在事务和非事务场景下复用同一套代码。

```go
type DBExec interface {
    Exec(query string, args ...interface{}) (sql.Result, error)
    NamedExec(query string, arg interface{}) (sql.Result, error)
    Get(dest interface{}, query string, args ...interface{}) error
    Select(dest interface{}, query string, args ...interface{}) error
    Rebind(query string) string
}
```

**获取非事务执行器：**

```go
exec := mysql.GetDbExec() // 返回 DBExec，开启 LogTiming 时自动包装计时器
db   := mysql.GetDb()     // 返回原始 *sqlx.DB
```

**DAO 层写法（推荐）：**

```go
// dao/user.go

func GetUser(exec mysql.DBExec, id int64) (*User, error) {
    var u User
    err := exec.Get(&u, "SELECT * FROM user WHERE id = ?", id)
    return &u, err
}

func AddUser(exec mysql.DBExec, name string) error {
    _, err := exec.Exec("INSERT INTO user (name) VALUES (?)", name)
    return err
}

// NamedExec：使用结构体字段名作为参数
type AddUserReq struct {
    Name  string `db:"name"`
    Score int    `db:"score"`
}
func AddUserNamed(exec mysql.DBExec, req *AddUserReq) error {
    _, err := exec.NamedExec("INSERT INTO user (name, score) VALUES (:name, :score)", req)
    return err
}
```

---

### WithTransaction 事务

`WithTransaction` 开启一个事务，在 `fn` 内执行所有操作。`fn` 返回 `nil` 时自动提交，返回 `error` 或发生 `panic` 时自动回滚。

```go
func WithTransaction(ctx context.Context, fn func(tx *sqlx.Tx) error, dbs ...*sqlx.DB) error
```

**基本用法：**

```go
err := mysql.WithTransaction(ctx, func(tx *sqlx.Tx) error {
    // tx 实现了 DBExec 接口，可直接传给 DAO 函数
    if err := dao.AddUser(tx, "张三"); err != nil {
        return err // 触发回滚
    }
    if err := dao.DeductScore(tx, userId, 100); err != nil {
        return err // 触发回滚
    }
    return nil // 提交
})
```

**多表操作保证原子性：**

```go
err := mysql.WithTransaction(ctx, func(tx *sqlx.Tx) error {
    // 插入订单
    orderId, err := dao.InsertOrder(tx, order)
    if err != nil {
        return err
    }
    // 扣减库存
    if err := dao.DeductStock(tx, order.ProductId, order.Qty); err != nil {
        return err
    }
    // 记录流水
    return dao.InsertLog(tx, orderId, "create")
})
if err != nil {
    return nil, err
}
```

**测试场景注入 mockDb：**

`WithTransaction` 的第三个参数可传入自定义 `*sqlx.DB`，用于单元测试时注入测试数据库：

```go
// 测试代码
err := mysql.WithTransaction(ctx, func(tx *sqlx.Tx) error {
    return dao.AddUser(tx, "测试用户")
}, testDB) // 传入测试 DB
```

**注意事项：**

- `fn` 内不要自行调用 `tx.Commit()` 或 `tx.Rollback()`，由 `WithTransaction` 统一管理
- `fn` 内发生 `panic` 时会先回滚再重新抛出，不会吞掉 panic
- 事务内的操作通过 `tx`（`DBExec`）执行，与非事务代码共用同一套 DAO 函数

---

### 查询耗时日志

开启 `LogTiming: true` 后：

- **非事务查询**（通过 `GetDbExec()` 获取的执行器）：每条 SQL 单独计时，格式为：
  ```
  [MySQL] SELECT * FROM user WHERE id = ? 5ms
  [MySQL] INSERT INTO score (user_id, value) VALUES (?, ?) 230ms
  ```

- **事务**（`WithTransaction`）：记录从开启到提交/回滚的整体耗时，格式为：
  ```
  [MySQL] transaction 312ms
  ```

只有超过 `LogTimingThreshold`（ms）的查询才会打印。设为 `0` 则打印全部。

---

### 存储过程（旧方式）

以下为兼容旧代码保留的存储过程接口，新代码推荐使用 `WithTransaction` + `DBExec`。

**TxAuto：**

```go
err = mysql.TxAuto(func(rows *sql.Rows, tx *sql.Tx) error {
    _, err := mysql.Mysql.TxExecProc(tx, "proc_add_user", userId, name)
    if err != nil {
        return err // 自动回滚
    }
    return nil // 自动提交
})
```

**手动事务：**

```go
tx, err := mysql.Mysql.TxBegin()
if err != nil {
    return err
}
var rows *sql.Rows
defer mysql.Mysql.RowsCloseAndTxEnd(rows, tx, err)

rows, err = mysql.Mysql.TxQueryProc(tx, "proc_get_users", companyId)
if err != nil {
    return err
}
for rows.Next() {
    var u User
    rows.Scan(&u.Id, &u.Name)
}
```

`TxExecProc` / `TxQueryProc` 支持直接传入结构体，按字段顺序展开为存储过程参数：

```go
type AddReq struct {
    Name  string
    Score int64
}
mysql.Mysql.TxExecProc(tx, "proc_add", AddReq{Name: "张三", Score: 100})
// 等同于 CALL proc_add(?, ?)  → ("张三", 100)
```

---

## Redis

### 初始化

```go
redis.Server{
    Addr:     "127.0.0.1:6379",
    Password: "",
    DB:       0,
    Flush:    false, // true 时启动清空所有数据，慎用
}.Run()
```

### 常用操作

```go
// String
redis.Redis.Set("key", "value")
redis.Redis.Set("key", "value", 10*time.Minute) // 带过期时间
bytes, err := redis.Redis.Get("key")

// Hash
redis.Redis.HSet("hash", "field", "value")
bytes, err := redis.Redis.HGet("hash", "field")
exists, err := redis.Redis.HExists("hash", "field")
redis.Redis.HDel("hash", "field1", "field2")

// Hash + 结构体（自动 JSON 序列化）
type UserInfo struct { Name string; Score int }
redis.Redis.HSetStruct("users", userId, &UserInfo{Name: "张三", Score: 100})

var info UserInfo
redis.Redis.HGetStruct("users", userId, &info)

// Key 操作
exists, err := redis.Redis.Exists("key")
redis.Redis.Del("key1", "key2")

// 获取原生客户端（用于管道、发布订阅等高级操作）
client := redis.Redis.Client()
```

---

## ID 生成

基于 Snowflake 算法，多节点部署时每个节点需配置不同 `Node` 值（0-1023）。

### 初始化

```go
id.Server{Node: 1}.Run() // Node 取值 0-1023，同一集群内唯一
```

### 使用

```go
// 生成新 ID
strId := id.SId.String() // Base58 字符串，如 "5Kt7v3QmX"
int64Id := id.SId.Int()  // int64，如 1234567890123456

// 格式转换（数据库存 int64，接口传 Base58）
str := id.SId.ToString(int64Id) // int64 → Base58
num := id.SId.ToInt(strId)      // Base58 → int64
```

> **注意：** `id.Server` 必须在 `token` 模块使用前初始化，`token.Encode()` 内部调用 `id.SId.Int()` 生成 session。

---

## Token

Token 为 24 字节二进制（时间戳 8 + 用户 ID 8 + session 8），Base64 编码后传输。

### 生成 Token（登录时）

```go
import "github.com/qiaojun2016/basic/token"

tk := token.Token{Id: userId} // userId 为 int64
tokenStr := tk.Encode()       // 返回 Base64 字符串，存入客户端
```

### 解析 Token

```go
tk := token.Token{}
err := tk.Decode(tokenStr)
if err != nil {
    // token 格式非法
}

tk.Id          // int64 用户 ID
tk.Session()   // int64 session ID
tk.Timestamp() // int64 纳秒时间戳
tk.AccessKeyID() // string HMAC 密钥（用于签名校验）
```

> 框架的 `authMiddleware` 会自动完成 token 解析与 HMAC 校验，业务代码通常只在**登录接口**调用 `Encode()`。

---

## 定时任务

### 初始化

路由注册方式与 HTTP 相同，在 `init()` 中注册，通过空白导入触发：

```go
// task/daily.go
func init() {
    task.Task{
        Spec:      "0 0 1 * * *", // 每天凌晨 1 点（秒 分 时 日 月 周）
        Name:      "daily_reset",
        Immediate: false,          // true 表示启动时立即执行一次
    }.Register(func() {
        // 任务逻辑
        log.Println("执行每日重置任务")
    })
}
```

```go
// main.go
import _ "yourproject/task"

task.Server{}.Run() // Block: true 时阻塞主协程
```

### Cron 表达式格式

库使用 6 位 cron 表达式：`秒 分 时 日 月 周`

```
"*/5 * * * * *"    每 5 秒
"0 */10 * * * *"   每 10 分钟
"0 0 9 * * 1"      每周一 9:00
"0 0 1 1 * *"      每月 1 日 1:00
```

### 取消任务

```go
task.Task{Name: "daily_reset"}.Cancel()
```

---

## WebSocket

### 启动

```go
ws.Server{
    Addr:            ":8081",
    ReadBufferSize:  1024,
    WriteBufferSize: 1024,
    OnStart: func() error {
        // 服务启动前的准备工作
        return nil
    },
    OnConn: func(uid, data string) error {
        // 新连接建立时，uid 为用户 Base58 ID，data 为连接参数 d 字段
        log.Printf("用户 %s 已连接，data=%s", uid, data)
        return nil // 返回 error 则拒绝连接
    },
    OnMessage: func(data []byte) {
        // 收到消息时
        log.Printf("收到消息: %s", data)
    },
    OnClose: func(uid string) {
        // 连接断开时
        log.Printf("用户 %s 已断开", uid)
    },
    Block: false,
}.Run()
```

### 主动推送

```go
err := ws.WS.SendMessage(uid, []byte(`{"type":"notify","msg":"hello"}`))
if err != nil {
    // 用户不在线
}
```

### 客户端连接 URL 格式

```
ws://host:8081/?t=<token>&s=<签名>&sec=<秒时间戳>&d=<自定义数据>
```

签名计算：`HMAC-SHA256(sec + token + data, accessKeyID)`，有效期 15 秒。

---

## OSS 对象存储

基于阿里云 OSS。

### 初始化

```go
oss.Server{
    Endpoint:        "https://oss-cn-beijing.aliyuncs.com",
    AccessKeyId:     "your-key-id",
    AccessKeySecret: "your-key-secret",
    BucketName:      "your-bucket",
}.Run()
```

### 使用

```go
// 获取文件公开访问 URL
url := oss.Oss.GetURL("path/to/file.jpg")

// 上传 Base64 图片
path, err := oss.Oss.UploadBase64("images/avatar.jpg", base64Str)

// 上传网络图片
path, err := oss.Oss.UploadUrl("images/cover.jpg", "https://example.com/img.jpg")

// 生成前端直传签名（POST 表单上传）
uploadUrl, err := oss.Oss.PutSignPolicyFileIdURL(fileId)
// uploadUrl.Url, uploadUrl.OSSAccessKeyId, uploadUrl.Policy, uploadUrl.Signature, uploadUrl.Key

// 生成 PUT 签名 URL（前端直接 PUT）
signUrl, err := oss.Oss.PutSignFileIdURL(fileId)

// 获取原始 Bucket 对象（高级操作）
bucket := oss.GetBucket()
```

---

## 短信 dysms

基于阿里云短信服务，`Send` 方法自动生成 6 位验证码并发送。

### 初始化

```go
dysms.Server{
    AccessKeyId:     "your-key-id",
    AccessKeySecret: "your-key-secret",
    Dev:             false, // true 时不真实发送，仅打印验证码到日志
}.Run()
```

### 发送验证码

```go
code, err := dysms.Dysms.Send(
    "13800138000",       // 手机号
    "你的签名名称",        // 短信签名
    "SMS_000000000",     // 短信模板 CODE
)
// code 为生成的 6 位验证码，需自行存入 Redis 等待校验
if err != nil {
    return nil, err
}
// 将 code 存入 Redis，设置过期时间
redis.Redis.Set("sms:"+phone, code, 5*time.Minute)
```
