package proxy

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"time"

	"github.com/quic-go/quic-go"
)

// quicStreamConn 将 *quic.Stream 适配为 net.Conn（补充 LocalAddr/RemoteAddr）
type quicStreamConn struct {
	*quic.Stream
	localAddr  net.Addr
	remoteAddr net.Addr
}

func (c *quicStreamConn) LocalAddr() net.Addr  { return c.localAddr }
func (c *quicStreamConn) RemoteAddr() net.Addr { return c.remoteAddr }

// QUIC 基于 QUIC 协议的出站转发客户端。
//
// 它作为 HTTP 代理的后端传输层，使用 QUIC 协议代替 TCP 连接目标服务器。
// 这是一个纯客户端角色，不提供服务端监听功能。
//
// 服务端功能请参见 QUICServer。
type QUIC struct {
	timeout    time.Duration
	clientTLS  *tls.Config
	clientQUIC *quic.Config
}

// NewQUIC 创建 QUIC 客户端实例。
//
// 与 NewQUICServer 不同，NewQUIC 不需要自签名 TLS 证书，
// 因为出站连接跳过对端证书验证，使用通用 ALPN。
func NewQUIC(timeout time.Duration) (*QUIC, error) {
	if timeout <= 0 || timeout > 5*time.Second {
		timeout = 5 * time.Second
	}

	return &QUIC{
		timeout: timeout,
		clientTLS: &tls.Config{
			InsecureSkipVerify: true,
			NextProtos:         []string{"h3", "hq", "quic-proxy"},
			MinVersion:         tls.VersionTLS13,
		},
		clientQUIC: &quic.Config{
			HandshakeIdleTimeout: timeout,
			MaxIdleTimeout:       timeout,
		},
	}, nil
}

// Name 返回代理名称
func (q *QUIC) Name() string {
	return "QUIC"
}

// DialStream 作为客户端向目标服务器发起 QUIC 连接，
// 返回的 net.Conn 包装了 QUIC 流，适配 utils.RelayConns。
//
// 调用方应使用带有超时的 context：
//
//	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
//	defer cancel()
//	conn, err := q.DialStream(ctx, "example.com:443")
func (q *QUIC) DialStream(ctx context.Context, addr string) (net.Conn, error) {
	conn, err := quic.DialAddr(ctx, addr, q.clientTLS, q.clientQUIC)
	if err != nil {
		return nil, fmt.Errorf("QUIC 拨号失败: %w", err)
	}

	stream, err := conn.OpenStreamSync(ctx)
	if err != nil {
		conn.CloseWithError(0, "")
		return nil, fmt.Errorf("QUIC 打开流失败: %w", err)
	}

	return &quicStreamConn{
		Stream:     stream,
		localAddr:  conn.LocalAddr(),
		remoteAddr: conn.RemoteAddr(),
	}, nil
}
