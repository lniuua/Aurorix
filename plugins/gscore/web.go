package gscore

import (
	"embed"
	"encoding/json"
	"io/fs"
	"net/http"
	"time"

	"github.com/KasumiYuku/Aurorix/lib/plugin"
)

// 插件自带的控制台: 静态页 + 两个接口。
// 走的是框架的 plugin.WebUI 扩展点, 不需要改框架代码:
//   - 静态页挂 /gscore/
//   - 接口挂 /gscore/api/...(已带管理台鉴权)
//
//go:embed all:web
var webFiles embed.FS

func webUI() *plugin.WebUI {
	sub, err := fs.Sub(webFiles, "web")
	if err != nil {
		return nil
	}
	return &plugin.WebUI{SPA: sub, API: registerWebAPI}
}

func registerWebAPI(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/status", handleWebStatus)
	mux.HandleFunc("POST /api/reconnect", handleWebReconnect)
}

// handleWebStatus 返回连接状态快照; 时间点一律用毫秒时间戳, 0 表示从未发生。
func handleWebStatus(w http.ResponseWriter, r *http.Request) {
	st := inst.snapshotStatus()
	writeWebJSON(w, http.StatusOK, map[string]any{
		"connected":          st.Connected,
		"url":                st.URL,
		"bot_id":             st.BotID,
		"bot_self_id":        st.BotSelfID,
		"last_error":         st.LastError,
		"connected_since_ms": unixMilli(st.ConnectedAt),
		"last_recv_ms":       unixMilli(st.LastRecvAt),
		"last_send_ms":       unixMilli(st.LastSendAt),
		"recv_total":         st.RecvTotal,
		"send_total":         st.SendTotal,
		"connect_total":      st.ConnectTotal,
		"error_total":        st.ErrorTotal,
		"pending":            deliverer.backlog(),
		"now":                time.Now().UnixMilli(),
	})
}

func handleWebReconnect(w http.ResponseWriter, r *http.Request) {
	inst.reconnect()
	writeWebJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func unixMilli(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.UnixMilli()
}

func writeWebJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
