package proxy

import (
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/LouYuanbo1/go-proxy/internal/utils"
	"github.com/stretchr/testify/assert"
)

func TestCopyHeaders(t *testing.T) {
	dst := http.Header{}
	src := http.Header{
		"Content-Type":      {"application/json"},
		"X-Custom":          {"value1", "value2"},
		"Connection":        {"keep-alive"},
		"Transfer-Encoding": {"chunked"},
		"Upgrade":           {"websocket"},
		"Proxy-Connection":  {"keep-alive"},
	}

	utils.CopyHeaders(dst, src)

	assert.Equal(t, "application/json", dst.Get("Content-Type"))
	assert.Equal(t, []string{"value1", "value2"}, dst["X-Custom"])
	assert.Empty(t, dst.Get("Connection"))
	assert.Empty(t, dst.Get("Transfer-Encoding"))
	assert.Empty(t, dst.Get("Upgrade"))
	assert.Empty(t, dst.Get("Proxy-Connection"))
}

func TestHandleHTTP(t *testing.T) {
	targetServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "GET", r.Method)
		assert.Equal(t, "test-value", r.Header.Get("X-Test"))
		w.Header().Set("X-Response", "ok")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("hello from target"))
	}))
	defer targetServer.Close()

	hp := NewHTTP(DefaultTransport, 10*time.Second)
	proxyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hp.handleHTTP(w, r)
	}))
	defer proxyServer.Close()

	req, err := http.NewRequest("GET", targetServer.URL, nil)
	assert.NoError(t, err)
	req.Header.Set("X-Test", "test-value")

	transport := &http.Transport{
		Proxy: func(req *http.Request) (*url.URL, error) {
			return url.Parse(proxyServer.URL)
		},
	}
	client := &http.Client{Transport: transport, Timeout: 5 * time.Second}

	resp, err := client.Do(req)
	assert.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "ok", resp.Header.Get("X-Response"))
	body, _ := io.ReadAll(resp.Body)
	assert.Equal(t, "hello from target", string(body))
}

func TestHandleHTTP_ErrorTarget(t *testing.T) {
	hp := NewHTTP(DefaultTransport, 10*time.Second)
	req := httptest.NewRequest("GET", "http://invalid-host-that-does-not-exist.local/test", nil)
	w := httptest.NewRecorder()
	hp.handleHTTP(w, req)
	assert.Equal(t, http.StatusBadGateway, w.Code)
}

func TestHandleTunnel_NoHijacker(t *testing.T) {
	hp := NewHTTP(DefaultTransport, 10*time.Second)
	req := httptest.NewRequest("CONNECT", "https://192.0.2.1:443", nil)
	w := httptest.NewRecorder()
	hp.handleTunnel(w, req)
	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}

func TestHandleTunnel_InvalidTarget(t *testing.T) {
	conn, err := net.DialTimeout("tcp", "127.0.0.1:1", 100*time.Millisecond)
	assert.Error(t, err)
	assert.Nil(t, conn)
}

func TestHandleHTTP_ChunkedResponse(t *testing.T) {
	targetServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Transfer-Encoding", "chunked")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("chunk1"))
		w.(http.Flusher).Flush()
		w.Write([]byte("chunk2"))
	}))
	defer targetServer.Close()

	hp := NewHTTP(DefaultTransport, 10*time.Second)
	proxyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hp.handleHTTP(w, r)
	}))
	defer proxyServer.Close()

	req, _ := http.NewRequest("GET", targetServer.URL, nil)
	transport := &http.Transport{
		Proxy: func(req *http.Request) (*url.URL, error) {
			return url.Parse(proxyServer.URL)
		},
	}
	client := &http.Client{Transport: transport, Timeout: 5 * time.Second}

	resp, err := client.Do(req)
	assert.NoError(t, err)
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	assert.Equal(t, "chunk1chunk2", string(body))
}

func TestHandleHTTP_POST(t *testing.T) {
	targetServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "POST", r.Method)
		body, _ := io.ReadAll(r.Body)
		assert.Equal(t, "request body", string(body))
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte("created"))
	}))
	defer targetServer.Close()

	hp := NewHTTP(DefaultTransport, 10*time.Second)
	proxyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hp.handleHTTP(w, r)
	}))
	defer proxyServer.Close()

	req, _ := http.NewRequest("POST", targetServer.URL, strings.NewReader("request body"))
	transport := &http.Transport{
		Proxy: func(req *http.Request) (*url.URL, error) {
			return url.Parse(proxyServer.URL)
		},
	}
	client := &http.Client{Transport: transport, Timeout: 5 * time.Second}

	resp, err := client.Do(req)
	assert.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusCreated, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	assert.Equal(t, "created", string(body))
}
