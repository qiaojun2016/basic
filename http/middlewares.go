package http

import (
	"encoding/json"
	"fmt"
	"github.com/qiaojun2016/basic/cipher"
	"github.com/qiaojun2016/basic/http/contextx"
	"github.com/qiaojun2016/basic/http/model"
	"github.com/qiaojun2016/basic/http/route"
	"github.com/qiaojun2016/basic/token"
	"log"
	"net/http"
)

// auth 中间件，解析 auth 并写入 context
func authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		config := contextx.GetConfig(r)
		if config.Debug {
			log.Printf("DEBUG: authMiddleware executed - authMiddleware")
		}
		rw := w.(*responseWriter) // 类型断言获取包装器
		rp := contextx.GetRoutePattern(r)
		sig := r.Header.Get(contentSign)
		pattern := rp.Pattern
		if rp.Auth == route.Enable {
			if sig == "" {
				errStr := fmt.Sprintf("%s : %s", rp.Pattern, "缺少数据签名")
				fmt.Println(errStr)
				rw.WriteError(http.StatusForbidden, errStr)
				return
			}
		}
		/*
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
					errStr := fmt.Sprintf("%s : %s", pattern, "读取url参数错误")
					fmt.Println(errStr)
					//http.Error(w, errStr, http.StatusInternalServerError)
					rw.WriteError(http.StatusInternalServerError, errStr)
					return
				}
			} else {
				//读body
				r.Body = http.MaxBytesReader(w, r.Body, int64(1<<20))
				paramByte, err = io.ReadAll(r.Body)
				if err != nil {
					errStr := fmt.Sprintf("%s : %s", pattern, "读取body错误")
					fmt.Println(errStr)
					//http.Error(w, errStr, http.StatusRequestEntityTooLarge)
					rw.WriteError(http.StatusRequestEntityTooLarge, errStr)

					return
				}
				// 恢复body 方便后续调用使用
				// todo 认证相关的信息放到 header 里面
				r.Body = io.NopCloser(bytes.NewBuffer(paramByte))
			}*/

		paramByte := contextx.GetRequestBody(r)

		if len(paramByte) == 0 {
			errStr := fmt.Sprintf("%s : %s", pattern, "body为空")
			fmt.Println(errStr)
			//http.Error(w, errStr, http.StatusNoContent)
			rw.WriteError(http.StatusNoContent, errStr)
			return
		}

		var tId int64
		var tSession int64
		var ak []byte

		userAuth := &model.Auth{}
		// 提取 token、deviceId、version
		err := json.Unmarshal(paramByte, userAuth)
		if err != nil {
			errStr := fmt.Sprintf("%s : %s", pattern, err)
			fmt.Println(errStr)
			//http.Error(w, errStr, http.StatusInternalServerError)
			rw.WriteError(http.StatusInternalServerError, errStr)
			return
		}

		if userAuth.Version < rp.Version {
			//客户端版本太低
			errStr := fmt.Sprintf(
				"client version is %d, server version is %d. version is too low.",
				userAuth.Version, rp.Version,
			)
			fmt.Println(errStr)
			//http.Error(w, errStr, http.StatusGone)
			rw.WriteError(http.StatusGone, errStr)
			return
		}

		//认证
		if rp.Auth == route.Enable { //启用认证
			if userAuth.Token == "" {
				errStr := fmt.Sprintf("%s : %s", pattern, "缺少令牌")
				fmt.Println(errStr)
				///http.Error(w, errStr, http.StatusNotAcceptable)
				rw.WriteError(http.StatusNotAcceptable, errStr)
				return
			}

			//提起令牌内容
			tk := token.Token{}
			err = tk.Decode(userAuth.Token)
			if err != nil {
				errStr := fmt.Sprintf("%s : %s", pattern, "令牌错误")
				fmt.Println(errStr)
				//http.Error(w, errStr, http.StatusNotAcceptable)
				rw.WriteError(http.StatusNotAcceptable, errStr)
				return
			}

			tId = tk.Id
			tSession = tk.Session()
			ak = []byte(tk.AccessKeyID())

			//校验签名
			if !cipher.CheckSign(sig, paramByte, ak) {
				errStr := fmt.Sprintf("%s : %s", pattern, "指纹检验失败")
				fmt.Println(errStr)
				//http.Error(w, errStr, http.StatusNotAcceptable)
				rw.WriteError(http.StatusNotAcceptable, errStr)
				return
			}
			r = contextx.SetAuth(r, &contextx.Auth{
				Ak:      ak,
				Uid:     tId,
				Session: tSession,
				Token:   userAuth.Token,
			})
		}
		next(w, r)
	}
}
