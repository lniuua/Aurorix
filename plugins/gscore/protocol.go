package gscore

import (
	"encoding/json"
	"strings"

	appcfg "github.com/KasumiYuku/Aurorix/lib/config"
	"github.com/KasumiYuku/Aurorix/lib/constant"
	"github.com/KasumiYuku/Aurorix/lib/event"
	"github.com/KasumiYuku/Aurorix/lib/structers"
)

// Message 与 gscore 的 msgspec Message 对齐。
type Message struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

// MessageReceive 上行消息：平台 → gscore。
type MessageReceive struct {
	BotID     string         `json:"bot_id"`
	BotSelfID string         `json:"bot_self_id"`
	MsgID     string         `json:"msg_id"`
	UserType  string         `json:"user_type"` // group | direct
	GroupID   string         `json:"group_id,omitempty"`
	UserID    string         `json:"user_id"`
	Sender    map[string]any `json:"sender,omitempty"`
	UserPM    int            `json:"user_pm"`
	Content   []Message      `json:"content"`
}

// MessageSend 下行消息：gscore → 平台。
type MessageSend struct {
	Content    []Message `json:"content"`
	BotID      string    `json:"bot_id"`
	BotSelfID  string    `json:"bot_self_id"`
	TargetType string    `json:"target_type"` // group | direct
	TargetID   string    `json:"target_id"`
	MsgID      string    `json:"msg_id"`
	Echo       string    `json:"echo"`
}

func quoteJSON(s string) []byte {
	b, _ := json.Marshal(s)
	return b
}

func textSegment(s string) Message {
	return Message{Type: "text", Data: json.RawMessage(quoteJSON(s))}
}

func atSegment(id string) Message {
	return Message{Type: "at", Data: json.RawMessage(quoteJSON(id))}
}

// eventToReceive 把平台消息事件转换为 gscore 上行消息。
// atMe 为 true 表示该事件本身即「@机器人」消息（GROUP_AT_MESSAGE_CREATE）。
func eventToReceive(e *event.MessageEvent, cfg config, atMe bool) *MessageReceive {
	userType := "group"
	groupID := e.GroupID
	if e.Origin == constant.PrivateMessage {
		userType = "direct"
		groupID = ""
	}

	content := make([]Message, 0, 4)
	if s := strings.TrimSpace(e.Content); s != "" {
		content = append(content, textSegment(s))
	}

	selfID := cfg.selfID()
	isTome := atMe
	for _, m := range e.Mentions {
		if m.IsYou {
			isTome = true
			continue
		}
		if m.ID != "" {
			content = append(content, atSegment(m.ID))
		}
	}
	if isTome {
		content = append(content, atSegment(selfID))
	}
	for _, a := range e.Attachments {
		if strings.HasPrefix(a.ContentType, "image/") && a.URL != "" {
			content = append(content, Message{Type: "image", Data: json.RawMessage(quoteJSON(a.URL))})
		}
	}
	if e.Quote != nil {
		if q := strings.TrimSpace(e.Quote.Content); q != "" {
			content = append(content, Message{Type: "reply", Data: json.RawMessage(quoteJSON(q))})
		}
		for _, a := range e.Quote.Attachments {
			if strings.HasPrefix(a.ContentType, "image/") && a.URL != "" {
				content = append(content, Message{Type: "image", Data: json.RawMessage(quoteJSON(a.URL))})
			}
		}
	}

	if len(content) == 0 {
		return nil
	}

	sender := map[string]any{}
	if e.UserID != "" {
		sender["user_id"] = e.UserID
	}
	sender["bot"] = false
	if e.Username != "" {
		sender["nickname"] = e.Username
	}
	if appID := appcfg.Current().AppId; appID != "" && e.UserID != "" {
		sender["avatar"] = structers.AvatarURL(appID, e.UserID)
	}

	return &MessageReceive{
		BotID:     cfg.BotID,
		BotSelfID: selfID,
		MsgID:     e.MessageID,
		UserType:  userType,
		GroupID:   groupID,
		UserID:    e.UserID,
		Sender:    sender,
		UserPM:    6,
		Content:   content,
	}
}
