package http

import (
	"github.com/qiaojun2016/basic/cipher"
	"github.com/qiaojun2016/basic/http/contextx"
	"github.com/qiaojun2016/basic/http/route"
	"log"
	"net/http"
)

func BodySigningMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rw := w.(*responseWriter) // 类型断言获取包装器
		rp := contextx.GetRoutePattern(r)
		a := contextx.GetAuth(r)
		config := contextx.GetConfig(r)
		if config != nil && config.Debug {
			log.Printf("DEBUG: BodySigningMiddleware executed")
		}
		next(rw, r)
		if rp.Auth == route.Enable {
			responseSig := cipher.Sign(rw.body.Bytes(), a.Ak)
			//写入header
			rw.Header().Set(contentSign, responseSig)
		}
		if config != nil && config.Debug {
			log.Printf("DEBUG: BodySigningMiddleware completed")
		}
	}
}
