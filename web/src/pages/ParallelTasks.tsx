import { useEffect, useMemo, useState } from 'react'
import { Activity, AlertTriangle, CheckCircle2, Clock3, GitBranch, Layers3, PauseCircle, PlayCircle, RefreshCw, ShieldCheck, SlidersHorizontal, Zap } from 'lucide-react'
import type { LucideIcon } from 'lucide-react'
import { fetchParallelTasksStatus, updateParallelTasksConfig, type ParallelOrchestrationConfig, type ParallelOrchestrationSnapshot, type ParallelSubtask } from '../api'

const fallbackConfig: ParallelOrchestrationConfig = {
  enabled: true,
  max_concurrency: 4,
  require_approval_high: true,
  require_approval_remote: true,
  dry_run: false,
  serial_fallback: true,
  auto_threshold: 'complex',
}

const fallbackSnapshot: ParallelOrchestrationSnapshot = {
  active: false,
  status: 'idle',
  run_id: 'local-preview',
  title: 'Belum ada orchestration aktif',
  updated_at: new Date().toISOString(),
  config: fallbackConfig,
  summary: { total: 6, running: 0, success: 0, failed: 0, skipped: 0, gated: 0 },
  batches: [
    { id: 'wave-1', name: 'Discovery parallel wave', mode: 'parallel', status: 'preview', subtasks: [
      { id: 'scan-structure', title: 'Scan struktur workspace', status: 'preview', risk: 'low', kind: 'read-only' },
      { id: 'read-config', title: 'Baca config/package utama', status: 'preview', risk: 'low', kind: 'read-only' },
      { id: 'grep-todo', title: 'Cari TODO/FIXME/error marker', status: 'preview', risk: 'low', kind: 'read-only' },
    ] },
    { id: 'wave-2', name: 'Analysis & gated action', mode: 'gated', status: 'preview', subtasks: [
      { id: 'analyze', title: 'Analisis hasil discovery', status: 'preview', risk: 'low', kind: 'reasoning' },
      { id: 'mutate-if-approved', title: 'Perubahan file bila diperlukan', status: 'preview', risk: 'medium', kind: 'mutating' },
      { id: 'verify', title: 'Build/test/verifikasi', status: 'preview', risk: 'low', kind: 'command' },
    ] },
  ],
  report_markdown: 'Belum ada orchestration yang pernah dijalankan di sesi server ini. Kirim prompt kompleks dari Chat untuk melihat batch/subtask live di sini.',
}

function normalizeConfig(config?: ParallelOrchestrationConfig): ParallelOrchestrationConfig {
  return { ...fallbackConfig, ...(config || {}) }
}

function numberValue(value: unknown, fallback = 0): number {
  return typeof value === 'number' && Number.isFinite(value) ? value : fallback
}

function pickNumber(source: Record<string, unknown> | undefined, snake: string, pascal: string): number {
  if (!source) return 0
  return numberValue(source[snake], numberValue(source[pascal]))
}

function normalizeSnapshot(data?: Partial<ParallelOrchestrationSnapshot>): ParallelOrchestrationSnapshot {
  const rawSummary = (data?.summary || {}) as unknown as Record<string, unknown>
  const config = normalizeConfig(data?.config)
  return {
    ...fallbackSnapshot,
    ...(data || {}),
    active: Boolean(data?.active),
    status: data?.status || fallbackSnapshot.status,
    run_id: data?.run_id || fallbackSnapshot.run_id,
    title: data?.title || fallbackSnapshot.title,
    updated_at: data?.updated_at || new Date().toISOString(),
    config,
    summary: {
      total: pickNumber(rawSummary, 'total', 'Total'),
      running: pickNumber(rawSummary, 'running', 'Running'),
      success: pickNumber(rawSummary, 'success', 'Success'),
      failed: pickNumber(rawSummary, 'failed', 'Failed'),
      skipped: pickNumber(rawSummary, 'skipped', 'Skipped'),
      gated: pickNumber(rawSummary, 'gated', 'Gated'),
    },
    batches: Array.isArray(data?.batches) ? data!.batches : [],
    tasks: Array.isArray(data?.tasks) ? data!.tasks : [],
    agents: Array.isArray(data?.agents) ? data!.agents : [],
    events: Array.isArray(data?.events) ? data!.events : [],
  }
}

