import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { Activity, AlertTriangle, Bot, CheckCircle2, Clock3, GitBranch, Layers3, Network, PauseCircle, PlayCircle, RefreshCw, ShieldCheck, SlidersHorizontal, Sparkles, Zap } from 'lucide-react'
import type { PointerEvent as ReactPointerEvent } from 'react'
import type { LucideIcon } from 'lucide-react'
import { fetchParallelTasksStatus, updateParallelTasksConfig, type ParallelOrchestrationConfig, type ParallelOrchestrationSnapshot } from '../api'

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
  title: 'Belum ada agent orchestration aktif',
  updated_at: new Date().toISOString(),
  config: fallbackConfig,
  summary: { total: 6, running: 0, success: 0, failed: 0, skipped: 0, gated: 0 },
  batches: [
    { id: 'wave-1', name: 'Discovery agent wave', mode: 'parallel', status: 'preview', subtasks: [
      { id: 'frontend-agent', title: 'Frontend Agent', status: 'preview', risk: 'low', kind: 'read-only' },
      { id: 'backend-agent', title: 'Backend Agent', status: 'preview', risk: 'low', kind: 'read-only' },
      { id: 'qa-agent', title: 'QA/Test Agent', status: 'preview', risk: 'low', kind: 'command' },
    ] },
    { id: 'wave-2', name: 'Specialist agent coordination', mode: 'gated', status: 'preview', subtasks: [
      { id: 'architect-agent', title: 'Architecture Agent', status: 'preview', risk: 'low', kind: 'reasoning' },
      { id: 'devops-agent', title: 'DevOps Agent', status: 'preview', risk: 'medium', kind: 'mutating' },
      { id: 'review-agent', title: 'Review Agent', status: 'preview', risk: 'low', kind: 'reasoning' },
    ] },
  ],
  report_markdown: 'Belum ada parallel agent run di sesi server ini. Kirim prompt kompleks dari Chat untuk melihat coordinator dan specialist agent live di sini.',
}

type AgentNode = { id: string; name: string; role: string; status?: string; risk?: string; kind?: string; parentId?: string; x: number; y: number }

function normalizeConfig(config?: ParallelOrchestrationConfig): ParallelOrchestrationConfig { return { ...fallbackConfig, ...(config || {}) } }
function numberValue(value: unknown, fallback = 0): number { return typeof value === 'number' && Number.isFinite(value) ? value : fallback }
function pickNumber(source: Record<string, unknown> | undefined, snake: string, pascal: string): number { if (!source) return 0; return numberValue(source[snake], numberValue(source[pascal])) }

function normalizeSnapshot(data?: Partial<ParallelOrchestrationSnapshot>): ParallelOrchestrationSnapshot {
  const rawSummary = (data?.summary || {}) as unknown as Record<string, unknown>
  const config = normalizeConfig(data?.config)
  return { ...fallbackSnapshot, ...(data || {}), active: Boolean(data?.active), status: data?.status || fallbackSnapshot.status, run_id: data?.run_id || fallbackSnapshot.run_id, title: data?.title || fallbackSnapshot.title, updated_at: data?.updated_at || new Date().toISOString(), config, summary: { total: pickNumber(rawSummary, 'total', 'Total'), running: pickNumber(rawSummary, 'running', 'Running'), success: pickNumber(rawSummary, 'success', 'Success'), failed: pickNumber(rawSummary, 'failed', 'Failed'), skipped: pickNumber(rawSummary, 'skipped', 'Skipped'), gated: pickNumber(rawSummary, 'gated', 'Gated') }, batches: Array.isArray(data?.batches) ? data!.batches : [], tasks: Array.isArray(data?.tasks) ? data!.tasks : [], agents: Array.isArray(data?.agents) ? data!.agents : [], events: Array.isArray(data?.events) ? data!.events : [] }
}

