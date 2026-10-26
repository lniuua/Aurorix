import { Select, Textarea } from '@aurorix/webui'
import type { GlobalAccess } from '../lib/types'

const MODE_OPTIONS = [
  { value: 'off', label: '关闭' },
  { value: 'whitelist', label: '白名单' },
  { value: 'blacklist', label: '黑名单' },
] as const

function toLines(v?: string[]): string {
  return (v ?? []).join('\n')
}

function fromLines(s: string): string[] {
  return s.split('\n')
}

interface AccessEditorProps {
  value?: GlobalAccess | null
  onChange: (next: GlobalAccess) => void
}

/** 全局黑白名单编辑器: 群与用户各自独立开关与名单。 */
export function AccessEditor({ value, onChange }: AccessEditorProps) {
  const access: GlobalAccess = {
    group_mode: 'off',
    user_mode: 'off',
    groups: [],
    users: [],
    ...(value ?? {}),
  }
  const patch = (partial: Partial<GlobalAccess>) => onChange({ ...access, ...partial })

  return (
    <div className="grid gap-4 md:grid-cols-2">
      <div className="rounded-xl border border-line bg-surface p-3">
        <div className="mb-2 flex items-center justify-between gap-3">
          <p className="text-[13px] font-medium">群</p>
          <Select
            value={access.group_mode}
            options={[...MODE_OPTIONS]}
            onChange={(e) => patch({ group_mode: e.target.value as GlobalAccess['group_mode'] })}
          />
        </div>
        <p className="mb-1 text-[11px] text-ink-faint">每行一个群 openid</p>
        <Textarea
          rows={6}
          className="font-mono text-sm"
          placeholder="每行一个群 openid"
          value={toLines(access.groups)}
          onChange={(e) => patch({ groups: fromLines(e.target.value) })}
        />
      </div>

      <div className="rounded-xl border border-line bg-surface p-3">
        <div className="mb-2 flex items-center justify-between gap-3">
          <p className="text-[13px] font-medium">用户</p>
          <Select
            value={access.user_mode}
            options={[...MODE_OPTIONS]}
            onChange={(e) => patch({ user_mode: e.target.value as GlobalAccess['user_mode'] })}
          />
        </div>
        <p className="mb-1 text-[11px] text-ink-faint">每行一个用户 openid（群/私聊通用）</p>
        <Textarea
          rows={6}
          className="font-mono text-sm"
          placeholder="每行一个用户 openid"
          value={toLines(access.users)}
          onChange={(e) => patch({ users: fromLines(e.target.value) })}
        />
      </div>
    </div>
  )
}
