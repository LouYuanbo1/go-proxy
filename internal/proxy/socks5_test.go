package proxy

import (
	"context"
	"encoding/binary"
	"io"
	"net"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// ======================== handleAuth 测试 ========================

func TestSOCKS5_HandleAuth_Valid(t *testing.T) {
	p := NewSOCKS5(10 * time.Second)
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	go func() {
		client.Write([]byte{0x05, 0x01, 0x00})
	}()

	var reply []byte
	var readErr error
	readDone := make(chan struct{})
	go func() {
		buf := make([]byte, 2)
		_, readErr = io.ReadFull(client, buf)
		reply = buf
		close(readDone)
	}()

	err := p.handleAuth(server)
	assert.NoError(t, err)

	<-readDone
	assert.NoError(t, readErr)
	assert.Equal(t, []byte{0x05, 0x00}, reply)
}

func TestSOCKS5_HandleAuth_MultipleMethods(t *testing.T) {
	p := NewSOCKS5(10 * time.Second)
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	go func() {
		client.Write([]byte{0x05, 0x03, 0x02, 0x00, 0x01})
	}()

	var reply []byte
	var readErr error
	readDone := make(chan struct{})
	go func() {
		buf := make([]byte, 2)
		_, readErr = io.ReadFull(client, buf)
		reply = buf
		close(readDone)
	}()

	err := p.handleAuth(server)
	assert.NoError(t, err)

	<-readDone
	assert.NoError(t, readErr)
	assert.Equal(t, []byte{0x05, 0x00}, reply)
}

func TestSOCKS5_HandleAuth_NoAcceptableMethod(t *testing.T) {
	p := NewSOCKS5(10 * time.Second)
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	go func() {
		client.Write([]byte{0x05, 0x02, 0x01, 0x02})
	}()

	var reply []byte
	var readErr error
	readDone := make(chan struct{})
	go func() {
		buf := make([]byte, 2)
		_, readErr = io.ReadFull(client, buf)
		reply = buf
		close(readDone)
	}()

	err := p.handleAuth(server)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "不支持无认证方式")

	<-readDone
	assert.NoError(t, readErr)
	assert.Equal(t, []byte{0x05, 0xFF}, reply)
}

func TestSOCKS5_HandleAuth_InvalidVersion(t *testing.T) {
	p := NewSOCKS5(10 * time.Second)
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	go func() {
		client.Write([]byte{0x04, 0x01, 0x00})
	}()

	err := p.handleAuth(server)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "不支持的 SOCKS 版本")
}

func TestSOCKS5_HandleAuth_EmptyMethods(t *testing.T) {
	p := NewSOCKS5(10 * time.Second)
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	go func() {
		client.Write([]byte{0x05, 0x00})
	}()

	var reply []byte
	var readErr error
	readDone := make(chan struct{})
	go func() {
		buf := make([]byte, 2)
		_, readErr = io.ReadFull(client, buf)
		reply = buf
		close(readDone)
	}()

	err := p.handleAuth(server)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "不支持无认证方式")

	<-readDone
	assert.NoError(t, readErr)
	assert.Equal(t, []byte{0x05, 0xFF}, reply)
}

// ======================== handleRequest 测试 ========================

func TestSOCKS5_HandleRequest_ConnectIPv4(t *testing.T) {
	p := NewSOCKS5(10 * time.Second)
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	go func() {
		client.Write([]byte{0x05, 0x01, 0x00, 0x01})
		client.Write([]byte{127, 0, 0, 1})
		portBytes := make([]byte, 2)
		binary.BigEndian.PutUint16(portBytes, 8080)
		client.Write(portBytes)
	}()

	addr, err := p.handleRequest(server)
	assert.NoError(t, err)
	assert.Equal(t, "127.0.0.1:8080", addr)
}

