package gscore

import (
	"strconv"

	appcfg "github.com/KasumiYuku/Aurorix/lib/config"
	"github.com/KasumiYuku/Aurorix/lib/plugin"
)

// config 适配器运行配置，由 plugin_settings 面板驱动。
type config struct {
	URL               string // gscore WebSocket 服务地址，如 ws://127.0.0.1:8765
	Token             string // 与 gscore 核心配置 WS_TOKEN 一致
	BotID             string // gscore 侧平台标识，决定 /ws/{bot_id} 路由与消息归属
	BotSelfID         string // 机器人自身 ID，用于传递给gscore的bot_self_id字段，默认使用AppID
	ForwardAt         bool   // 转发群内 @机器人的消息
	ForwardAll        bool   // 转发群内所有消息
	ForwardPriv       bool   // 转发私聊消息
	ReconnectInterval int    // 断开后重连的固定间隔（秒）
}

func configFields() []plugin.ConfigField {
	return []plugin.ConfigField{
		{
			Key:         "url",
			Label:       "gscore WS 地址",
			Type:        "string",
			Placeholder: "ws://127.0.0.1:8765",
			Description: "gscore 的 WebSocket 服务地址，不含 /ws/{bot_id}",
		},
		{
			Key:         "token",
			Label:       "WS Token",
			Type:        "string",
			Description: "与 gscore 核心配置中的 WS_TOKEN 一致",
		},
		{
			Key:         "bot_id",
			Label:       "平台 ID",
			Type:        "string",
			Description: "gscore 侧平台标识，决定 /ws/{bot_id} 路由与消息归属",
			Default:     "Aurorix",
		},
		{
			Key:         "bot_self_id",
			Label:       "机器人自身 ID",
			Type:        "string",
			Description: "机器人自身 ID，用于传递给gscore的bot_self_id字段，默认使用AppID",
		},
		{
			Key:         "reconnect_interval",
			Label:       "重连间隔（秒）",
			Type:        "number",
			Description: "断开后按固定间隔重连",
			Default:     5,
		},
		{
			Key:         "forward_at",
			Label:       "转发群内@机器人消息",
			Type:        "boolean",
			Description: "开启后，群内 @机器人 的消息会转发给 gscore",
			Default:     true,
		},
		{
			Key:         "forward_all",
			Label:       "转发群内所有消息",
			Type:        "boolean",
			Description: "开启后，群内所有消息都会转发给 gscore（需要平台 GROUP_MESSAGE_CREATE 权限）",
			Default:     false,
		},
		{
			Key:         "forward_private",
			Label:       "转发私聊消息",
			Type:        "boolean",
			Description: "开启后，私聊消息会转发给 gscore",
			Default:     true,
		},
	}
}

func parseConfig(values map[string]any) config {
	return config{
		URL:               strDefault(values, "url", "ws://127.0.0.1:8765"),
		Token:             strDefault(values, "token", ""),
		BotID:             strDefault(values, "bot_id", "Aurorix"),
		BotSelfID:         strDefault(values, "bot_self_id", ""),
		ForwardAt:         boolDefault(values, "forward_at", true),
		ForwardAll:        boolDefault(values, "forward_all", false),
		ForwardPriv:       boolDefault(values, "forward_private", true),
		ReconnectInterval: intDefault(values, "reconnect_interval", 5),
	}
}

// selfID 返回用于 @ 判定的标识：优先 bot_self_id，其次主配置 appid，最后 bot_id 兜底。
// gscore 只能靠「at 段 == bot_self_id」判定 is_tome，所以取其一、保持一致即可，
// 完全不需要机器人的真实 openid。
func (c config) selfID() string {
	if c.BotSelfID != "" {
		return c.BotSelfID
	}
	if appID := appcfg.Current().AppId; appID != "" {
		return appID
	}
	return c.BotID
}

func strDefault(m map[string]any, key, def string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok && s != "" {
			return s
		}
	}
	return def
}

func boolDefault(m map[string]any, key string, def bool) bool {
	if v, ok := m[key]; ok {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return def
}

func intDefault(m map[string]any, key string, def int) int {
	v, ok := m[key]
	if !ok {
		return def
	}
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	case int64:
		return int(n)
	case string:
		if i, err := strconv.Atoi(n); err == nil {
			return i
		}
	}
	return def
}