function statusTone(status?: string) {
  if (status === 'success' || status === 'completed') return 'text-emerald-300 border-emerald-400/25 bg-emerald-400/10'
  if (status === 'failed' || status === 'error') return 'text-red-300 border-red-400/25 bg-red-400/10'
  if (status === 'running' || status === 'active') return 'text-sky-300 border-sky-400/25 bg-sky-400/10'
  if (status === 'gated' || status === 'waiting_approval') return 'text-amber-300 border-amber-400/25 bg-amber-400/10'
  return 'text-neutral-300 border-white/10 bg-white/5'
}

function riskTone(risk?: string) {
  if (risk === 'high' || risk === 'destructive') return 'bg-red-500/15 text-red-200 border-red-400/20'
  if (risk === 'medium') return 'bg-amber-500/15 text-amber-200 border-amber-400/20'
  return 'bg-lime-500/10 text-lime-200 border-lime-400/20'
}

function SubtaskCard({ task }: { task: ParallelSubtask }) {
  return (
    <div className="rounded-2xl border border-neutral-800/80 bg-neutral-950/45 p-4 shadow-lg shadow-black/10">
      <div className="flex items-start justify-between gap-3"><div><div className="text-sm font-semibold text-white">{task.title}</div><div className="mt-1 text-xs text-neutral-500">{task.id} · {task.kind || 'task'}</div></div><span className={`rounded-full border px-2 py-0.5 text-[10px] uppercase tracking-wide ${statusTone(task.status)}`}>{task.status || 'pending'}</span></div>
      <div className="mt-3 flex flex-wrap gap-2"><span className={`rounded-full border px-2 py-0.5 text-[10px] ${riskTone(task.risk)}`}>risk: {task.risk || 'low'}</span>{task.duration_ms !== undefined && <span className="rounded-full border border-white/10 bg-white/5 px-2 py-0.5 text-[10px] text-neutral-400">{task.duration_ms}ms</span>}</div>
      {task.error && <div className="mt-3 rounded-xl border border-red-400/20 bg-red-500/10 p-2 text-xs text-red-200">{task.error}</div>}
      {task.output && <pre className="mt-3 max-h-24 overflow-auto rounded-xl bg-black/35 p-2 text-[11px] text-neutral-300">{task.output}</pre>}
    </div>
  )
}