func TestSOCKS5_HandleRequest_ConnectDomain(t *testing.T) {
	p := NewSOCKS5(10 * time.Second)
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	go func() {
		client.Write([]byte{0x05, 0x01, 0x00, 0x03})
		client.Write([]byte{9})
		client.Write([]byte("localhost"))
		portBytes := make([]byte, 2)
		binary.BigEndian.PutUint16(portBytes, 443)
		client.Write(portBytes)
	}()

	addr, err := p.handleRequest(server)
	assert.NoError(t, err)
	assert.Equal(t, "localhost:443", addr)
}

func TestSOCKS5_HandleRequest_ConnectIPv6(t *testing.T) {
	p := NewSOCKS5(10 * time.Second)
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	go func() {
		client.Write([]byte{0x05, 0x01, 0x00, 0x04})
		ipv6 := net.ParseIP("::1").To16()
		client.Write(ipv6)
		portBytes := make([]byte, 2)
		binary.BigEndian.PutUint16(portBytes, 80)
		client.Write(portBytes)
	}()

	addr, err := p.handleRequest(server)
	assert.NoError(t, err)
	assert.Equal(t, "[::1]:80", addr)
}

func TestSOCKS5_HandleRequest_UnsupportedCommand(t *testing.T) {
	p := NewSOCKS5(10 * time.Second)
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	go func() {
		client.Write([]byte{0x05, 0x02, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00})
	}()

	var reply []byte
	var readErr error
	readDone := make(chan struct{})
	go func() {
		buf := make([]byte, 10)
		_, readErr = io.ReadFull(client, buf)
		reply = buf
		close(readDone)
	}()

	addr, err := p.handleRequest(server)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "不支持的命令")
	assert.Empty(t, addr)

	<-readDone
	assert.NoError(t, readErr)
	assert.Equal(t, byte(0x05), reply[0])
	assert.Equal(t, byte(0x07), reply[1])
}

func TestSOCKS5_HandleRequest_UnsupportedAtyp(t *testing.T) {
	p := NewSOCKS5(10 * time.Second)
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	go func() {
		client.Write([]byte{0x05, 0x01, 0x00, 0x02, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00})
	}()

	var reply []byte
	var readErr error
	readDone := make(chan struct{})
	go func() {
		buf := make([]byte, 10)
		_, readErr = io.ReadFull(client, buf)
		reply = buf
		close(readDone)
	}()

	addr, err := p.handleRequest(server)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "不支持的地址类型")
	assert.Empty(t, addr)

	<-readDone
	assert.NoError(t, readErr)
	assert.Equal(t, byte(0x05), reply[0])
	assert.Equal(t, byte(0x08), reply[1])
}

func TestSOCKS5_HandleRequest_InvalidVersion(t *testing.T) {
	p := NewSOCKS5(10 * time.Second)
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	go func() {
		client.Write([]byte{0x04, 0x01, 0x00, 0x01})
	}()

	addr, err := p.handleRequest(server)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "不支持的请求版本")
	assert.Empty(t, addr)
}

// ======================== parseAddress 测试 ========================

func TestSOCKS5_ParseAddress_IPv4(t *testing.T) {
	p := NewSOCKS5(10 * time.Second)
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	go func() {
		client.Write([]byte{192, 168, 1, 100})
	}()

	addr, err := p.parseAddress(server, atypIPv4)
	assert.NoError(t, err)
	assert.Equal(t, "192.168.1.100", addr)
}

func TestSOCKS5_ParseAddress_Domain(t *testing.T) {
	p := NewSOCKS5(10 * time.Second)
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	go func() {
		client.Write([]byte{11})
		client.Write([]byte("example.com"))
	}()

	addr, err := p.parseAddress(server, atypDomain)
	assert.NoError(t, err)
	assert.Equal(t, "example.com", addr)
}

func TestSOCKS5_ParseAddress_IPv6(t *testing.T) {
	p := NewSOCKS5(10 * time.Second)
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	go func() {
		ipv6 := net.ParseIP("2001:db8::1").To16()
		client.Write(ipv6)
	}()

	addr, err := p.parseAddress(server, atypIPv6)
	assert.NoError(t, err)
	assert.Equal(t, "2001:db8::1", addr)
}

