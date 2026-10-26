package middleware

import (
	"fmt"
	"github.com/KasumiYuku/Aurorix/lib/api"
	"github.com/KasumiYuku/Aurorix/lib/buttons"
	"github.com/KasumiYuku/Aurorix/lib/config"
	"github.com/KasumiYuku/Aurorix/lib/constant"
	"github.com/KasumiYuku/Aurorix/lib/context"
	"github.com/KasumiYuku/Aurorix/lib/event"
	"github.com/KasumiYuku/Aurorix/lib/logx"
	"github.com/KasumiYuku/Aurorix/lib/message"
	"github.com/KasumiYuku/Aurorix/lib/parser"
	"github.com/KasumiYuku/Aurorix/lib/plugin"
	"github.com/KasumiYuku/Aurorix/lib/role"
	"github.com/KasumiYuku/Aurorix/lib/state"
	"github.com/KasumiYuku/Aurorix/lib/stats"
	"github.com/KasumiYuku/Aurorix/lib/structers"
	"github.com/KasumiYuku/Aurorix/lib/templates"
	"github.com/KasumiYuku/Aurorix/lib/utils"
	"strings"
)

var messageLog = logx.New("message")

func init() {
	event.SetBuiltinHook(ProcessPayload)
	event.SetPreFilter(globalAccessFilter)
	event.SetTaskScheduler(func(f func()) { pool.Go(f) })
}

type commandDispatchOpts struct {
	userID       string
	groupID      string
	origin       constant.MessageOrigin
	raw          string
	checkPrivate bool
	// sandbox 非空时同步执行并把回复收集进它, 用于管理台沙盒。
	sandbox *message.SandboxSink
}

