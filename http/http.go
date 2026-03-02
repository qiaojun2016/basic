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
		LogTiming       bool //是否记录请求耗时
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
		h.WriteTimeout = 5
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
							return
					}
					//限流

					if !ipItem.limiter.Allow() {
						//抛弃多余流量
						errStr := fmt.Sprintf("%s请求过快", realIp)
						log.Println(errStr)
							rw.WriteHeader(http.StatusTooManyRequests)
						return
					}
				}

				//处理header
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
		
							rw.WriteError(http.StatusForbidden, errStr)

							return
						}

					}

				}

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
							rw.WriteError(http.StatusInternalServerError, errStr)
						return
					}
					if route.ContentType != "" {
						rw.Header().Set("Content-Type", route.ContentType)
					}
					rw.WriteHeader(http.StatusOK)

					res := result.([]byte)
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

				}

				//json错误
				if err != nil {
					errStr := fmt.Sprintf("%s : %s", pattern, err)
					fmt.Println(errStr)
					rw.WriteError(http.StatusInternalServerError, errStr)
					return
				}


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
				createResponseWrapperMiddleware(pattern, h.LogTiming),
			)
			if h.Web {
				middlewares = append(middlewares, CORSMiddleware(h.CorsCfg))
			}
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
