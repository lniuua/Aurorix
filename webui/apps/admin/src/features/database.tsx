import { useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import {
  Badge,
  Button,
  ConfirmDialog,
  DangerNote,
  EmptyState,
  Field,
  Input,
  Modal,
  Panel,
  SkeletonRows,
  Table,
  Td,
  Textarea,
  Th,
  cn,
} from '@aurorix/webui'
import { Database as DatabaseIcon, Pencil, Play, Plus, RefreshCw, Search, Trash2 } from 'lucide-react'
import { PageHeader } from '../components/layout'
import { delWith, get, post, put, withQuery } from '../lib/api'
import { qk } from '../lib/query'
import { toast } from '../lib/toast'
import type {
  DBColumn,
  DBQueryResult,
  DBRowsView,
  DBTable,
  DBTableView,
  DBValue,
  DBWriteResult,
} from '../lib/types'

const PAGE_SIZE = 20

const enc = encodeURIComponent

function isTextType(type: string): boolean {
  const up = type.toUpperCase()
  return up === '' || up.includes('CHAR') || up.includes('TEXT') || up.includes('CLOB')
}

/** 表单里的字符串按列类型还原成能入库的值；空串对非文本列视为 NULL。 */
function coerce(col: DBColumn, raw: string): DBValue {
  if (raw === '') return isTextType(col.type) ? '' : null
  if (/INT/i.test(col.type)) {
    const n = Number(raw)
    return Number.isFinite(n) ? Math.trunc(n) : raw
  }
  if (/REAL|FLOA|DOUB|NUM|DEC|MONEY/i.test(col.type)) {
    const n = Number(raw)
    return Number.isFinite(n) ? n : raw
  }
  return raw
}

function isNumericType(type: string): boolean {
  return /INT|REAL|FLOA|DOUB|NUM|DEC|MONEY/i.test(type)
}

type Draft = {
  mode: 'insert' | 'update'
  values: Record<string, string>
  key: Record<string, DBValue>
}

export default function DatabasePage() {
  const queryClient = useQueryClient()
  const [table, setTable] = useState('')
  const [page, setPage] = useState(0)
  const [qInput, setQInput] = useState('')
  const [q, setQ] = useState('')
  const [draft, setDraft] = useState<Draft | null>(null)
  const [saving, setSaving] = useState(false)
  const [pendingDelete, setPendingDelete] = useState<{ key: Record<string, DBValue>; label: string } | null>(
    null,
  )
  const [sql, setSql] = useState('SELECT name FROM sqlite_master WHERE type = \'table\' LIMIT 20')
  const [sqlResult, setSqlResult] = useState<DBQueryResult | null>(null)
  const [running, setRunning] = useState(false)

  const tablesQuery = useQuery({
    queryKey: qk.dbTables,
    queryFn: () => get<{ tables: DBTable[] }>('/db/tables'),
  })
  const viewQuery = useQuery({
    queryKey: qk.dbTable(table),
    queryFn: () => get<DBTableView>(`/db/tables/${enc(table)}`),
    enabled: table !== '',
  })
  const rowsQuery = useQuery({
    queryKey: qk.dbRows(table, page, q),
    queryFn: () =>
      get<DBRowsView>(
        withQuery(`/db/tables/${enc(table)}/rows`, {
          limit: PAGE_SIZE,
          offset: page * PAGE_SIZE,
          q,
        }),
      ),
    enabled: table !== '',
  })

  const tables = tablesQuery.data?.tables ?? []
  const view = viewQuery.data
  const rows = rowsQuery.data

  const reload = () => queryClient.invalidateQueries({ queryKey: ['db'] })

  const selectTable = (name: string) => {
    setTable(name)
    setPage(0)
    setQ('')
    setQInput('')
  }

  const openInsert = (schema: DBTableView) => {
    const values: Record<string, string> = {}
    for (const col of schema.columns) values[col.name] = ''
    setDraft({ mode: 'insert', values, key: {} })
  }

  const openEdit = (schema: DBTableView, row: DBValue[]) => {
    const values: Record<string, string> = {}
    const key: Record<string, DBValue> = {}
    schema.columns.forEach((col, index) => {
      const value = row[index]
      values[col.name] = value === null ? '' : String(value)
      if (col.pk > 0) key[col.name] = value
    })
    setDraft({ mode: 'update', values, key })
  }

  const submitDraft = async () => {
    if (!draft || !view) return
    const values: Record<string, DBValue> = {}
    for (const col of view.columns) values[col.name] = coerce(col, draft.values[col.name] ?? '')
    setSaving(true)
    try {
      if (draft.mode === 'insert') {
        await post<DBWriteResult>(`/db/tables/${enc(view.name)}/rows`, { values })
        toast('已插入一行')
      } else {
        await put<DBWriteResult>(`/db/tables/${enc(view.name)}/rows`, { values, key: draft.key })
        toast('已保存修改')
      }
      setDraft(null)
      await reload()
    } catch (err) {
      toast(err instanceof Error ? err.message : '保存失败', 'err')
    } finally {
      setSaving(false)
    }
  }

  const confirmDelete = async () => {
    if (!pendingDelete || !view) return
    try {
      const res = await delWith<DBWriteResult>(`/db/tables/${enc(view.name)}/rows`, {
        key: pendingDelete.key,
      })
      toast(`已删除 ${res.affected ?? 0} 行`)
      setPendingDelete(null)
      await reload()
    } catch (err) {
      toast(err instanceof Error ? err.message : '删除失败', 'err')
    }
  }

  const runSQL = async () => {
    if (sql.trim() === '') return
    setRunning(true)
    try {
      const result = await post<DBQueryResult>('/db/query', { sql })
      setSqlResult(result)
      await reload()
    } catch (err) {
      setSqlResult(null)
      toast(err instanceof Error ? err.message : '执行失败', 'err')
    } finally {
      setRunning(false)
    }
  }

  const totalPages = rows ? Math.max(1, Math.ceil(rows.total / rows.limit)) : 1

  return (
    <>
      <PageHeader
        title="数据库"
        description="SQLite 表结构与行数据；增删改会直接落库，改动不可撤销，操作前请确认"
        actions={
          <>
            <Badge>{tables.length > 0 ? `${tables.length} 张表` : '读取中'}</Badge>
            <Button
              variant="secondary"
              size="sm"
              icon={RefreshCw}
              loading={tablesQuery.isFetching}
              onClick={() => reload()}
            >
              刷新
            </Button>
          </>
        }
      />

      <div className="grid items-start gap-6 lg:grid-cols-[220px_minmax(0,1fr)]">
        <div className="rounded-[1.5rem] border border-line bg-surface p-2">
          <p className="px-2.5 pb-1.5 pt-1.5 text-[11px] tracking-[0.18em] text-ink-faint">数据表</p>
          <div className="flex max-h-[62vh] flex-col gap-0.5 overflow-y-auto">
            {tablesQuery.isPending && <div className="px-2.5 py-2 text-xs text-ink-faint">读取中…</div>}
            {tablesQuery.isError && (
              <div className="px-2.5 py-2 text-xs text-danger-text">读取失败，稍后重试</div>
            )}
            {tables.map((item) => (
              <button
                key={item.name}
                type="button"
                onClick={() => selectTable(item.name)}
                className={cn(
                  'nav-item text-left',
                  table === item.name && 'bg-accent-soft text-accent-text',
                )}
              >
                <DatabaseIcon aria-hidden className="size-4 shrink-0" />
                <span className="min-w-0 flex-1 truncate font-mono text-[12.5px]">{item.name}</span>
                <span className="shrink-0 font-mono text-[11px] text-ink-faint">
                  {item.rows >= 0 ? item.rows : '—'}
                </span>
              </button>
            ))}
          </div>
        </div>

        <div className="min-w-0">
          {table === '' ? (
            <div className="rounded-[1.5rem] border border-line bg-surface">
              <EmptyState
                title="选择左侧的数据表"
                description="可以浏览行数据、按主键编辑或删除，也可以直接执行单条 SQL。"
              />
            </div>
          ) : viewQuery.isPending ? (
            <SkeletonRows rows={8} cols={4} />
          ) : viewQuery.isError || !view ? (
            <EmptyState
              title="读取表结构失败"
              description={viewQuery.error instanceof Error ? viewQuery.error.message : '请稍后重试'}
            />
          ) : (
            <div className="min-w-0 rounded-[1.5rem] border border-line bg-surface p-4 md:p-5">
              <div className="flex flex-wrap items-center justify-between gap-3">
                <div className="min-w-0">
                  <p className="truncate font-mono text-[15px] text-ink">{view.name}</p>
                  <p className="mt-0.5 flex flex-wrap items-center gap-2 text-[11px] text-ink-faint">
                    <span>{view.rows} 行</span>
                    <span>·</span>
                    <span>{view.columns.length} 列</span>
                    {view.pk.length > 0 ? (
                      <>
                        <span>·</span>
                        <span>主键 {view.pk.join(', ')}</span>
                      </>
                    ) : (
                      <>
                        <span>·</span>
                        <span className="text-warn-text">无主键（只读）</span>
                      </>
                    )}
                  </p>
                </div>
                <div className="flex flex-wrap items-center gap-2">
                  <form
                    className="flex items-center gap-2"
                    onSubmit={(e) => {
                      e.preventDefault()
                      setQ(qInput.trim())
                      setPage(0)
                    }}
                  >
                    <div className="relative">
                      <Search
                        aria-hidden
                        className="pointer-events-none absolute left-3 top-1/2 size-3.5 -translate-y-1/2 text-ink-faint"
                      />
                      <Input
                        value={qInput}
                        placeholder="搜索文本列…"
                        className="pl-8"
                        onChange={(e) => setQInput(e.target.value)}
                      />
                    </div>
                    <Button variant="secondary" size="sm" type="submit">
                      搜索
                    </Button>
                  </form>
                  {view.editable && (
                    <Button size="sm" icon={Plus} onClick={() => openInsert(view)}>
                      新增行
                    </Button>
                  )}
                </div>
              </div>

              {rowsQuery.isPending ? (
                <div className="mt-4">
                  <SkeletonRows rows={6} cols={4} />
                </div>
              ) : rowsQuery.isError || !rows ? (
                <p className="mt-4 text-sm text-danger-text">
                  {rowsQuery.error instanceof Error ? rowsQuery.error.message : '读取行数据失败'}
                </p>
              ) : rows.rows.length === 0 ? (
                <p className="mt-6 text-sm text-ink-soft">{q ? '没有匹配的行' : '这张表还没有数据'}</p>
              ) : (
                <>
                  <div className="mt-4">
                    <Table>
                      <thead>
                        <tr>
                          {view.editable && <Th className="w-px whitespace-nowrap">操作</Th>}
                          {rows.columns.map((name) => (
                            <Th key={name} className="whitespace-nowrap font-mono text-[11.5px]">
                              {name}
                            </Th>
                          ))}
                        </tr>
                      </thead>
                      <tbody>
                        {rows.rows.map((row, rowIndex) => (
                          <tr key={rowIndex}>
                            {view.editable && (
                              <Td className="whitespace-nowrap">
                                <div className="flex items-center gap-1">
                                  <Button
                                    variant="ghost"
                                    size="icon"
                                    aria-label="编辑"
                                    icon={Pencil}
                                    onClick={() => openEdit(view, row)}
                                  />
                                  <Button
                                    variant="ghost"
                                    size="icon"
                                    aria-label="删除"
                                    icon={Trash2}
                                    onClick={() =>
                                      setPendingDelete({
                                        key: Object.fromEntries(
                                          view.columns
                                            .map((col, index) => [col, row[index]] as const)
                                            .filter(([col]) => col.pk > 0)
                                            .map(([col, value]) => [col.name, value]),
                                        ),
                                        label: view.pk
                                          .map((pk) => {
                                            const index = view.columns.findIndex((c) => c.name === pk)
                                            return `${pk}=${index >= 0 ? String(row[index]) : '?'}`
                                          })
                                          .join(' · '),
                                      })
                                    }
                                  />
                                </div>
                              </Td>
                            )}
                            {row.map((value, cellIndex) => (
                              <Td key={cellIndex} className="max-w-[260px] truncate">
                                {value === null ? (
                                  <span className="text-ink-faint">NULL</span>
                                ) : (
                                  <span
                                    className={cn(
                                      'block truncate',
                                      isNumericType(view.columns[cellIndex]?.type ?? '') && 'font-mono tabular-nums',
                                    )}
                                    title={String(value)}
                                  >
                                    {String(value)}
                                  </span>
                                )}
                              </Td>
                            ))}
                          </tr>
                        ))}
                      </tbody>
                    </Table>
                  </div>

                  <div className="mt-4 flex flex-wrap items-center justify-between gap-3 text-xs text-ink-soft">
                    <span>
                      共 {rows.total} 行 · 第 {page + 1} / {totalPages} 页
                    </span>
                    <div className="flex items-center gap-2">
                      <Button
                        variant="secondary"
                        size="sm"
                        disabled={page === 0}
                        onClick={() => setPage((prev) => Math.max(0, prev - 1))}
                      >
                        上一页
                      </Button>
                      <Button
                        variant="secondary"
                        size="sm"
                        disabled={page + 1 >= totalPages}
                        onClick={() => setPage((prev) => prev + 1)}
                      >
                        下一页
                      </Button>
                    </div>
                  </div>
                </>
              )}
            </div>
          )}
        </div>
      </div>

      <Panel title="SQL 控制台" description="一次只能执行一条语句；SELECT / PRAGMA / WITH 返回结果集，其余返回影响行数">
        <DangerNote>
          直接改库会绕开框架校验：kv_data 里存的是 JSON，inbox_* 与 stats_* 由程序自己维护，
          手改可能导致数据对不上。改之前建议先备份 bot.db。
        </DangerNote>
        <Textarea
          rows={5}
          className="font-mono text-sm"
          value={sql}
          onChange={(e) => setSql(e.target.value)}
          placeholder="SELECT * FROM kv_data LIMIT 10"
        />
        <div className="mt-3 flex items-center gap-2">
          <Button icon={Play} loading={running} onClick={runSQL}>
            执行
          </Button>
          {sqlResult && (
            <span className="text-xs text-ink-soft">
              {sqlResult.kind === 'rows'
                ? `${sqlResult.rows?.length ?? 0} 行`
                : `影响 ${sqlResult.affected ?? 0} 行`}{' '}
              · {sqlResult.elapsed_ms}ms
            </span>
          )}
        </div>
        {sqlResult?.kind === 'rows' && (sqlResult.rows?.length ?? 0) > 0 && (
          <div className="mt-4 max-h-[420px] overflow-auto">
            <Table>
              <thead>
                <tr>
                  {(sqlResult.columns ?? []).map((name) => (
                    <Th key={name} className="whitespace-nowrap font-mono text-[11.5px]">
                      {name}
                    </Th>
                  ))}
                </tr>
              </thead>
              <tbody>
                {(sqlResult.rows ?? []).map((row, rowIndex) => (
                  <tr key={rowIndex}>
                    {row.map((value, cellIndex) => (
                      <Td key={cellIndex} className="max-w-[260px] truncate">
                        {value === null ? (
                          <span className="text-ink-faint">NULL</span>
                        ) : (
                          <span className="block truncate" title={String(value)}>
                            {String(value)}
                          </span>
                        )}
                      </Td>
                    ))}
                  </tr>
                ))}
              </tbody>
            </Table>
          </div>
        )}
      </Panel>

      <Modal
        open={draft !== null}
        onClose={() => setDraft(null)}
        title={draft?.mode === 'insert' ? '新增行' : '编辑行'}
        description={
          draft?.mode === 'update'
            ? '按主键定位这一行；留空对非文本列写入 NULL'
            : '留空表示不写入该列（使用列默认值）或 NULL'
        }
        width="lg"
        footer={
          <>
            <Button variant="secondary" onClick={() => setDraft(null)}>
              取消
            </Button>
            <Button loading={saving} onClick={submitDraft}>
              保存
            </Button>
          </>
        }
      >
        {draft && view && (
          <div className="grid gap-4 md:grid-cols-2">
            {view.columns.map((col) => (
              <Field
                key={col.name}
                label={
                  <span className="flex items-center gap-2">
                    <span className="font-mono text-[12.5px]">{col.name}</span>
                    <span className="text-[11px] text-ink-faint">{col.type || 'ANY'}</span>
                    {col.pk > 0 && <Badge tone="neutral">PK</Badge>}
                  </span>
                }
                hint={[col.not_null ? 'NOT NULL' : '', col.default !== null ? `默认 ${col.default}` : '']
                  .filter(Boolean)
                  .join(' · ')}
                className={col.pk > 0 && draft.mode === 'update' ? 'opacity-70' : undefined}
              >
                <Input
                  value={draft.values[col.name] ?? ''}
                  disabled={draft.mode === 'update' && col.pk > 0}
                  placeholder={col.default !== null ? String(col.default) : col.not_null ? '必填' : 'NULL'}
                  onChange={(e) =>
                    setDraft((prev) =>
                      prev ? { ...prev, values: { ...prev.values, [col.name]: e.target.value } } : prev,
                    )
                  }
                />
              </Field>
            ))}
          </div>
        )}
      </Modal>

      <ConfirmDialog
        open={pendingDelete !== null}
        onClose={() => setPendingDelete(null)}
        onConfirm={confirmDelete}
        title="删除这一行"
        description={`将从 ${view?.name ?? ''} 删除 ${pendingDelete?.label ?? ''}，不可撤销。确认删除？`}
        confirmLabel="删除"
        danger
      />
    </>
  )
}