export default function ParallelTasks() {
  const [snapshot, setSnapshot] = useState<ParallelOrchestrationSnapshot>(fallbackSnapshot)
  const [config, setConfig] = useState<ParallelOrchestrationConfig>(fallbackConfig)
  const [loading, setLoading] = useState(false)
  const [apiState, setApiState] = useState<'live' | 'preview'>('preview')

  const refresh = async () => {
    setLoading(true)
    try {
      const nextSnapshot = normalizeSnapshot(await fetchParallelTasksStatus())
      setSnapshot(nextSnapshot)
      setConfig(nextSnapshot.config)
      setApiState('live')
    } catch {
      setSnapshot(prev => normalizeSnapshot({ ...fallbackSnapshot, config: normalizeConfig(prev.config) }))
      setApiState('preview')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    refresh()
    let fallbackTimer: number | undefined
    const es = new EventSource('/api/orchestration/events')
    es.addEventListener('snapshot', event => {
      try {
        const nextSnapshot = normalizeSnapshot(JSON.parse((event as MessageEvent).data) as ParallelOrchestrationSnapshot)
        setSnapshot(nextSnapshot)
        setConfig(nextSnapshot.config)
        setApiState('live')
      } catch {}
    })
    es.onerror = () => {
      setApiState('preview')
      if (!fallbackTimer) fallbackTimer = window.setInterval(refresh, 4000)
    }
    return () => { es.close(); if (fallbackTimer) window.clearInterval(fallbackTimer) }
  }, [])

  const summary = snapshot.summary || { total: 0, running: 0, success: 0, failed: 0, skipped: 0, gated: 0 }
  const completion = useMemo(() => summary.total ? Math.round(((summary.success + summary.skipped + summary.failed) / summary.total) * 100) : 0, [summary])
  const summaryCards: Array<[string, number, LucideIcon]> = [['Total', summary.total, Layers3], ['Running', summary.running, Activity], ['Success', summary.success, CheckCircle2], ['Gated', summary.gated, ShieldCheck], ['Failed', summary.failed, AlertTriangle]]

  const patchConfig = async (patch: Partial<ParallelOrchestrationConfig>) => {
    const next = { ...config, ...patch }
    setConfig(next)
    setSnapshot(s => ({ ...s, config: next }))
    try { await updateParallelTasksConfig(next); setApiState('live') } catch { setApiState('preview') }
  }

  return (
    <div className="h-full overflow-y-auto bg-[#12190e] p-6 text-neutral-100"><div className="mx-auto max-w-7xl space-y-6">
      <header className="rounded-[2rem] border border-neutral-800/80 bg-gradient-to-br from-neutral-950/80 via-[#17220f]/85 to-neutral-950/75 p-6 shadow-2xl shadow-black/25"><div className="flex flex-col gap-4 lg:flex-row lg:items-center lg:justify-between"><div><div className="flex items-center gap-3 text-sm text-smara-200"><Zap className="h-4 w-4" /> Parallel Orchestration</div><h1 className="mt-2 text-3xl font-semibold tracking-tight text-white">Parallel Tasks</h1><p className="mt-2 max-w-2xl text-sm text-neutral-400">Monitor batch, subtask, guardrail, dan report saat Smara otomatis memecah pekerjaan kompleks menjadi eksekusi paralel yang aman.</p></div><div className="flex flex-wrap gap-2"><span className={`rounded-full border px-3 py-1 text-xs ${apiState === 'live' ? 'border-emerald-400/25 bg-emerald-400/10 text-emerald-200' : 'border-amber-400/25 bg-amber-400/10 text-amber-200'}`}>{apiState === 'live' ? 'Live stream' : 'Preview mode'}</span><button onClick={refresh} className="inline-flex items-center gap-2 rounded-full border border-white/10 bg-white/5 px-3 py-1 text-xs text-neutral-200 hover:bg-white/10"><RefreshCw className={`h-3.5 w-3.5 ${loading ? 'animate-spin' : ''}`} /> Refresh</button></div></div></header>
      <section className="grid gap-4 md:grid-cols-2 xl:grid-cols-5">{summaryCards.map(([label, value, Icon]) => <div key={label} className="rounded-3xl border border-neutral-800/80 bg-neutral-950/45 p-4"><div className="flex items-center justify-between text-neutral-500"><span className="text-xs uppercase tracking-wide">{label}</span><Icon className="h-4 w-4" /></div><div className="mt-3 text-3xl font-semibold text-white">{String(value ?? 0)}</div></div>)}</section>
      <section className="grid gap-6 lg:grid-cols-[360px_1fr]"><aside className="space-y-4"><div className="rounded-3xl border border-neutral-800/80 bg-neutral-950/45 p-5"><div className="mb-4 flex items-center gap-2 text-white"><SlidersHorizontal className="h-4 w-4 text-smara-300" /> Auto Orchestration Config</div><label className="flex items-center justify-between gap-3 rounded-2xl bg-white/5 p-3 text-sm"><span>Enabled otomatis</span><input type="checkbox" checked={!!config.enabled} onChange={e => patchConfig({ enabled: e.target.checked })} /></label><label className="mt-3 flex items-center justify-between gap-3 rounded-2xl bg-white/5 p-3 text-sm"><span>Dry-run default</span><input type="checkbox" checked={!!config.dry_run} onChange={e => patchConfig({ dry_run: e.target.checked })} /></label><label className="mt-3 flex items-center justify-between gap-3 rounded-2xl bg-white/5 p-3 text-sm"><span>Serial fallback</span><input type="checkbox" checked={!!config.serial_fallback} onChange={e => patchConfig({ serial_fallback: e.target.checked })} /></label><label className="mt-3 flex items-center justify-between gap-3 rounded-2xl bg-white/5 p-3 text-sm"><span>Approval risk tinggi</span><input type="checkbox" checked={!!config.require_approval_high} onChange={e => patchConfig({ require_approval_high: e.target.checked })} /></label><label className="mt-3 flex items-center justify-between gap-3 rounded-2xl bg-white/5 p-3 text-sm"><span>Approval remote/VPS</span><input type="checkbox" checked={!!config.require_approval_remote} onChange={e => patchConfig({ require_approval_remote: e.target.checked })} /></label><label className="mt-3 block rounded-2xl bg-white/5 p-3 text-sm"><span className="text-neutral-400">Auto threshold</span><select className="mt-2 w-full rounded-xl border border-white/10 bg-neutral-950 px-3 py-2 text-sm text-white outline-none focus:border-smara-300" value={config.auto_threshold || 'complex'} onChange={e => patchConfig({ auto_threshold: e.target.value })}><option value="simple">Simple - lebih sering paralel</option><option value="complex">Complex - seimbang</option><option value="aggressive">Aggressive - paling agresif</option></select></label><label className="mt-3 block rounded-2xl bg-white/5 p-3 text-sm"><span className="text-neutral-400">Max concurrency</span><input className="mt-2 w-full accent-lime-300" type="range" min={1} max={12} value={config.max_concurrency || 1} onChange={e => patchConfig({ max_concurrency: Number(e.target.value) })} /><div className="text-right text-smara-200">{config.max_concurrency || 1} workers</div></label></div><div className="rounded-3xl border border-neutral-800/80 bg-neutral-950/45 p-5"><div className="flex items-center gap-2 text-white"><Clock3 className="h-4 w-4 text-smara-300" /> Current Run</div><div className="mt-4 space-y-2 text-sm text-neutral-400"><div className="flex justify-between"><span>Status</span><span className="text-white">{snapshot.status || '-'}</span></div><div className="flex justify-between"><span>Run ID</span><span className="max-w-[170px] truncate text-white">{snapshot.run_id || '-'}</span></div><div className="flex justify-between"><span>Updated</span><span className="text-white">{snapshot.updated_at ? new Date(snapshot.updated_at).toLocaleTimeString() : '-'}</span></div></div><div className="mt-4 h-2 overflow-hidden rounded-full bg-neutral-800"><div className="h-full bg-gradient-to-r from-smara-400 to-lime-200" style={{ width: `${completion}%` }} /></div><div className="mt-1 text-right text-xs text-neutral-500">{completion}% complete</div></div></aside><main className="space-y-5"><div className="rounded-3xl border border-neutral-800/80 bg-neutral-950/45 p-5"><div className="flex items-center gap-2 text-white"><GitBranch className="h-4 w-4 text-smara-300" /> {snapshot.title}</div><div className="mt-5 space-y-5">{(snapshot.batches || []).map(batch => <div key={batch.id} className="rounded-3xl border border-white/10 bg-white/[0.03] p-4"><div className="mb-4 flex flex-wrap items-center justify-between gap-3"><div><div className="font-semibold text-white">{batch.name}</div><div className="text-xs text-neutral-500">{batch.id} · {batch.mode}</div></div><span className={`rounded-full border px-3 py-1 text-xs ${statusTone(batch.status)}`}>{batch.status}</span></div><div className="grid gap-3 md:grid-cols-2 xl:grid-cols-3">{(batch.subtasks || []).map(task => <SubtaskCard key={task.id} task={task} />)}</div></div>)}</div></div><div className="rounded-3xl border border-neutral-800/80 bg-neutral-950/45 p-5"><div className="mb-3 flex items-center gap-2 text-white">{snapshot.active ? <PlayCircle className="h-4 w-4 text-emerald-300" /> : <PauseCircle className="h-4 w-4 text-neutral-500" />} Final Report</div><pre className="max-h-72 overflow-auto whitespace-pre-wrap rounded-2xl bg-black/30 p-4 text-sm leading-6 text-neutral-300">{snapshot.report_markdown || 'Report akan muncul setelah orchestration selesai.'}</pre></div></main></section>
    </div></div>
  )
}
