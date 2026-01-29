package http

import (
	"net/http"
)

func CORSMiddleware(cfg *CORSConfig) func(http.HandlerFunc) http.HandlerFunc {
	// 提前构建 origin 白名单，避免每次请求重复构建
	originSet := make(map[string]struct{}, len(cfg.AllowedOrigins))
	for _, o := range cfg.AllowedOrigins {
		originSet[o] = struct{}{}
	}
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")

			// ===== CORS 核心逻辑 =====
			if origin != "" {
				if _, ok := originSet[origin]; ok {
					w.Header().Set("Access-Control-Allow-Origin", origin)
					w.Header().Set("Vary", "Origin")
					w.Header().Set("Access-Control-Expose-Headers", "Content-Sign")
					// 如果以后需要 cookie，再打开
					// w.Header().Set("Access-Control-Allow-Credentials", "true")
				}
			}

			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set(
				"Access-Control-Allow-Headers",
				"Content-Type, Authorization, X-Requested-With, Content-Sign",
			)

			// 禁用缓存（你原有逻辑）
			w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
			w.Header().Set("Pragma", "no-cache")
			w.Header().Set("Expires", "0")

			// ===== 预检请求直接返回 =====
			if r.Method == http.MethodOptions {
				// ⚠️ 这里不要写 body
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next(w, r)
		}
	}
}