function statusTone(status?: string) {
  if (status === 'success' || status === 'completed') return 'text-emerald-300 border-emerald-400/25 bg-emerald-400/10'
  if (status === 'failed' || status === 'error') return 'text-red-300 border-red-400/25 bg-red-400/10'
  if (status === 'running' || status === 'active') return 'text-sky-300 border-sky-400/25 bg-sky-400/10'
  if (status === 'gated' || status === 'waiting_approval') return 'text-amber-300 border-amber-400/25 bg-amber-400/10'
  return 'text-neutral-300 border-white/10 bg-white/5'
}
function riskTone(risk?: string) { if (risk === 'high' || risk === 'destructive') return 'bg-red-500/15 text-red-200 border-red-400/20'; if (risk === 'medium') return 'bg-amber-500/15 text-amber-200 border-amber-400/20'; return 'bg-lime-500/10 text-lime-200 border-lime-400/20' }
function isActive(status?: string) { return status === 'running' || status === 'active' }
function activeBatchStatus(batch: NonNullable<ParallelOrchestrationSnapshot['batches']>[number], snapshotStatus?: string): string {
  if (isActive(batch.status) || (batch.subtasks || []).some(task => isActive(task.status))) return 'active'
  if ((batch.subtasks || []).some(task => task.status === 'failed' || task.status === 'error')) return 'error'
  if ((batch.subtasks || []).length && (batch.subtasks || []).every(task => task.status === 'success' || task.status === 'completed' || task.status === 'done')) return 'completed'
  return batch.status || snapshotStatus || 'idle'
}
function spreadX(index: number, total: number): number {
  if (total <= 1) return 50
  const width = total <= 2 ? 44 : total <= 3 ? 64 : 78
  return 50 - width / 2 + (width * index) / (total - 1)
}

function buildAgentNodes(snapshot: ParallelOrchestrationSnapshot): AgentNode[] {
  const batches = (snapshot.batches || []).slice(0, 4)
  const nodes: AgentNode[] = [{ id: 'coordinator', name: 'Coordinator Agent', role: snapshot.active ? 'Orchestrating live agents' : 'Planning & routing', status: snapshot.active ? 'active' : snapshot.status, risk: 'low', kind: 'coordinator', x: 50, y: 16 }]

  if (batches.length) {
    batches.forEach((batch, batchIndex) => {
      const waveId = `wave:${batch.id}`
      const waveX = spreadX(batchIndex, batches.length)
      const waveStatus = activeBatchStatus(batch, snapshot.status)
      nodes.push({ id: waveId, name: batch.name, role: `${batch.mode || 'parallel'} wave`, status: waveStatus, risk: 'low', kind: 'wave', parentId: 'coordinator', x: waveX, y: 43 })
      ;(batch.subtasks || []).slice(0, 3).forEach((task, taskIndex, tasks) => {
        const offset = tasks.length <= 1 ? 0 : (taskIndex - (tasks.length - 1) / 2) * 11
        nodes.push({ id: task.id, name: task.title, role: task.kind || 'Specialist agent', status: task.status, risk: task.risk, kind: task.kind || 'agent', parentId: waveId, x: Math.min(92, Math.max(8, waveX + offset)), y: 73 + Math.min(batchIndex, 1) * 12 })
      })
    })
    return nodes.slice(0, 17)
  }

  const rawAgents = (snapshot.agents || []) as unknown as Array<Record<string, unknown>>
  const items = rawAgents.map((a, i) => ({ id: String(a.id || a.name || `agent-${i}`), name: String(a.name || a.title || `Agent ${i + 1}`), role: String(a.role || a.kind || 'Specialist'), status: String(a.status || snapshot.status || 'idle'), risk: String(a.risk || 'low'), kind: String(a.kind || 'agent') })).slice(0, 10)
  items.forEach((item, index) => nodes.push({ ...item, parentId: 'coordinator', x: spreadX(index, items.length), y: 66 + (index % 2) * 14 }))
  return nodes
}

function edgePath(parent: AgentNode, node: AgentNode): string {
  const dy = Math.max(10, Math.abs(node.y - parent.y) * 0.42)
  return `M ${parent.x} ${parent.y} C ${parent.x} ${parent.y + dy}, ${node.x} ${node.y - dy}, ${node.x} ${node.y}`
}

