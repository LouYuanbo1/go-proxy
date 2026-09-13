package utils

import "net/http"

// defaultHopHeaders 是默认的逐跳头，不能直接复制给客户端
var defaultHopHeaders = map[string]bool{
	"Accept-Encoding":     true, // Go Transport 自动管理压缩，不需要透传
	"Connection":          true,
	"Keep-Alive":          true,
	"Proxy-Authenticate":  true,
	"Proxy-Authorization": true,
	"Te":                  true,
	"Trailer":             true,
	"Transfer-Encoding":   true,
	"Upgrade":             true,
	"Proxy-Connection":    true,
}

// CopyHeaders 复制 HTTP 头，跳过逐跳头
func CopyHeaders(dst, src http.Header) {
	for key, values := range src {
		if defaultHopHeaders[key] {
			continue
		}
		for _, v := range values {
			dst.Add(key, v)
		}
	}
}
