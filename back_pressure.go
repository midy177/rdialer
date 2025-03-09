package rdialer

import (
	"errors"
	"sync"
	"time"
)

// BackPressure 接口定义了背压控制的基本方法
type BackPressure interface {
	ShouldWait() bool
	Wait() error
	Update(bytesWritten int)
	Close()
}

// streamBackPressure 实现了针对单个 WebTransport Stream 的背压控制
type streamBackPressure struct {
	mu             sync.Mutex
	pendingBytes   int           // 待处理的字节数
	threshold      int           // 触发背压的阈值
	waitTimeout    time.Duration // 等待超时时间
	closed         bool
	lastUpdateTime time.Time
}

var streamBackPressurePool = sync.Pool{
	New: func() interface{} {
		return &streamBackPressure{
			threshold:      1024 * 1024, // 默认 1MB 阈值
			waitTimeout:    5 * time.Second,
			lastUpdateTime: time.Now(),
		}
	},
}

// NewStreamBackPressure 创建一个新的流背压控制器
func NewStreamBackPressure() BackPressure {
	bp := streamBackPressurePool.Get().(*streamBackPressure)
	bp.pendingBytes = 0
	bp.closed = false
	bp.lastUpdateTime = time.Now()
	return bp
}

// ShouldWait 判断是否需要等待（应用背压）
func (s *streamBackPressure) ShouldWait() bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return false
	}

	// 如果待处理字节数超过阈值，或者最后更新时间超过一定时间，则应用背压
	return s.pendingBytes > s.threshold ||
		time.Since(s.lastUpdateTime) > 2*time.Second
}

// Wait 等待直到可以继续发送数据
func (s *streamBackPressure) Wait() error {
	if !s.ShouldWait() {
		return nil
	}

	// 简单的等待策略：睡眠一小段时间
	timer := time.NewTimer(s.waitTimeout)
	defer timer.Stop()

	start := time.Now()
	for s.ShouldWait() {
		if time.Since(start) > s.waitTimeout {
			return errors.New("背压等待超时")
		}
		time.Sleep(10 * time.Millisecond)
	}

	return nil
}

// Update 更新背压状态
func (s *streamBackPressure) Update(bytesWritten int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return
	}

	// 更新待处理字节数和最后更新时间
	s.pendingBytes += bytesWritten
	s.lastUpdateTime = time.Now()

	// 如果确认处理了一些数据，减少待处理字节数
	if bytesWritten < 0 {
		s.pendingBytes += bytesWritten // bytesWritten 为负值，表示已处理的字节数
		if s.pendingBytes < 0 {
			s.pendingBytes = 0
		}
	}
}

// Close 关闭背压控制器并释放资源
func (s *streamBackPressure) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.closed {
		s.closed = true
		s.pendingBytes = 0
		streamBackPressurePool.Put(s)
	}
}
