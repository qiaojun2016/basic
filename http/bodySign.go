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
		log.Printf("DEBUG:BodySigningMiddleware executed -  BodySigningMiddleware")
		rw := w.(*responseWriter) // 类型断言获取包装器
		rp := contextx.GetRoutePattern(r)
		a := contextx.GetAuth(r)
		next(rw, r)
		if rp.Auth == route.Enable {
			responseSig := cipher.Sign(rw.body.Bytes(), a.Ak)
			//写入header
			rw.Header().Set(contentSign, responseSig)
		}
		log.Printf("DEBUG:  BodySigningMiddleware completed")
	}
}
