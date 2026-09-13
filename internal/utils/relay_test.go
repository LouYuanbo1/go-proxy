package utils

import (
	"context"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// ======================== relay 测试 ========================

func TestRelay_Bidirectional(t *testing.T) {
	clientA, clientB := net.Pipe()
	targetA, targetB := net.Pipe()
	defer clientA.Close()
	defer clientB.Close()
	defer targetA.Close()
	defer targetB.Close()

	ctx := context.Background()

	// clientB 代表客户端连接，targetA 代表目标连接
	// relay 期望: clientB -> targetA, targetA -> clientB

	var wg sync.WaitGroup
	wg.Go(func() {
		RelayConns(ctx, clientB, targetA)
	})

	// 客户端 -> 目标: 通过 clientA 写入, targetB 读取
	clientA.Write([]byte("hello from client"))
	buf := make([]byte, 17)
	n, err := io.ReadFull(targetB, buf)
	assert.NoError(t, err)
	assert.Equal(t, "hello from client", string(buf[:n]))

	// 目标 -> 客户端: 通过 targetB 写入, clientA 读取
	targetB.Write([]byte("hello from target"))
	buf = make([]byte, 17)
	n, err = io.ReadFull(clientA, buf)
	assert.NoError(t, err)
	assert.Equal(t, "hello from target", string(buf[:n]))

	// 关闭客户端连接, relay 应该退出
	clientA.Close()
	wg.Wait()
}

func TestRelay_ClosePropagation(t *testing.T) {
	clientA, clientB := net.Pipe()
	targetA, targetB := net.Pipe()
	defer clientA.Close()
	defer clientB.Close()
	defer targetA.Close()
	defer targetB.Close()

	ctx := context.Background()

	var wg sync.WaitGroup
	wg.Go(func() {
		RelayConns(ctx, clientB, targetA)
	})

	// 关闭客户端 -> relay 退出 -> target 也被关闭
	clientA.Close()
	wg.Wait()

	// 验证 targetB 已关闭 (读取返回 EOF)
	_, err := targetB.Read(make([]byte, 1))
	assert.Error(t, err)
	//assert.Contains(t, err.Error(), "closed")
	assert.Contains(t, err.Error(), "EOF")
}

func TestRelay_ContextCancel(t *testing.T) {
	clientA, clientB := net.Pipe()
	targetA, targetB := net.Pipe()
	defer clientA.Close()
	defer clientB.Close()
	defer targetA.Close()
	defer targetB.Close()

	ctx, cancel := context.WithCancel(context.Background())

	var wg sync.WaitGroup
	wg.Go(func() {
		RelayConns(ctx, clientB, targetA)
	})

	// 写入一些数据确保转发已启动
	clientA.Write([]byte("ping"))
	buf := make([]byte, 4)
	io.ReadFull(targetB, buf)

	// 取消 context → relay 应关闭连接并退出
	cancel()
	wg.Wait()

	// 验证两个连接都被关闭
	_, err := clientA.Read(make([]byte, 1))
	assert.Error(t, err)

	_, err = targetB.Read(make([]byte, 1))
	assert.Error(t, err)
}

func TestRelay_ContextTimeout(t *testing.T) {
	clientA, clientB := net.Pipe()
	targetA, targetB := net.Pipe()
	defer clientA.Close()
	defer clientB.Close()
	defer targetA.Close()
	defer targetB.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	err := RelayConns(ctx, clientB, targetA)
	assert.Error(t, err)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
}
