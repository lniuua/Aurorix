import { useEffect, useRef, useState, type FormEvent, type ReactNode } from 'react'
import Markdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import { Bot, SlidersHorizontal, Trash2, User } from 'lucide-react'
import { Button, Input, Segmented, Spinner, cn } from '@aurorix/webui'
import { PageHeader } from '../components/layout'
import { ZoomImage } from '../components/lightbox'
import { get, post, withQuery } from '../lib/api'
import { useUIConfig } from '../lib/background'
import { toast } from '../lib/toast'
import type { SandboxMessage, SandboxPollView, SandboxReply } from '../lib/types'

type Scene = 'group' | 'c2c'
type Role = 'member' | 'admin' | 'owner' | 'bot_owner'

type Turn = {
  id: number
  text: string
  messages: SandboxMessage[]
}

const mdComponents = {
  img: (props: { src?: string; alt?: string }) => (
    <ZoomImage src={props.src} alt={props.alt} className="my-1 max-h-64 rounded-xl object-contain" />
  ),
  a: (props: { href?: string; children?: ReactNode }) => (
    <a href={props.href} target="_blank" rel="noreferrer" className="text-accent-text underline">
      {props.children}
    </a>
  ),
  code: (props: { children?: ReactNode }) => (
    <code className="rounded bg-black/10 px-1 py-0.5 font-mono text-[12px]">{props.children}</code>
  ),
  pre: (props: { children?: ReactNode }) => (
    <pre className="my-1 overflow-x-auto rounded-lg bg-black/10 p-2 text-[12px]">{props.children}</pre>
  ),
  blockquote: (props: { children?: ReactNode }) => (
    <blockquote className="my-1 border-l-2 border-line pl-2 text-ink-faint">{props.children}</blockquote>
  ),
}

/** 机器人头像：管理台配置了 Logo 就用它，否则回退默认图标。 */
function BotAvatar() {
  const { data } = useUIConfig()
  const logo = (data?.logo ?? '').trim()
  if (logo) {
    return (
      <img
        src={logo}
        alt=""
        referrerPolicy="no-referrer"
        className="size-8 shrink-0 rounded-full border border-line object-cover"
      />
    )
  }
  return (
    <span className="grid size-8 shrink-0 place-items-center rounded-full bg-subtle text-ink-soft">
      <Bot className="size-4" />
    </span>
  )
}

function BotBubble({ message }: { message: SandboxMessage }) {
  const isMarkdown = message.kind === 'markdown'
  const isEmpty = message.content.trim() === ''
  return (
    <div className="flex items-start gap-2">
      <BotAvatar />
      <div className="max-w-[min(680px,85%)] rounded-2xl bg-surface px-3.5 py-2 text-[13px] leading-relaxed">
        {message.kind === 'image' && message.url ? (
          <ZoomImage
            src={message.url}
            alt={message.content}
            className="max-h-64 max-w-full rounded-lg object-contain"
          />
        ) : isMarkdown ? (
          <Markdown remarkPlugins={[remarkGfm]} components={mdComponents}>
            {message.content}
          </Markdown>
        ) : isEmpty ? (
          <span className="italic text-ink-faint">（空消息）</span>
        ) : message.kind === 'text' ? (
          <span className="whitespace-pre-wrap">{message.content}</span>
        ) : (
          <span className="italic text-ink-faint">{message.content}</span>
        )}
      </div>
    </div>
  )
}

