package http

import (
	"fmt"
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
}

func (rw *responseWriter) WriteHeader(code int) {
	if !rw.wroteHeader {
		rw.statusCode = code
		rw.wroteHeader = true
		rw.headers = rw.Header().Clone() // 记录头部快照
		rw.ResponseWriter.WriteHeader(code)
	}
}

func (rw *responseWriter) Write(data []byte) (int, error) {
	if !rw.wroteHeader {
		rw.WriteHeader(http.StatusOK)
	}
	size, err := rw.ResponseWriter.Write(data)
	rw.contentLength += size
	return size, err
}

func (rw *responseWriter) WriteError(code int, message string) {
	rw.errorMsg = message
	rw.WriteHeader(code)
	_, err := fmt.Fprintf(rw, message)
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
	}
}

// 响应包装器中间件
func responseWrapperMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Printf("DEBUG: responseWrapperMiddleware executed - wrapping ResponseWriter")
		rw := &responseWriter{ResponseWriter: w}
		next(rw, r)
		log.Printf("DEBUG: responseWrapperMiddleware completed")
	}
}
