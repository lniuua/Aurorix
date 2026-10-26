# gscore 适配器

把 [gscore (GenshinUID Core)](https://github.com/Genshin-bots/gsuid_core) 接到 Aurorix 上：QQ 群/私聊消息转发给 gscore，gscore 的回复再送回 QQ。

## 工作原理

- 订阅 `GROUP_AT_MESSAGE_CREATE` / `GROUP_MESSAGE_CREATE` / `C2C_MESSAGE_CREATE` 事件，把消息编码为 gscore 的 `MessageReceive`，通过 WebSocket 发到 gscore 的 `/ws/{bot_id}`。
- 持续读取 gscore 的 `MessageSend`，把 `text` / `at` / `markdown` / `image` / `record` / `video` / `file` / `buttons` / `node` 等消息段映射为 QQ 消息并投递到目标会话。
- 断线自动重连（指数退避），面板保存配置即重连。

## 接入步骤

1. 把插件放进实例：`cp -r plugins/gscore mybot/plugins/gscore`，然后 `aurx add <实例module>/plugins/gscore`。
2. 启动 gscore 核心（默认监听 `8765`），并在其配置里设置 `WS_TOKEN`（可选，非本机访问时建议设置）。
3. 在 Aurorix 管理台 → 插件 → gscore 适配器 里填写：
   - `url`：gscore 的 WebSocket 地址，例如 `ws://127.0.0.1:8765`
   - `token`：与 gscore `WS_TOKEN` 一致
   - `bot_id`：gscore 侧平台标识（决定 `/ws/{bot_id}` 路由与消息归属），默认 `Aurorix`
   - `bot_self_id`：@ 判定标识；留空时依次回退主配置 `appid`、`bot_id`，无需机器人真实 openid
   - 转发开关：`forward_at`（群内 @机器人，默认开）/ `forward_all`（群内全部，默认关）/ `forward_private`（私聊，默认开）

## gscore 侧建议配置

gscore 默认只给已知平台转 Markdown / 按钮。建议在 gscore 核心配置中把本插件的 `bot_id` 加入：

- `SendMDPlatform`：让文本转 Markdown、图片内嵌；
- `SendButtonsPlatform`：让按钮以 `buttons` 段下发（本适配器会转成 QQ 按钮）。

否则 gscore 会下发普通 `text` 与 `template_markdown`（后者当前适配器不支持，会告警跳过）。

## 命令

| 命令 | 权限 | 说明 |
|---|---|---|
| `/gs` | 机器人主人 | 查看连接状态 |
| `/gs reconnect` | 机器人主人 | 强制重连 |

## 控制台

插件自带一个轻量状态页（走框架的 `plugin.WebUI` 扩展点，不改框架代码）：

- 页面：`/gscore/` —— 管理台侧边栏「插件控制台」里有入口，插件详情页也会显示
- 接口：
  - `GET /gscore/api/status` —— 连接状态快照（连接态、服务地址、累计收/发、积压、最近错误等）
  - `POST /gscore/api/reconnect` —— 等价于 `/gs reconnect`
- 页面每 2 秒自动刷新，可直接点「强制重连」；接口走管理台登录态，未登录会提示先登录

## 注意

- 命令按钮（`action=2`）点击后自动发送命令文本，该消息会以普通群消息回传；若关闭了 `forward_all`，群内命令按钮的点击可能不会被 gscore 收到。
- 回调按钮（`action=1`）点击后通过 `INTERACTION_CREATE` 回传，无需 `forward_all`。
