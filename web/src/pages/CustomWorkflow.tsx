import { useState, useEffect, useRef, useCallback, useMemo } from 'react'
import { FolderTree, Plus, Trash2, Save, Play, CheckCircle, AlertCircle, FolderOpen, BrainCircuit, MessageSquare, Bot, Database, Wrench, Repeat2 } from 'lucide-react'
import {
  ReactFlow,
  Background,
  Controls,
  MiniMap,
  Handle,
  Position,
  addEdge,
  applyNodeChanges,
  applyEdgeChanges,
  type Node,
  type Edge,
  type Connection,
  type NodeChange,
  type EdgeChange,
} from '@xyflow/react'
import '@xyflow/react/dist/style.css'
import { fetchJSON } from '../api'
import type { CustomWorkflowItem, CustomWorkflowAgent, CustomWorkflowTask, CustomWorkflowSummary, LoopMode, LoopNodeConfig } from '../api'
import FolderPicker from '../components/FolderPicker'

function defaultAgent(role = ''): CustomWorkflowAgent {
  return { role, description: '', skills: [], tasks: [{ id: 'main', description: '' }], depends_on: [], inputs_from: {} }
}
function defaultWorkflow(): CustomWorkflowItem {
  return {
    name: '',
    description: '',
    project_dir: '',
    agents: [
      { role: 'master', description: 'Node master dari sesi chat / prompt utama. Hubungkan ke agent lain untuk routing tugas.', skills: ['orchestrator'], tasks: [{ id: 'main', description: 'Terima prompt utama dan koordinasikan agent.' }], depends_on: [], inputs_from: {} },
    ],
  }
}


function professionalWebsiteTestingWorkflow(): CustomWorkflowItem {
  return {
    name: 'website-testing-profesional',
    description: 'Workflow end-to-end untuk testing website secara profesional: discovery, browser functional test, visual regression, accessibility, performance, security, SEO/content, dan executive report.',
    project_dir: '',
    agents: [
      { role: 'master', description: 'QA Test Manager yang mengorkestrasi seluruh proses testing website. Terima target URL, tujuan bisnis, credential opsional, dan batasan test; lalu distribusikan scope ke semua auditor.', skills: ['orchestrator', 'qa', 'browser-testing'], tasks: [{ id: 'intake', description: 'Kumpulkan target URL, environment, credential opsional, browser/device target, critical user journey, dan acceptance criteria. Jika data belum lengkap, buat asumsi eksplisit dan lanjut dengan scope aman.' }, { id: 'strategy', description: 'Susun test strategy ringkas: scope, risiko utama, prioritas halaman, dan definisi severity P0/P1/P2/P3.' }], depends_on: [], inputs_from: {} },
      { role: 'test-planner', description: 'QA Analyst yang mengubah scope menjadi test plan profesional dan checklist terstruktur.', skills: ['qa', 'test-plan'], tasks: [{ id: 'map-pages', description: 'Identifikasi halaman/fitur penting: homepage, navigasi, login/register jika ada, search/filter, detail page, form, checkout/contact, dashboard, dan error state.' }, { id: 'test-cases', description: 'Buat test cases dengan format: ID, Area, Precondition, Steps, Expected Result, Priority, dan Severity bila gagal.' }], depends_on: ['master'], inputs_from: { master: ['intake', 'strategy'] } },
      { role: 'browser-functional-tester', description: 'Browser automation tester. Jalankan real browser untuk membuka URL, klik navigasi/CTA, isi form aman, tunggu elemen, cek broken image/link, console error, dan ambil screenshot bukti.', skills: ['browser', 'web-test', 'qa'], tasks: [{ id: 'smoke-browser', description: 'Jalankan smoke test browser: buka target URL, tunggu halaman load, ambil screenshot awal, cek title/heading utama, dan catat console/network error yang terlihat.' }, { id: 'navigation-clicks', description: 'Klik menu/CTA utama satu per satu. Validasi URL berubah atau konten muncul sesuai expected. Ambil screenshot untuk state penting.' }, { id: 'media-and-links', description: 'Cek gambar yang tidak tampil, asset 404/403, link rusak, mixed content, dan iframe/video embed yang gagal.' }], depends_on: ['test-planner'], inputs_from: { 'test-planner': ['map-pages', 'test-cases'] } },
      { role: 'visual-ux-auditor', description: 'Visual QA dan UX auditor yang menilai layout, responsive behavior, konsistensi komponen, readability, empty/loading/error state, dan masalah visual dari screenshot.', skills: ['qa', 'ui', 'ux', 'visual-audit'], tasks: [{ id: 'visual-desktop', description: 'Audit tampilan desktop: alignment, spacing, overflow, kontras visual, gambar pecah, komponen tumpang tindih, dan konsistensi desain.' }, { id: 'responsive', description: 'Audit responsive untuk mobile/tablet secara konseptual atau via screenshot jika tersedia. Catat risiko layout dan rekomendasi breakpoint.' }], depends_on: ['browser-functional-tester'], inputs_from: { 'browser-functional-tester': ['smoke-browser', 'navigation-clicks', 'media-and-links'] } },
      { role: 'accessibility-auditor', description: 'Accessibility tester berbasis WCAG. Fokus keyboard navigation, semantic landmark, alt text, label form, focus state, color contrast, dan ARIA risk.', skills: ['qa', 'accessibility', 'wcag'], tasks: [{ id: 'a11y-checklist', description: 'Audit WCAG praktis: heading hierarchy, alt image, label input, keyboard/focus, contrast, link text, button name, dan aria misuse. Beri severity dan rekomendasi fix.' }], depends_on: ['browser-functional-tester'], inputs_from: { 'browser-functional-tester': ['smoke-browser', 'navigation-clicks', 'media-and-links'] } },
      { role: 'performance-auditor', description: 'Performance QA yang menilai loading, asset berat, render blocking, image optimization, cache, lazy loading, dan bottleneck frontend.', skills: ['qa', 'performance', 'web-vitals'], tasks: [{ id: 'perf-audit', description: 'Analisis risiko performance: ukuran gambar, third-party script, blocking resource, cache header, lazy loading, LCP/CLS/INP risk, dan rekomendasi optimasi prioritas.' }], depends_on: ['browser-functional-tester'], inputs_from: { 'browser-functional-tester': ['smoke-browser', 'media-and-links'] } },
      { role: 'security-auditor', description: 'Security sanity checker untuk website publik. Fokus header keamanan, mixed content, exposed sensitive data, unsafe form, CORS/basic auth risk, dan dependency/third-party risk secara non-intrusif.', skills: ['qa', 'security', 'web-security'], tasks: [{ id: 'security-sanity', description: 'Lakukan audit non-destruktif: HTTPS, mixed content, security headers, cookie flags jika terlihat, exposed tokens/secrets di HTML/JS, form tanpa proteksi dasar, dan risiko third-party asset.' }], depends_on: ['browser-functional-tester'], inputs_from: { 'browser-functional-tester': ['smoke-browser', 'media-and-links'] } },
      { role: 'seo-content-auditor', description: 'SEO dan content QA. Periksa title/meta, heading, canonical/robots risk, structured data bila ada, internal link, image alt, duplicate/empty content, dan clarity copy.', skills: ['qa', 'seo', 'content'], tasks: [{ id: 'seo-content', description: 'Audit SEO/content: title, meta description, H1/H2, canonical, robots, sitemap hint, OpenGraph, image alt, internal linking, dan kualitas copy utama.' }], depends_on: ['browser-functional-tester'], inputs_from: { 'browser-functional-tester': ['smoke-browser', 'navigation-clicks', 'media-and-links'] } },
      { role: 'report-writer', description: 'QA Lead yang mengompilasi semua hasil menjadi laporan profesional yang actionable.', skills: ['qa', 'reporting'], tasks: [{ id: 'final-report', description: 'Buat laporan final Markdown: Executive Summary, Test Scope, Environment, Evidence/Screenshot list, Findings table dengan Severity/Priority/Repro Steps/Expected/Actual/Recommendation, Quick Wins, dan Next Test Plan.' }, { id: 'release-verdict', description: 'Berikan verdict: PASS / PASS WITH ISSUES / FAIL, beserta alasan dan daftar blocking issues.' }], depends_on: ['visual-ux-auditor', 'accessibility-auditor', 'performance-auditor', 'security-auditor', 'seo-content-auditor'], inputs_from: { 'visual-ux-auditor': ['visual-desktop', 'responsive'], 'accessibility-auditor': ['a11y-checklist'], 'performance-auditor': ['perf-audit'], 'security-auditor': ['security-sanity'], 'seo-content-auditor': ['seo-content'] } },
    ],
  }
}

