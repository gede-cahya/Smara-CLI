import { useState, useEffect, useCallback, useMemo } from 'react'
import {
  ReactFlow,
  Background,
  Controls,
  MiniMap,
  type Node,
  type Edge,
  useNodesState,
  useEdgesState,
} from '@xyflow/react'
import '@xyflow/react/dist/style.css'
import { Sparkles, RefreshCw, Search, Link2, Trash2, X, Tag } from 'lucide-react'
import { fetchJSON } from '../api'

interface MemGraphNode {
  id: number
  label: string
  content: string
  tags: string[]
  source: string
  category_id?: number
  degree: number
}

interface MemGraphEdge {
  from: number
  to: number
  relation: string
  weight: number
  auto: boolean
}

interface MemGraphData {
  nodes: MemGraphNode[]
  edges: MemGraphEdge[]
}

interface MemoryLink {
  id: number
  source_id: number
  target_id: number
  relation: string
  weight: number
  auto_linked: boolean
  note?: string
  created_at: string
}

function hashNoise(seed: number): number {
  const x = Math.sin(seed * 999.13) * 43758.5453
  return x - Math.floor(x)
}

function layoutNodes(nodes: MemGraphNode[], edges: MemGraphEdge[]): Map<number, { x: number; y: number }> {
  const positions = new Map<number, { x: number; y: number }>()
  const n = nodes.length
  if (n === 0) return positions

  const sorted = [...nodes].sort((a, b) => b.degree - a.degree)
  const center = { x: 560, y: 380 }
  const maxDegree = Math.max(1, ...nodes.map(x => x.degree))
  const hubCount = Math.min(Math.max(1, Math.ceil(n / 12)), 8)
  const hubs = sorted.slice(0, hubCount)
  const hubIds = new Set(hubs.map(h => h.id))

  hubs.forEach((node, i) => {
    if (i === 0) { positions.set(node.id, center); return }
    const angle = ((i - 1) / Math.max(1, hubCount - 1)) * Math.PI * 2 - Math.PI / 2
    const r = 110 + (i % 3) * 34
    positions.set(node.id, { x: center.x + Math.cos(angle) * r, y: center.y + Math.sin(angle) * r })
  })

  const hubLinks = new Map<number, MemGraphEdge[]>()
  edges.forEach(e => {
    if (hubIds.has(e.from) && !hubIds.has(e.to)) hubLinks.set(e.from, [...(hubLinks.get(e.from) || []), e])
    if (hubIds.has(e.to) && !hubIds.has(e.from)) hubLinks.set(e.to, [...(hubLinks.get(e.to) || []), { ...e, from: e.to, to: e.from }])
  })

  const placed = new Set<number>(hubs.map(h => h.id))
  hubs.forEach((hub, hi) => {
    const origin = positions.get(hub.id) || center
    const linked = (hubLinks.get(hub.id) || []).sort((a, b) => b.weight - a.weight)
    const baseAngle = (hi / Math.max(1, hubs.length)) * Math.PI * 2
    linked.forEach((e, i) => {
      if (placed.has(e.to)) return
      const target = nodes.find(x => x.id === e.to)
      const degreeBoost = target ? 1 - Math.min(target.degree / maxDegree, 0.7) : 0.5
      const angle = baseAngle + (i / Math.max(1, linked.length)) * Math.PI * 2 + (hashNoise(e.to) - 0.5) * 0.9
      const r = 140 + degreeBoost * 260 + hashNoise(e.to + 17) * 95
      positions.set(e.to, { x: origin.x + Math.cos(angle) * r, y: origin.y + Math.sin(angle) * r })
      placed.add(e.to)
    })
  })

  const remaining = sorted.filter(node => !placed.has(node.id))
  remaining.forEach((node, i) => {
    const angle = (i / Math.max(1, remaining.length)) * Math.PI * 2 - Math.PI / 2 + (hashNoise(node.id) - 0.5) * 0.55
    const r = 410 + (i % 4) * 55 + hashNoise(node.id + 31) * 80
    positions.set(node.id, { x: center.x + Math.cos(angle) * r, y: center.y + Math.sin(angle) * r })
  })

  return positions
}

