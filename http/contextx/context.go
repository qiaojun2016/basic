package contextx

import (
	"context"
	"net/http"
)

// 私有类型，防止冲突
type authContextKey struct{}
type routePatternContextKey struct{}

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
	Path    string
	Auth    bool
	Version int64
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