const DRAFT_KEY = 'smara_custom_workflow_draft'
const RUN_LOG_KEY = 'smara_custom_workflow_runlog'
const RUN_ERROR_KEY = 'smara_custom_workflow_runerror'
const RUN_SUCCESS_KEY = 'smara_custom_workflow_runsuccess'
const RUN_PHASES_KEY = 'smara_custom_workflow_runphases'
const SELECTED_KEY = 'smara_custom_workflow_selected'
const POS_KEY = 'smara_custom_workflow_node_positions'

const spinnerFrames = ['\u280B','\u2819','\u2839','\u2838','\u283C','\u2834','\u2826','\u2827','\u2807','\u280F']

type FlowData = {
  label: string
  role: string
  kind: 'master' | 'agent' | 'tool' | 'memory' | 'loop'
  description: string
  skills: string[]
  tasks: CustomWorkflowTask[]
  loop?: LoopNodeConfig
}
function WorkflowNode({ data }: { data: FlowData }) {
  const isMaster = data.kind === 'master'
  const isMemory = data.kind === 'memory'
  const isTool = data.kind === 'tool'
  const isLoop = data.kind === 'loop'
  const border = isMaster ? 'border-smara-300/50 bg-smara-950/75 shadow-smara-950/40' : isMemory ? 'border-emerald-300/40 bg-emerald-950/70 shadow-emerald-950/30' : isTool ? 'border-smara-300/35 bg-smara-950/70 shadow-smara-950/30' : isLoop ? 'border-amber-300/45 bg-amber-950/70 shadow-amber-950/30' : 'border-lime-300/25 bg-[#070905]/85 shadow-black/40'
  const header = isMaster ? 'bg-smara-400/15' : isMemory ? 'bg-emerald-400/15' : isTool ? 'bg-smara-400/15' : isLoop ? 'bg-amber-400/15' : 'bg-lime-400/10'
  return (
    <div className={`min-w-[210px] max-w-[260px] rounded-2xl border shadow-2xl backdrop-blur-xl overflow-hidden ${border}`}>
      <Handle type="target" position={Position.Left} className="!w-3 !h-3 !bg-smara-300 !border-gray-950" />
      <div className={`px-3 py-2 flex items-center gap-2 ${header}`}>
        {isMaster ? <MessageSquare className="w-4 h-4 text-smara-200" /> : isMemory ? <Database className="w-4 h-4 text-emerald-200" /> : isTool ? <Wrench className="w-4 h-4 text-smara-200" /> : isLoop ? <Repeat2 className="w-4 h-4 text-amber-200" /> : <Bot className="w-4 h-4 text-lime-200" />}
        <div className="text-xs font-semibold text-white truncate">{data.label}</div>
      </div>
      <div className="p-3 space-y-2">
        <div className="text-[10px] uppercase tracking-wider text-gray-500">{isMaster ? 'Master Chat Node' : isMemory ? 'Workspace Memory Node' : isTool ? 'Tool Node' : isLoop ? `Loop Node · ${data.loop?.mode || 'count'}` : 'Agent Node'}</div>
        <div className="text-[11px] text-gray-300 line-clamp-3">{data.description || 'Belum ada deskripsi/prompt.'}</div>
        <div className="flex flex-wrap gap-1">
          {(data.skills || []).slice(0, 3).map(s => <span key={s} className="text-[9px] px-1.5 py-0.5 rounded-full bg-white/10 text-gray-300">{s}</span>)}
          {data.loop?.max_iterations ? <span className="text-[9px] px-1.5 py-0.5 rounded-full bg-amber-500/20 text-amber-100">max {data.loop.max_iterations}x</span> : null}
          {data.tasks?.length ? <span className="text-[9px] px-1.5 py-0.5 rounded-full bg-smara-500/20 text-smara-200">{data.tasks.length} task</span> : null}
        </div>
      </div>
      <Handle type="source" position={Position.Right} className="!w-3 !h-3 !bg-lime-300 !border-gray-950" />
    </div>
  )
}

