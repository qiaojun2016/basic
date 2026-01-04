package contextx

import (
	"context"
	"github.com/qiaojun2016/basic/http/route"
	"net/http"
)

// 私有类型，防止冲突
type authContextKey struct{}
type routePatternContextKey struct{}
type configContextKey struct{}
type contextKey string

const (
	requestBodyKey contextKey = "request_body"
	ParsedJSONKey  contextKey = "parsed_json"
)

// auth 结构体
type Auth struct {
	Uid      int64
	Session  int64
	Ak       []byte
	Token    string
	DeviceId string
	Version  int64
}

// routePattern 结构体示例
type RoutePattern struct {
	Pattern     string
	Version     int64
	Auth        route.PatternType //认证
	Cache       route.PatternType //缓存
	CacheExpire int64             //缓存保留时间单位秒，当Cache开启的时候有效
	Encrypt     route.PatternType //加密
	UserAgent   route.PatternType //user-agent
	General     route.PatternType //通用模式

}

type Config struct {
	Debug bool
}

func SetConfig(r *http.Request, c *Config) *http.Request {
	ctx := r.Context()
	ctx = context.WithValue(ctx, configContextKey{}, c)
	return r.WithContext(ctx)
}

func GetConfig(r *http.Request) *Config {
	v := r.Context().Value(configContextKey{})
	if a, ok := v.(*Config); ok {
		return a
	}
	return nil
}

// 写入 context 的辅助函数
func SetAuth(r *http.Request, a *Auth) *http.Request {
	ctx := r.Context()
	ctx = context.WithValue(ctx, authContextKey{}, a)
	return r.WithContext(ctx)
}

func GetAuth(r *http.Request) *Auth {
	v := r.Context().Value(authContextKey{})
	if a, ok := v.(*Auth); ok {
		return a
	}
	return nil
}

func SetRoutePattern(r *http.Request, pattern *RoutePattern) *http.Request {
	ctx := r.Context()
	ctx = context.WithValue(ctx, routePatternContextKey{}, pattern)
	return r.WithContext(ctx)
}

func GetRoutePattern(r *http.Request) *RoutePattern {
	v := r.Context().Value(routePatternContextKey{})
	if p, ok := v.(*RoutePattern); ok {
		return p
	}
	return nil
}

func SetRequestBody(r *http.Request, body []byte) *http.Request {
	ctx := r.Context()
	ctx = context.WithValue(ctx, requestBodyKey, body)
	return r.WithContext(ctx)
}

func GetRequestBody(r *http.Request) []byte {
	v := r.Context().Value(requestBodyKey)
	if p, ok := v.([]byte); ok {
		return p
	}
	return nil
}

func SetLogger(r *http.Request, debug bool) *http.Request {
	l := &Logger{debug: debug}
	ctx := context.WithValue(r.Context(), "logger_key", l)
	return r.WithContext(ctx)
}

func L(ctx context.Context) *Logger {
	if l, ok := ctx.Value("logger_key").(*Logger); ok {
		return l
	}
	// 返回一个默认的，防止空指针
	return &Logger{debug: false}
}