export default function SandboxPage() {
  const [scene, setScene] = useState<Scene>('group')
  const [role, setRole] = useState<Role>('member')
  const [user, setUser] = useState('')
  const [group, setGroup] = useState('')
  const [input, setInput] = useState('')
  const [turns, setTurns] = useState<Turn[]>([])
  const [pending, setPending] = useState(false)
  const [advanced, setAdvanced] = useState(false)
  const scroller = useRef<HTMLDivElement>(null)

  useEffect(() => {
    const box = scroller.current
    if (box) box.scrollTop = box.scrollHeight
  }, [turns, pending])

  // 指令是异步执行的: 轮询取回回复。done 只代表框架指令跑完, 适配器(gscore)的回复可能晚很多。
  const pollReplies = async (session: string, turnId: number, start = 0) => {
    let after = start
    let idle = 0
    for (let i = 0; i < 360; i++) {
      await new Promise((resolve) => window.setTimeout(resolve, 500))
      try {
        const res = await get<SandboxPollView>(withQuery('/sandbox/poll', { id: session, after }))
        if (res.messages.length > 0) {
          after = res.total
          idle = 0
          setTurns((prev) =>
            prev.map((turn) =>
              turn.id === turnId ? { ...turn, messages: [...turn.messages, ...res.messages] } : turn,
            ),
          )
        } else {
          idle++
        }
        // done 之后再等 60 秒没有新增才收工: gscore 这类适配器的慢指令可能拖到一分钟。
        // 未 done 就一直等, 最多 180 秒。
        if (res.done && idle >= 120) return
      } catch {
        return
      }
    }
  }

  const send = async (event?: FormEvent) => {
    event?.preventDefault()
    const text = input.trim()
    if (text === '' || pending) return
    setPending(true)
    try {
      const res = await post<SandboxReply>('/sandbox', { text, scene, role, user, group })
      const turnId = Date.now()
      setTurns((prev) => [...prev, { id: turnId, text, messages: res.messages }])
      setInput('')
      if (res.session) void pollReplies(res.session, turnId, res.messages.length)
    } catch (err) {
      toast(err instanceof Error ? err.message : '沙盒执行失败', 'err')
    } finally {
      setPending(false)
    }
  }

  return (
    <div className="flex flex-col gap-4">
      <PageHeader
        title="沙盒"
        description="在管理台直接给机器人发消息"
        actions={
          turns.length > 0 ? (
            <Button variant="ghost" size="sm" icon={Trash2} onClick={() => setTurns([])}>
              清空
            </Button>
          ) : undefined
        }
      />

      <div className="flex h-[calc(100dvh-13rem)] min-h-[420px] flex-col overflow-hidden rounded-2xl border border-line bg-background">
        <div className="border-b border-line bg-surface px-3 py-2 sm:px-4 sm:py-3">
          <div className="flex flex-wrap items-center gap-2 sm:gap-3">
            <Segmented
              size="sm"
              value={scene}
              onChange={setScene}
              ariaLabel="会话类型"
              options={[
                { value: 'group', label: '群聊' },
                { value: 'c2c', label: '私聊' },
              ]}
            />
            <Segmented
              size="sm"
              value={role}
              onChange={setRole}
              ariaLabel="模拟角色"
              options={[
                { value: 'member', label: '成员' },
                { value: 'admin', label: '管理员' },
                { value: 'owner', label: '群主' },
                { value: 'bot_owner', label: '机器人主人' },
              ]}
            />
            <button
              type="button"
              onClick={() => setAdvanced((value) => !value)}
              aria-expanded={advanced}
              className={cn(
                'ml-auto inline-flex items-center gap-1 rounded-full px-2.5 py-1 text-[12px] transition-colors',
                advanced ? 'bg-accent-soft text-accent-text' : 'text-ink-soft hover:text-accent-text',
              )}
            >
              <SlidersHorizontal className="size-3.5" />
              高级
            </button>
          </div>
          {advanced && (
            <div className="mt-2 grid gap-2 sm:grid-cols-2">
              <Input
                value={user}
                onChange={(e) => setUser(e.target.value)}
                placeholder="用户 openid（可选）"
                className="w-full"
              />
              {scene === 'group' && (
                <Input
                  value={group}
                  onChange={(e) => setGroup(e.target.value)}
                  placeholder="群 openid（可选）"
                  className="w-full"
                />
              )}
            </div>
          )}
        </div>

        <div ref={scroller} className="flex min-h-0 flex-1 flex-col gap-3 overflow-y-auto p-4">
          {turns.length === 0 ? (
            <div className="m-auto max-w-md text-center text-[12.5px] leading-relaxed text-ink-faint">
              输入指令试试吧
            </div>
          ) : (
            turns.map((turn) => (
              <div key={turn.id} className="flex flex-col gap-2">
                <div className="flex flex-row-reverse items-start gap-2">
                  <span className="grid size-8 shrink-0 place-items-center rounded-full bg-accent text-accent-ink">
                    <User className="size-4" />
                  </span>
                  <div className="max-w-[min(680px,85%)] whitespace-pre-wrap rounded-2xl bg-accent-soft px-3.5 py-2 text-[13px] leading-relaxed">
                    {turn.text}
                  </div>
                </div>
                {turn.messages.map((message, index) => (
                  <BotBubble key={index} message={message} />
                ))}
              </div>
            ))
          )}
          {pending && (
            <div className="flex items-center gap-2 text-[12px] text-ink-faint">
              <Spinner className="size-4" /> 机器人处理中…
            </div>
          )}
        </div>

        <form onSubmit={send} className="flex items-end gap-2 border-t border-line bg-surface px-3 py-2.5">
          <textarea
            value={input}
            onChange={(e) => setInput(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === 'Enter' && !e.shiftKey) {
                e.preventDefault()
                void send()
              }
            }}
            rows={2}
            placeholder="输入消息，Enter 发送，Shift+Enter 换行"
            className={cn(
              'min-h-[44px] flex-1 resize-y rounded-lg border border-line bg-background px-3 py-2 text-[13px] outline-none focus:border-accent',
            )}
          />
          <Button type="submit" disabled={pending || input.trim() === ''}>
            {pending ? <Spinner className="size-4" /> : '发送'}
          </Button>
        </form>
      </div>
    </div>
  )
}
