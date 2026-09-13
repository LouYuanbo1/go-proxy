package utils

import (
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
	var err error

	wg.Go(func() {
		_, copyErr := io.Copy(targetConn, clientConn)
		if copyErr != nil {
			err = copyErr
		}
		closeConns()
	})
	wg.Go(func() {
		_, copyErr := io.Copy(clientConn, targetConn)
		if copyErr != nil {
			err = copyErr
		}
		closeConns()
	})
	wg.Wait()

	// 如果ctx被取消，优先返回ctx.Err()，否则返回copy错误
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ctxErr
	}
	return err
}
