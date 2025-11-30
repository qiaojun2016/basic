package http

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/qiaojun2016/basic/http/contextx"
	"io"
	"log"
	"net/http"
)

// 定义 Context 键类型（避免键冲突）

// BodyParsingMiddleware 解析请求体并存入 Context
func BodyParsingMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Printf("DEBUG: BodyParsingMiddleware executed -   BodyParsingMiddleware")
		rw := w.(*responseWriter) // 类型断言获取包装器
		rp := contextx.GetRoutePattern(r)
		//请求数据
		var paramByte []byte
		var err error
		//根据方法不同处理参数
		if r.Method == http.MethodGet { //TODO get没有测试
			var m = make(map[string]string)
			for key, value := range r.URL.Query() {
				m[key] = value[0]
			}
			//TODO 论证这里会不会有错
			paramByte, err = json.Marshal(m)
			if err != nil {
				errStr := fmt.Sprintf("%s : %s", rp.Pattern, "读取url参数错误")
				fmt.Println(errStr)
				//http.Error(w, errStr, http.StatusInternalServerError)
				rw.WriteError(http.StatusInternalServerError, errStr)
				return
			}
		} else {
			//读body
			r.Body = http.MaxBytesReader(w, r.Body, int64(int64(1<<20)))
			paramByte, err = io.ReadAll(r.Body)
			if err != nil {
				errStr := fmt.Sprintf("%s : %s", rp.Pattern, "读取body错误")
				fmt.Println(errStr)
				rw.WriteError(http.StatusRequestEntityTooLarge, errStr)
				return
			}
		}

		if len(paramByte) == 0 {
			errStr := fmt.Sprintf("%s : %s", rp.Pattern, "body为空")
			log.Println(errStr)
			//http.Error(w, errStr, http.StatusNoContent)
			rw.WriteError(http.StatusNoContent, errStr)
			return
		}
		// 2. 恢复 r.Body，以便后续使用
		r.Body = io.NopCloser(bytes.NewBuffer(paramByte))

		r = contextx.SetRequestBody(r, paramByte)
		log.Println("DEBUG:   BodyParsingMiddleware body", string(paramByte))
		next(w, r)
		log.Printf("DEBUG:   BodyParsingMiddleware completed")
	}
}

func hasRequestBody(r *http.Request) bool {
	return r.Body != nil && r.Method != "GET" && r.Method != "HEAD"
}

func isJSONRequest(r *http.Request) bool {
	contentType := r.Header.Get("Content-Type")
	return contentType == "application/json" ||
		len(contentType) >= 16 && contentType[:16] == "application/json"
}
