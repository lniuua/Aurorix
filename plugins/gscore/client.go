package gscore

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/KasumiYuku/Aurorix/lib/api"
	"github.com/KasumiYuku/Aurorix/lib/logx"
	"github.com/gorilla/websocket"
)

var adapterLog = logx.New("gscore")

var errNotConnected = fmt.Errorf("gscore 未连接")

type adapter struct {
	mu        sync.Mutex // 保护 cfg / conn / connected / lastErr / botAPI
	cfg       config
	conn      *websocket.Conn
	connected bool
	lastErr   string
	botAPI    *api.BotAPI

	reloadCh chan struct{}
	writeMu  sync.Mutex // 串行化写与关连接，避免与 gorilla 并发冲突

	startOnce sync.Once
}

var inst = &adapter{reloadCh: make(chan struct{}, 1)}

func (a *adapter) ensureStarted() {
	a.startOnce.Do(func() { go a.run() })
}

func (a *adapter) updateConfig(cfg config) {
	a.mu.Lock()
	a.cfg = cfg
	oldConn := a.conn
	a.conn = nil
	a.connected = false
	a.mu.Unlock()

	if oldConn != nil {
		a.writeMu.Lock()
		_ = oldConn.Close()
		a.writeMu.Unlock()
	}
	signalReload(a.reloadCh)
}

func (a *adapter) reconnect() {
	a.mu.Lock()
	oldConn := a.conn
	a.conn = nil
	a.connected = false
	a.mu.Unlock()

	if oldConn != nil {
		a.writeMu.Lock()
		_ = oldConn.Close()
		a.writeMu.Unlock()
	}
	signalReload(a.reloadCh)
}

func signalReload(ch chan struct{}) {
	select {
	case ch <- struct{}{}:
	default:
	}
}

func reloadTriggered(ch chan struct{}, d time.Duration) bool {
	select {
	case <-ch:
		return true
	case <-time.After(d):
		return false
	}
}

// reconnectInterval 取重连固定间隔：配置的秒数，非法或未配置回退 5 秒。
func reconnectInterval(cfg config) time.Duration {
	d := time.Duration(cfg.ReconnectInterval) * time.Second
	if d <= 0 {
		d = 5 * time.Second
	}
	return d
}

// run 连接生命周期：拨号 → 读循环 → 固定间隔重连。配置变化通过 reloadCh 打断。
func (a *adapter) run() {
	for {
		a.mu.Lock()
		cfg := a.cfg
		a.mu.Unlock()

		interval := reconnectInterval(cfg)

		if cfg.URL == "" {
			select {
			case <-a.reloadCh:
			case <-time.After(5 * time.Second):
			}
			continue
		}

		conn, err := dial(cfg)
		if err != nil {
			a.setError(err.Error())
			adapterLog.Warnf("连接 gscore 失败: %v（%v 后重试）", err, interval)
			reloadTriggered(a.reloadCh, interval)
			continue
		}

		a.setConn(conn)
		adapterLog.Infof("已连接 gscore %s (bot_id=%s)", cfg.URL, cfg.BotID)

		a.readLoop(conn)

		a.mu.Lock()
		if a.conn == conn {
			a.conn = nil
			a.connected = false
		}
		a.mu.Unlock()
		a.writeMu.Lock()
		_ = conn.Close()
		a.writeMu.Unlock()

		adapterLog.Warnf("gscore 连接断开，%v 后重连", interval)
		reloadTriggered(a.reloadCh, interval)
	}
}

func dial(cfg config) (*websocket.Conn, error) {
	base := strings.TrimRight(strings.TrimSpace(cfg.URL), "/")
	if base == "" {
		return nil, fmt.Errorf("gscore URL 为空")
	}
	u := fmt.Sprintf("%s/ws/%s", base, url.PathEscape(cfg.BotID))
	if cfg.Token != "" {
		u += "?token=" + url.QueryEscape(cfg.Token)
	}
	d := websocket.Dialer{HandshakeTimeout: 10 * time.Second}
	conn, _, err := d.Dial(u, nil)
	if err != nil {
		return nil, err
	}
	return conn, nil
}

func (a *adapter) readLoop(conn *websocket.Conn) {
	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			a.setError(err.Error())
			return
		}
		var send MessageSend
		if err := json.Unmarshal(data, &send); err != nil {
			adapterLog.Warnf("解析 gscore 下行消息失败: %v", err)
			continue
		}
		// 只入队、不投递：慢速的媒体上传交给投递池，读循环保持畅通。
		deliverer.submit(&send)
	}
}

func (a *adapter) send(data []byte) error {
	a.writeMu.Lock()
	defer a.writeMu.Unlock()
	a.mu.Lock()
	conn := a.conn
	a.mu.Unlock()
	if conn == nil {
		return errNotConnected
	}
	return conn.WriteMessage(websocket.BinaryMessage, data)
}

func (a *adapter) setConn(conn *websocket.Conn) {
	a.mu.Lock()
	a.conn = conn
	a.connected = true
	a.lastErr = ""
	a.mu.Unlock()
}

func (a *adapter) setError(err string) {
	a.mu.Lock()
	a.conn = nil
	a.connected = false
	a.lastErr = err
	a.mu.Unlock()
}

func (a *adapter) rememberBotAPI(b *api.BotAPI) {
	if b == nil {
		return
	}
	a.mu.Lock()
	if a.botAPI == nil {
		a.botAPI = b
	}
	a.mu.Unlock()
}

func (a *adapter) snapshotBotAPI() *api.BotAPI {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.botAPI
}

func (a *adapter) snapshotConfig() config {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.cfg
}

func (a *adapter) statusLine() (url string, connected bool, botID string, lastErr string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.cfg.URL, a.connected, a.cfg.BotID, a.lastErr
}
