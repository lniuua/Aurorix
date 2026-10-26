package admin

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/KasumiYuku/Aurorix/lib/api"
	"github.com/KasumiYuku/Aurorix/lib/constant"
	"github.com/KasumiYuku/Aurorix/lib/event"
	"github.com/KasumiYuku/Aurorix/lib/message"
	"github.com/KasumiYuku/Aurorix/lib/middleware"
	"github.com/KasumiYuku/Aurorix/lib/sandbox"
	"github.com/KasumiYuku/Aurorix/lib/structers"
)

type sandboxRequest struct {
	Text  string `json:"text"`
	Scene string `json:"scene"` // group(默认) / c2c
	Role  string `json:"role"`  // member(默认) / admin / owner
	User  string `json:"user"`  // 可选, 模拟的发送者 openid
	Group string `json:"group"` // 可选, 模拟的群 openid
}

type sandboxReply struct {
	// Session 是本次交互的会话 ID: 适配器(如 gscore)的异步回复要靠它轮询。
	Session  string                   `json:"session"`
	Messages []message.SandboxMessage `json:"messages"`
	Scene    string                   `json:"scene"`
	Text     string                   `json:"text"`
}

// handleSandboxSend 在沙盒里同步执行一条消息指令, 返回机器人本该发出去的全部内容。
// 不会投递到平台, 也不进消息存档与统计。
func handleSandboxSend(client *api.BotAPI) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req sandboxRequest
		if err := readJSON(r, &req); err != nil {
			writeJSON(w, http.StatusBadRequest, H{"error": "请求体不是合法 JSON"})
			return
		}
		text := strings.TrimSpace(req.Text)
		if text == "" {
			writeJSON(w, http.StatusBadRequest, H{"error": "消息内容不能为空"})
			return
		}
		if client == nil {
			writeJSON(w, http.StatusServiceUnavailable, H{"error": "机器人尚未完成初始化"})
			return
		}

		userID := strings.TrimSpace(req.User)
		if userID == "" {
			userID = "sandbox-user"
		}
		groupID := strings.TrimSpace(req.Group)
		if groupID == "" {
			groupID = "sandbox-group"
		}
		scene := "group"
		if req.Scene == "c2c" {
			scene = "c2c"
		}

		// 沙盒默认按普通成员跑; 要测管理员/群主/机器人主人指令就在前端切角色。
		role := constant.RoleMember
		switch req.Role {
		case "admin":
			role = constant.RoleAdmin
		case "owner":
			role = constant.RoleOwner
		case "bot_owner":
			role = constant.RoleBotOwner
		}

		stamp := time.Now().UnixMilli()
		payload := structers.Payload{
			ID: fmt.Sprintf("sandbox-event-%d", stamp),
		}
		payload.Data.Id = fmt.Sprintf("sandbox-msg-%d", stamp)
		payload.Data.Content = text
		payload.Data.Author.ID = userID
		payload.Data.Author.Username = "沙盒用户"
		payload.Data.Author.Role = role
		if scene == "c2c" {
			payload.T = string(constant.C2C_MESSAGE_CREATE)
			payload.EventType = constant.C2C_MESSAGE_CREATE
			payload.Data.Author.UserOpenID = userID
			payload.Data.UserOpenID = userID
		} else {
			payload.T = string(constant.GROUP_AT_MESSAGE_CREATE)
			payload.EventType = constant.GROUP_AT_MESSAGE_CREATE
			payload.Data.Author.MemberOpenID = userID
			payload.Data.GroupMemberOpenID = userID
			payload.Data.GroupOpenID = groupID
		}

		target := groupID
		if scene == "c2c" {
			target = userID
		}
		sess := sandbox.New(target)

		// 1) 框架指令: 异步执行(慢指令不再拖住 HTTP 请求), 跑完标记 done;
		//    期间的回复全部进 sink, 由前端轮询取走。
		go func() {
			middleware.Sandbox(payload, client, sess.Sink)
			sess.MarkDone()
		}()
		// 2) 适配器(gscore 等): 只推给订阅者, 它们的异步回复按 target 认领回同一个 sink。
		event.Emit(payload, client)

		writeJSON(w, http.StatusOK, sandboxReply{
			Session:  sess.ID,
			Messages: sess.Sink.Messages(),
			Scene:    scene,
			Text:     text,
		})
	}
}

// handleSandboxPoll 取回沙盒会话里新增的回复(适配器的异步回复靠它)。
// after 是前端已收到的条数, 返回该下标之后的消息。
func handleSandboxPoll(w http.ResponseWriter, r *http.Request) {
	sess := sandbox.ByID(r.URL.Query().Get("id"))
	if sess == nil {
		writeJSON(w, http.StatusNotFound, H{"error": "沙盒会话不存在或已过期"})
		return
	}
	sess.Touch()

	all := sess.Sink.Messages()
	after, _ := strconv.Atoi(r.URL.Query().Get("after"))
	if after < 0 {
		after = 0
	}
	if after > len(all) {
		after = len(all)
	}
	writeJSON(w, http.StatusOK, H{
		"messages": all[after:],
		"total":    len(all),
		"done":     sess.Done(),
	})
}
