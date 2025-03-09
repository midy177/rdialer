package rdialer

import (
	"context"
	"errors"
	"io"
	"log"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/quic-go/webtransport-go"
)

// handleStream 处理单个流
func handleStream(stream webtransport.Stream) {
	defer stream.Close()

	// 创建解码缓冲区
	decodeBuffer := NewDecodeBuffer()

	// 读取消息
	n, err := decodeBuffer.ReadFrom(stream)
	if err != nil {
		log.Printf("读取消息失败: %v\n", err)
		return
	}

	if decodeBuffer.MessageType != Connect {
		_ = stream.Close()
		return
	}
	str := strings.Split(string(decodeBuffer.Buffer[:n]), "/")

	if len(str) != 2 {
		log.Printf("Connect proto address error\n")
		return
	}
	conn, err := newConnection(stream, str[0], str[1])
	if err != nil {
		log.Println(err)
		return
	}
	log.Printf("proto: %s address: %s\n", str[0], str[1])
	DoDial(context.Background(), conn, str[0], str[1])
}

type Hijacker func(ctx context.Context, conn net.Conn, proto, address string) (next bool)

var DialHijack Hijacker = func(ctx context.Context, conn net.Conn, proto, address string) (next bool) {
	return true
}

func DoDial(ctx context.Context, conn net.Conn, proto, address string) {
	// Do client hijacker
	if !DialHijack(ctx, conn, proto, address) {
		_ = conn.Close()
		return
	}

	defer func(conn net.Conn) {
		_ = conn.Close()
	}(conn)

	d := net.Dialer{}
	netConn, err := d.DialContext(ctx, proto, address)

	if err != nil {
		_ = conn.Close()
		return
	}
	defer func(netConn net.Conn) {
		_ = netConn.Close()
	}(netConn)

	pipe(conn, netConn)
}

func pipe(client net.Conn, server net.Conn) {
	ch := make(chan error, 2)
	done := make(chan struct{})
	closeOnce := sync.Once{}

	redirect := func(dst net.Conn, src net.Conn) {
		timeout := 30 * time.Second
		for {
			select {
			case <-done:
				ch <- nil
				return
			default:
				_ = src.SetReadDeadline(time.Now().Add(timeout))
				_ = dst.SetWriteDeadline(time.Now().Add(timeout))
				_, err := io.CopyN(dst, src, 32*1024)
				if err != nil {
					closeOnce.Do(func() { close(done) })
					if isNormalNetError(err) || isTimeout(err) {
						ch <- nil // 正常关闭
						return
					}
					ch <- err // 异常关闭
					return
				}
			}
		}
	}

	// 启动两个方向的数据复制
	go redirect(server, client)
	go redirect(client, server)

	// 等待两个方向都完成
	err1, err2 := <-ch, <-ch

	// 关闭连接
	_ = client.Close()
	_ = server.Close()

	// 只记录非超时、非EOF的错误
	if err1 != nil {
		log.Printf("pipe error: %v", err1)
	}
	if err2 != nil {
		log.Printf("pipe error: %v", err2)
	}
}

// 判断是否为超时错误
func isTimeout(err error) bool {
	if err == nil {
		return false
	}
	var netErr net.Error
	ok := errors.As(err, &netErr)
	return ok && netErr.Timeout()
}

// 判断是否为正常关闭的网络错误
func isNormalNetError(err error) bool {
	if err == nil {
		return false
	}
	if err == io.EOF {
		return true
	}
	return false
}
