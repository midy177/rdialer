package rdialer

import (
	"context"
	"github.com/midy177/webtransport-go"
	"github.com/rs/zerolog/log"
	"golang.org/x/time/rate"
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
	remoteAddr := session.RemoteAddr()
	localAddr := session.LocalAddr()
	// 添加连接级别的限流器
	limiter := rate.NewLimiter(rate.Limit(RateLimit), RateBurst) // 每秒1000个请求，突发100
	var stats streamStats
	log.Info().Str("LocalAddr", localAddr.String()).Str("RemoteAddr", remoteAddr.String()).
		Str("clientKey", clientKey).Msg("session accept stream loop")
	for {
		// 限流控制
		if err := limiter.Wait(context.TODO()); err != nil {
			log.Warn().Str("LocalAddr", localAddr.String()).Str("RemoteAddr", remoteAddr.String()).
				Str("ClientKey", clientKey).Err(err).Msg("rate limit exceeded")
			continue
		}
		// 接受新的流
		stream, err := session.AcceptStream(context.TODO())
		if err != nil {
			log.Warn().Str("LocalAddr", localAddr.String()).Str("RemoteAddr", remoteAddr.String()).
				Str("ClientKey", clientKey).Err(err).Msg("accept stream failed")
			return
		}

		atomic.AddInt64(&stats.activeStreams, 1)

		// 处理流
		go func() {
			handleStream(stream, remoteAddr, localAddr)
			atomic.AddInt64(&stats.activeStreams, -1)
		}()
	}
}
