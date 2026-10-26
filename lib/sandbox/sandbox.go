// Package sandbox 管理台沙盒会话登记表。
//
// 一次沙盒请求可能产生两条回复路径: 框架指令的同步回复, 以及 gscore 这类适配器的
// 异步回复(WebSocket 往返)。两者都写进同一个 sink, 前端按 session 轮询取走。
package sandbox

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
	"sync/atomic"
	"time"

	"github.com/KasumiYuku/Aurorix/lib/message"
)

// TTL 是会话存活时间: 过期后自动回收, 避免长跑进程里堆积。
const TTL = 5 * time.Minute

// Session 一次沙盒交互。
type Session struct {
	ID     string
	Target string // 本次交互的 group/user openid, 适配器下行按它认领
	Sink   *message.SandboxSink
	at     time.Time
	done   atomic.Bool // 框架指令已执行完(适配器回复可能还在路上)
}

var (
	mu       sync.Mutex
	byID     = make(map[string]*Session)
	byTarget = make(map[string]*Session)
)

// New 新建并登记会话。target 为空时随机生成一个。
func New(target string) *Session {
	mu.Lock()
	defer mu.Unlock()
	reapLocked()
	if target == "" {
		target = "sandbox-" + randomID(8)
	}
	s := &Session{
		ID:     randomID(12),
		Target: target,
		Sink:   message.NewSandboxSink(),
		at:     time.Now(),
	}
	byID[s.ID] = s
	byTarget[target] = s
	return s
}

// ByID 按会话 ID 查找; 不存在返回 nil。
func ByID(id string) *Session {
	if id == "" {
		return nil
	}
	mu.Lock()
	defer mu.Unlock()
	return byID[id]
}

// ByTarget 按 group/user openid 查找, 供适配器下行认领。
func ByTarget(target string) *Session {
	if target == "" {
		return nil
	}
	mu.Lock()
	defer mu.Unlock()
	return byTarget[target]
}

// Touch 续期: 每次轮询都算活跃。
func (s *Session) Touch() {
	if s == nil {
		return
	}
	mu.Lock()
	s.at = time.Now()
	mu.Unlock()
}

// MarkDone 标记框架指令已执行完; 适配器的异步回复可能仍在路上。
func (s *Session) MarkDone() {
	if s != nil {
		s.done.Store(true)
	}
}

// Done 报告框架指令是否已执行完。
func (s *Session) Done() bool { return s != nil && s.done.Load() }

func reapLocked() {
	now := time.Now()
	for id, s := range byID {
		if now.Sub(s.at) > TTL {
			delete(byID, id)
			if byTarget[s.Target] == s {
				delete(byTarget, s.Target)
			}
		}
	}
}

func randomID(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return time.Now().Format("150405.000000000")
	}
	return hex.EncodeToString(b)
}
