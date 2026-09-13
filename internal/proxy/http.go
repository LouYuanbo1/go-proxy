package proxy

import (
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/LouYuanbo1/go-proxy/internal/utils"
)

// HTTP 是 HTTP/HTTPS 正向代理的具体实现
type HTTP struct {
	transport *http.Transport
	timeout   time.Duration
}

// NewHTTP 创建 HTTP 正向代理实例
func NewHTTP(transport *http.Transport, timeout time.Duration) *HTTP {
	if transport == nil {
		transport = DefaultTransport
	}
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return &HTTP{
		transport: transport,
		timeout:   timeout,
	}
}

// Name 返回代理名称
func (hp *HTTP) Name() string {
	return "HTTP/HTTPS"
}

// ServeHTTP 实现 http.Handler 接口
func (hp *HTTP) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	log.Printf("[HTTP] 📥 %s %s (来自 %s)", r.Method, r.URL.String(), r.RemoteAddr)

	if r.Method == http.MethodConnect {
		hp.handleTunnel(w, r)
	} else {
		hp.handleHTTP(w, r)
	}
}

func (hp *HTTP) handleHTTP(w http.ResponseWriter, r *http.Request) {
	outReq, err := http.NewRequest(r.Method, r.URL.String(), r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	outReq.Host = r.Host
	utils.CopyHeaders(outReq.Header, r.Header)
	outReq.Header.Del("Proxy-Connection")

	resp, err := hp.transport.RoundTrip(outReq)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	utils.CopyHeaders(w.Header(), resp.Header)
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

// Transport 返回内部传输层，供 gateway 等组件使用
func (hp *HTTP) Transport() *http.Transport {
	return hp.transport
}

func (hp *HTTP) handleTunnel(w http.ResponseWriter, r *http.Request) {
	targetAddr := r.URL.Host
	if !strings.Contains(targetAddr, ":") {
		targetAddr += ":443"
	}

	targetConn, err := net.DialTimeout("tcp", targetAddr, hp.timeout)
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}

	hijacker, ok := w.(http.Hijacker)
	if !ok {
		targetConn.Close()
		http.Error(w, "Hijacking not supported", http.StatusInternalServerError)
		return
	}
	clientConn, _, err := hijacker.Hijack()
	if err != nil {
		targetConn.Close()
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}

	if _, err := clientConn.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n")); err != nil {
		return
	}

	if err := utils.RelayConns(r.Context(), clientConn, targetConn); err != nil {
		return
	}
}