func dispatchCommand(payload structers.Payload, client *api.BotAPI, opts commandDispatchOpts) {
	tokens := strings.Fields(payload.Data.Content)
	if len(tokens) == 0 {
		return
	}
	rootCommand, afterRoot, ok := plugin.ResolveRoot(tokens)
	if !ok {
		return
	}
	if !plugin.Enabled(rootCommand.PluginId) {
		return
	}
	resolvedCommand, commandPath, rest := plugin.Resolve(rootCommand, afterRoot)
	if resolvedCommand == rootCommand && len(afterRoot) > 0 && rootCommand.SubCommandFallback != nil {
		fallback := *rootCommand
		fallback.Handle = rootCommand.SubCommandFallback
		resolvedCommand = &fallback
	}
	ctx := &context.MessageContext{
		UserMessage: message.UserMessage{
			Content:     payload.Data.Content,
			Attachments: payload.Data.Attachments,
		},
		Raw:      opts.raw,
		Username: payload.Data.Author.Username,
		Mentions: payload.Data.Mentions,
		Quote:    structers.ExtractQuote(payload.Data.MsgElements, payload.Data.MessageScene.Ext),
	}
	ctx.Init(payload.Data.Id, payload.ID, client)
	ctx.BindStorage(resolvedCommand.PluginId, commandPath)
	ctx.PluginId = resolvedCommand.PluginId
	// 记下触发本次指令的原消息文本, 供底部「复读」按钮把原文回填到输入框。
	ctx.MessageManager.OriginalInput = payload.Data.Content
	// 指令声明了豁免(或它挂在声明的根指令下)时, 本次回复一律不追加底部按钮。
	ctx.MessageManager.NoFooter = rootCommand.NoFooter || resolvedCommand.NoFooter
	ctx.MessageManager.Sandbox = opts.sandbox
	if opts.groupID != "" {
		ctx.SetGroupId(opts.groupID)
	}
	ctx.SetUserId(opts.userID)
	ctx.SetMessageOrigin(opts.origin)

	authorRole := role.ResolveAny(payloadUserIDs(payload), opts.groupID, payload.Data.Author.Role)

	// 角色门槛: 访问规则里配了覆盖值就用它, 否则用指令声明的。
	rootRole := plugin.EffectiveRole(rootCommand.PluginId, rootCommand.Prefix, rootCommand.Role)
	if !rootRole.CanUse(authorRole) {
		messageLog.Warnf("用户%v无权限使用%v指令", payload.Data.Author.Username, rootCommand.Prefix)
		permissionDenied(rootCommand, ctx)
		return
	}
	if resolvedCommand != rootCommand {
		subRole := plugin.EffectiveRole(resolvedCommand.PluginId, commandPath, resolvedCommand.Role)
		if !subRole.CanUse(authorRole) {
			messageLog.Warnf("用户%v无权限使用%v指令", payload.Data.Author.Username, commandPath)
			permissionDenied(resolvedCommand, ctx)
			return
		}
	}
	if opts.checkPrivate && (rootCommand.DisablePrivate || resolvedCommand.DisablePrivate) {
		permissionDenied(resolvedCommand, ctx)
		return
	}
	if !plugin.CanUse(rootCommand.PluginId, commandPath, opts.userID, opts.groupID) {
		permissionDenied(resolvedCommand, ctx)
		return
	}

	if resolvedCommand.Handle == nil {
		return
	}

	if resolvedCommand.Args != nil {
		parsed, usage, err := parser.ParseArgs(commandPath, resolvedCommand.Args, rest)
		if err != nil {
			content, terr := templates.FillMarkdownTemplate("Card", templates.Args{
				"title": "❌ 指令参数错误",
				"fields": []any{
					map[string]any{"label": "原因", "content": err.Error()},
					map[string]any{"label": "用法", "content": usageOrPath(usage, commandPath)},
				},
			})
			if terr != nil {
				messageLog.Errorf("生成用法提示失败: %v", terr)
				return
			}
			msg := ctx.Msg()
			if opts.groupID != "" && ctx.UserId != "" {
				msg.At(ctx.UserId, true)
			}
			if sendErr := msg.Markdown(content).Send(); sendErr != nil {
				messageLog.Errorf("发送用法提示失败: %v", sendErr)
			}
			return
		}
		ctx.Parsed = parsed
	} else {
		ctx.Parsed = rawArgs(payload.Data.Content, tokens, rest, rootCommand)
	}
	if opts.sandbox != nil {
		// 沙盒同步执行: 调用方要等回复收集完才能返回。
		executeCommand(resolvedCommand, ctx)
		return
	}
	pool.Go(func() { executeCommand(resolvedCommand, ctx) })
}

func rawArgs(content string, tokens, rest []string, rootCommand *plugin.Command) string {
	n := len(tokens) - len(rest)
	if n > 0 {
		pos := 0
		for _, tok := range tokens[:n] {
			i := strings.Index(content[pos:], tok)
			if i < 0 {
				return strings.Join(rest, " ")
			}
			pos += i + len(tok)
		}
		return strings.TrimLeft(content[pos:], " \t\n")
	}
	for _, p := range constant.PrefixChars() {
		if p != "" && strings.HasPrefix(tokens[0], p) {
			i := strings.Index(content, tokens[0])
			if i < 0 {
				return strings.Join(rest, " ")
			}
			return content[i+len(p)+len(rootCommand.Prefix):]
		}
	}
	return strings.Join(rest, " ")
}

// usageOrPath 优先用解析器给的完整用法(命令 + 位置参数 + flags 及其说明);
// 解析器拿不到时才退回"前缀 + 命令路径" —— 后者只说命令名, 等于没告诉用户参数怎么写。
func usageOrPath(usage, commandPath string) string {
	if strings.TrimSpace(usage) != "" {
		return usage
	}
	return usageText(commandPath)
}

func usageText(path string) string {
	prefix := ""
	for _, p := range constant.PrefixChars() {
		if p != "" {
			prefix = p
			break
		}
	}
	return prefix + path
}

// globalAccessFilter 全局黑白名单前置过滤: 命中拦截时整条事件(含订阅者)都不处理。
// 对所有事件统一按用户/群判定; 没有用户也没有群的事件在 off/blacklist 下照常放行。
func globalAccessFilter(payload structers.Payload) bool {
	return config.Current().Access.Allow(payloadUserIDs(payload), payload.Data.GroupOpenID)
}

