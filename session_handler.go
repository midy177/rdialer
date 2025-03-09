package rdialer

import (
	"context"
	"github.com/quic-go/webtransport-go"
	"golang.org/x/time/rate"
	"log"
	"sync/atomic"
)

// 活动流
type streamStats struct {
	activeStreams int64
}

var (
	// RateLimit 每秒请求数
	RateLimit = 10000

	// RateBurst 突发请求数
	RateBurst = 15000
)

// handleSession 处理单个 WebTransport 会话
func handleSession(clientKey string, session *webtransport.Session) {
	// 添加连接级别的限流器
	limiter := rate.NewLimiter(rate.Limit(RateLimit), RateBurst) // 每秒1000个请求，突发100
	var stats streamStats
	for {
		// 限流控制
		if err := limiter.Wait(context.Background()); err != nil {
			log.Printf("[%s]:Rate limit exceeded: %s\n", clientKey, err)
			continue
		}
		// 接受新的流
		stream, err := session.AcceptStream(context.Background())
		if err != nil {
			log.Printf("[%s]:Stream accept failed: %s\n", clientKey, err)
			return
		}

		atomic.AddInt64(&stats.activeStreams, 1)

		// 处理流
		go func() {
			handleStream(stream)
			atomic.AddInt64(&stats.activeStreams, -1)
		}()
	}
}
