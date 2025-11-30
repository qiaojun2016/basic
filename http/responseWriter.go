package http

import (
	"bytes"
	"log"
	"net/http"
	"time"
)

type responseWriter struct {
	http.ResponseWriter
	statusCode    int
	wroteHeader   bool
	errorMsg      string
	headers       http.Header // 记录设置的头部
	contentLength int
	startTime     time.Time
	body          *bytes.Buffer
}

// 构造函数：确保所有字段正确初始化
func newResponseWriter(w http.ResponseWriter) *responseWriter {
	return &responseWriter{
		ResponseWriter: w,
		statusCode:     http.StatusOK, // 默认状态码
		wroteHeader:    false,
		headers:        make(http.Header),
		body:           &bytes.Buffer{},
		startTime:      time.Now(),
	}
}

func (rw *responseWriter) Header() http.Header {
	// 修复：如果 headers 为 nil，则初始化
	if rw.headers == nil {
		rw.headers = make(http.Header)
	}
	return rw.headers
}

func (rw *responseWriter) WriteHeader(code int) {
	if !rw.wroteHeader {
		rw.statusCode = code
		rw.wroteHeader = true
		rw.headers = rw.Header().Clone() // 记录头部快照
		//rw.ResponseWriter.WriteHeader(code)
	}
}

func (rw *responseWriter) Write(data []byte) (int, error) {
	if !rw.wroteHeader {
		rw.WriteHeader(http.StatusOK)
	}
	size, err := rw.body.Write(data)
	rw.contentLength += size
	return size, err
}

func (rw *responseWriter) WriteError(code int, message string) {
	rw.errorMsg = message
	rw.WriteHeader(code)
	// 修复：确保 body 已经初始化
	if rw.body == nil {
		rw.body = &bytes.Buffer{}
	}
	// 修复：直接写入到 body，而不是使用 fmt.Fprintf(rw, message)
	_, err := rw.body.WriteString(message)
	if err != nil {
		log.Printf("Error writing response: %v", err)
		return
	}
}

// ResponseInfo 获取完整的响应信息
func (rw *responseWriter) ResponseInfo() map[string]interface{} {
	return map[string]interface{}{
		"status_code":    rw.statusCode,
		"error_message":  rw.errorMsg,
		"content_length": rw.contentLength,
		"headers":        rw.headers,
		"wrote_header":   rw.wroteHeader,
		"body":           rw.body.String(),
	}
}

// Flush 真正写入数据到客户端
func (rw *responseWriter) Flush() {

	// 1. 复制所有头部到原始 ResponseWriter
	for key, values := range rw.headers {
		for _, value := range values {
			rw.ResponseWriter.Header().Set(key, value)
		}
	}

	// 2. 写入状态码（这会发送头部）
	rw.ResponseWriter.WriteHeader(rw.statusCode)

	// 3. 写入响应体
	if rw.body.Len() > 0 {
		rw.ResponseWriter.Write(rw.body.Bytes())
	}
}

// 响应包装器中间件
func responseWrapperMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Printf("DEBUG: responseWrapperMiddleware executed - wrapping ResponseWriter")
		rw := newResponseWriter(w)
		next(rw, r)
		// 发送全部响应结果
		rw.Flush()
		log.Printf("DEBUG: responseWrapperMiddleware completed")
	}
}