function AgentNetwork({ snapshot }: { snapshot: ParallelOrchestrationSnapshot }) {
  const nodes = useMemo(() => buildAgentNodes(snapshot), [snapshot])
  const graphRef = useRef<HTMLDivElement>(null)
  const dragRef = useRef<{ id: string; pointerId: number } | null>(null)
  const [nodePositions, setNodePositions] = useState<Record<string, { x: number; y: number }>>({})
  const positionedNodes = useMemo(() => nodes.map(node => ({ ...node, ...(nodePositions[node.id] || {}) })), [nodePositions, nodes])
  const byID = new Map(positionedNodes.map(node => [node.id, node]))
  const edges = positionedNodes.slice(1).map(node => ({ node, parent: byID.get(node.parentId || 'coordinator') || positionedNodes[0] }))
  const moveNode = useCallback((id: string, event: ReactPointerEvent<HTMLDivElement>) => {
    const rect = graphRef.current?.getBoundingClientRect()
    if (!rect) return
    const x = Math.min(92, Math.max(8, ((event.clientX - rect.left) / rect.width) * 100))
    const y = Math.min(92, Math.max(8, ((event.clientY - rect.top) / rect.height) * 100))
    setNodePositions(prev => ({ ...prev, [id]: { x, y } }))
  }, [])
  const startDrag = useCallback((id: string, event: ReactPointerEvent<HTMLDivElement>) => {
    event.preventDefault()
    event.currentTarget.setPointerCapture(event.pointerId)
    dragRef.current = { id, pointerId: event.pointerId }
    moveNode(id, event)
  }, [moveNode])
  const dragNode = useCallback((event: ReactPointerEvent<HTMLDivElement>) => {
    const drag = dragRef.current
    if (!drag || drag.pointerId !== event.pointerId) return
    moveNode(drag.id, event)
  }, [moveNode])
  const stopDrag = useCallback((event: ReactPointerEvent<HTMLDivElement>) => {
    if (dragRef.current?.pointerId === event.pointerId) dragRef.current = null
  }, [])

  return <div className="relative min-h-[560px] overflow-hidden rounded-3xl border border-neutral-800/80 bg-[radial-gradient(circle_at_50%_16%,oklch(0.72_0.16_135/.20),transparent_34%),linear-gradient(145deg,oklch(0.14_0.03_135/.96),oklch(0.18_0.04_250/.82))] p-4">
    <div className="mb-3 flex items-center justify-between gap-3"><div><div className="flex items-center gap-2 text-white"><Network className="h-4 w-4 text-lime-300" /> Agent Network</div><p className="mt-1 text-xs text-neutral-400">Drag node untuk mengatur posisi. Wave aktif saat batch atau agent di dalamnya running.</p></div><span className={`rounded-full border px-3 py-1 text-xs ${snapshot.active ? 'border-sky-300/30 bg-sky-400/10 text-sky-200' : 'border-white/10 bg-white/5 text-neutral-300'}`}>{snapshot.active ? 'Agents active' : 'Standby graph'}</span></div>
    <div ref={graphRef} className="absolute inset-x-4 bottom-4 top-16 overflow-hidden rounded-3xl border border-white/5 bg-black/15 touch-none">
      <svg className="pointer-events-none absolute inset-0 h-full w-full" viewBox="0 0 100 100" preserveAspectRatio="none">
        <defs>
          <filter id="agent-edge-glow" x="-20%" y="-20%" width="140%" height="140%">
            <feGaussianBlur stdDeviation="0.7" result="blur" />
            <feMerge><feMergeNode in="blur" /><feMergeNode in="SourceGraphic" /></feMerge>
          </filter>
        </defs>
        {edges.map(({ node, parent }) => {
          const active = isActive(node.status)
          const stroke = active ? 'rgba(56,189,248,.86)' : node.kind === 'wave' ? 'rgba(163,230,53,.48)' : 'rgba(148,163,184,.34)'
          return <g key={`${parent.id}-${node.id}`}>
            <path d={edgePath(parent, node)} fill="none" stroke="rgba(2,6,23,.72)" strokeWidth={node.kind === 'wave' ? 1.25 : 0.95} strokeLinecap="round" />
            <path d={edgePath(parent, node)} fill="none" stroke={stroke} strokeWidth={node.kind === 'wave' ? 0.62 : 0.42} strokeLinecap="round" strokeDasharray={active ? '1.8 1.4' : node.kind === 'wave' ? '0' : '0.8 1.8'} filter={active ? 'url(#agent-edge-glow)' : undefined} />
            <circle cx={node.x} cy={node.y} r={active ? 0.9 : 0.58} fill={active ? 'rgba(56,189,248,.95)' : 'rgba(163,230,53,.62)'} />
          </g>
        })}
      </svg>
      {positionedNodes.map(n => {
        const isCoordinator = n.kind === 'coordinator'
        const isWave = n.kind === 'wave'
        const active = isActive(n.status)
        const shell = isCoordinator
          ? 'border-smara-300/50 bg-smara-950/75 shadow-smara-950/40'
          : isWave
          ? 'border-cyan-300/45 bg-cyan-950/70 shadow-cyan-950/35'
          : active
          ? 'border-sky-300/55 bg-sky-950/72 shadow-sky-950/35'
          : 'border-lime-300/25 bg-[#070905]/88 shadow-black/40'
        const header = isCoordinator ? 'bg-smara-400/15' : isWave ? 'bg-cyan-400/15' : active ? 'bg-sky-400/15' : 'bg-lime-400/10'
        return <div key={n.id} onPointerDown={event => startDrag(n.id, event)} onPointerMove={dragNode} onPointerUp={stopDrag} onPointerCancel={stopDrag} className={`absolute ${isCoordinator ? 'w-56' : isWave ? 'w-52' : 'w-44'} -translate-x-1/2 -translate-y-1/2 cursor-grab select-none overflow-hidden rounded-2xl border shadow-2xl transition-shadow active:cursor-grabbing ${shell} ${active ? 'animate-pulse' : ''}`} style={{ left: `${n.x}%`, top: `${n.y}%` }}>
          <div className="absolute -left-1.5 top-1/2 h-3 w-3 -translate-y-1/2 rounded-full border border-gray-950 bg-smara-300" />
          <div className="absolute -right-1.5 top-1/2 h-3 w-3 -translate-y-1/2 rounded-full border border-gray-950 bg-lime-300" />
          <div className={`flex items-center gap-2 px-3 py-2 ${header}`}>
            {isCoordinator ? <Sparkles className="h-4 w-4 text-smara-200" /> : isWave ? <GitBranch className="h-4 w-4 text-cyan-200" /> : <Bot className="h-4 w-4 text-lime-200" />}
            <div className="min-w-0 text-xs font-semibold text-white"><div className="truncate">{n.name}</div></div>
          </div>
          <div className="space-y-2 p-3">
            <div className="text-[10px] uppercase tracking-wider text-gray-500">{isCoordinator ? 'Coordinator Node' : isWave ? 'Wave Node' : 'Agent Node'}</div>
            <div className="line-clamp-2 text-[11px] text-gray-300">{n.role}</div>
            <div className="flex flex-wrap gap-1.5">
              <span className={`rounded-full border px-2 py-0.5 text-[9px] uppercase tracking-wide ${statusTone(n.status)}`}>{n.status || 'idle'}</span>
              {!isWave && <span className={`rounded-full border px-2 py-0.5 text-[9px] ${riskTone(n.risk)}`}>{n.risk || 'low'}</span>}
              <span className="rounded-full bg-white/10 px-2 py-0.5 text-[9px] text-gray-300">drag</span>
            </div>
          </div>
        </div>
      })}
    </div>
  </div>
}


