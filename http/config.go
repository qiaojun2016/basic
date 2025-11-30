package http

import (
	"github.com/qiaojun2016/basic/http/contextx"
	"log"
	"net/http"
)

// 服务器配置中间件
func createConfigMiddleware(config *contextx.Config, pattern *contextx.RoutePattern) Middleware {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			r = contextx.SetConfig(r, config)
			r = contextx.SetRoutePattern(r, pattern)
			log.Printf("DEBUG: createConfigMiddleware executed")
			next(w, r)
			log.Printf("DEBUG: createConfigMiddleware completed")
		}
	}
}
