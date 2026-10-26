package gscore

import (
	"encoding/json"
	"fmt"

	"github.com/KasumiYuku/Aurorix/lib/constant"
	"github.com/KasumiYuku/Aurorix/lib/context"
	"github.com/KasumiYuku/Aurorix/lib/event"
	"github.com/KasumiYuku/Aurorix/lib/plugin"
)

const pluginID = "gscore"

func init() {
	commands := []*plugin.Command{
		{
			Prefix:   "gs",
			Role:     constant.RoleBotOwner,
			Describe: "gscore 适配器状态",
			Handle:   statusCommand,
			SubCommand: []*plugin.Command{
				{
					Prefix:   "reconnect",
					Role:     constant.RoleBotOwner,
					Describe: "强制重连 gscore",
					Handle:   reconnectCommand,
				},
			},
		},
	}

	plugin.Register(&plugin.Plugin{
		Id:          pluginID,
		Name:        "gscore 适配器",
		Description: "连接 gscore (GenshinUID Core) 的适配器插件",
		Commands:    commands,
		Config:      configFields(),
		ApplyConfig: applyConfig,
	})

	event.On(constant.GROUP_AT_MESSAGE_CREATE, onGroupAtMessage)
	event.On(constant.GROUP_MESSAGE_CREATE, onGroupMessage)
	event.On(constant.C2C_MESSAGE_CREATE, onPrivateMessage)
}

func applyConfig(values map[string]any) error {
	cfg := parseConfig(values)
	inst.ensureStarted()
	inst.updateConfig(cfg)
	return nil
}

func onGroupAtMessage(ctx *event.Context, e *event.MessageEvent) {
	inst.rememberBotAPI(ctx.Qapi)
	handleGroupMessage(e, inst.snapshotConfig(), true)
}

func onGroupMessage(ctx *event.Context, e *event.MessageEvent) {
	inst.rememberBotAPI(ctx.Qapi)
	handleGroupMessage(e, inst.snapshotConfig(), false)
}

func onPrivateMessage(ctx *event.Context, e *event.MessageEvent) {
	inst.rememberBotAPI(ctx.Qapi)
	cfg := inst.snapshotConfig()
	if !cfg.ForwardPriv {
		return
	}
	forwardEvent(e, cfg, false)
}

// handleGroupMessage 统一处理群消息。平台订阅了 GROUP_MESSAGE_CREATE 后，
// @机器人 的消息可能不以 GROUP_AT_MESSAGE_CREATE 下发，而是混进 GROUP_MESSAGE_CREATE，
// 所以这里除事件类型外，还看 Mentions 里有没有 IsYou 来判定「是否 @ 机器人」。
func handleGroupMessage(e *event.MessageEvent, cfg config, isAtEvent bool) {
	atMe := isAtEvent || hasBotMention(e)
	if atMe {
		if cfg.ForwardAt {
			forwardEvent(e, cfg, true)
		}
		return
	}
	if cfg.ForwardAll {
		forwardEvent(e, cfg, false)
	}
}

func hasBotMention(e *event.MessageEvent) bool {
	for _, m := range e.Mentions {
		if m.IsYou {
			return true
		}
	}
	return false
}

func forwardEvent(e *event.MessageEvent, cfg config, atMe bool) {
	recv := eventToReceive(e, cfg, atMe)
	if recv == nil {
		adapterLog.Debugf("事件 %s 无内容可转发 (user=%s group=%s atMe=%v)", e.Type(), e.UserID, e.GroupID, atMe)
		return
	}
	data, err := json.Marshal(recv)
	if err != nil {
		adapterLog.Warnf("编码上行消息失败: %v", err)
		return
	}
	if err := inst.send(data); err != nil {
		adapterLog.Debugf("转发消息到 gscore 失败: %v", err)
	} else {
		adapterLog.Debugf("已转发到 gscore: %s atMe=%v content=%d 段", e.Type(), atMe, len(recv.Content))
	}
}

func statusCommand(ctx *context.MessageContext) error {
	u, connected, botID, lastErr := inst.statusLine()
	state := "已连接"
	if !connected {
		state = "未连接"
		if lastErr != "" {
			state += "（" + lastErr + "）"
		}
	}
	return ctx.Markdown(fmt.Sprintf(
		"## gscore 适配器\n\n- 地址：`%s`\n- 平台 ID：`%s`\n- 状态：**%s**\n\n子命令：`/gs reconnect` 强制重连",
		u, botID, state,
	)).Send()
}

func reconnectCommand(ctx *context.MessageContext) error {
	inst.reconnect()
	return ctx.Text("已触发 gscore 重连").Send()
}
