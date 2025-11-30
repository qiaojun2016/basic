package http

import (
	"bytes"
	"fmt"
	"github.com/qiaojun2016/basic/http/contextx"
	"github.com/qiaojun2016/basic/http/model"
	"github.com/qiaojun2016/basic/http/route"
	"github.com/qiaojun2016/basic/redis"
	"log"
	"net/http"
)

// 响应包装器中间件
func ResponseCacheMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Printf("DEBUG: ResponseCacheMiddleware executed -  ResponseCacheMiddleware")
		rp := contextx.GetRoutePattern(r)

		paramByte := contextx.GetRequestBody(r)
		rw := w.(*responseWriter) // 类型断言获取包装器
		if rp.Cache == route.Enable {
			userAuth := model.FromContextAuth(contextx.GetAuth(r))
			//var bytes []byte
			result, cacheErr := getCache(userAuth, rp.Pattern, paramByte)
			//没找到
			if cacheErr != nil {
				//缓存穿透
				log.Println(fmt.Sprintf("%s : %s", rp.Pattern, "Cache Penetration"))
				log.Println(cacheErr)
				next.ServeHTTP(w, r)
				// 缓存
				cache(userAuth, rp.Pattern, paramByte, rw.body.Bytes())
				log.Printf("DEBUG:  ResponseCacheMiddleware cache completed")
			} else {
				// 找到了直接返回
				rw.WriteHeader(http.StatusOK)
				_, err := rw.Write(result)
				if err != nil {
					errStr := fmt.Sprintf("%s : %s", rp.Pattern, err)
					log.Println(errStr)
					return
				}
				return
			}
		} else {
			log.Printf("DEBUG: ResponseCacheMiddleware  ServeHTTP")
			next.ServeHTTP(w, r)
		}
		log.Printf("DEBUG:  ResponseCacheMiddleware completed")
	}
}

func getCache(userAuth *model.Auth, pattern string, param []byte) (result []byte, err error) {
	if redis.Redis != nil {
		//去掉auth
		param = bytes.Replace(param, []byte(userAuth.Token), []byte{}, 1)
		param = bytes.Replace(param, []byte(userAuth.DeviceId), []byte{}, 1)
		result, err = redis.Redis.HGet(pattern, string(param))
		if err != nil {
			log.Println(err)
			return
		}
		//log.Println("hit cache")
	} else {
		err = fmt.Errorf("redis not run")
		log.Println(err)
	}
	return
}
func cache(userAuth *model.Auth, pattern string, param, result []byte) {
	//去掉param的d和t
	if redis.Redis != nil {
		//去掉auth
		param = bytes.Replace(param, []byte(userAuth.Token), []byte{}, 1)
		param = bytes.Replace(param, []byte(userAuth.DeviceId), []byte{}, 1)
		if err := redis.Redis.HSet(pattern, string(param), result); err != nil {
			log.Println(err)
			return
		}
		//log.Println("cached")
	} else {
		log.Println("redis not run")
	}
}