// payloadUserIDs 收集发送者的全部可匹配标识: 群内 member_openid、私聊 user_openid,
// union_openid、以及按钮回调里的 user_id 等, 便于全局黑白名单跨群/私聊统一命中。
func payloadUserIDs(payload structers.Payload) []string {
	a := payload.Data.Author
	d := payload.Data
	return []string{
		payload.ActorID(),
		a.UserOpenID,
		a.UnionID,
		a.MemberOpenID,
		a.ID,
		d.UserOpenID,
		d.GroupMemberOpenID,
		d.OpenID,
		d.MemberOpenId,
		d.OpMemberOpenID,
		d.Callback.Resolved.UserId,
	}
}

func ProcessPayload(payload structers.Payload, client *api.BotAPI) {
	switch payload.Type() {
	case constant.GROUP_AT_MESSAGE_CREATE, constant.GROUP_MESSAGE_CREATE:
		state.IncRecv()
		stats.Recv()
		stats.Group(payload.Data.GroupOpenID, "")
		raw := payload.Data.Content
		payload.Data.Content = utils.FilterAt(payload.Data.Content)
		dispatchCommand(payload, client, commandDispatchOpts{
			userID:  payload.ActorID(),
			groupID: payload.Data.GroupOpenID,
			origin:  constant.GroupMessage,
			raw:     raw,
		})
	case constant.C2C_MESSAGE_CREATE:
		state.IncRecv()
		stats.Recv()
		stats.Peer(payload.ActorID())
		raw := payload.Data.Content
		payload.Data.Content = strings.TrimSpace(payload.Data.Content)
		if payload.Data.Content == "" {
			return
		}
		dispatchCommand(payload, client, commandDispatchOpts{
			userID:       payload.ActorID(),
			origin:       constant.PrivateMessage,
			raw:          raw,
			checkPrivate: true,
		})
	case constant.INTERACTION_CREATE:
		state.IncButton()
		stats.Button()
		ctx := &context.CallbackContext{}
		ctx.Init(payload.ID, client)
		ctx.InteractionID = payload.Data.Id
		ctx.ButtonId = payload.Data.Callback.Resolved.ButtonId
		ctx.Data = payload.Data.Callback.Resolved.ButtonData
		if structers.InteractionIsPrivate(payload.Data.ChatType, string(payload.Data.Scene)) {
			ctx.SetMessageOrigin(constant.PrivateMessage)
		} else {
			ctx.SetGroupId(payload.Data.GroupOpenID)
			ctx.SetMessageOrigin(constant.GroupMessage)
		}
		ctx.SetUserId(payload.ActorID())
		if err := ctx.Done(); err != nil {
			messageLog.Errorf("回执按钮 %v 失败: %v", ctx.ButtonId, err)
		}
		callbackFunc, ok := buttons.GetCallbackFunc(ctx.ButtonId)
		if !ok {
			// 把按钮数据一起打出来: 平台偶尔回一个空 button_id, 只打 id 的话下次还是查不出是哪个按钮。
			messageLog.Warnf("回调按钮: id=%q 未注册回调函数, 已回执交互 (data=%q 交互=%s)", ctx.ButtonId, ctx.Data, ctx.InteractionID)
			return
		}
		pool.Go(func() { callbackHandleFunc(callbackFunc, ctx) })
	case constant.GROUP_JOIN_REQUEST:
		var answer string
		switch payload.Data.VerifyInfo.Method {
		case "verify_message":
			answer = payload.Data.VerifyInfo.VerifyMsg
		case "admin_review_qa":
			if len(payload.Data.VerifyInfo.AnswerList) < 1 {
				return
			}
			answer = payload.Data.VerifyInfo.AnswerList[0].Answer
		default:
			return
		}
		ctx := &context.ApplyJoinGroupContext{
			Answer: answer,
		}
		ctx.Init(payload.Data.JoinRequestId, payload.Data.GroupOpenID, payload.ActorID(), client)
		err := plugin.CallGlobalJoinGroupHandle(ctx)
		if err != nil {
		}
		return
	case constant.MESSAGE_AUDIT_PASS, constant.MESSAGE_AUDIT_REJECT:
		api.ResolveAudit(payload.Data.AuditID, payload.Data.MessageId, payload.EventType == constant.MESSAGE_AUDIT_PASS)
		return
	}
}

