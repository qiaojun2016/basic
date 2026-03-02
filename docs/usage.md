# basic 库使用文档

`github.com/qiaojun2016/basic` 是一个 Go 后端开发工具库，提供 HTTP 框架、数据库、缓存、ID 生成、定时任务等常用能力。

---

## 目录

- [安装](#安装)
- [初始化模式](#初始化模式)
- [HTTP 框架](#http-框架)
- [verify — 参数解析与校验](#verify--参数解析与校验)
- [MySQL](#mysql)
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

### 启动服务

```go
import (
    basicHttp "github.com/qiaojun2016/basic/http"
    _ "yourproject/api_http" // 触发路由注册的 init()
)

func main() {
    // ... 其他模块初始化 ...

    s := &basicHttp.Server{
        Addr:      ":8080",
        Web:       true, // 开启 CORS
        CorsCfg: &basicHttp.CORSConfig{
            AllowedOrigins: []string{
                "https://yourdomain.com",
            },
        },
        UserAgent: "yourapp-*",  // 限制 User-Agent 前缀，* 为通配符
        Rate:      10,           // 每秒令牌数（IP 限流）
        Burst:     15,           // 令牌桶容量
        LogTiming: true,         // 开启请求耗时日志
        Debug:     false,        // 开启后输出中间件 DEBUG 日志
    }.Run()
}
```

### 注册路由

路由通常在各业务模块的 `init()` 中注册，通过空白导入触发：

```go
// api_http/user.go
package api_http

import (
    "github.com/qiaojun2016/basic/http/route"
    "github.com/qiaojun2016/basic/verify"
)

func init() {
    // 标准 handle：参数为 uid（Base58 字符串）+ 请求体 JSON
    route.Route{Url: "/api/user/info"}.Register(
        func(uid string, body []byte) (interface{}, error) {
            req := &InfoReq{}
            if err := verify.Unmarshal(body, req); err != nil {
                return nil, err
            }
            return db.GetUserInfo(uid, req)
        },
    )

    // 携带客户端 IP 的 handle
    route.Route{Url: "/api/log/visit"}.IpRegister(
        func(ip, uid string, body []byte) (interface{}, error) {
            return db.RecordVisit(ip, uid)
        },
    )

    // 携带 session 的 handle
    route.Route{Url: "/api/session/check"}.SessionRegister(
        func(session string, body []byte) (interface{}, error) {
            return db.CheckSession(session)
        },
    )

    // 携带 User-Agent 的 handle
    route.Route{Url: "/api/user/login"}.UserAgentRegister(
        func(agent, uid string, body []byte) (interface{}, error) {
            return db.Login(agent)
        },
    )
}
```

### Pattern 配置

通过 `route.Route.Pattern` 控制路由行为，未设置时使用默认值：

```go
route.Route{
    Url: "/api/public/list",
    Pattern: route.Pattern{
        Auth:    route.AuthDisable,    // 关闭 token 认证（默认开启）
        Cache:   route.Enable,         // 开启 Redis 响应缓存（默认关闭）
        CacheExpire: 300,              // 缓存过期秒数（Cache 开启时有效）
        UserAgent: route.UserAgentDisable, // 关闭 UserAgent 校验（默认开启）
        General: route.Enable,         // 通用模式，handler 必须返回 []byte（默认关闭）
        Version: 2,                    // 要求客户端版本 >= 2（默认 0 不限制）
    },
}.Register(handler)
```

### 请求 / 响应格式

**请求（需认证的路由）：**
```json
// Header
Content-Sign: <HMAC-SHA256 签名>

// Body JSON
{
    "t": "<token>",
    "d": "<deviceId>",
    "v": 1,
    "field1": "value1"
}
```

**响应：**
```json
// 成功
{"version": 1, "state": "OK", "data": {...}}

// 失败（handler 返回 error）
{"version": 1, "state": "错误描述", "data": null}
```

**General 模式**（`General: route.Enable`）：handler 返回 `[]byte`，框架直接写出，不包装 JSON 格式，适合文件下载、SSE 等场景：

```go
route.Route{
    Url:         "/api/export",
    ContentType: "application/octet-stream",
    Pattern:     route.Pattern{General: route.Enable},
}.Register(func(uid string, body []byte) (interface{}, error) {
    data := generateFile()
    return data, nil // 必须返回 []byte
})
```

### 自定义全局中间件

```go
s := &basicHttp.Server{...}
s.UseGlobal(func(next http.HandlerFunc) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        // 在 handler 前执行
        next(w, r)
        // 在 handler 后执行
    }
})
s.Run()
```

### 从 Context 获取请求信息

在自定义中间件中可从 context 读取框架注入的数据：

```go
import "github.com/qiaojun2016/basic/http/contextx"

// 获取已认证的用户信息
auth := contextx.GetAuth(r)
if auth != nil {
    auth.Uid     // int64 用户 ID
    auth.Session // int64 session ID
    auth.Ak      // []byte HMAC 密钥
    auth.Token   // string 原始 token
}

// 获取请求体（已读取，可重复获取）
body := contextx.GetRequestBody(r)

// 获取路由 Pattern 配置
rp := contextx.GetRoutePattern(r)
rp.Pattern // 路由路径
rp.Auth    // Auth 标志
```

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
    DataSource: "user:password@tcp(127.0.0.1:3306)/dbname",
    MaxOpen:    20, // 最大连接数
}.Run()
```

连接字符串自动附加 `charset=utf8mb4&loc=Asia/Shanghai&parseTime=true&multiStatements=true`。

### 事务与存储过程

库以**存储过程**为主要查询方式，所有操作均在事务内执行。

**方式一：TxAuto（推荐）**

```go
err = mysql.TxAuto(func(rows *sql.Rows, tx *sql.Tx) error {
    // 执行（增删改）
    _, err := mysql.Mysql.TxExecProc(tx, "proc_add_user", userId, name)
    if err != nil {
        return err // 自动回滚
    }
    return nil // 自动提交
})
```

**方式二：手动事务**

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

**传入结构体参数：**

`TxExecProc` / `TxQueryProc` 支持直接传入结构体，会按字段顺序展开为存储过程参数：

```go
type AddReq struct {
    Name  string
    Score int64
}
mysql.Mysql.TxExecProc(tx, "proc_add", AddReq{Name: "张三", Score: 100})
// 等同于 CALL proc_add(?, ?)  → ("张三", 100)
```

**获取原始连接：**

```go
db := mysql.GetDb()          // *sqlx.DB
exec := mysql.GetDbExec()    // DBExec 接口
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