const nodeTypes = { workflowNode: WorkflowNode }

export default function CustomWorkflow() {
  const [workflows, setWorkflows] = useState<CustomWorkflowSummary[]>([])
  const [selected, setSelected] = useState<CustomWorkflowItem | null>(null)
  const [editing, setEditing] = useState<CustomWorkflowItem | null>(null)
  const [loading, setLoading] = useState(false)
  const [saving, setSaving] = useState(false)
  const [running, setRunning] = useState(false)
  const [runLog, setRunLog] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [success, setSuccess] = useState<string | null>(null)
  const [folderPickerOpen, setFolderPickerOpen] = useState(false)
  const [phases, setPhases] = useState<Array<{phase:string;description:string;status:'running'|'done'}>>([])
  const [spinnerIdx, setSpinnerIdx] = useState(0)
  const [nodes, setNodes] = useState<Node<FlowData>[]>([])
  const [edges, setEdges] = useState<Edge[]>([])
  const [activeNode, setActiveNode] = useState<string | null>(null)
  const [positions, setPositions] = useState<Record<string, {x:number;y:number}>>({})
  const timer = useRef<ReturnType<typeof setInterval>|null>(null)
  const fileRef = useRef<HTMLInputElement | null>(null)
  const [importName, setImportName] = useState('')
  const [importMode, setImportMode] = useState<'merge' | 'replace'>('merge')

  const load = async () => {
    setLoading(true)
    try {
      const data = await fetchJSON<{ workflows: CustomWorkflowSummary[] }>('/api/custom-workflow/list')
      setWorkflows(data.workflows || [])
    } catch (e: any) { setError('Gagal load workflows: ' + (e.message || e)) }
    finally { setLoading(false) }
  }

  useEffect(() => {
    const saved = localStorage.getItem(DRAFT_KEY)
    if (saved) { try { setEditing(JSON.parse(saved)) } catch {} }
    const pos = localStorage.getItem(POS_KEY)
    if (pos) { try { setPositions(JSON.parse(pos)) } catch {} }
    const savedSelected = localStorage.getItem(SELECTED_KEY)
    if (savedSelected) selectWorkflow(savedSelected)
    const savedRunLog = localStorage.getItem(RUN_LOG_KEY); if (savedRunLog) setRunLog(savedRunLog)
    const savedError = localStorage.getItem(RUN_ERROR_KEY); if (savedError) setError(savedError)
    const savedSuccess = localStorage.getItem(RUN_SUCCESS_KEY); if (savedSuccess) setSuccess(savedSuccess)
    const savedPhases = localStorage.getItem(RUN_PHASES_KEY)
    if (savedPhases) { try { setPhases(JSON.parse(savedPhases).map((p: any) => ({ ...p, status: 'done' }))) } catch {} }
  }, [])

  useEffect(() => { load() }, [])
  useEffect(() => { if (editing) localStorage.setItem(DRAFT_KEY, JSON.stringify(editing)) }, [editing])
  useEffect(() => { localStorage.setItem(POS_KEY, JSON.stringify(positions)) }, [positions])
  useEffect(() => { runLog ? localStorage.setItem(RUN_LOG_KEY, runLog) : localStorage.removeItem(RUN_LOG_KEY) }, [runLog])
  useEffect(() => { error ? localStorage.setItem(RUN_ERROR_KEY, error) : localStorage.removeItem(RUN_ERROR_KEY) }, [error])
  useEffect(() => { success ? localStorage.setItem(RUN_SUCCESS_KEY, success) : localStorage.removeItem(RUN_SUCCESS_KEY) }, [success])
  useEffect(() => { phases.length ? localStorage.setItem(RUN_PHASES_KEY, JSON.stringify(phases)) : localStorage.removeItem(RUN_PHASES_KEY) }, [phases])
  useEffect(() => { selected?.name ? localStorage.setItem(SELECTED_KEY, selected.name) : localStorage.removeItem(SELECTED_KEY) }, [selected])
  useEffect(() => {
    if (running) timer.current = setInterval(() => setSpinnerIdx(i => (i+1)%spinnerFrames.length), 80)
    else if (timer.current) { clearInterval(timer.current); timer.current = null }
    return () => { if (timer.current) clearInterval(timer.current) }
  }, [running])

  const roleId = (role: string, idx: number) => role?.trim() || `agent-${idx + 1}`

  const syncGraphFromEditing = useCallback((wf: CustomWorkflowItem | null, posOverride?: Record<string, {x:number;y:number}>) => {
    if (!wf) { setNodes([]); setEdges([]); return }
    const pos = posOverride || positions
    const ns: Node<FlowData>[] = wf.agents.map((a, i) => {
      const id = roleId(a.role, i)
      const isMaster = a.role.toLowerCase() === 'master' || i === 0
      const isMemory = (a.skills || []).some(s => s.toLowerCase() === 'memory') || a.role.toLowerCase().startsWith('memory') || !!a.memory
      const isTool = (a.skills || []).some(s => s.toLowerCase() === 'tool') || a.role.toLowerCase().startsWith('tool')
      const isLoop = (a.skills || []).some(s => s.toLowerCase() === 'loop') || a.role.toLowerCase().startsWith('loop') || !!a.loop
      return {
        id,
        type: 'workflowNode',
        position: pos[id] || { x: i === 0 ? 40 : 360 + ((i - 1) % 3) * 290, y: i === 0 ? 170 : 60 + Math.floor((i - 1) / 3) * 210 },
        data: { label: a.role || id, role: a.role || id, kind: isMaster ? 'master' : isMemory ? 'memory' : isTool ? 'tool' : isLoop ? 'loop' : 'agent', description: a.description, skills: a.skills || [], tasks: a.tasks || [], loop: a.loop },
      }
    })
    const roleSet = new Set(wf.agents.map((a, i) => roleId(a.role, i)))
    const es: Edge[] = []
    wf.agents.forEach((a, i) => {
      const target = roleId(a.role, i)
      ;(a.depends_on || []).forEach(dep => {
        if (roleSet.has(dep) && dep !== target) {
          es.push({ id: `${dep}->${target}`, source: dep, target, animated: dep === 'master', style: { stroke: dep === 'master' ? '#22d3ee' : '#c084fc', strokeWidth: 2 } })
        }
      })
    })
    setNodes(ns); setEdges(es)
  }, [positions])

  useEffect(() => { syncGraphFromEditing(editing) }, [editing, syncGraphFromEditing])

  const selectWorkflow = async (name: string) => {
    try {
      const data = await fetchJSON<CustomWorkflowItem>(`/api/custom-workflow/get?name=${encodeURIComponent(name)}`)
      setSelected(data); setEditing(JSON.parse(JSON.stringify(data))); setError(null); setSuccess(null); setRunLog(null); setPhases([])
    } catch (e: any) { setError('Gagal load workflow: ' + (e.message || e)) }
  }

  const save = async () => {
    if (!editing) return
    if (!editing.name.trim()) { setError('Nama workflow wajib diisi'); return }
    setSaving(true); setError(null); setSuccess(null)
    try {
      await fetchJSON('/api/custom-workflow/save', { method: 'POST', body: JSON.stringify(editing) })
      setSuccess('Workflow disimpan!'); load()
      if (selected?.name === editing.name) setSelected(JSON.parse(JSON.stringify(editing)))
    } catch (e: any) { setError('Gagal simpan: ' + (e.message || e)) }
    finally { setSaving(false) }
  }

  const del = async (name: string) => {
    if (!confirm(`Hapus workflow '${name}'?`)) return
    setError(null)
    try {
      await fetchJSON('/api/custom-workflow/delete', { method: 'POST', body: JSON.stringify({ name }) })
      if (selected?.name === name) { setSelected(null); setEditing(null) }
      load()
    } catch (e: any) { setError('Gagal hapus: ' + (e.message || e)) }
  }

  const addPhase = (phase: string, description: string) => {
    setPhases(prev => {
      const next: Array<{ phase: string; description: string; status: 'running' | 'done' }> = prev.map(p => ({ ...p, status: 'done' as const }))
      const idx = next.findIndex(p => p.phase === phase)
      if (idx >= 0) next[idx] = { phase, description, status: 'running' }
      else next.push({ phase, description, status: 'running' })
      return next
    })
  }

  const run = async () => {
    if (!editing) return
    setRunning(true); setError(null); setSuccess(null); setRunLog(null); setPhases([])
    const steps = [{ phase:'prepare', desc:'Mempersiapkan node master & agent...' }, { phase:'execute', desc:'Menjalankan koneksi graph workflow...' }, { phase:'compile', desc:'Mengompilasi hasil agent...' }, { phase:'finalize', desc:'Menyelesaikan workflow...' }]
    let stepIdx = 0; addPhase(steps[0].phase, steps[0].desc)
    const stepTimer = setInterval(() => { stepIdx++; if (stepIdx < steps.length) addPhase(steps[stepIdx].phase, steps[stepIdx].desc) }, 2000)
    try {
      const result = await fetchJSON('/api/custom-workflow/run', { method: 'POST', body: JSON.stringify({ name: editing.name, project_dir: editing.project_dir || undefined }) })
      setSuccess('Workflow berhasil dijalankan!'); setRunLog(JSON.stringify(result, null, 2)); addPhase('done', 'Workflow selesai!')
    } catch (e: any) { setError('Gagal run: ' + (e.message || e)) }
    finally { clearInterval(stepTimer); setRunning(false) }
  }

  const updateAgent = (idx: number, patch: Partial<CustomWorkflowAgent>) => {
    if (!editing) return
    const next = { ...editing, agents: [...editing.agents] }
    next.agents[idx] = { ...next.agents[idx], ...patch }
    setEditing(next)
  }

  const defaultLoopConfig = (): LoopNodeConfig => ({ mode: 'count', max_iterations: 3, delay_ms: 1000, timeout_ms: 0, condition: '{{lastResult.status}} == "ready"', items_source: '{{input.items}}', retry: { max_attempts: 3, initial_delay_ms: 1000, multiplier: 2, max_delay_ms: 30000 }, on_error: 'stop' })

  const addAgent = (kind: 'agent' | 'tool' | 'memory' | 'loop' = 'agent') => {
    if (!editing) return
    const count = editing.agents.length
    const role = kind === 'agent' ? `agent-${count}` : `${kind}-${count}`
    const desc = kind === 'tool' ? 'Tool node untuk menjalankan aksi eksternal.' : kind === 'memory' ? 'Memory node untuk baca/tulis/search Workspace Memory Store.' : kind === 'loop' ? 'Loop node untuk mengulang child/agent berikutnya dengan pola count, condition, foreach, interval, retry, atau infinite guarded.' : ''
    const memory = kind === 'memory' ? { action: 'read' as const, limit: 5 } : undefined
    const loop = kind === 'loop' ? defaultLoopConfig() : undefined
    setEditing({ ...editing, agents: [...editing.agents, { ...defaultAgent(role), description: desc, skills: kind === 'tool' ? ['tool'] : kind === 'memory' ? ['memory'] : kind === 'loop' ? ['loop'] : [], memory, loop }] })
  }
  const removeAgent = (idx: number) => {
    if (!editing || idx === 0) return
    const removed = roleId(editing.agents[idx].role, idx)
    const nextAgents = editing.agents.filter((_, i) => i !== idx).map(a => ({ ...a, depends_on: (a.depends_on || []).filter(d => d !== removed) }))
    setEditing({ ...editing, agents: nextAgents })
    setActiveNode(null)
  }

  const updateTask = (agentIdx: number, taskIdx: number, patch: Partial<CustomWorkflowTask>) => {
    if (!editing) return
    const next = { ...editing, agents: [...editing.agents] }
    const tasks = [...next.agents[agentIdx].tasks]
    tasks[taskIdx] = { ...tasks[taskIdx], ...patch }
    next.agents[agentIdx] = { ...next.agents[agentIdx], tasks }
    setEditing(next)
  }
  const addTask = (agentIdx: number) => {
    if (!editing) return
    const next = { ...editing, agents: [...editing.agents] }
    const tasks = [...next.agents[agentIdx].tasks, { id: `t${next.agents[agentIdx].tasks.length+1}`, description: '' }]
    next.agents[agentIdx] = { ...next.agents[agentIdx], tasks }
    setEditing(next)
  }
  const removeTask = (agentIdx: number, taskIdx: number) => {
    if (!editing) return
    const next = { ...editing, agents: [...editing.agents] }
    const tasks = [...next.agents[agentIdx].tasks]; tasks.splice(taskIdx, 1)
    next.agents[agentIdx] = { ...next.agents[agentIdx], tasks }
    setEditing(next)
  }

  const onNodesChange = useCallback((changes: NodeChange[]) => {
    setNodes(nds => applyNodeChanges(changes, nds) as Node<FlowData>[])
    const dragStops = changes.filter(c => c.type === 'position' && 'position' in c && c.position)
    if (dragStops.length) {
      setPositions(prev => {
        const next = { ...prev }
        dragStops.forEach((c: any) => { next[c.id] = c.position })
        return next
      })
    }
  }, [])
  const onEdgesChange = useCallback((changes: EdgeChange[]) => setEdges(eds => applyEdgeChanges(changes, eds)), [])
  const onConnect = useCallback((conn: Connection) => {
    if (!conn.source || !conn.target || conn.source === conn.target || !editing) return
    setEdges(eds => addEdge({ ...conn, animated: conn.source === 'master', style: { stroke: conn.source === 'master' ? '#22d3ee' : '#c084fc', strokeWidth: 2 } }, eds))
    setEditing(prev => {
      if (!prev) return prev
      return { ...prev, agents: prev.agents.map((a, i) => roleId(a.role, i) === conn.target ? { ...a, depends_on: Array.from(new Set([...(a.depends_on || []), conn.source!])) } : a) }
    })
  }, [editing])

  const onDeleteEdges = useCallback((deleted: Edge[]) => {
    setEditing(prev => {
      if (!prev) return prev
      return { ...prev, agents: prev.agents.map((a, i) => {
        const id = roleId(a.role, i)
        const removeSources = deleted.filter(e => e.target === id).map(e => e.source)
        return removeSources.length ? { ...a, depends_on: (a.depends_on || []).filter(d => !removeSources.includes(d)) } : a
      }) }
    })
  }, [])

  const activeAgentIndex = useMemo(() => editing?.agents.findIndex((a, i) => roleId(a.role, i) === activeNode) ?? -1, [editing, activeNode])
  const strArrInput = (value: string[]) => value.join(', ')
  const parseStrArr = (value: string) => value.split(',').map(s => s.trim()).filter(Boolean)
  const loopModes: Array<{ value: LoopMode; label: string }> = [
    { value: 'count', label: 'Repeat by count' },
    { value: 'until_success', label: 'Repeat until success' },
    { value: 'until_condition', label: 'Repeat until condition' },
    { value: 'while_condition', label: 'While condition' },
    { value: 'for_each', label: 'For each item' },
    { value: 'interval', label: 'Interval loop' },
    { value: 'retry_backoff', label: 'Retry with backoff' },
    { value: 'infinite_guarded', label: 'Infinite guarded' },
  ]
  const isLoopAgent = (agent: CustomWorkflowAgent) => (agent.skills || []).some(s => s.toLowerCase() === 'loop') || !!agent.loop || agent.role.toLowerCase().startsWith('loop')
  const updateLoop = (idx: number, patch: Partial<LoopNodeConfig>) => updateAgent(idx, { loop: { ...(editing?.agents[idx].loop || defaultLoopConfig()), ...patch } })
  const setLoopMode = (idx: number, mode: LoopMode) => updateLoop(idx, { mode })
  const handleImportFile = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0]; if (!file) return
    const text = await file.text()
    const selectedName = selected?.name?.trim() || ''
    const fileName = file.name.replace(/\.json$/i, '')
    const name = importName.trim() || (importMode === 'merge' ? selectedName : fileName)
    if (!name) { setError('Pilih workflow target dulu, atau isi Target workflow untuk merge'); return }
    try {
      const result = await fetchJSON<{ status: string; name: string; agents?: number; merged?: boolean }>('/api/custom-workflow/import', { method: 'POST', body: JSON.stringify({ name, json: text, mode: importMode }) })
      setSuccess(result.merged ? `Workflow '${name}' berhasil ditambahkan (${result.agents || 0} agent).` : `Workflow '${name}' berhasil di-import!`)
      setImportName('')
      await load()
      await selectWorkflow(name)
    } catch (err: any) { setError('Gagal import: ' + (err.message || err)) }
    if (fileRef.current) fileRef.current.value = ''
  }

  return (
    <div className="flex flex-col h-full p-4 overflow-hidden">
      <div className="flex items-center gap-2 mb-3">
        <FolderTree className="w-5 h-5 text-smara-400" />
        <h2 className="text-lg font-medium">Custom Workflow Node Builder</h2>
        <span className="text-xs text-gray-500">drag garis dari handle kanan ke handle kiri node lain</span>
      </div>
      <div className="flex gap-2 mb-3">
        <button onClick={() => { setSelected(null); setEditing(defaultWorkflow()); setError(null); setSuccess(null); setActiveNode('master') }} className="px-3 py-2 bg-smara-500 hover:bg-smara-400 text-black rounded-lg transition-colors flex items-center gap-1 text-sm"><Plus className="w-4 h-4" /> Baru</button>
        <button onClick={() => { setSelected(null); setEditing(professionalWebsiteTestingWorkflow()); setError(null); setSuccess('Template Website Testing Profesional siap. Isi target URL/scope di node master, lalu Simpan dan Jalankan.'); setActiveNode('master') }} className="px-3 py-2 bg-amber-700 hover:bg-amber-600 rounded-lg transition-colors flex items-center gap-1 text-sm"><CheckCircle className="w-4 h-4" /> Template Website Test</button>
        <button onClick={load} disabled={loading} className="px-3 py-2 bg-gray-700 hover:bg-gray-600 rounded-lg transition-colors text-sm">{loading ? 'Loading...' : 'Refresh'}</button>
        {editing && <><button onClick={() => addAgent('agent')} className="px-3 py-2 bg-lime-700 hover:bg-lime-600 rounded-lg transition-colors flex items-center gap-1 text-sm"><Bot className="w-4 h-4" /> Agent</button><button onClick={() => addAgent('tool')} className="px-3 py-2 bg-smara-500 hover:bg-smara-400 text-black rounded-lg transition-colors flex items-center gap-1 text-sm"><Wrench className="w-4 h-4" /> Tool</button><button onClick={() => addAgent('memory')} className="px-3 py-2 bg-emerald-700 hover:bg-emerald-600 rounded-lg transition-colors flex items-center gap-1 text-sm"><Database className="w-4 h-4" /> Memory</button><button onClick={() => addAgent('loop')} className="px-3 py-2 bg-amber-700 hover:bg-amber-600 rounded-lg transition-colors flex items-center gap-1 text-sm"><Repeat2 className="w-4 h-4" /> Loop</button></>}
        <div className="flex items-center gap-2 ml-auto"><input value={importName} onChange={e => setImportName(e.target.value)} placeholder={importMode === 'merge' ? (selected?.name ? `Target: ${selected.name}` : 'Pilih target...') : 'Nama workflow...'} className="bg-neutral-950/55 ring-1 ring-black/35 rounded-lg px-2 py-1.5 text-xs focus:outline-none focus:ring-2 focus:ring-smara-300/25 w-36" /><select value={importMode} onChange={e => setImportMode(e.target.value as 'merge' | 'replace')} className="bg-neutral-950/55 ring-1 ring-black/35 rounded-lg px-2 py-1.5 text-xs focus:outline-none focus:ring-2 focus:ring-smara-300/25"><option value="merge">Merge</option><option value="replace">Replace</option></select><input ref={fileRef} type="file" accept=".json" className="hidden" onChange={handleImportFile} /><button onClick={() => fileRef.current?.click()} className="px-3 py-2 bg-smara-500 hover:bg-smara-400 text-black rounded-lg transition-colors text-xs">Import JSON</button></div>
      </div>

      {error && <div className="mb-3 p-3 bg-red-900/20 border border-red-800 rounded-lg text-sm text-red-200 flex items-center gap-2"><AlertCircle className="w-4 h-4 shrink-0" /><span>{error}</span></div>}
      {success && <div className="mb-3 p-3 bg-green-900/20 border border-green-800 rounded-lg text-sm text-green-200 flex items-center gap-2"><CheckCircle className="w-4 h-4 shrink-0" /><span>{success}</span></div>}
      {(phases.length > 0 || running) && <div className="mb-3 bg-[#0c1009]/82 ring-1 ring-black/40 rounded-lg p-3 space-y-1.5"><div className="flex items-center gap-2 text-[10px] text-gray-500 uppercase tracking-wider font-medium"><BrainCircuit className="w-3 h-3" /> Proses Berjalan</div>{phases.map((ph, idx) => <div key={ph.phase + idx} className="flex items-center gap-2 text-xs">{ph.status === 'running' ? <span className="text-smara-400 font-mono w-4 text-center">{spinnerFrames[spinnerIdx]}</span> : <CheckCircle className="w-3.5 h-3.5 text-green-400 shrink-0" />}<span className={ph.status === 'running' ? 'text-gray-200 font-medium' : 'text-gray-500'}>{ph.description || ph.phase}</span></div>)}</div>}

      <div className="flex gap-4 flex-1 min-h-0 overflow-hidden">
        <div className="w-56 shrink-0 bg-[#0f1a0f]/60 ring-1 ring-black/35 rounded-lg overflow-y-auto">
          <div className="p-2 text-[10px] text-gray-500 uppercase tracking-wider font-medium border-b border-transparent">Workflows ({workflows.length})</div>
          {workflows.map(w => <div key={w.name} className="flex items-center justify-between p-2 border-b border-transparent"><button onClick={() => selectWorkflow(w.name)} className={`text-left text-xs transition-colors flex-1 ${selected?.name === w.name ? 'text-gray-200 font-medium' : 'text-gray-400 hover:text-gray-200'}`}><div className="truncate">{w.name}</div><div className="text-[10px] text-gray-500">{w.agents} agent</div></button><button onClick={() => del(w.name)} className="text-gray-500 hover:text-red-400 p-1"><Trash2 className="w-3 h-3" /></button></div>)}
        </div>

        <div className="flex-1 min-w-0 flex flex-col gap-3">
          {editing ? <>
            <div className="grid grid-cols-3 gap-2 bg-[#0f1a0f]/60 ring-1 ring-black/35 rounded-lg p-3">
              <input value={editing.name} onChange={e => setEditing({ ...editing, name: e.target.value })} className="bg-neutral-950/55 ring-1 ring-black/35 rounded-lg px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-smara-300/25" placeholder="Nama workflow" />
              <input value={editing.description} onChange={e => setEditing({ ...editing, description: e.target.value })} className="bg-neutral-950/55 ring-1 ring-black/35 rounded-lg px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-smara-300/25" placeholder="Deskripsi workflow" />
              <div className="flex gap-2"><input value={editing.project_dir || ''} onChange={e => setEditing({ ...editing, project_dir: e.target.value })} className="flex-1 bg-neutral-950/55 ring-1 ring-black/35 rounded-lg px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-smara-300/25" placeholder="Project dir" /><button onClick={() => setFolderPickerOpen(true)} className="px-3 py-2 bg-gray-700 hover:bg-gray-600 rounded-lg"><FolderOpen className="w-4 h-4" /></button></div>
            </div>
            <div className="flex-1 min-h-0 ring-1 ring-black/35 rounded-lg overflow-hidden bg-[#0a0a0a]/55">
              <ReactFlow nodeTypes={nodeTypes} nodes={nodes} edges={edges} onNodesChange={onNodesChange} onEdgesChange={onEdgesChange} onConnect={onConnect} onEdgesDelete={onDeleteEdges} onNodeClick={(_, node) => setActiveNode(node.id)} fitView>
                <Background gap={18} size={1} color="#374151" />
                <Controls />
                <MiniMap nodeColor={(n) => (n.data as FlowData)?.kind === 'master' ? '#0891b2' : '#a21caf'} maskColor="rgba(0,0,0,0.45)" />
              </ReactFlow>
            </div>
            <div className="flex items-center gap-2"><button onClick={save} disabled={saving} className="px-4 py-2 bg-smara-500 hover:bg-smara-400 text-black disabled:opacity-50 rounded-lg transition-colors flex items-center gap-1 text-sm"><Save className="w-4 h-4" /> {saving ? 'Menyimpan...' : 'Simpan'}</button><button onClick={run} disabled={running || saving} className="px-4 py-2 bg-green-700 hover:bg-green-600 disabled:opacity-50 rounded-lg transition-colors flex items-center gap-1 text-sm"><Play className="w-4 h-4" /> {running ? 'Menjalankan...' : 'Jalankan'}</button><button onClick={() => { localStorage.removeItem(DRAFT_KEY); localStorage.removeItem(RUN_LOG_KEY); localStorage.removeItem(RUN_ERROR_KEY); localStorage.removeItem(RUN_SUCCESS_KEY); localStorage.removeItem(RUN_PHASES_KEY); localStorage.removeItem(SELECTED_KEY); setEditing(defaultWorkflow()); setSelected(null); setError(null); setSuccess(null); setRunLog(null); setPhases([]); setActiveNode('master') }} className="px-3 py-2 bg-gray-700 hover:bg-gray-600 rounded-lg transition-colors text-xs ml-auto">Clear Draft</button></div>
            {runLog && <div className="bg-[#070905] ring-1 ring-black/35 rounded-lg p-3"><div className="text-xs text-gray-500 mb-1 font-medium">Hasil Eksekusi</div><pre className="text-[11px] text-gray-300 font-mono whitespace-pre-wrap overflow-y-auto max-h-36">{runLog}</pre></div>}
          </> : <div className="text-gray-500 text-sm">Pilih workflow dari sidebar atau buat baru.</div>}
        </div>

        <div className="w-80 shrink-0 bg-[#0f1a0f]/60 ring-1 ring-black/35 rounded-lg p-3 overflow-y-auto">
          <div className="text-xs font-medium text-gray-300 mb-3">Node Inspector</div>
          {editing && activeAgentIndex >= 0 ? (() => { const agent = editing.agents[activeAgentIndex]; return <div className="space-y-3"><div className="flex items-center justify-between"><span className="text-[10px] text-gray-500 uppercase">{activeAgentIndex === 0 ? 'Master Node' : 'Agent Node'}</span>{activeAgentIndex > 0 && <button onClick={() => removeAgent(activeAgentIndex)} className="text-gray-500 hover:text-red-400"><Trash2 className="w-4 h-4" /></button>}</div><input value={agent.role} onChange={e => updateAgent(activeAgentIndex, { role: e.target.value })} placeholder="Role / ID node" className="w-full bg-neutral-950/55 ring-1 ring-black/35 rounded-lg px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-smara-300/25" /><textarea value={agent.description} onChange={e => updateAgent(activeAgentIndex, { description: e.target.value })} placeholder="Prompt / deskripsi node" rows={4} className="w-full bg-neutral-950/55 ring-1 ring-black/35 rounded-lg px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-smara-300/25" /><input value={strArrInput(agent.skills || [])} onChange={e => updateAgent(activeAgentIndex, { skills: parseStrArr(e.target.value) })} placeholder="Skills, pisah koma" className="w-full bg-neutral-950/55 ring-1 ring-black/35 rounded-lg px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-smara-300/25" /><input value={strArrInput(agent.depends_on || [])} onChange={e => updateAgent(activeAgentIndex, { depends_on: parseStrArr(e.target.value) })} placeholder="Depends on / koneksi masuk" className="w-full bg-neutral-950/55 ring-1 ring-black/35 rounded-lg px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-smara-300/25" />{isLoopAgent(agent) && <div className="space-y-2 bg-amber-950/20 border border-amber-800/40 rounded-lg p-2"><div className="text-[10px] text-amber-300 uppercase tracking-wider">Loop Pattern</div><select value={agent.loop?.mode || 'count'} onChange={e => setLoopMode(activeAgentIndex, e.target.value as LoopMode)} className="w-full bg-neutral-950/55 ring-1 ring-black/35 rounded px-2 py-1 text-xs">{loopModes.map(m => <option key={m.value} value={m.value}>{m.label}</option>)}</select><div className="grid grid-cols-2 gap-2"><input type="number" min={0} value={agent.loop?.max_iterations || 0} onChange={e => updateLoop(activeAgentIndex, { max_iterations: Number(e.target.value) || 0 })} placeholder="Max iter" className="bg-neutral-950/55 ring-1 ring-black/35 rounded px-2 py-1 text-xs" /><input type="number" min={0} value={agent.loop?.delay_ms || 0} onChange={e => updateLoop(activeAgentIndex, { delay_ms: Number(e.target.value) || 0 })} placeholder="Delay ms" className="bg-neutral-950/55 ring-1 ring-black/35 rounded px-2 py-1 text-xs" /></div><input type="number" min={0} value={agent.loop?.timeout_ms || 0} onChange={e => updateLoop(activeAgentIndex, { timeout_ms: Number(e.target.value) || 0 })} placeholder="Timeout ms" className="w-full bg-neutral-950/55 ring-1 ring-black/35 rounded px-2 py-1 text-xs" />{['until_condition','while_condition'].includes(agent.loop?.mode || '') && <input value={agent.loop?.condition || ''} onChange={e => updateLoop(activeAgentIndex, { condition: e.target.value })} placeholder={'Condition, contoh: {{lastResult.status}} == "ready"'} className="w-full bg-neutral-950/55 ring-1 ring-black/35 rounded px-2 py-1 text-xs" />}{agent.loop?.mode === 'for_each' && <input value={agent.loop?.items_source || ''} onChange={e => updateLoop(activeAgentIndex, { items_source: e.target.value })} placeholder="Items source, contoh: {{input.files}}" className="w-full bg-neutral-950/55 ring-1 ring-black/35 rounded px-2 py-1 text-xs" />}{agent.loop?.mode === 'retry_backoff' && <div className="grid grid-cols-2 gap-2"><input type="number" min={1} value={agent.loop?.retry?.max_attempts || 3} onChange={e => updateLoop(activeAgentIndex, { retry: { ...(agent.loop?.retry || {}), max_attempts: Number(e.target.value) || 1 } })} placeholder="Attempts" className="bg-neutral-950/55 ring-1 ring-black/35 rounded px-2 py-1 text-xs" /><input type="number" min={0} value={agent.loop?.retry?.initial_delay_ms || 1000} onChange={e => updateLoop(activeAgentIndex, { retry: { ...(agent.loop?.retry || {}), initial_delay_ms: Number(e.target.value) || 0 } })} placeholder="Initial delay" className="bg-neutral-950/55 ring-1 ring-black/35 rounded px-2 py-1 text-xs" /><input type="number" min={1} step="0.1" value={agent.loop?.retry?.multiplier || 2} onChange={e => updateLoop(activeAgentIndex, { retry: { ...(agent.loop?.retry || {}), multiplier: Number(e.target.value) || 1 } })} placeholder="Multiplier" className="bg-neutral-950/55 ring-1 ring-black/35 rounded px-2 py-1 text-xs" /><input type="number" min={0} value={agent.loop?.retry?.max_delay_ms || 30000} onChange={e => updateLoop(activeAgentIndex, { retry: { ...(agent.loop?.retry || {}), max_delay_ms: Number(e.target.value) || 0 } })} placeholder="Max delay" className="bg-neutral-950/55 ring-1 ring-black/35 rounded px-2 py-1 text-xs" /></div>}<select value={agent.loop?.on_error || 'stop'} onChange={e => updateLoop(activeAgentIndex, { on_error: e.target.value as any })} className="w-full bg-neutral-950/55 ring-1 ring-black/35 rounded px-2 py-1 text-xs"><option value="stop">on error: stop</option><option value="continue">on error: continue</option><option value="retry">on error: retry</option><option value="skip">on error: skip</option></select><div className="text-[10px] text-amber-100/70">Safety: gunakan max iterations/timeout/delay agar loop tidak berjalan tanpa batas.</div></div>}{((agent.skills || []).some(s => s.toLowerCase() === 'memory') || !!agent.memory) && <div className="space-y-2 bg-emerald-950/20 border border-emerald-800/40 rounded-lg p-2"><div className="text-[10px] text-emerald-300 uppercase tracking-wider">Workspace Memory</div><select value={agent.memory?.action || 'shared'} onChange={e => updateAgent(activeAgentIndex, { memory: { ...(agent.memory || {}), action: e.target.value as any } })} className="w-full bg-neutral-950/55 ring-1 ring-black/35 rounded px-2 py-1 text-xs"><option value="shared">shared workflow only</option><option value="read">read recent workspace memory</option><option value="search">search workspace memory</option><option value="write">write/remember to workspace memory</option><option value="read_write">read + write</option></select><input value={agent.memory?.query || ''} onChange={e => updateAgent(activeAgentIndex, { memory: { ...(agent.memory || {}), query: e.target.value } })} placeholder="Query untuk search/read context" className="w-full bg-neutral-950/55 ring-1 ring-black/35 rounded px-2 py-1 text-xs" /><textarea value={agent.memory?.content || ''} onChange={e => updateAgent(activeAgentIndex, { memory: { ...(agent.memory || {}), content: e.target.value } })} placeholder="Content eksplisit untuk write/remember" rows={2} className="w-full bg-neutral-950/55 ring-1 ring-black/35 rounded px-2 py-1 text-xs" /><input type="number" min={1} max={20} value={agent.memory?.limit || 5} onChange={e => updateAgent(activeAgentIndex, { memory: { ...(agent.memory || {}), limit: Number(e.target.value) || 5 } })} className="w-24 bg-neutral-950/55 ring-1 ring-black/35 rounded px-2 py-1 text-xs" /></div>}<div className="space-y-2"><div className="flex items-center justify-between"><span className="text-xs text-gray-500">Tasks</span><button onClick={() => addTask(activeAgentIndex)} className="text-[10px] px-2 py-1 bg-neutral-950/55 hover:bg-gray-700 rounded flex items-center gap-1"><Plus className="w-3 h-3" /> Task</button></div>{agent.tasks.map((task, tIdx) => <div key={tIdx} className="space-y-1 bg-slate-950/20 rounded-lg p-2 border border-neutral-800/50"><div className="flex gap-1"><input value={task.id} onChange={e => updateTask(activeAgentIndex, tIdx, { id: e.target.value })} placeholder="ID" className="w-20 bg-neutral-950/55 ring-1 ring-black/35 rounded px-2 py-1 text-xs" />{agent.tasks.length > 1 && <button onClick={() => removeTask(activeAgentIndex, tIdx)} className="text-gray-500 hover:text-red-400 px-1"><Trash2 className="w-3 h-3" /></button>}</div><textarea value={task.description} onChange={e => updateTask(activeAgentIndex, tIdx, { description: e.target.value })} placeholder="Deskripsi task" rows={2} className="w-full bg-neutral-950/55 ring-1 ring-black/35 rounded px-2 py-1 text-xs" /></div>)}</div></div> })() : <div className="text-xs text-gray-500">Klik node di canvas untuk edit prompt, role, skills, task, dan koneksi.</div>}
        </div>
      </div>
      <FolderPicker open={folderPickerOpen} onClose={() => setFolderPickerOpen(false)} onSelect={(path: string) => { if (editing) setEditing({ ...editing, project_dir: path }) }} title="Pilih Project Directory" />
    </div>
  )
}
