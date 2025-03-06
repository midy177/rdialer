package rdialer

import (
	"context"
	"sync"
)

type backPressureConn interface {
	Pause()
	Resume()
}

type backPressure struct {
	cond   sync.Cond
	c      backPressureConn
	paused bool
	closed bool
}

var backPressurePool = sync.Pool{
	New: func() interface{} {
		return &backPressure{
			cond: sync.Cond{
				L: &sync.Mutex{},
			},
		}
	},
}

type BackPressure interface {
	OnPause()
	OnResume()
	Close()
	Pause()
	Resume()
	Wait(cancel context.CancelFunc)
}

func NewBackPressure(c backPressureConn) BackPressure {
	bp := backPressurePool.Get().(*backPressure)
	bp.c = c
	bp.paused = false
	bp.closed = false
	return bp
}

func (b *backPressure) OnPause() {
	b.cond.L.Lock()
	defer b.cond.L.Unlock()

	b.paused = true
	b.cond.Broadcast()
}

func (b *backPressure) Close() {
	b.cond.L.Lock()
	defer b.cond.L.Unlock()

	// 避免同一个 backPressure 对象被多次放回对象池
	if !b.closed {
		b.closed = true
		b.cond.Broadcast()
		// 重置状态并放回池中
		b.c = nil
		b.paused = false
		backPressurePool.Put(b)
	}
}

func (b *backPressure) OnResume() {
	b.cond.L.Lock()
	defer b.cond.L.Unlock()

	b.paused = false
	b.cond.Broadcast()
}

func (b *backPressure) Pause() {
	b.cond.L.Lock()
	defer b.cond.L.Unlock()
	if b.paused {
		return
	}
	b.c.Pause()
	b.paused = true
}

func (b *backPressure) Resume() {
	b.cond.L.Lock()
	defer b.cond.L.Unlock()
	if !b.paused {
		return
	}
	b.c.Resume()
	b.paused = false
}

func (b *backPressure) Wait(cancel context.CancelFunc) {
	b.cond.L.Lock()
	defer b.cond.L.Unlock()

	if b.closed || !b.paused {
		return
	}
	// 只在开始等待时调用一次 cancel
	cancel()

	for !b.closed && b.paused {
		b.cond.Wait()
	}
}