// ======================== sendReply 测试 ========================

func TestSOCKS5_SendReply_Success(t *testing.T) {
	p := NewSOCKS5(10 * time.Second)
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	go func() {
		p.sendReply(server, repSucceeded, "0.0.0.0", 0)
	}()

	reply := make([]byte, 10)
	_, err := io.ReadFull(client, reply)
	assert.NoError(t, err)
	assert.Equal(t, []byte{0x05, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}, reply)
}

func TestSOCKS5_SendReply_ConnRefused(t *testing.T) {
	p := NewSOCKS5(10 * time.Second)
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	go func() {
		p.sendReply(server, repConnRefused, "0.0.0.0", 0)
	}()

	reply := make([]byte, 10)
	io.ReadFull(client, reply)
	assert.Equal(t, byte(0x05), reply[0])
	assert.Equal(t, byte(0x05), reply[1])
}

func TestSOCKS5_SendReply_CmdNotSupported(t *testing.T) {
	p := NewSOCKS5(10 * time.Second)
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	go func() {
		p.sendReply(server, repCmdNotSupported, "0.0.0.0", 0)
	}()

	reply := make([]byte, 10)
	io.ReadFull(client, reply)
	assert.Equal(t, byte(0x07), reply[1])
}

func TestSOCKS5_SendReply_AtypNotSupported(t *testing.T) {
	p := NewSOCKS5(10 * time.Second)
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	go func() {
		p.sendReply(server, repAtypNotSupported, "0.0.0.0", 0)
	}()

	reply := make([]byte, 10)
	io.ReadFull(client, reply)
	assert.Equal(t, byte(0x08), reply[1])
}

func TestSOCKS5_SendReply_WithPort(t *testing.T) {
	p := NewSOCKS5(10 * time.Second)
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	go func() {
		p.sendReply(server, repSucceeded, "192.168.1.1", 8080)
	}()

	reply := make([]byte, 10)
	io.ReadFull(client, reply)
	assert.Equal(t, []byte{0x05, 0x00, 0x00, 0x01}, reply[:4])
	assert.Equal(t, []byte{192, 168, 1, 1}, reply[4:8])
	port := binary.BigEndian.Uint16(reply[8:10])
	assert.Equal(t, uint16(8080), port)
}

// ======================== HandleTCP 全流程集成测试 ========================

func TestSOCKS5_HandleTCP_Success(t *testing.T) {
	targetListener, err := net.Listen("tcp", "127.0.0.1:0")
	assert.NoError(t, err)
	defer targetListener.Close()

	targetAddr := targetListener.Addr().String()

	go func() {
		conn, err := targetListener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		io.Copy(conn, conn)
	}()

	proxyListener, err := net.Listen("tcp", "127.0.0.1:0")
	assert.NoError(t, err)
	defer proxyListener.Close()

	p := NewSOCKS5(10 * time.Second)

	go func() {
		conn, err := proxyListener.Accept()
		if err != nil {
			return
		}
		p.HandleTCP(context.Background(), conn)
	}()

	clientConn, err := net.Dial("tcp", proxyListener.Addr().String())
	assert.NoError(t, err)
	defer clientConn.Close()

	clientConn.Write([]byte{0x05, 0x01, 0x00})
	authReply := make([]byte, 2)
	_, err = io.ReadFull(clientConn, authReply)
	assert.NoError(t, err)
	assert.Equal(t, []byte{0x05, 0x00}, authReply)

	host, portStr, _ := net.SplitHostPort(targetAddr)
	port, _ := strconv.Atoi(portStr)
	ip := net.ParseIP(host).To4()

	req := make([]byte, 0, 10)
	req = append(req, 0x05, 0x01, 0x00, 0x01)
	req = append(req, ip...)
	portBytes := make([]byte, 2)
	binary.BigEndian.PutUint16(portBytes, uint16(port))
	req = append(req, portBytes...)
	clientConn.Write(req)

	reply := make([]byte, 10)
	_, err = io.ReadFull(clientConn, reply)
	assert.NoError(t, err)
	assert.Equal(t, byte(0x05), reply[0])
	assert.Equal(t, byte(0x00), reply[1])

	clientConn.SetDeadline(time.Now().Add(2 * time.Second))
	clientConn.Write([]byte("hello socks5"))
	echo := make([]byte, 12)
	n, err := io.ReadFull(clientConn, echo)
	assert.NoError(t, err)
	assert.Equal(t, "hello socks5", string(echo[:n]))
}

