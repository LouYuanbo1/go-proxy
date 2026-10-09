package utils

import (
	"cmp"
	"context"
	"io"
	"net"
	"sync"
)

// RelayConns 双向转发两个连接之间的数据。
// ctx 用于优雅关闭：当 ctx 被取消时，两个连接都会被关闭，转发停止。
func RelayConns(ctx context.Context, clientConn, targetConn net.Conn) error {
	var closeOnce sync.Once
	closeConns := func() {
		closeOnce.Do(func() {
			_ = clientConn.Close()
			_ = targetConn.Close()
		})
	}

	// 监听 ctx 取消 → 关闭连接 → io.Copy 因连接关闭而返回 → 协程退出
	/*
		        done := make (chan struct {})
		        defer close (done)
		        go func () {
		            select {
		            case <-ctx.Done ():
		                closeConns ()
		            case <-done:
		            }
		        }()
	*/

	// 注册 ctx 取消回调
	stop := context.AfterFunc(ctx, closeConns)
	defer stop()

	var wg sync.WaitGroup
	var copyErr1, copyErr2 error

	wg.Go(func() {
		_, copyErr1 = io.Copy(targetConn, clientConn)
		closeConns()
	})
	wg.Go(func() {
		_, copyErr2 = io.Copy(clientConn, targetConn)
		closeConns()
	})
	wg.Wait()

	return cmp.Or(ctx.Err(), copyErr1, copyErr2)
}
