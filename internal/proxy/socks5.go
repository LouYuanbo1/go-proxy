package proxy

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"slices"
	"time"

	"github.com/LouYuanbo1/go-proxy/internal/utils"
)

const (
	socks5Version = 0x05
	rsv           = 0x00

	authNoAuth       = 0x00
	authNoAcceptable = 0xFF

	cmdConnect = 0x01

	atypIPv4   = 0x01
	atypDomain = 0x03
	atypIPv6   = 0x04

	repSucceeded        = 0x00
	repGeneralFailure   = 0x01
	repConnRefused      = 0x05
	repCmdNotSupported  = 0x07
	repAtypNotSupported = 0x08
)

// SOCKS5 是 SOCKS5 代理的具体实现
type SOCKS5 struct {
	timeout time.Duration
}

// NewSOCKS5 创建 SOCKS5 代理实例
func NewSOCKS5(timeout time.Duration) *SOCKS5 {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return &SOCKS5{
		timeout: timeout,
	}
}

// HandleTCP 处理一个 SOCKS5 客户端连接
func (p *SOCKS5) HandleTCP(ctx context.Context, clientConn net.Conn) error {
	defer clientConn.Close()

	// 设置读取超时
	if err := clientConn.SetReadDeadline(time.Now().Add(p.timeout)); err != nil {
		return fmt.Errorf("设置读取超时失败: %w", err)
	}

	// 处理认证协商阶段
	if err := p.handleAuth(clientConn); err != nil {
		return err
	}

	if err := clientConn.SetReadDeadline(time.Time{}); err != nil {
		return fmt.Errorf("清除读取超时失败: %w", err)
	}

	// 解析目标地址
	targetAddr, err := p.handleRequest(clientConn)
	if err != nil {
		return err
	}

	// 作为客户端连接的目标地址
	targetConn, err := net.DialTimeout("tcp", targetAddr, p.timeout)
	if err != nil {
		p.sendReply(clientConn, repConnRefused, "0.0.0.0", 0)
		return fmt.Errorf("连接目标失败 %s: %w", targetAddr, err)
	}
	defer targetConn.Close()

	// 发送连接成功回复
	p.sendReply(clientConn, repSucceeded, "0.0.0.0", 0)

	// 代理数据流
	return utils.RelayConns(ctx, clientConn, targetConn)
}

// handleAuth 处理 SOCKS5 认证协商阶段
func (p *SOCKS5) handleAuth(clientConn net.Conn) error {
	header := make([]byte, 2)
	if _, err := io.ReadFull(clientConn, header); err != nil {
		return fmt.Errorf("读取认证头失败: %w", err)
	}

	if header[0] != socks5Version {
		return fmt.Errorf("不支持的 SOCKS 版本: %d", header[0])
	}

	nmethods := header[1]
	methods := make([]byte, nmethods)
	if _, err := io.ReadFull(clientConn, methods); err != nil {
		return fmt.Errorf("读取认证方法列表失败: %w", err)
	}

	if slices.Contains(methods, authNoAuth) {
		_, err := clientConn.Write([]byte{socks5Version, authNoAuth})
		return err
	}

	clientConn.Write([]byte{socks5Version, authNoAcceptable})
	return fmt.Errorf("客户端不支持无认证方式")
}

// handleRequest 解析 SOCKS5 请求，返回目标地址 "host:port"
func (p *SOCKS5) handleRequest(clientConn net.Conn) (string, error) {
	reqHeader := make([]byte, 4)
	if _, err := io.ReadFull(clientConn, reqHeader); err != nil {
		return "", fmt.Errorf("读取命令头失败: %w", err)
	}

	if reqHeader[0] != socks5Version {
		return "", fmt.Errorf("不支持的请求版本: %d", reqHeader[0])
	}

	cmd := reqHeader[1]
	if cmd != cmdConnect {
		p.sendReply(clientConn, repCmdNotSupported, "0.0.0.0", 0)
		return "", fmt.Errorf("不支持的命令: %d", cmd)
	}

	atyp := reqHeader[3]
	targetAddr, err := p.parseAddress(clientConn, atyp)
	if err != nil {
		return "", err
	}

	portBytes := make([]byte, 2)
	if _, err := io.ReadFull(clientConn, portBytes); err != nil {
		return "", fmt.Errorf("读取端口失败: %w", err)
	}
	targetPort := binary.BigEndian.Uint16(portBytes)

	return net.JoinHostPort(targetAddr, fmt.Sprintf("%d", targetPort)), nil
}

// parseAddress 根据地址类型解析目标地址
func (p *SOCKS5) parseAddress(clientConn net.Conn, atyp byte) (string, error) {
	switch atyp {
	case atypIPv4:
		ip := make([]byte, 4)
		if _, err := io.ReadFull(clientConn, ip); err != nil {
			return "", fmt.Errorf("读取 IPv4 地址失败: %w", err)
		}
		return net.IP(ip).String(), nil
	case atypDomain:
		domainLen := make([]byte, 1)
		if _, err := io.ReadFull(clientConn, domainLen); err != nil {
			return "", fmt.Errorf("读取域名长度失败: %w", err)
		}
		domain := make([]byte, domainLen[0])
		if _, err := io.ReadFull(clientConn, domain); err != nil {
			return "", fmt.Errorf("读取域名失败: %w", err)
		}
		return string(domain), nil
	case atypIPv6:
		ip := make([]byte, 16)
		if _, err := io.ReadFull(clientConn, ip); err != nil {
			return "", fmt.Errorf("读取 IPv6 地址失败: %w", err)
		}
		return net.IP(ip).String(), nil
	default:
		p.sendReply(clientConn, repAtypNotSupported, "0.0.0.0", 0)
		return "", fmt.Errorf("不支持的地址类型: %d", atyp)
	}
}

// sendReply 发送 SOCKS5 回复报文
func (p *SOCKS5) sendReply(conn net.Conn, rep byte, bindAddr string, bindPort uint16) {
	ip := net.ParseIP(bindAddr)
	if ip == nil {
		ip = net.IPv4zero
	}

	var atyp byte
	var addrBytes []byte
	if ip4 := ip.To4(); ip4 != nil {
		atyp = atypIPv4
		addrBytes = ip4
	} else {
		atyp = atypIPv6
		addrBytes = ip.To16()
	}

	reply := make([]byte, 4+len(addrBytes)+2)
	reply[0] = socks5Version
	reply[1] = rep
	reply[2] = rsv
	reply[3] = atyp
	copy(reply[4:], addrBytes)
	binary.BigEndian.PutUint16(reply[4+len(addrBytes):], bindPort)

	conn.Write(reply)
}