func TestSOCKS5_HandleTCP_ConnectFailure(t *testing.T) {
	proxyListener, err := net.Listen("tcp", "127.0.0.1:0")
	assert.NoError(t, err)
	defer proxyListener.Close()

	p := NewSOCKS5(10 * time.Second)
	p.timeout = 500 * time.Millisecond

	go func() {
		conn, err := proxyListener.Accept()
		if err != nil {
			return
		}
		p.HandleTCP(context.Background(), conn)
	}()

	clientConn, err := net.Dial("tcp", proxyListener.Addr().String())
	assert.NoError(t, err)
	defer clientConn.Close()

	clientConn.Write([]byte{0x05, 0x01, 0x00})
	authReply := make([]byte, 2)
	io.ReadFull(clientConn, authReply)
	assert.Equal(t, []byte{0x05, 0x00}, authReply)

	clientConn.Write([]byte{0x05, 0x01, 0x00, 0x01, 192, 0, 2, 1, 0x00, 0x50})

	reply := make([]byte, 10)
	_, err = io.ReadFull(clientConn, reply)
	assert.NoError(t, err)
	assert.Equal(t, byte(0x05), reply[0])
	assert.Equal(t, byte(0x05), reply[1])
}

func TestSOCKS5_HandleTCP_DomainName(t *testing.T) {
	targetListener, err := net.Listen("tcp", "127.0.0.1:0")
	assert.NoError(t, err)
	defer targetListener.Close()

	_, portStr, _ := net.SplitHostPort(targetListener.Addr().String())
	port, _ := strconv.Atoi(portStr)

	go func() {
		conn, err := targetListener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		io.Copy(conn, conn)
	}()

	proxyListener, err := net.Listen("tcp", "127.0.0.1:0")
	assert.NoError(t, err)
	defer proxyListener.Close()

	p := NewSOCKS5(10 * time.Second)

	go func() {
		conn, err := proxyListener.Accept()
		if err != nil {
			return
		}
		p.HandleTCP(context.Background(), conn)
	}()

	clientConn, err := net.Dial("tcp", proxyListener.Addr().String())
	assert.NoError(t, err)
	defer clientConn.Close()

	clientConn.Write([]byte{0x05, 0x01, 0x00})
	authReply := make([]byte, 2)
	io.ReadFull(clientConn, authReply)
	assert.Equal(t, []byte{0x05, 0x00}, authReply)

	domain := "localhost"
	req := []byte{0x05, 0x01, 0x00, 0x03, byte(len(domain))}
	req = append(req, []byte(domain)...)
	portBytes := make([]byte, 2)
	binary.BigEndian.PutUint16(portBytes, uint16(port))
	req = append(req, portBytes...)
	clientConn.Write(req)

	reply := make([]byte, 10)
	io.ReadFull(clientConn, reply)
	assert.Equal(t, byte(0x00), reply[1])

	clientConn.SetDeadline(time.Now().Add(2 * time.Second))
	clientConn.Write([]byte("domain test"))
	echo := make([]byte, 11)
	n, _ := io.ReadFull(clientConn, echo)
	assert.Equal(t, "domain test", string(echo[:n]))
}

func TestNewSOCKS5_Defaults(t *testing.T) {
	p := NewSOCKS5(10 * time.Second)
	assert.NotNil(t, p)
	assert.Equal(t, 10*time.Second, p.timeout)
}
