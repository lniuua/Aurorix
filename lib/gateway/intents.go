package gateway

const defaultIntents = intentUserMessage | intentInteractions | intentGroupMembers | intentMessageAudit

const (
	intentGuildMessages = 1 << 9
	intentGroupMembers  = 1 << 24
	intentUserMessage   = 1 << 25
	intentInteractions  = 1 << 26
	intentMessageAudit  = 1 << 27
)

var eventIntentMap = map[string]int{
	"GROUP_AT_MESSAGE_CREATE": intentUserMessage,
	"GROUP_MESSAGE_CREATE":    intentUserMessage,
	"C2C_MESSAGE_CREATE":      intentUserMessage,
	"INTERACTION_CREATE":      intentInteractions,
	"GROUP_JOIN_REQUEST":      intentUserMessage,
	"GROUP_MEMBER_ADD":        intentGroupMembers,
	"GROUP_MEMBER_REMOVE":     intentGroupMembers,
	"GROUP_ADD_ROBOT":         intentUserMessage,
	"GROUP_DEL_ROBOT":         intentUserMessage,
	"GROUP_MSG_RECEIVE":       intentUserMessage,
	"GROUP_MSG_REJECT":        intentUserMessage,
	"C2C_MSG_RECEIVE":         intentUserMessage,
	"C2C_MSG_REJECT":          intentUserMessage,
	"FRIEND_ADD":              intentUserMessage,
	"FRIEND_DEL":              intentUserMessage,
	"MESSAGE_AUDIT_PASS":      intentMessageAudit,
	"MESSAGE_AUDIT_REJECT":    intentMessageAudit,
}

// IntentEvents 可订阅事件全集(顺序固定), 供管理台枚举选择。
func IntentEvents() []string {
	return []string{
		"GROUP_AT_MESSAGE_CREATE",
		"GROUP_MESSAGE_CREATE",
		"C2C_MESSAGE_CREATE",
		"INTERACTION_CREATE",
		"GROUP_JOIN_REQUEST",
		"GROUP_MEMBER_ADD",
		"GROUP_MEMBER_REMOVE",
		"GROUP_ADD_ROBOT",
		"GROUP_DEL_ROBOT",
		"GROUP_MSG_RECEIVE",
		"GROUP_MSG_REJECT",
		"C2C_MSG_RECEIVE",
		"C2C_MSG_REJECT",
		"FRIEND_ADD",
		"FRIEND_DEL",
		"MESSAGE_AUDIT_PASS",
		"MESSAGE_AUDIT_REJECT",
	}
}

// Intents 将配置的事件名列表编译成 intent 位掩码。
func Intents(events []string) int {
	bits := 0
	for _, e := range events {
		bits |= eventIntentMap[e]
	}
	if bits == 0 {
		bits = defaultIntents
	}
	return bits
}

// IntentDescriptions 事件名 → 说明, 供管理台悬停展示。
func IntentDescriptions() map[string]string {
	return map[string]string{
		"GROUP_AT_MESSAGE_CREATE": "群内 @机器人 的消息",
		"GROUP_MESSAGE_CREATE":    "群内所有消息（含 @机器人）",
		"C2C_MESSAGE_CREATE":      "私聊消息",
		"INTERACTION_CREATE":      "消息按钮点击等互动事件",
		"GROUP_JOIN_REQUEST":      "入群申请",
		"GROUP_MEMBER_ADD":        "群成员增加",
		"GROUP_MEMBER_REMOVE":     "群成员减少/退群",
		"GROUP_ADD_ROBOT":         "机器人被拉入群",
		"GROUP_DEL_ROBOT":         "机器人被移出群",
		"GROUP_MSG_RECEIVE":       "群开启接收机器人消息",
		"GROUP_MSG_REJECT":        "群关闭接收机器人消息",
		"C2C_MSG_RECEIVE":         "用户开启接收机器人消息",
		"C2C_MSG_REJECT":          "用户关闭接收机器人消息",
		"FRIEND_ADD":              "用户添加机器人为好友",
		"FRIEND_DEL":              "用户删除机器人好友",
		"MESSAGE_AUDIT_PASS":      "消息审核通过",
		"MESSAGE_AUDIT_REJECT":    "消息审核不通过",
	}
}