// Sandbox 在沙盒里同步跑一条消息指令: 不进统计, 回复全部收集进 sink, 不投递到平台。
// 返回后 sink 里就是这条消息同步触发的全部回复; 插件另起协程发的消息捕获不到。
func Sandbox(payload structers.Payload, client *api.BotAPI, sink *message.SandboxSink) {
	raw := payload.Data.Content
	switch payload.Type() {
	case constant.GROUP_AT_MESSAGE_CREATE, constant.GROUP_MESSAGE_CREATE:
		payload.Data.Content = utils.FilterAt(payload.Data.Content)
		dispatchCommand(payload, client, commandDispatchOpts{
			userID:  payload.ActorID(),
			groupID: payload.Data.GroupOpenID,
			origin:  constant.GroupMessage,
			raw:     raw,
			sandbox: sink,
		})
	case constant.C2C_MESSAGE_CREATE:
		payload.Data.Content = strings.TrimSpace(payload.Data.Content)
		if payload.Data.Content == "" {
			return
		}
		dispatchCommand(payload, client, commandDispatchOpts{
			userID:       payload.ActorID(),
			origin:       constant.PrivateMessage,
			raw:          raw,
			checkPrivate: true,
			sandbox:      sink,
		})
	}
}

func executeCommand(command *plugin.Command, ctx *context.MessageContext) {
	defer func() {
		if recovered := recover(); recovered != nil {
			messageLog.Errorf("在执行指令%v (插件: %v)时出现panic: %v", command.Prefix, command.PluginId, recovered)
			invokeErrorHook(command, ctx, fmt.Errorf("command panic: %v", recovered))
		}
	}()
	if err := command.Handle(ctx); err != nil {
		messageLog.Errorf("在执行指令%v (插件: %v)时出现error: %v", command.Prefix, command.PluginId, err)
		invokeErrorHook(command, ctx, err)
	}
}

func invokeErrorHook(command *plugin.Command, ctx *context.MessageContext, commandErr error) {
	if command.HandleError == nil {
		return
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			messageLog.Errorf("在处理指令%v (插件: %v)的error时出现panic: %v", command.Prefix, command.PluginId, recovered)
		}
	}()
	if handleErr := command.HandleError(ctx, commandErr); handleErr != nil {
		messageLog.Errorf("在处理指令%v (插件: %v)的error时再次出现error: %v", command.Prefix, command.PluginId, handleErr)
	}
}

func permissionDenied(cmd *plugin.Command, ctx *context.MessageContext) {
	if cmd.PermissionDenied == nil {
		return
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			messageLog.Errorf("在执行指令%v (插件: %v)的权限拒绝处理函数时出现panic: %v", cmd.Prefix, cmd.PluginId, recovered)
		}
	}()
	if err := cmd.PermissionDenied(ctx); err != nil {
		messageLog.Errorf("在执行指令%v (插件: %v)的权限拒绝处理函数时出现error: %v", cmd.Prefix, cmd.PluginId, err)
	}
}

func callbackHandleFunc(handle buttons.CallbackButtonHandleFunc, ctx *context.CallbackContext) {
	defer func() {
		if r := recover(); r != nil {
			messageLog.Errorf("在执行回调按钮: %v 处理函数时候出现panic: %v", ctx.ButtonId, r)
		}
	}()
	if err := handle(ctx); err != nil {
		messageLog.Errorf("在执行回调按钮: %v 处理函数时候出现error: %v", ctx.ButtonId, err)
	}
}