export default function ParallelTasks() {
  const [snapshot, setSnapshot] = useState<ParallelOrchestrationSnapshot>(fallbackSnapshot)
  const [config, setConfig] = useState<ParallelOrchestrationConfig>(fallbackConfig)
  const [loading, setLoading] = useState(false)
  const [apiState, setApiState] = useState<'live' | 'preview'>('preview')

  const refresh = async () => { setLoading(true); try { const nextSnapshot = normalizeSnapshot(await fetchParallelTasksStatus()); setSnapshot(nextSnapshot); setConfig(nextSnapshot.config); setApiState('live') } catch { setSnapshot(prev => normalizeSnapshot({ ...fallbackSnapshot, config: normalizeConfig(prev.config) })); setApiState('preview') } finally { setLoading(false) } }
  useEffect(() => { refresh(); let fallbackTimer: number | undefined; const es = new EventSource('/api/orchestration/events'); es.addEventListener('snapshot', event => { try { const nextSnapshot = normalizeSnapshot(JSON.parse((event as MessageEvent).data) as ParallelOrchestrationSnapshot); setSnapshot(nextSnapshot); setConfig(nextSnapshot.config); setApiState('live') } catch {} }); es.onerror = () => { setApiState('preview'); if (!fallbackTimer) fallbackTimer = window.setInterval(refresh, 4000) }; return () => { es.close(); if (fallbackTimer) window.clearInterval(fallbackTimer) } }, [])

  const summary = snapshot.summary || { total: 0, running: 0, success: 0, failed: 0, skipped: 0, gated: 0 }
  const completion = useMemo(() => summary.total ? Math.round(((summary.success + summary.skipped + summary.failed) / summary.total) * 100) : 0, [summary])
  const summaryCards: Array<[string, number, LucideIcon]> = [['Agents', summary.total, Layers3], ['Active', summary.running, Activity], ['Completed', summary.success, CheckCircle2], ['Gated', summary.gated, ShieldCheck], ['Failed', summary.failed, AlertTriangle]]
  const patchConfig = async (patch: Partial<ParallelOrchestrationConfig>) => { const next = { ...config, ...patch }; setConfig(next); setSnapshot(s => ({ ...s, config: next })); try { await updateParallelTasksConfig(next); setApiState('live') } catch { setApiState('preview') } }

  return <div className="h-full overflow-y-auto bg-[oklch(0.14_0.025_135)] px-4 py-3 text-neutral-100 lg:px-5 lg:py-4"><div className="mx-auto flex max-w-7xl flex-col gap-4">
    <header className="rounded-[1.75rem] border border-neutral-800/80 bg-[linear-gradient(135deg,oklch(0.16_0.03_135/.92),oklch(0.18_0.045_250/.86))] px-5 py-4 shadow-xl shadow-black/20"><div className="flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between"><div className="min-w-0"><div className="flex items-center gap-3 text-xs font-medium uppercase tracking-[0.18em] text-smara-200"><Zap className="h-4 w-4" /> Parallel Agent Orchestration</div><div className="mt-2 flex flex-wrap items-end gap-x-4 gap-y-2"><h1 className="text-2xl font-semibold tracking-tight text-white">Parallel Agent</h1><p className="max-w-3xl text-sm text-neutral-400">Wave, specialist agent, guardrail, dan report dalam satu console padat.</p></div></div><div className="flex flex-wrap gap-2"><span className={`rounded-full border px-3 py-1 text-xs ${apiState === 'live' ? 'border-emerald-400/25 bg-emerald-400/10 text-emerald-200' : 'border-amber-400/25 bg-amber-400/10 text-amber-200'}`}>{apiState === 'live' ? 'Live stream' : 'Preview mode'}</span><button onClick={refresh} className="inline-flex items-center gap-2 rounded-full border border-white/10 bg-white/5 px-3 py-1 text-xs text-neutral-200 transition-colors hover:bg-white/10 focus:outline-none focus:ring-2 focus:ring-smara-300/60"><RefreshCw className={`h-3.5 w-3.5 ${loading ? 'animate-spin' : ''}`} /> Refresh</button></div></div></header>
    <section className="grid gap-3 md:grid-cols-2 xl:grid-cols-5">{summaryCards.map(([label, value, Icon]) => <div key={label} className="rounded-2xl border border-neutral-800/80 bg-neutral-950/45 px-4 py-3"><div className="flex items-center justify-between gap-3"><div><div className="text-[10px] uppercase tracking-wide text-neutral-500">{label}</div><div className="mt-1 text-2xl font-semibold text-white">{String(value ?? 0)}</div></div><Icon className="h-4 w-4 text-neutral-500" /></div></div>)}</section>
    <section className="grid min-h-0 gap-4 lg:grid-cols-[320px_minmax(0,1fr)]"><aside className="space-y-4 lg:sticky lg:top-0 lg:self-start"><div className="rounded-3xl border border-neutral-800/80 bg-neutral-950/45 p-4"><div className="mb-3 flex items-center gap-2 text-white"><Clock3 className="h-4 w-4 text-smara-300" /> Current Run</div><div className="space-y-2 text-sm text-neutral-400"><div className="flex justify-between gap-4"><span>Status</span><span className="text-white">{snapshot.status || '-'}</span></div><div className="flex justify-between gap-4"><span>Run ID</span><span className="max-w-[170px] truncate text-white">{snapshot.run_id || '-'}</span></div><div className="flex justify-between gap-4"><span>Updated</span><span className="text-white">{snapshot.updated_at ? new Date(snapshot.updated_at).toLocaleTimeString() : '-'}</span></div></div><div className="mt-4 h-2 overflow-hidden rounded-full bg-neutral-800"><div className="h-full bg-gradient-to-r from-smara-400 to-lime-200" style={{ width: `${completion}%` }} /></div><div className="mt-1 text-right text-xs text-neutral-500">{completion}% complete</div></div><div className="rounded-3xl border border-neutral-800/80 bg-neutral-950/45 p-4"><div className="mb-3 flex items-center gap-2 text-white"><SlidersHorizontal className="h-4 w-4 text-smara-300" /> Config</div><label className="flex items-center justify-between gap-3 rounded-2xl bg-white/5 p-3 text-sm"><span>Enabled otomatis</span><input type="checkbox" checked={!!config.enabled} onChange={e => patchConfig({ enabled: e.target.checked })} /></label><label className="mt-2 flex items-center justify-between gap-3 rounded-2xl bg-white/5 p-3 text-sm"><span>Dry-run default</span><input type="checkbox" checked={!!config.dry_run} onChange={e => patchConfig({ dry_run: e.target.checked })} /></label><label className="mt-2 flex items-center justify-between gap-3 rounded-2xl bg-white/5 p-3 text-sm"><span>Serial fallback</span><input type="checkbox" checked={!!config.serial_fallback} onChange={e => patchConfig({ serial_fallback: e.target.checked })} /></label><label className="mt-2 flex items-center justify-between gap-3 rounded-2xl bg-white/5 p-3 text-sm"><span>Approval risk tinggi</span><input type="checkbox" checked={!!config.require_approval_high} onChange={e => patchConfig({ require_approval_high: e.target.checked })} /></label><label className="mt-2 flex items-center justify-between gap-3 rounded-2xl bg-white/5 p-3 text-sm"><span>Approval remote/VPS</span><input type="checkbox" checked={!!config.require_approval_remote} onChange={e => patchConfig({ require_approval_remote: e.target.checked })} /></label><label className="mt-2 block rounded-2xl bg-white/5 p-3 text-sm"><span className="text-neutral-400">Auto threshold</span><select className="mt-2 w-full rounded-xl border border-white/10 bg-neutral-950 px-3 py-2 text-sm text-white outline-none focus:border-smara-300" value={config.auto_threshold || 'complex'} onChange={e => patchConfig({ auto_threshold: e.target.value })}><option value="simple">Simple, lebih sering paralel</option><option value="complex">Complex, seimbang</option><option value="aggressive">Aggressive, paling agresif</option></select></label><label className="mt-2 block rounded-2xl bg-white/5 p-3 text-sm"><span className="text-neutral-400">Max concurrency</span><input className="mt-2 w-full accent-lime-300" type="range" min={1} max={12} value={config.max_concurrency || 1} onChange={e => patchConfig({ max_concurrency: Number(e.target.value) })} /><div className="text-right text-smara-200">{config.max_concurrency || 1} agents</div></label></div></aside><main className="min-w-0 space-y-4"><div className="rounded-3xl border border-neutral-800/80 bg-neutral-950/45 p-4"><div className="flex flex-wrap items-center justify-between gap-3"><div className="min-w-0"><div className="flex items-center gap-2 text-white"><GitBranch className="h-4 w-4 text-smara-300" /> <span className="truncate">{snapshot.title}</span></div><p className="mt-1 text-xs text-neutral-500">Wave tidak lagi berupa card terpisah. Semuanya menjadi node dalam graph di bawah.</p></div><span className={`rounded-full border px-3 py-1 text-xs ${statusTone(snapshot.status)}`}>{snapshot.status || 'idle'}</span></div></div><AgentNetwork snapshot={snapshot} /><div className="rounded-3xl border border-neutral-800/80 bg-neutral-950/45 p-4"><div className="mb-3 flex items-center gap-2 text-white">{snapshot.active ? <PlayCircle className="h-4 w-4 text-emerald-300" /> : <PauseCircle className="h-4 w-4 text-neutral-500" />} Final Report</div><pre className="max-h-64 overflow-auto whitespace-pre-wrap rounded-2xl bg-black/30 p-4 text-sm leading-6 text-neutral-300">{snapshot.report_markdown || 'Report akan muncul setelah agent orchestration selesai.'}</pre></div></main></section>
  </div></div>
}
