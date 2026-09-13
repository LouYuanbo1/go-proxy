package proxy

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"log"
	"math/big"
	"net"
	"strings"
	"time"

	"github.com/LouYuanbo1/go-proxy/internal/utils"
	"github.com/quic-go/quic-go"
)

// QUICServer 基于 QUIC 协议的代理服务端。
//
// 客户端通过 QUIC（UDP）连接到代理，打开一条流（Stream），
// 发送目标地址 host:port 并换行，代理即转发到目标。
//
// 协议格式：
//
//	客户端 → 代理: "host:port\n"
//	代理 → 客户端: "OK\n"                    (连接成功)
//	代理 → 客户端: "error message\n"          (连接失败)
//	之后双向原始数据转发
//
// 客户端角色请参见 QUIC。
type QUICServer struct {
	timeout    time.Duration
	tlsConfig  *tls.Config
	quicConfig *quic.Config
}

// generateQUICTLSConfig 生成自签名 TLS 证书（QUIC 必须使用 TLS）
func generateQUICTLSConfig() (*tls.Config, error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, err
	}

	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, err
	}

	template := x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName:   "Go-Proxy QUIC",
			Organization: []string{"Go-Proxy"},
		},
		NotBefore:             time.Now().Add(-1 * time.Hour),
		NotAfter:              time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		return nil, err
	}

	return &tls.Config{
		Certificates: []tls.Certificate{{
			Certificate: [][]byte{certDER},
			PrivateKey:  key,
		}},
		NextProtos: []string{"quic-proxy"},
		MinVersion: tls.VersionTLS13,
	}, nil
}

// NewQUICServer 创建 QUIC 服务端实例。
//
// 与 NewQUIC 不同，服务端需要自签名 TLS 证书（QUIC 强制要求 TLS 1.3）。
func NewQUICServer(timeout time.Duration) (*QUICServer, error) {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}

	tlsConfig, err := generateQUICTLSConfig()
	if err != nil {
		return nil, fmt.Errorf("生成 QUIC TLS 证书失败: %w", err)
	}

	return &QUICServer{
		timeout:   timeout,
		tlsConfig: tlsConfig,
		quicConfig: &quic.Config{
			MaxIdleTimeout:        timeout,
			KeepAlivePeriod:       timeout / 2,
			Allow0RTT:             false,
			MaxIncomingStreams:    1000,
			MaxIncomingUniStreams: -1,
		},
	}, nil
}

// Name 返回代理名称
func (s *QUICServer) Name() string {
	return "QUIC-Server"
}

// Serve 在指定 UDP 地址上启动 QUIC 代理服务（阻塞调用）。
//
// addr 格式如 ":1443" 或 "0.0.0.0:1443"。
func (s *QUICServer) Serve(ctx context.Context, addr string) error {
	// ── 解析 UDP 地址 ──
	udpAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return fmt.Errorf("解析 UDP 地址失败: %w", err)
	}

	// ── 监听 UDP 地址 ──
	udpConn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		return fmt.Errorf("监听 UDP %s 失败: %w", addr, err)
	}
	defer udpConn.Close()

	// ── 启动 QUIC 监听 ──
	listener, err := quic.Listen(udpConn, s.tlsConfig, s.quicConfig)
	if err != nil {
		return fmt.Errorf("启动 QUIC 监听失败: %w", err)
	}
	defer listener.Close()

	log.Printf("[QUIC-Server] 📥 服务端启动在 udp://%s", addr)

	for {
		conn, err := listener.Accept(ctx)
		if err != nil {
			return fmt.Errorf("接受 QUIC 连接失败: %w", err)
		}
		go s.handleConn(ctx, conn)
	}
}

// handleConn 处理一个 QUIC 连接（管理其下的所有流）
func (s *QUICServer) handleConn(ctx context.Context, conn *quic.Conn) {
	remoteAddr := conn.RemoteAddr().String()
	log.Printf("[QUIC-Server] 🔗 新连接来自 %s", remoteAddr)
	defer func() {
		conn.CloseWithError(0, "主机关闭")
		log.Printf("[QUIC-Server] 🔚 连接关闭 %s", remoteAddr)
	}()

	for {
		stream, err := conn.AcceptStream(ctx)
		if err != nil {
			return
		}
		go s.handleStream(ctx, stream, conn.LocalAddr(), conn.RemoteAddr())
	}
}

// handleStream 处理一条 QUIC 流
//
// 协议：
//
//  1. 读取一行目标地址 "host:port"
//  2. 返回 "OK\n" 或错误信息
//  3. 双向转发数据
func (s *QUICServer) handleStream(ctx context.Context, stream *quic.Stream, localAddr, remoteAddr net.Addr) {
	defer stream.Close()

	// ── 读取目标地址 ──
	addr, err := bufio.NewReader(stream).ReadString('\n')
	if err != nil {
		log.Printf("[QUIC-Server] ⚠️ 读取地址失败: %v", err)
		return
	}
	addr = strings.TrimSpace(addr)
	log.Printf("[QUIC-Server] 🔗 转发到 %s", addr)

	// ── 连接目标（TCP） ──
	target, err := net.DialTimeout("tcp", addr, s.timeout)
	if err != nil {
		errMsg := fmt.Sprintf("连接目标失败: %v\n", err)
		stream.Write([]byte(errMsg))
		log.Printf("[QUIC-Server] ❌ %s → %s", addr, strings.TrimSpace(errMsg))
		return
	}
	defer target.Close()

	// ── 通知客户端连接成功 ──
	if _, err := stream.Write([]byte("OK\n")); err != nil {
		log.Printf("[QUIC-Server] ⚠️ 发送确认失败: %v", err)
		return
	}

	log.Printf("[QUIC-Server] ✅ %s 隧道已建立", addr)

	// ── 双向转发 ──
	sc := &quicStreamConn{
		Stream:     stream,
		localAddr:  localAddr,
		remoteAddr: remoteAddr,
	}
	if err := utils.RelayConns(ctx, target, sc); err != nil {
		log.Printf("[QUIC-Server] 🔚 %s 转发结束", addr)
	}
}
