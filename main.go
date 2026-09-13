package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/LouYuanbo1/go-proxy/internal/gateway"
	"github.com/LouYuanbo1/go-proxy/internal/proxy"
	"github.com/LouYuanbo1/go-proxy/internal/proxyctl"
)

func main() {
	// ── 系统代理管理 ───────────────────────────────────
	proxyAddr := "127.0.0.1:8090"
	var ps proxyctl.ProxySettings
	// 保存当前代理设置
	ps.Save()
	// 启用代理
	ps.Enable(proxyAddr)
	// 保证无论何种方式退出（panic / Ctrl+C / 正常返回），都恢复代理设置
	defer ps.Restore()

	httpProxy := proxy.NewHTTP(proxy.DefaultTransport, 10*time.Second)
	socks5Proxy := proxy.NewSOCKS5(10 * time.Second)
	webProxy := gateway.NewWeb(proxy.DefaultTransport, 30*time.Second)

	quicProxy, err := proxy.NewQUIC(10 * time.Second)
	if err != nil {
		log.Fatalf("创建 QUIC 代理失败: %v", err)
	}
	webProxy.SetQUIC(quicProxy) // Web 网关启用 QUIC 优先策略

	httpServer := &http.Server{
		Addr:         ":8080",
		Handler:      httpProxy,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	webServer := &http.Server{
		Addr:         ":8090",
		Handler:      webProxy,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	socks5Listener, err := net.Listen("tcp", ":1080")
	if err != nil {
		log.Fatalf("SOCKS5 监听失败: %v", err)
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		fmt.Println("HTTP/HTTPS 代理启动在 :8080")
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("HTTP 服务错误: %v", err)
		}
	}()

	go func() {
		fmt.Println("🌐 网页代理网关启动在 http://localhost:8090")
		if err := webServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("网页代理服务错误: %v", err)
		}
	}()

	go func() {
		fmt.Println("SOCKS5 代理启动在 :1080")
		for {
			conn, err := socks5Listener.Accept()
			if err != nil {
				// 关闭时 listener.Close() 会导致 Accept 报错，直接退出
				return
			}
			go socks5Proxy.HandleTCP(context.Background(), conn)
		}
	}()

	/*
		go func() {
			fmt.Println("QUIC 代理启动在 udp://:1443")
			quicCtx, quicCancel := context.WithCancel(context.Background())
			defer quicCancel()
			if err := quicProxy.Serve(quicCtx, ":1443"); err != nil {
				// 关闭时 listener.Close() 会导致 Serve 返回错误，直接忽略
			}
		}()
	*/

	<-stop
	fmt.Println("\n正在关闭服务...")

	// 先关闭 SOCKS5 监听器，让 Accept() 立即返回错误，goroutine 退出
	socks5Listener.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	httpServer.Shutdown(ctx)
	webServer.Shutdown(ctx)
	fmt.Println("服务已关闭")
}
