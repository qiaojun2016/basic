package http

import (
	"github.com/qiaojun2016/basic/http/contextx"
	"net/http"
)

// 服务器配置中间件
func createConfigMiddleware(config *contextx.Config, pattern *contextx.RoutePattern) Middleware {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			r = contextx.SetConfig(r, config)
			r = contextx.SetRoutePattern(r, pattern)
			r = contextx.SetLogger(r, config.Debug) // 注入
			next(w, r)
		}
	}
}
