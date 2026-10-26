package gscore

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/KasumiYuku/Aurorix/lib/buttons"
	"github.com/KasumiYuku/Aurorix/lib/constant"
	"github.com/KasumiYuku/Aurorix/lib/context"
	"github.com/KasumiYuku/Aurorix/lib/sandbox"
)

// deliverSend 把 gscore 下行消息投递到平台目标。
func deliverSend(send *MessageSend) {
	if isLogMessage(send) {
		adapterLog.Debugf("忽略 gscore 日志回传: %s", summarizeSend(send))
		return
	}
	botAPI := inst.snapshotBotAPI()
	if botAPI == nil {
		adapterLog.Warnf("gscore 下行消息未投递：尚未收到平台事件，BotAPI 为空")
		return
	}
	targetID := send.TargetID
	if targetID == "" {
		adapterLog.Warnf("gscore 下行消息缺少 target_id，已丢弃: %s", summarizeSend(send))
		return
	}

	mm := &context.MessageManager{Qapi: botAPI}
	targetType := send.TargetType
	if targetType == "" {
		targetType = "group"
	}
	if targetType == "group" {
		mm.Target = constant.GroupMessage
		mm.GroupId = targetID
	} else {
		mm.Target = constant.PrivateMessage
		mm.UserId = targetID
	}

	// 沙盒: 目标落在进行中的沙盒会话上时, 回复只进沙盒 sink, 不投递到 QQ。
	if sess := sandbox.ByTarget(targetID); sess != nil {
		sess.Touch()
		mm.Sandbox = sess.Sink
		adapterLog.Debugf("gscore 下行消息进沙盒: %s", summarizeSend(send))
	}

	dispatchContent(mm, targetType, targetID, send.Content)
}

// summarizeSend 把下行消息序列化为便于排查的摘要文本（截断超长内容）。
func summarizeSend(send *MessageSend) string {
	b, err := json.Marshal(send)
	if err != nil {
		return fmt.Sprintf("bot_id=%s target_type=%s msg_id=%s", send.BotID, send.TargetType, send.MsgID)
	}
	s := string(b)
	if len(s) > 1024 {
		s = s[:1024] + "...(截断)"
	}
	return s
}

// isLogMessage 判断是否为 gscore 日志回传消息（所有消息段均为 log_* 类型）。
func isLogMessage(send *MessageSend) bool {
	if len(send.Content) == 0 {
		return false
	}
	for _, m := range send.Content {
		if !strings.HasPrefix(m.Type, "log_") {
			return false
		}
	}
	return true
}

// dispatchContent 按顺序处理下行消息段。文本/@ 合并为一条消息，markdown/图片/媒体各自成条。
func dispatchContent(mm *context.MessageManager, targetType, targetID string, content []Message) {
	var b *context.MsgBuilder
	flush := func() {
		if b == nil {
			return
		}
		_ = b.Initiative().Send()
		b = nil
	}

	for i := 0; i < len(content); i++ {
		m := content[i]
		switch m.Type {
		case "text":
			if b == nil {
				b = mm.Msg()
			}
			b.Text(rawString(m.Data))
		case "at":
			if b == nil {
				b = mm.Msg()
			}
			b.At(rawString(m.Data))
		case "markdown":
			flush()
			md := mm.Markdown(rawString(m.Data))
			if i+1 < len(content) && content[i+1].Type == "buttons" {
				if kb := buildKeyboard(content[i+1].Data); kb != nil {
					md.Keyboard(kb)
				}
				i++
			}
			md.SetInitiativeMessage()
			_ = md.Send()
		case "image":
			flush()
			img := mm.Image(gscoreImageSrc(rawString(m.Data)), "图片")
			img.SetInitiativeMessage()
			_ = img.Send()
		case "record":
			flush()
			v := mm.Voice(gscoreMediaSrc(rawString(m.Data)))
			v.SetInitiativeMessage()
			_ = v.Send()
		case "video":
			flush()
			v := mm.Video(gscoreMediaSrc(rawString(m.Data)))
			v.SetInitiativeMessage()
			_ = v.Send()
		case "file":
			flush()
			name, src := splitFile(rawString(m.Data))
			f := mm.File(src, name)
			f.SetInitiativeMessage()
			_ = f.Send()
		case "node":
			flush()
			var nested []Message
			if json.Unmarshal(m.Data, &nested) == nil {
				dispatchContent(mm, targetType, targetID, nested)
			}
		case "excute_delete_message":
			flush()
			var d struct {
				MessageID string `json:"message_id"`
			}
			if json.Unmarshal(m.Data, &d) == nil && d.MessageID != "" {
				if api := inst.snapshotBotAPI(); api != nil {
					if targetType == "group" {
						_ = api.RecallGroupMessage(targetID, d.MessageID, false)
					} else {
						_ = api.RecallC2CMessage(targetID, d.MessageID, false)
					}
				}
			}
		case "excute_ban_user":
			adapterLog.Debugf("忽略 excute_ban_user：Aurorix 无禁言 API")
		case "image_size", "group":
			// 元信息段，忽略
		case "template_markdown", "template_buttons":
			adapterLog.Warnf("gscore 下发 %s，当前适配器不支持模板消息；建议在 gscore 中将本 bot_id 加入 SendMDPlatform", m.Type)
		default:
			adapterLog.Debugf("忽略未知 gscore 消息类型: %s", m.Type)
		}
	}
	flush()
}

func rawString(raw json.RawMessage) string {
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return ""
	}
	return s
}

