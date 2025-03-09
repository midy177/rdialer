package rdialer

import (
	"fmt"
	"sync"

	"github.com/quic-go/webtransport-go"
	"golang.org/x/exp/rand"
)

type sessionListener interface {
	sessionAdded(clientKey string, sessionKey int64)
	sessionRemoved(clientKey string, sessionKey int64)
}

// 使用 sync.Map 存储客户端会话
type sessionManager struct {
	clients sync.Map // map[string]*clientSessions
}

// 每个客户端的会话列表
type clientSessions struct {
	mu       sync.Mutex
	sessions []*webtransport.Session
}

func newSessionManager() *sessionManager {
	return &sessionManager{}
}

func (sm *sessionManager) getDialer(clientKey string) (Dialer, error) {
	value, ok := sm.clients.Load(clientKey)
	if !ok {
		return nil, fmt.Errorf("failed to find Session for client %s", clientKey)
	}

	cs := value.(*clientSessions)
	cs.mu.Lock()
	sessionsLen := len(cs.sessions)

	if sessionsLen == 0 {
		cs.mu.Unlock()
		return nil, fmt.Errorf("client %s has no active sessions", clientKey)
	}

	// 使用简单的随机选择来实现负载均衡
	selectedIndex := rand.Intn(sessionsLen)
	selectedSession := cs.sessions[selectedIndex]
	cs.mu.Unlock()

	// 使用选中的会话创建拨号器
	return toDialer(selectedSession, ""), nil
}

func (sm *sessionManager) add(clientKey string, session *webtransport.Session) *webtransport.Session {
	// 获取或创建客户端会话列表
	value, _ := sm.clients.LoadOrStore(clientKey, &clientSessions{
		sessions: []*webtransport.Session{},
	})

	cs := value.(*clientSessions)
	cs.mu.Lock()
	cs.sessions = append(cs.sessions, session)
	cs.mu.Unlock()

	return session
}

func (sm *sessionManager) remove(clientKey string, session *webtransport.Session) {
	value, ok := sm.clients.Load(clientKey)
	if !ok {
		return
	}

	cs := value.(*clientSessions)
	cs.mu.Lock()

	for i, s := range cs.sessions {
		if s == session {
			cs.sessions = append(cs.sessions[:i], cs.sessions[i+1:]...)
			break
		} else {
			_ = s.CloseWithError(0, "服务器关闭")
		}
	}

	isEmpty := len(cs.sessions) == 0
	cs.mu.Unlock()

	if isEmpty {
		sm.clients.Delete(clientKey)
	}
}

func (sm *sessionManager) removeAll() {

	sm.clients.Range(func(key, value interface{}) bool {
		cs := value.(*clientSessions)
		for _, s := range cs.sessions {
			_ = s.CloseWithError(0, "服务器关闭")
		}
		return true
	})
	sm.clients.Clear()
}
