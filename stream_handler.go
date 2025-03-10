package rdialer

import (
	"context"
	"errors"
	"github.com/rs/zerolog/log"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/midy177/webtransport-go"
)

// handleStream 处理单个流
func handleStream(stream webtransport.Stream, remoteAddr, localAddr net.Addr) {
	defer stream.Close()

	// 创建解码缓冲区
	decodeBuffer := NewDecodeBuffer()
	// 读取消息
	n, err := decodeBuffer.ReadFrom(stream)
	if err != nil {
		log.Error().Str("LocalAddr", localAddr.String()).Str("RemoteAddr", remoteAddr.String()).Err(err).Msg("read from stream")
		return
	}

	switch decodeBuffer.MessageType {
	case KeepAlive:
		doKeepalive(stream, remoteAddr, localAddr)
	case Connect:
		str := strings.Split(string(decodeBuffer.Buffer[:n]), "/")
		if len(str) != 2 {
			log.Error().Str("LocalAddr", localAddr.String()).Str("RemoteAddr", remoteAddr.String()).Msg("connect proto address error")
			return
		}
		conn, err := newConnection(stream, str[0], str[1])
		if err != nil {
			log.Error().Str("LocalAddr", localAddr.String()).Str("RemoteAddr", remoteAddr.String()).Err(err).Msg("new connection")
			return
		}
		log.Debug().Str("LocalAddr", localAddr.String()).Str("RemoteAddr", remoteAddr.String()).Msgf("proto: %s address: %s", str[0], str[1])
		doDial(context.TODO(), conn, str[0], str[1])
	default:
		log.Error().Str("LocalAddr", localAddr.String()).Str("RemoteAddr", remoteAddr.String()).
			Str("Type", strconv.FormatInt(int64(decodeBuffer.MessageType), 10)).Msg("Unsupported message type")
	}
}

func doKeepalive(stream webtransport.Stream, remoteAddr, localAddr net.Addr) {
	streamID := stream.StreamID()
	keepMsg := make([]byte, 1)
	for {
		_, err := stream.Read(keepMsg)
		if err != nil {
			log.Error().Str("LocalAddr", localAddr.String()).Str("RemoteAddr", remoteAddr.String()).Err(err).Msg("read keepalive")
			return
		}
		log.Debug().Str("LocalAddr", localAddr.String()).Str("RemoteAddr", remoteAddr.String()).
			Int64("StreamID", int64(streamID)).Str("Role", "server").Msg("read keepalive")
		_, err = stream.Write(keepMsg)
		if err != nil {
			log.Error().Str("LocalAddr", localAddr.String()).Str("RemoteAddr", remoteAddr.String()).Err(err).Msg("write keepalive")
			return
		}
		log.Debug().Str("LocalAddr", localAddr.String()).Str("RemoteAddr", remoteAddr.String()).
			Int64("StreamID", int64(streamID)).Str("Role", "server").Msg("send keepalive")
	}
}

type Hijacker func(ctx context.Context, conn net.Conn, proto, address string) (next bool)

var DialHijack Hijacker = func(ctx context.Context, conn net.Conn, proto, address string) (next bool) {
	return true
}

func doDial(ctx context.Context, conn net.Conn, proto, address string) {
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