func decodeBase64(s string) []byte {
	for _, enc := range []*base64.Encoding{
		base64.StdEncoding,
		base64.RawStdEncoding,
		base64.URLEncoding,
		base64.RawURLEncoding,
	} {
		if b, err := enc.DecodeString(s); err == nil {
			return b
		}
	}
	if pad := len(s) % 4; pad != 0 {
		if b, err := base64.StdEncoding.DecodeString(s + strings.Repeat("=", 4-pad)); err == nil {
			return b
		}
	}
	return nil
}

// gscoreImageSrc 把 gscore 图片数据转换为 Aurorix 可接受的 src。
func gscoreImageSrc(data string) any {
	if strings.HasPrefix(data, "base64://") {
		if b := decodeBase64(strings.TrimPrefix(data, "base64://")); b != nil {
			return b
		}
		return data
	}
	if strings.HasPrefix(data, "link://") {
		return strings.TrimPrefix(data, "link://")
	}
	return data
}

// gscoreMediaSrc 把 gscore 语音/视频数据转换为 Aurorix 可接受的 src。
func gscoreMediaSrc(data string) any {
	if strings.HasPrefix(data, "base64://") {
		if b := decodeBase64(strings.TrimPrefix(data, "base64://")); b != nil {
			return b
		}
	}
	return data
}

// splitFile 解析 gscore 文件段 data = "文件名|base64..." 或 "文件名|link://..."。
func splitFile(data string) (string, any) {
	name := "file"
	if idx := strings.Index(data, "|"); idx >= 0 {
		name = data[:idx]
		data = data[idx+1:]
	}
	var src any = data
	if strings.HasPrefix(data, "link://") {
		src = strings.TrimPrefix(data, "link://")
	} else if b := decodeBase64(data); b != nil {
		src = b
	}
	if name == "" {
		name = "file"
	}
	return name, src
}

// gscoreButton 对应 gscore message_models.Button。
type gscoreButton struct {
	Text           string   `json:"text"`
	Data           string   `json:"data"`
	PressedText    string   `json:"pressed_text"`
	Style          int      `json:"style"`
	Action         int      `json:"action"`    // 0 链接 / 1 回调 / 2 命令 / -1 自适应
	Permisson      int      `json:"permisson"` // 0 指定用户 / 1 管理者 / 2 所有人
	SpecifyUserIDs []string `json:"specify_user_ids"`
	UnsupportTips  string   `json:"unsupport_tips"`
}

func buildKeyboard(raw json.RawMessage) *buttons.Keyboard {
	var rows [][]gscoreButton
	if err := json.Unmarshal(raw, &rows); err != nil {
		var flat []gscoreButton
		if err2 := json.Unmarshal(raw, &flat); err2 != nil {
			adapterLog.Debugf("解析 gscore 按钮失败: %v", err)
			return nil
		}
		rows = [][]gscoreButton{flat}
	}

	kb := &buttons.Keyboard{}
	row := 0
	for _, r := range rows {
		for ci, gb := range r {
			btn, err := kb.AppendButton(
				fmt.Sprintf("gscore_%d_%d", row, ci),
				gb.Text,
				gb.PressedText,
				buttons.ButtonStyle(gb.Style),
				row,
			)
			if err != nil {
				adapterLog.Debugf("添加 gscore 按钮失败: %v", err)
				continue
			}
			applyGscoreButton(btn, gb)
		}
		if len(r) > 0 {
			row++
		}
	}
	if row == 0 {
		return nil
	}
	return kb
}

func applyGscoreButton(btn *buttons.Button, gb gscoreButton) {
	if gb.UnsupportTips != "" {
		btn.SetUnsupportedTip(gb.UnsupportTips)
	}
	switch gb.Action {
	case 0: // 链接
		btn.SetHref(gb.Data)
	case 1: // 回调：点击后把数据作为消息回传给 gscore
		btn.SetCallback(gb.Data, forwardCallback)
	case 2: // 命令：点击后自动发送命令文本
		btn.SetAutoCommand(gb.Data, true, false)
	default: // -1 自适应
		if strings.HasPrefix(gb.Data, "http://") || strings.HasPrefix(gb.Data, "https://") {
			btn.SetHref(gb.Data)
		} else {
			btn.SetAutoCommand(gb.Data, true, false)
		}
	}
	switch gb.Permisson {
	case 0:
		btn.SetUserWhiteList(gb.SpecifyUserIDs)
	case 1:
		btn.SetPermission(buttons.Admin)
	default:
		btn.SetPermission(buttons.AllUser)
	}
}

// forwardCallback 把 gscore 回调按钮的数据作为消息回传给 gscore。
func forwardCallback(ctx *context.CallbackContext) error {
	inst.rememberBotAPI(ctx.Qapi)
	cfg := inst.snapshotConfig()
	recv := interactionToReceive(ctx, cfg)
	if recv == nil {
		return nil
	}
	data, err := json.Marshal(recv)
	if err != nil {
		adapterLog.Warnf("编码按钮回调消息失败: %v", err)
		return nil
	}
	if err := inst.send(data); err != nil {
		adapterLog.Debugf("回传按钮回调到 gscore 失败: %v", err)
	}
	return nil
}

func interactionToReceive(ctx *context.CallbackContext, cfg config) *MessageReceive {
	if ctx.Data == "" {
		return nil
	}
	userType := "group"
	groupID := ctx.GroupId
	if ctx.Target == constant.PrivateMessage {
		userType = "direct"
		groupID = ""
	}
	return &MessageReceive{
		BotID:     cfg.BotID,
		BotSelfID: cfg.selfID(),
		MsgID:     ctx.MessageId,
		UserType:  userType,
		GroupID:   groupID,
		UserID:    ctx.UserId,
		Sender:    map[string]any{},
		UserPM:    6,
		Content:   []Message{textSegment(ctx.Data)},
	}
}
