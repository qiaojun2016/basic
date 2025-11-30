package http

import (
	"encoding/json"
	"fmt"
	"github.com/qiaojun2016/basic/color"
	"github.com/qiaojun2016/basic/http/contextx"
	. "github.com/qiaojun2016/basic/http/route"
	"github.com/qiaojun2016/basic/id"
	"github.com/qiaojun2016/basic/ip"
	"golang.org/x/time/rate"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

//返回数据格式
/*
head:{
	"content-sign":"signature",
}
body:{
	"state":"OK",
	"data":{
		"aaa":"bbb",
		"ccc":"ddd"
	}
}
*/

//收到数据格式
/*
head:{
	"content-sign":"signature",
}
body: {
	"t":"token",
	"d":"deviceId",
	"aaa":"bbb",
	"ccc":"ddd"
}
*/

const (
	contentSign     = "Content-Sign"   //指纹
	maxRequestCount = 2000             //存活周期内的最大请求数 1200
	dumpPeriod      = 10 * time.Minute //清理周期 10
	maxAliveTime    = 10 * time.Minute //存活周期 10
)

type (
	Server struct {
		Addr            string      //监听地址
		MaxPayloadBytes int         //最大消息长度
		MaxHeaderBytes  int         //最大head息长度
		Rate            rate.Limit  //每秒产生令牌的个数
		Burst           int         //令牌桶大小个数
		ReadTimeout     int         //读超时秒
		WriteTimeout    int         //写超时秒
		Web             bool        //是否是用于web，跨域
		UserAgent       string      //允许的UserAgent
		CorsCfg         *CORSConfig // cros配置，web 为 true  有效
		Middlewares     []Middleware
		Debug           bool
	}

	CORSConfig struct {
		AllowedOrigins []string
	}

	//response 返回数据
	response struct {
		Version int64       `json:"version"`
		State   string      `json:"state"`
		Data    interface{} `json:"data"`
	}

	iPItem struct {
		count    int           //访问次数
		lastDate time.Time     //最后的活跃时间
		limiter  *rate.Limiter //限流器
	}

	//iPRateLimiter ip限流
	iPRateLimiter struct {
		ips   map[string]*iPItem
		mu    *sync.RWMutex
		rate  rate.Limit //每秒像桶中放入的令牌数量
		burst int        //令牌桶大小
	}
)

type Middleware func(http.HandlerFunc) http.HandlerFunc

func (h *Server) UseGlobal(m Middleware) {
	h.Middlewares = append(h.Middlewares, m)
}
func (i *iPRateLimiter) ipLimiter(ip string) (ipItem *iPItem) {
	i.mu.Lock()
	ipItem, exists := i.ips[ip]
	if !exists { //不存在
		ipItem = &iPItem{
			limiter: rate.NewLimiter(i.rate, i.burst),
		}
		i.ips[ip] = ipItem
	}
	ipItem.lastDate = time.Now()
	ipItem.count++
	i.mu.Unlock()
	return ipItem
}

// dump 清除不活跃的ip，重置高频ip，释放内存
func (i *iPRateLimiter) dump() {
	ticker := time.NewTicker(dumpPeriod)
	go func() {
		for {
			select {
			case <-ticker.C:
				now := time.Now()
				//log.Println("触发清理")
				i.mu.Lock()
				for k, v := range i.ips {
					//判断是否在存活周期内
					if v.lastDate.Add(maxAliveTime).Before(now) { //不再周期内
						//清除不活跃ip
						delete(i.ips, k)
					} else { //在周期内
						//初始高频ip为0
						v.count = 0
					}

				}
				i.mu.Unlock()
			}
		}
	}()
}

// Run 启动服务
func (h *Server) Run() {
	//当不配置的时候，使用以下默认配置
	if h.Addr == "" {
		h.Addr = ":80"
	}
	if h.MaxPayloadBytes == 0 {
		h.MaxPayloadBytes = 1 << 20
	}
	if h.MaxHeaderBytes == 0 {
		h.MaxHeaderBytes = 1 << 20
	}
	if h.Rate == 0 {
		h.Rate = 10
	}
	if h.Burst == 0 {
		h.Burst = 15
	}
	if h.ReadTimeout == 0 {
		h.ReadTimeout = 5
	}
	if h.WriteTimeout == 0 {
		h.ReadTimeout = 5
	}

	//限流器
	iPLimiter := iPRateLimiter{
		ips:   make(map[string]*iPItem),
		mu:    &sync.RWMutex{},
		rate:  h.Rate,
		burst: h.Burst,
	}
	if h.Rate > 0 && h.Burst > 0 {
		iPLimiter.dump()
	}

	mux := http.NewServeMux()

	//执行路由表
	routeList := All()
	for s, r := range routeList {
		//闭包保存路由
		func(pattern string, route Route) {

			handler := func(w http.ResponseWriter, r *http.Request) {

				rw := w.(*responseWriter) // 类型断言获取包装器

				//关闭
				defer func() {
					_ = r.Body.Close()
				}()

				realIp := ip.XRealIp(r)

				if h.Rate > 0 && h.Burst > 0 {
					//阻止高频ip
					ipItem := iPLimiter.ipLimiter(realIp)
					if ipItem.count > maxRequestCount { //高频ip
						errStr := fmt.Sprintf("%s判定为高频请求ip", realIp)
						fmt.Println(errStr)
						rw.WriteError(http.StatusTooManyRequests, errStr)
						//http.Error(w, errStr, http.StatusTooManyRequests)
						return
					}
					//限流
					/*err := ipItem.limiter.Wait(context.Background())
					if err != nil {
						log.Printf("%s : %s", pattern, err)
						http.Error(w, err.Error(), http.StatusInternalServerError)
						return
					}*/
					if !ipItem.limiter.Allow() {
						//抛弃多余流量
						errStr := fmt.Sprintf("%s请求过快", realIp)
						log.Println(errStr)
						//http.Error(w, errStr, http.StatusTooManyRequests)
						rw.WriteHeader(http.StatusTooManyRequests)
						return
					}
				}

				if h.Web == true {
					//跨域
					originSet := make(map[string]struct{}, len(h.CorsCfg.AllowedOrigins))
					for _, o := range h.CorsCfg.AllowedOrigins {
						originSet[o] = struct{}{}
					}

					origin := r.Header.Get("Origin")
					if _, ok := originSet[origin]; ok {
						w.Header().Set("Access-Control-Allow-Origin", origin)
						w.Header().Set("Vary", "Origin")
						w.Header().Set("access-control-expose-headers", "Content-Sign")
						//w.Header().Set("Access-Control-Allow-Credentials", "true")
					}

					w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
					w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Requested-With, Content-Sign")
					w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
					w.Header().Set("Pragma", "no-cache")
					w.Header().Set("Expires", "0")

					if r.Method == http.MethodOptions {
						rw.WriteHeader(http.StatusOK)
						return
					}

				}

				//处理header
				//header := r.Header
				userAgent := r.Header.Get("User-Agent")

				if h.UserAgent != "" && route.Pattern.UserAgent == Enable {
					if userAgent != "dev tool" {
						agent := false
						if strings.HasSuffix(h.UserAgent, "-*") { //包含通配符
							ua := h.UserAgent[0 : len(h.UserAgent)-2]
							agent = !strings.HasPrefix(userAgent, ua)
						} else {
							agent = userAgent != h.UserAgent
						}

						if agent {
							errStr := fmt.Sprintf("%s : %s", pattern, "User-Agent 错误")
							fmt.Println(errStr)
							//http.Error(w, errStr, http.StatusForbidden)

							rw.WriteError(http.StatusForbidden, errStr)

							return
						}

					}

				}

				//检查签名
				/*
					sig := r.Header.Get(contentSign)
					if route.Pattern.Auth == Enable && !h.NoAuth {
						//有认证必须要校验签名
						if sig == "" {
							errStr := fmt.Sprintf("%s : %s", pattern, "缺少数据签名")
							fmt.Println(errStr)
							//http.Error(w, errStr, http.StatusForbidden)
							rw.WriteError(http.StatusForbidden, errStr)
							return
						}
					}*/

				//请求数据
				/*
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
						r.Body = http.MaxBytesReader(w, r.Body, int64(h.MaxPayloadBytes))
						paramByte, err = io.ReadAll(r.Body)
						if err != nil {
							errStr := fmt.Sprintf("%s : %s", pattern, "读取body错误")
							fmt.Println(errStr)
							//http.Error(w, errStr, http.StatusRequestEntityTooLarge)
							rw.WriteError(http.StatusRequestEntityTooLarge, errStr)

							return
						}
					}

					if len(paramByte) == 0 {
						errStr := fmt.Sprintf("%s : %s", pattern, "body为空")
						log.Println(errStr)
						//http.Error(w, errStr, http.StatusNoContent)
						rw.WriteError(http.StatusNoContent, errStr)
						return
					}*/

				//var tId int64
				//var tSession int64
				//var ak []byte
				/*
					userAuth := &auth{}
					//提取 token、deviceId、version
					err = json.Unmarshal(paramByte, userAuth)
					if err != nil {
						errStr := fmt.Sprintf("%s : %s", pattern, err)
						fmt.Println(errStr)
						//http.Error(w, errStr, http.StatusInternalServerError)
						rw.WriteError(http.StatusInternalServerError, errStr)
						return
					}*/

				//判断版本
				/*
					if userAuth.Version < route.Pattern.Version {
						//客户端版本太低
						errStr := fmt.Sprintf(
							"client version is %d, server version is %d. version is too low.",
							userAuth.Version, route.Pattern.Version,
						)
						fmt.Println(errStr)
						//http.Error(w, errStr, http.StatusGone)
						rw.WriteError(http.StatusGone, errStr)
						return
					}*/

				//认证
				/*
					if route.Pattern.Auth == Enable { //启用认证
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

					}*/

				a := contextx.GetAuth(r)
				uid := ""
				session := ""
				if a != nil {
					uid = id.SId.ToString(a.Uid)
					session = id.SId.ToString(a.Session)
					session = id.SId.ToString(a.Session)
				}
				paramByte := contextx.GetRequestBody(r)
				log.Println("http handler", string(paramByte))
				var err error
				/*
					userAuth := &auth{
						Token:    a.Token,
						DeviceId: a.DeviceId,
						Version:  a.Version,
					}
					//var jsonErr error

					// 查找缓存，缓存一定是正确的结果
					if route.Pattern.Cache == Enable {
						//var bytes []byte
						result, cacheErr := getCache(userAuth, pattern, paramByte)
						//没找到
						if cacheErr != nil {
							//缓存穿透
							log.Println(fmt.Sprintf("%s : %s", pattern, "Cache Penetration"))
							log.Println(cacheErr)
						} else {
							//找到了
							if route.Pattern.General == Enable {
								//通用不格式直接输出
								//输出
								_, err = w.Write(result)
								if err != nil {
									errStr := fmt.Sprintf("%s : %s", pattern, err)
									fmt.Println(errStr)
									return
								}
							} else {
								//签名输出
								if route.Pattern.Auth == Enable {
									responseSig := cipher.Sign(result, a.Ak)
									//写入header
									w.Header().Set(contentSign, responseSig)
								}
								w.WriteHeader(http.StatusOK)
								_, err = w.Write(result)
								if err != nil {
									errStr := fmt.Sprintf("%s : %s", pattern, err)
									log.Println(errStr)
									return
								}
							}
							return
						}
					}*/

				//执行
				var result interface{}
				//检查是否有特殊的handle
				//携带ip和id的Handle
				ipHandle := route.IpHandle()
				//携带session的Handle
				sessionHandle := route.SessionHandle()
				//携带User-Agent的Handle
				userAgentHandle := route.UserAgentHandle()

				//Handle
				if userAgentHandle != nil {
					result, err = userAgentHandle(userAgent, uid, paramByte)
				} else if sessionHandle != nil {
					result, err = sessionHandle(session, paramByte)
				} else if ipHandle != nil {
					result, err = ipHandle(realIp, uid, paramByte)
				} else {
					result, err = route.Handle()(uid, paramByte)
				}

				// 通用不格式直接输出
				if route.Pattern.General == Enable {

					//这里的错误是不格式化的错误
					if err != nil {
						errStr := fmt.Sprintf("%s : %s", pattern, err)
						fmt.Println(errStr)
						//http.Error(w, errStr, http.StatusInternalServerError)
						rw.WriteError(http.StatusInternalServerError, errStr)
						return
					}

					if result == nil {
						rw.WriteHeader(http.StatusOK)
						return
					}

					//判断是bytes
					switch value := result.(type) {
					case []byte:
					default:
						errStr := fmt.Sprintf("%v is not []byte or []uint8", value)
						fmt.Println(errStr)
						//http.Error(w, errStr, http.StatusInternalServerError)
						rw.WriteError(http.StatusInternalServerError, errStr)
						return
					}
					if route.ContentType != "" {
						rw.Header().Set("Content-Type", route.ContentType)
					}
					rw.WriteHeader(http.StatusOK)
					//w.WriteHeader(http.StatusOK)

					//缓存
					res := result.([]byte)
					/*
						if route.Pattern.Cache == Enable {
							cache(userAuth, pattern, paramByte, res)
						}*/
					//输出
					_, err = rw.Write(res)

					if err != nil {
						errStr := fmt.Sprintf("%s : %s", pattern, err)
						fmt.Println(errStr)
						return
					}
					return
				}

				// 处理结果
				var jsonBytes []byte

				// 这里的错误是经过格式化的错误
				if err != nil {
					errStr := fmt.Sprintf("%s : %s", pattern, err)
					fmt.Println(errStr)
					jsonBytes, err = json.Marshal(response{
						route.Pattern.Version,
						err.Error(),
						nil,
					})
				} else {
					jsonBytes, err = json.Marshal(response{
						route.Pattern.Version,
						"OK",
						result,
					})

					//缓存
					/*
						if route.Pattern.Cache == Enable {
							cache(userAuth, pattern, paramByte, jsonBytes)
						} */
				}

				//json错误
				if err != nil {
					errStr := fmt.Sprintf("%s : %s", pattern, err)
					fmt.Println(errStr)
					rw.WriteError(http.StatusInternalServerError, errStr)
					//http.Error(w, errStr, http.StatusInternalServerError)
					return
				}

				//TODO 判断是否使用gzip

				//计算hmac
				/*
					if route.Pattern.Auth == Enable && !h.NoAuth {
						responseSig := cipher.Sign(jsonBytes, a.Ak)
						//写入header
						w.Header().Set(contentSign, responseSig)
					}*/
				rw.WriteHeader(http.StatusOK)
				//写出结果
				_, err = rw.Write(jsonBytes)
				if err != nil {
					errStr := fmt.Sprintf("%s : %s", pattern, err)
					log.Println(errStr)
					return
				}
			}

			finalHandler := handler
			var middlewares []Middleware
			middlewares = append(middlewares, h.Middlewares...)
			// 最后添加的先执行
			rp := &contextx.RoutePattern{
				Pattern:     pattern,
				Auth:        route.Pattern.Auth,
				Cache:       route.Pattern.Cache,
				CacheExpire: route.Pattern.CacheExpire,
				Encrypt:     route.Pattern.Encrypt,
				UserAgent:   route.Pattern.UserAgent,
				General:     route.Pattern.General,
				Version:     route.Pattern.Version,
			}
			configMiddleware := createConfigMiddleware(&contextx.Config{Debug: h.Debug}, rp)
			middlewares = append(
				middlewares,
				ResponseCacheMiddleware,
				BodySigningMiddleware,
				authMiddleware,
				BodyParsingMiddleware,
				configMiddleware,
				responseWrapperMiddleware)
			for _, mw := range middlewares {
				finalHandler = mw(finalHandler)
			}
			mux.HandleFunc(pattern, finalHandler)
		}(s, r)
	}

	ips, err := ip.BoundLocalIP()
	if err != nil {
		log.Println(err)
		return
	}
	if len(ips) == 0 {
		ips = []string{
			"127.0.0.1",
		}
	}

	color.Success(fmt.Sprintf(
		"[http] %s listening http://%s%s ,routes total:%d,ip limit:%d/%gs",
		h.UserAgent,
		ips[0],
		h.Addr,
		len(routeList),
		h.Burst,
		h.Rate,
	))
	//启动服务
	server := &http.Server{
		Addr:           h.Addr,
		ReadTimeout:    time.Duration(h.ReadTimeout) * time.Second,
		WriteTimeout:   time.Duration(h.WriteTimeout) * time.Second,
		MaxHeaderBytes: h.MaxHeaderBytes,
		Handler:        mux,
	}
	err = server.ListenAndServe()
	if err != nil {
		log.Println("[http] Listen error!", err)
	}
}
