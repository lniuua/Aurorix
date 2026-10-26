import { useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import {
  Badge,
  Button,
  Card,
  EmptyState,
  KeyValueList,
  Panel,
  SkeletonRows,
  StatCard,
  Table,
  Td,
  Th,
} from '@aurorix/webui'
import { RefreshCw, Users, Wifi, WifiOff } from 'lucide-react'
import { PageHeader, useNow, useOverview, type OverviewData } from '../components/layout'
import { RingGauge } from '../components/ring-gauge'
import { post } from '../lib/api'
import { fmtBytes, fmtDur, fmtNum, fmtRate, fmtRelative } from '../lib/format'
import { qk } from '../lib/query'
import { toast } from '../lib/toast'
import type { ProfileView } from '../lib/types'

/** 带环形进度的统计卡，与 StatCard 同款外框，用于整机资源这类比率指标。 */
function RingStat({
  label,
  percent,
  value,
  hint,
}: {
  label: string
  percent?: number
  value: string
  hint?: string
}) {
  return (
    <div className="flex items-center gap-3 rounded-[1.5rem] border border-line bg-surface px-4 py-3">
      <RingGauge value={percent} />
      <div className="min-w-0">
        <p className="text-xs text-ink-faint">{label}</p>
        <p className="mt-0.5 font-mono text-base tabular-nums text-ink">{value}</p>
        {hint && <p className="mt-0.5 truncate text-[11px] text-ink-soft">{hint}</p>}
      </div>
    </div>
  )
}

export default function OverviewPage() {
  const { data, isPending, isError, error } = useOverview()
  const queryClient = useQueryClient()
  const now = useNow()
  const [refreshing, setRefreshing] = useState(false)

  if (!data) {
    if (isPending) return <SkeletonRows rows={8} cols={3} />
    return (
      <EmptyState
        title="读取概览失败"
        description={isError && error instanceof Error ? error.message : '请稍后重试'}
      />
    )
  }

  const { runtime, counts, logs, stats, system, host, gateway, profile, assets } = data
  const uptime = data.now
    ? fmtDur(Math.max(0, (now - (data.now - runtime.uptime_sec * 1000)) / 1000))
    : fmtDur(runtime.uptime_sec)
  const gatewayOnline = runtime.protocol === 'websocket' ? Boolean(gateway?.connected) : true
  const memPercent =
    system && system.mem_total > 0 ? (system.mem_used / system.mem_total) * 100 : undefined
  const diskPercent =
    system && system.disk_total > 0 ? (system.disk_used / system.disk_total) * 100 : undefined
  const netReady = Boolean(system && (system.net_rx_total > 0 || system.net_tx_total > 0))

  const refreshProfile = async () => {
    setRefreshing(true)
    try {
      const result = await post<ProfileView>('/profile/refresh')
      queryClient.setQueryData(qk.overview, (prev: OverviewData | undefined) =>
        prev ? { ...prev, profile: result.profile } : prev,
      )
      toast('机器人档案已刷新')
    } catch (err) {
      toast(err instanceof Error ? err.message : '刷新档案失败', 'err')
    } finally {
      setRefreshing(false)
    }
  }

  return (
    <>
      <PageHeader
        title="概览"
        description="运行状态、消息统计与网关现状，实时快照每秒刷新"
        actions={
          <Badge tone={gatewayOnline ? 'ok' : 'danger'} icon={gatewayOnline ? Wifi : WifiOff}>
            {gatewayOnline ? '网关在线' : '网关离线'}
          </Badge>
        }
      />

      <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-6">
        <StatCard label="运行时长" value={uptime} hint="进程启动至今" />
        <RingStat
          label="系统 CPU"
          percent={system?.cpu_percent}
          value={system ? `${Math.round(system.cpu_percent)}%` : '—'}
          hint="整机占用"
        />
        <RingStat
          label="系统内存"
          percent={memPercent}
          value={memPercent === undefined ? '—' : `${Math.round(memPercent)}%`}
          hint={
            system && system.mem_total > 0
              ? `${fmtBytes(system.mem_used)} / ${fmtBytes(system.mem_total)}`
              : '暂不可用'
          }
        />
        <RingStat
          label="磁盘占用"
          percent={diskPercent}
          value={diskPercent === undefined ? '—' : `${Math.round(diskPercent)}%`}
          hint={
            system && system.disk_total > 0
              ? `${fmtBytes(system.disk_used)} / ${fmtBytes(system.disk_total)}`
              : '暂不可用'
          }
        />
        <RingStat
          label="进程 CPU"
          percent={system?.proc_cpu_percent}
          value={system ? `${system.proc_cpu_percent.toFixed(1)}%` : '—'}
          hint="占整机"
        />
        <StatCard
          label="进程内存"
          value={system && system.proc_rss > 0 ? fmtBytes(system.proc_rss) : '—'}
          hint={
            system && system.proc_rss > 0 && system.mem_total > 0
              ? `占整机 ${((system.proc_rss / system.mem_total) * 100).toFixed(1)}%`
              : 'RSS 常驻内存'
          }
        />
      </div>

      <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-6">
        <StatCard
          label="网络下行"
          value={netReady ? fmtRate(system?.net_rx_rate) : '—'}
          hint={netReady ? `累计 ${fmtBytes(system?.net_rx_total)}` : '采样中'}
        />
        <StatCard
          label="网络上行"
          value={netReady ? fmtRate(system?.net_tx_rate) : '—'}
          hint={netReady ? `累计 ${fmtBytes(system?.net_tx_total)}` : '采样中'}
        />
        <StatCard
          label="系统负载"
          value={system && system.load1 >= 0 ? system.load1.toFixed(2) : '—'}
          hint={host ? `${fmtNum(host.cpu_cores)} 核心 · 1 分钟均值` : '1 分钟均值'}
        />
        <StatCard label="goroutine" value={fmtNum(runtime.goroutines)} />
        <StatCard
          label="堆内存"
          value={fmtBytes(runtime.mem.heap_alloc)}
          hint={`系统 ${fmtBytes(runtime.mem.heap_sys)}`}
        />
        <StatCard
          label="错误日志"
          value={fmtNum(logs.errors)}
          hint={`累计 ${fmtNum(logs.total)} 条`}
          tone={logs.errors > 0 ? 'warn' : 'default'}
        />
      </div>

      <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
        <StatCard label="插件" value={fmtNum(counts.plugins)} />
        <StatCard label="指令" value={fmtNum(counts.commands)} />
        <StatCard label="定时任务" value={fmtNum(counts.jobs)} />
        <StatCard label="Markdown 模板" value={fmtNum(counts.templates)} />
      </div>

      <div className="grid gap-6 lg:grid-cols-2">
        <Card title="运行状态" description="进程与协议">
          <KeyValueList
            items={[
              { label: '协议模式', value: runtime.protocol || '—' },
              { label: '监听端口', value: runtime.port },
              { label: 'Go 版本', value: runtime.go_version },
              { label: 'GC 次数', value: fmtNum(runtime.mem.num_gc) },
              { label: '上次 GC', value: runtime.mem.last_gc ? fmtRelative(runtime.mem.last_gc, now) : '—' },
              { label: '接收 / 发送', value: `${fmtNum(runtime.counters.recv)} / ${fmtNum(runtime.counters.sent)}` },
              { label: '按钮点击', value: fmtNum(runtime.counters.button) },
              {
                label: '堆内存上限',
                value: runtime.mem.limit_bytes ? fmtBytes(runtime.mem.limit_bytes) : '未设',
              },
              {
                label: 'GOGC',
                value:
                  runtime.mem.gc_percent === undefined
                    ? '—'
                    : runtime.mem.gc_percent < 0
                      ? '已关闭'
                      : `${runtime.mem.gc_percent}%`,
              },
            ]}
          />
        </Card>

        <Card
          title="机器人档案"
          description="启动时拉取一次，可手动刷新"
          actions={
            <Button variant="secondary" size="sm" icon={RefreshCw} loading={refreshing} onClick={refreshProfile}>
              刷新
            </Button>
          }
        >
          {profile ? (
            <div className="flex flex-col gap-4">
              <div className="flex items-center gap-4">
                <img
                  src={profile.avatar}
                  alt=""
                  className="size-14 shrink-0 rounded-full border border-line object-cover"
                />
                <div className="min-w-0">
                  <p className="font-serif text-lg">{profile.username || '—'}</p>
                  <p className="truncate font-mono text-xs text-ink-faint">{profile.appid || '—'}</p>
                </div>
              </div>
              <KeyValueList
                items={[
                  { label: '机器人', value: profile.bot ? '是' : '否' },
                  { label: 'Union OpenID', value: profile.union_openid || '—' },
                  { label: 'Union 账号', value: profile.union_user_account || '—' },
                  {
                    label: '分享链接',
                    value: profile.share_url ? (
                      <a className="text-accent-text underline" href={profile.share_url} target="_blank" rel="noreferrer">
                        打开
                      </a>
                    ) : (
                      '—'
                    ),
                  },
                ]}
              />
              {profile.welcome_msg && <p className="text-xs text-ink-soft">{profile.welcome_msg}</p>}
            </div>
          ) : (
            <EmptyState title="尚未获取到档案" description="网关连通后会自动拉取，也可以点右上角刷新。" />
          )}
        </Card>

        <Card title="主机信息" description="进程所在机器与运行环境">
          <KeyValueList
            items={[
              { label: '主机名', value: host?.hostname || '—' },
              { label: '系统 / 架构', value: host ? `${host.os} / ${host.arch}` : '—' },
              { label: 'CPU 核心', value: host ? fmtNum(host.cpu_cores) : '—' },
              {
                label: '开机时长',
                value: system && system.sys_uptime_sec > 0 ? fmtDur(system.sys_uptime_sec) : '—',
              },
              { label: '进程 PID', value: host ? fmtNum(host.pid) : '—' },
              { label: 'Go 版本', value: runtime.go_version || '—' },
              { label: '工作目录', value: host?.cwd || '—' },
              { label: '可执行文件', value: host?.executable || '—' },
            ]}
          />
        </Card>

        <Card title="网关" description="WebSocket 模式下的连接现状">
          {gateway ? (
            <KeyValueList
              items={[
                { label: '连接状态', value: gateway.connected ? '已连接' : '未连接' },
                { label: '会话 ID', value: gateway.session_id || '—' },
                { label: '序列号', value: gateway.seq === undefined ? '—' : fmtNum(gateway.seq) },
                { label: '心跳 ACK', value: gateway.heartbeat_ack ? '正常' : '待确认' },
                { label: '已连接时长', value: gateway.since_ms ? fmtDur(gateway.since_ms / 1000) : '—' },
              ]}
            />
          ) : (
            <p className="text-sm text-ink-soft">当前为 webhook 模式，无网关会话。</p>
          )}
        </Card>
      </div>

      <Panel title="消息统计" description="进程启动以来的累计量">
        <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
          <StatCard label="群" value={fmtNum(stats.groups)} />
          <StatCard label="好友" value={fmtNum(stats.peers)} />
          <StatCard label="累计接收" value={fmtNum(stats.total_recv)} />
          <StatCard label="累计发送" value={fmtNum(stats.total_sent)} />
        </div>
        <div className="mt-3 grid gap-3 sm:grid-cols-3">
          <StatCard label="今日接收" value={fmtNum(stats.today.recv)} />
          <StatCard label="今日发送" value={fmtNum(stats.today.sent)} />
          <StatCard label="今日按钮" value={fmtNum(stats.today.button)} />
        </div>
        {stats.top_groups.length > 0 && (
          <div className="mt-5">
            <Table>
              <thead>
                <tr>
                  <Th>活跃群</Th>
                  <Th>消息数</Th>
                </tr>
              </thead>
              <tbody>
                {stats.top_groups.map((group) => (
                  <tr key={group.id}>
                    <Td>
                      <span className="flex items-center gap-2">
                        <Users aria-hidden className="size-3.5 shrink-0 text-ink-faint" />
                        <span className="truncate">{group.name || group.id}</span>
                      </span>
                    </Td>
                    <Td className="font-mono tabular-nums">{fmtNum(group.msg)}</Td>
                  </tr>
                ))}
              </tbody>
            </Table>
          </div>
        )}
      </Panel>

      {assets && (
        <Panel title="图床" description="已启用的上传通道">
          <div className="grid gap-3 sm:grid-cols-3">
            <StatCard label="Provider 总数" value={fmtNum(assets.providers)} />
            <StatCard label="已启用" value={fmtNum(assets.enabled)} />
            <StatCard label="直通白名单" value={fmtNum(assets.whitelist)} />
          </div>
        </Panel>
      )}
    </>
  )
}