function nodeSize(degree: number): number {
  if (degree >= 20) return 34
  if (degree >= 12) return 29
  if (degree >= 8) return 24
  if (degree >= 4) return 19
  if (degree >= 1) return 13
  return 8
}

function nodeColor(degree: number): { bg: string; border: string; glow: string } {
  if (degree >= 10) return { bg: '#bef264', border: '#ecfccb', glow: 'rgba(190,242,100,0.46)' }
  if (degree >= 6) return { bg: '#84cc16', border: '#bef264', glow: 'rgba(132,204,22,0.38)' }
  if (degree >= 3) return { bg: '#4ade80', border: '#86efac', glow: 'rgba(74,222,128,0.30)' }
  if (degree >= 1) return { bg: '#22d3ee', border: '#67e8f9', glow: 'rgba(34,211,238,0.24)' }
  return { bg: '#94a3b8', border: '#cbd5e1', glow: 'rgba(148,163,184,0.22)' }
}

function shortText(s: string, max = 96): string {
  const clean = (s || '').replace(/\s+/g, ' ').trim()
  return clean.length > max ? `${clean.slice(0, max - 1)}…` : clean
}

export default function MemoryGraph({ workspace }: { workspace?: string }) {
  const [data, setData] = useState<MemGraphData>({ nodes: [], edges: [] })
  const [loading, setLoading] = useState(false)
  const [autolinking, setAutolinking] = useState(false)
  const [selected, setSelected] = useState<MemGraphNode | null>(null)
  const [linksOfSelected, setLinksOfSelected] = useState<MemoryLink[]>([])
  const [filter, setFilter] = useState('')
  const [threshold, setThreshold] = useState(0.78)
  const [topK, setTopK] = useState(5)
  const [toast, setToast] = useState<string | null>(null)
  const [highlightedId, setHighlightedId] = useState<number | null>(null)
  const [nodes, setNodes, onNodesChange] = useNodesState<Node>([])
  const [edges, setEdges, onEdgesChange] = useEdgesState<Edge>([])

  const showToast = (msg: string) => {
    setToast(msg)
    window.setTimeout(() => setToast(null), 2400)
  }

  const load = useCallback(async () => {
    setLoading(true)
    try {
      const qs = workspace ? `?workspace=${encodeURIComponent(workspace)}` : ''
      const d = await fetchJSON<MemGraphData>(`/api/memories/graph${qs}`)
      setData({ nodes: d.nodes || [], edges: d.edges || [] })
    } catch (e) {
      console.error(e)
      showToast('Gagal memuat graph')
    } finally {
      setLoading(false)
    }
  }, [workspace])

  useEffect(() => { load() }, [load])

  useEffect(() => {
    if (!selected) { setLinksOfSelected([]); return }
    fetchJSON<{ links: MemoryLink[] }>(`/api/memories/links?memory_id=${selected.id}`)
      .then(r => setLinksOfSelected(r.links || []))
      .catch(() => setLinksOfSelected([]))
  }, [selected])

  const runAutolink = async () => {
    setAutolinking(true)
    try {
      const r = await fetchJSON<{ mode: 'semantic' | 'lexical' | 'none'; created: number; wikilinks_created?: number; memories_scanned: number; with_embedding: number; fell_back_to_lexical: boolean }>('/api/memories/autolink', {
        method: 'POST',
        body: JSON.stringify({ threshold, top_k: topK, replace: true, wikilinks: true }),
      })
      const modeLabel = r.mode === 'semantic' ? '🧠 semantic' : r.mode === 'lexical' ? '📝 lexical' : 'no data'
      const fallback = r.fell_back_to_lexical ? ` (fallback — ${r.with_embedding}/${r.memories_scanned} punya embedding)` : ''
      const wiki = r.wikilinks_created ? ` + ${r.wikilinks_created} [[wikilink]]` : ''
      showToast(`${modeLabel}: ${r.created} link dibuat${wiki}${fallback}`)
      await load()
    } catch (e) {
      console.error(e)
      showToast('Auto-link gagal')
    } finally {
      setAutolinking(false)
    }
  }

  const removeLink = async (id: number) => {
    try {
      await fetchJSON(`/api/memories/links?id=${id}`, { method: 'DELETE' })
      showToast('Link dihapus')
      await load()
      if (selected) {
        const r = await fetchJSON<{ links: MemoryLink[] }>(`/api/memories/links?memory_id=${selected.id}`)
        setLinksOfSelected(r.links || [])
      }
    } catch (e) {
      console.error(e)
      showToast('Gagal menghapus link')
    }
  }

  const visibleIds = useMemo(() => {
    if (!filter.trim()) return null
    const q = filter.toLowerCase()
    const ids = new Set<number>()
    data.nodes.forEach(n => {
      if (n.label.toLowerCase().includes(q) || n.content.toLowerCase().includes(q) || n.tags.some(t => t.toLowerCase().includes(q))) ids.add(n.id)
    })
    return ids
  }, [filter, data.nodes])

  const highlightedNeighborIds = useMemo(() => {
    if (highlightedId === null) return new Set<number>()
    const ids = new Set<number>([highlightedId])
    data.edges.forEach(e => {
      if (e.from === highlightedId) ids.add(e.to)
      if (e.to === highlightedId) ids.add(e.from)
    })
    return ids
  }, [highlightedId, data.edges])

  const connectedEdgeIds = useMemo(() => {
    if (highlightedId === null) return new Set<string>()
    const ids = new Set<string>()
    data.edges.forEach((e, i) => {
      if (e.from === highlightedId || e.to === highlightedId) ids.add(`e${i}`)
    })
    return ids
  }, [highlightedId, data.edges])

  const flowNodes = useMemo<Node[]>(() => {
    const positions = layoutNodes(data.nodes, data.edges)
    return data.nodes.map(n => {
      const c = nodeColor(n.degree)
      const dimByFilter = visibleIds && !visibleIds.has(n.id)
      const dimByHighlight = highlightedId !== null && !highlightedNeighborIds.has(n.id)
      const selectedNode = highlightedId === n.id
      const neighborNode = highlightedId !== null && highlightedNeighborIds.has(n.id) && !selectedNode
      const size = nodeSize(n.degree)
      const glow = selectedNode
        ? '0 0 0 6px rgba(250,204,21,0.22), 0 0 34px rgba(250,204,21,0.85), inset 0 0 9px rgba(255,255,255,0.42)'
        : neighborNode
          ? '0 0 0 4px rgba(34,211,238,0.16), 0 0 26px rgba(34,211,238,0.72), inset 0 0 7px rgba(255,255,255,0.36)'
          : `0 0 ${Math.max(7, size * 0.72)}px ${c.glow}, inset 0 0 ${Math.max(3, size / 5)}px rgba(255,255,255,0.28)`
      return {
        id: String(n.id),
        position: positions.get(n.id) || { x: 0, y: 0 },
        data: { label: '' },
        style: {
          width: size,
          height: size,
          minWidth: size,
          minHeight: size,
          padding: 0,
          background: selectedNode
            ? 'radial-gradient(circle at 38% 32%, #ffffff 0%, #fde68a 18%, #facc15 58%, #78350f 100%)'
            : neighborNode
              ? 'radial-gradient(circle at 38% 32%, #ffffff 0%, #a5f3fc 18%, #22d3ee 58%, #164e63 100%)'
              : `radial-gradient(circle at 38% 32%, #ffffff 0%, ${c.bg} 18%, ${c.bg} 58%, #172033 100%)`,
          border: selectedNode ? '2px solid #fef3c7' : neighborNode ? '2px solid #cffafe' : `1px solid ${c.border}`,
          borderRadius: 999,
          boxShadow: glow,
          opacity: dimByFilter || dimByHighlight ? 0.13 : 1,
          transition: 'opacity 200ms, box-shadow 180ms, background 180ms, border 180ms',
          cursor: 'grab',
        },
        ariaLabel: `Memory ${n.id}: ${n.label || shortText(n.content, 40)}. Degree ${n.degree}`,
        className: `memory-dot-node ${n.degree >= 6 ? 'memory-dot-hub' : ''}`,
        draggable: true,
      }
    })
  }, [data.nodes, data.edges, visibleIds, highlightedId, highlightedNeighborIds])

  const flowEdges = useMemo<Edge[]>(() => data.edges.map((e, i): Edge => {
    const id = `e${i}`
    const dimByFilter = visibleIds && (!visibleIds.has(e.from) || !visibleIds.has(e.to))
    const highlighted = highlightedId !== null && connectedEdgeIds.has(id)
    const dimByHighlight = highlightedId !== null && !highlighted
    return {
      id,
      source: String(e.from),
      target: String(e.to),
      animated: highlighted || !e.auto,
      label: e.relation === 'similar' ? '' : e.relation,
      labelStyle: { fontSize: 9, fill: highlighted ? '#fde68a' : '#9ca3af' },
      labelBgStyle: { fill: '#0b1120', fillOpacity: 0.75 },
      style: {
        stroke: highlighted ? 'rgba(250,204,21,0.95)' : e.auto ? 'rgba(148,163,184,0.30)' : 'rgba(148,163,184,0.55)',
        strokeWidth: highlighted ? Math.max(2.2, Math.min(4.4, e.weight * 4.2)) : Math.max(0.55, Math.min(2.4, e.weight * 2.6)),
        opacity: dimByFilter || dimByHighlight ? 0.07 : highlighted ? 1 : 0.72,
        transition: 'opacity 180ms, stroke 180ms, stroke-width 180ms',
      },
    }
  }), [data.edges, visibleIds, highlightedId, connectedEdgeIds])

  useEffect(() => { setNodes(flowNodes) }, [flowNodes, setNodes])
  useEffect(() => { setEdges(flowEdges) }, [flowEdges, setEdges])

  const stats = `${data.nodes.length} nodes · ${data.edges.length} edges`

  return (
    <div className="relative flex h-full overflow-hidden">
      <div className="flex-1 flex flex-col min-w-0">
        <div className="flex items-center gap-2 p-3 border-b border-gray-800 bg-gray-900/40">
          <div className="flex items-center gap-2 text-sm font-medium text-gray-300">
            <Link2 className="w-4 h-4 text-smara-400" />
            Memory Graph
            <span className="text-xs text-gray-500 font-normal">{stats}</span>
          </div>
          <div className="ml-4 flex items-center gap-1 bg-gray-800 border border-gray-700 rounded-md px-2">
            <Search className="w-3 h-3 text-gray-500" />
            <input value={filter} onChange={e => setFilter(e.target.value)} placeholder="Filter node…" className="bg-transparent text-xs px-1 py-1.5 w-44 focus:outline-none" />
          </div>
          <div className="ml-auto flex items-center gap-2">
            <label className="flex items-center gap-1 text-xs text-gray-500">threshold<input type="number" step="0.01" min="0" max="1" value={threshold} onChange={e => setThreshold(parseFloat(e.target.value) || 0.78)} className="w-14 bg-gray-800 border border-gray-700 rounded px-1.5 py-1 text-gray-300 focus:outline-none focus:border-smara-500" /></label>
            <label className="flex items-center gap-1 text-xs text-gray-500">top-k<input type="number" min="1" max="20" value={topK} onChange={e => setTopK(parseInt(e.target.value) || 5)} className="w-12 bg-gray-800 border border-gray-700 rounded px-1.5 py-1 text-gray-300 focus:outline-none focus:border-smara-500" /></label>
            <button onClick={runAutolink} disabled={autolinking} className="px-2.5 py-1.5 bg-smara-700 hover:bg-smara-600 disabled:opacity-50 rounded text-xs text-white flex items-center gap-1" title="Bangun bidirectional links: [[wikilink]] + similarity"><Sparkles className="w-3 h-3" />{autolinking ? 'Linking…' : 'Build Links'}</button>
            <button onClick={load} disabled={loading} className="px-2 py-1.5 bg-gray-800 hover:bg-gray-700 disabled:opacity-50 rounded text-xs text-gray-300 flex items-center gap-1"><RefreshCw className={`w-3 h-3 ${loading ? 'animate-spin' : ''}`} />Reload</button>
          </div>
        </div>

        <div className="flex-1 relative overflow-hidden bg-[radial-gradient(circle_at_50%_42%,rgba(30,41,59,0.72),transparent_36%),radial-gradient(circle_at_20%_12%,rgba(45,212,191,0.08),transparent_28%),radial-gradient(circle_at_78%_24%,rgba(132,204,22,0.08),transparent_24%),linear-gradient(180deg,#020617,#060914_48%,#020617)]">
          <div className="pointer-events-none absolute inset-0 opacity-35" style={{ backgroundImage: 'radial-gradient(circle, rgba(148,163,184,0.16) 1px, transparent 1.2px)', backgroundSize: '28px 28px' }} />
          {data.nodes.length === 0 && !loading && <div className="absolute inset-0 flex items-center justify-center z-10"><div className="text-center text-gray-500"><Link2 className="w-8 h-8 mx-auto mb-2 opacity-50" /><div className="text-sm">Belum ada memori untuk divisualisasi.</div><div className="text-xs mt-1">Tambah memori dulu, atau klik Auto-link untuk koneksi otomatis.</div></div></div>}
          <ReactFlow
            nodes={nodes}
            edges={edges}
            onNodesChange={onNodesChange}
            onEdgesChange={onEdgesChange}
            onNodeClick={(_, node) => {
              const mem = data.nodes.find(x => String(x.id) === node.id) || null
              setSelected(mem)
              setHighlightedId(mem?.id ?? null)
            }}
            onPaneClick={() => { setSelected(null); setHighlightedId(null) }}
            fitView
            minZoom={0.2}
            maxZoom={2.5}
            nodesDraggable
            nodesConnectable={false}
            elementsSelectable
            proOptions={{ hideAttribution: true }}
          >
            <Background gap={28} size={1} color="rgba(148,163,184,0.12)" />
            <Controls position="bottom-right" />
            <MiniMap nodeColor={(n) => (n.style?.background as string) || '#22d3ee'} maskColor="rgba(2,6,23,0.68)" style={{ background: '#020617', border: '1px solid rgba(148,163,184,0.18)', borderRadius: 12 }} />
          </ReactFlow>

          <div className="absolute bottom-3 left-3 bg-slate-950/70 backdrop-blur-xl border border-white/10 rounded-xl px-3 py-2 text-[10px] text-gray-300 shadow-2xl">
            <div className="font-semibold text-[11px] text-gray-100 mb-1.5">Interactive Memory Graph · drag node, klik node untuk highlight tetangga</div>
            <div className="flex flex-wrap gap-3">
              <span className="flex items-center gap-1.5"><span className="w-2.5 h-2.5 rounded-full bg-slate-400 shadow-[0_0_10px_rgba(148,163,184,0.55)]" /> node / file catatan</span>
              <span className="flex items-center gap-1.5"><span className="w-4 h-4 rounded-full bg-lime-300 shadow-[0_0_16px_rgba(190,242,100,0.7)]" /> hub / MoC</span>
              <span className="flex items-center gap-1.5"><span className="w-3 h-3 rounded-full bg-yellow-300 shadow-[0_0_16px_rgba(250,204,21,0.8)]" /> selected</span>
              <span className="flex items-center gap-1.5"><span className="w-3 h-3 rounded-full bg-cyan-300 shadow-[0_0_16px_rgba(34,211,238,0.8)]" /> neighbor</span>
            </div>
          </div>

          {selected && <div className="absolute top-4 right-4 w-96 max-w-[calc(100%-2rem)] bg-slate-950/85 backdrop-blur-2xl border border-white/10 rounded-2xl shadow-[0_22px_80px_rgba(0,0,0,0.55)] overflow-hidden z-20">
            <div className="p-4 border-b border-white/10 bg-gradient-to-r from-lime-400/10 via-cyan-400/10 to-transparent">
              <div className="flex items-start justify-between gap-3"><div className="min-w-0"><div className="text-[10px] uppercase tracking-[0.24em] text-lime-200/80">Memory #{selected.id}{selected.source ? ` · ${selected.source}` : ''}</div><h3 className="mt-1 text-sm font-semibold text-white leading-snug">{selected.label || shortText(selected.content, 70)}</h3></div><button onClick={() => { setSelected(null); setHighlightedId(null) }} className="p-1 rounded-lg text-gray-500 hover:text-white hover:bg-white/10"><X className="w-4 h-4" /></button></div>
              <div className="mt-3 flex items-center gap-2 text-[11px] text-gray-300"><span className="px-2 py-1 rounded-full bg-lime-400/10 border border-lime-300/20 text-lime-200">degree {selected.degree}</span><span className="px-2 py-1 rounded-full bg-cyan-400/10 border border-cyan-300/20 text-cyan-200">{linksOfSelected.length} links</span></div>
            </div>
            <div className="p-4 space-y-3 max-h-[62vh] overflow-y-auto">
              <div className="flex flex-wrap gap-1.5">{(selected.tags || []).map((t, i) => <span key={i} className="text-[10px] bg-smara-700/30 text-smara-200 border border-smara-400/20 px-2 py-0.5 rounded-full"><Tag className="w-2.5 h-2.5 inline mr-0.5" />{t}</span>)}{(!selected.tags || selected.tags.length === 0) && <span className="text-[10px] text-gray-600">no tags</span>}</div>
              <div className="bg-black/30 border border-white/10 rounded-xl p-3 text-xs leading-relaxed text-gray-200 whitespace-pre-wrap">{selected.content}</div>
              <div className="pt-2 border-t border-white/10"><div className="text-[10px] text-gray-500 uppercase tracking-wider mb-2">Connected memories</div>{linksOfSelected.length === 0 && <div className="text-xs text-gray-600">Belum ada link.</div>}<div className="space-y-1.5">{linksOfSelected.map(l => { const otherId = l.source_id === selected.id ? l.target_id : l.source_id; const other = data.nodes.find(n => n.id === otherId); return <div key={l.id} className="flex items-center gap-2 text-xs bg-white/[0.03] border border-white/10 rounded-lg px-2 py-2 group"><span className={`text-[10px] px-1.5 py-0.5 rounded ${l.auto_linked ? 'bg-cyan-900/40 text-cyan-300' : 'bg-lime-900/40 text-lime-300'}`}>{l.relation}</span><span className="text-gray-500 text-[10px]">w={l.weight.toFixed(2)}</span><button className="flex-1 text-left text-gray-300 hover:text-lime-200 truncate" onClick={() => { if (other) { setSelected(other); setHighlightedId(other.id) } }} title={other?.content || `Memory #${otherId}`}>{other ? `[${other.id}] ${other.label || shortText(other.content, 40)}` : `[${otherId}] —`}</button><button onClick={() => removeLink(l.id)} className="opacity-0 group-hover:opacity-100 text-gray-600 hover:text-red-400 transition-opacity" title="Hapus link"><Trash2 className="w-3 h-3" /></button></div> })}</div></div>
            </div>
          </div>}
        </div>
      </div>

      {toast && <div className="absolute bottom-6 left-1/2 -translate-x-1/2 bg-gray-800 border border-smara-700/40 text-smara-200 text-xs px-3 py-2 rounded-lg shadow-lg z-50">{toast}</div>}
    </div>
  )
}
