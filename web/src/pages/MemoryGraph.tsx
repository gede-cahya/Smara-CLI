import { useState, useEffect, useCallback, useMemo } from 'react'
import {
  ReactFlow,
  Background,
  Controls,
  MiniMap,
  type Node,
  type Edge,
} from '@xyflow/react'
import '@xyflow/react/dist/style.css'
import { Sparkles, RefreshCw, Search, Link2, Trash2, X, Tag, Clock } from 'lucide-react'
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

// Lay nodes out on a circle (small graphs) or grid (large graphs).
function layoutNodes(nodes: MemGraphNode[]): Map<number, { x: number; y: number }> {
  const positions = new Map<number, { x: number; y: number }>()
  const n = nodes.length
  if (n === 0) return positions

  if (n <= 24) {
    // Concentric layout — high-degree nodes inner, others outer.
    const sorted = [...nodes].sort((a, b) => b.degree - a.degree)
    const inner = sorted.slice(0, Math.min(6, Math.ceil(n / 4)))
    const outer = sorted.slice(inner.length)
    const innerR = 140
    const outerR = 320

    inner.forEach((node, i) => {
      const angle = (i / Math.max(1, inner.length)) * Math.PI * 2
      positions.set(node.id, {
        x: 480 + Math.cos(angle) * innerR,
        y: 360 + Math.sin(angle) * innerR,
      })
    })
    outer.forEach((node, i) => {
      const angle = (i / Math.max(1, outer.length)) * Math.PI * 2
      positions.set(node.id, {
        x: 480 + Math.cos(angle) * outerR,
        y: 360 + Math.sin(angle) * outerR,
      })
    })
  } else {
    // Grid layout for big graphs.
    const cols = Math.ceil(Math.sqrt(n))
    nodes.forEach((node, i) => {
      positions.set(node.id, {
        x: (i % cols) * 220,
        y: Math.floor(i / cols) * 140,
      })
    })
  }
  return positions
}

function nodeColor(degree: number): { bg: string; border: string; fg: string } {
  if (degree >= 10) return { bg: '#bef264', border: '#e7ffb0', fg: '#0b0d0a' }
  if (degree >= 6)  return { bg: '#557a25', border: '#bef264', fg: '#f4ffe0' }
  if (degree >= 3)  return { bg: '#3a5018', border: '#8aae3a', fg: '#e6f0d4' }
  if (degree >= 1)  return { bg: '#243018', border: '#5a7a2a', fg: '#cfd8c0' }
  return { bg: '#1a1f15', border: '#3a4a25', fg: '#9ca58a' }
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
      const r = await fetchJSON<{
        mode: 'semantic' | 'lexical' | 'none'
        created: number
        memories_scanned: number
        with_embedding: number
        embedding_ratio: number
        fell_back_to_lexical: boolean
      }>('/api/memories/autolink', {
        method: 'POST',
        body: JSON.stringify({ threshold, top_k: topK, replace: true }),
      })
      const modeLabel =
        r.mode === 'semantic' ? '🧠 semantic'
        : r.mode === 'lexical' ? '📝 lexical'
        : 'no data'
      const fallback = r.fell_back_to_lexical
        ? ` (fallback — ${r.with_embedding}/${r.memories_scanned} punya embedding)`
        : ''
      showToast(`${modeLabel}: ${r.created} link dibuat${fallback}`)
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

  // Filter nodes by search query.
  const visibleIds = useMemo(() => {
    if (!filter.trim()) return null
    const q = filter.toLowerCase()
    const ids = new Set<number>()
    data.nodes.forEach(n => {
      if (
        n.label.toLowerCase().includes(q) ||
        n.content.toLowerCase().includes(q) ||
        n.tags.some(t => t.toLowerCase().includes(q))
      ) ids.add(n.id)
    })
    return ids
  }, [filter, data.nodes])

  const flowNodes = useMemo<Node[]>(() => {
    const positions = layoutNodes(data.nodes)
    return data.nodes.map(n => {
      const c = nodeColor(n.degree)
      const dim = visibleIds && !visibleIds.has(n.id)
      return {
        id: String(n.id),
        position: positions.get(n.id) || { x: 0, y: 0 },
        data: { label: n.label || `#${n.id}` },
        style: {
          background: c.bg,
          color: c.fg,
          border: `1px solid ${c.border}`,
          borderRadius: 8,
          padding: '8px 12px',
          fontSize: 11,
          fontWeight: 500,
          minWidth: 100,
          maxWidth: 220,
          textAlign: 'center' as const,
          opacity: dim ? 0.18 : 1,
          transition: 'opacity 200ms',
          cursor: 'pointer',
        },
      }
    })
  }, [data.nodes, visibleIds])

  const flowEdges = useMemo<Edge[]>(() => {
    return data.edges.map((e, i): Edge => {
      const dim = visibleIds && (!visibleIds.has(e.from) || !visibleIds.has(e.to))
      return {
        id: `e${i}`,
        source: String(e.from),
        target: String(e.to),
        animated: !e.auto,
        label: e.relation === 'similar' ? '' : e.relation,
        labelStyle: { fontSize: 9, fill: '#8a9078' },
        labelBgStyle: { fill: '#11140f' },
        style: {
          stroke: e.auto ? 'rgba(106,138,58,0.55)' : 'rgba(190,242,100,0.85)',
          strokeWidth: Math.max(0.6, Math.min(3.5, e.weight * 3.5)),
          strokeDasharray: e.auto ? '5,4' : undefined,
          opacity: dim ? 0.1 : 1,
        },
      }
    })
  }, [data.edges, visibleIds])

  const stats = `${data.nodes.length} nodes · ${data.edges.length} edges`

  return (
    <div className="flex h-full">
      <div className="flex-1 flex flex-col">
        {/* Toolbar */}
        <div className="flex items-center gap-2 p-3 border-b border-gray-800 bg-gray-900/40">
          <div className="flex items-center gap-2 text-sm font-medium text-gray-300">
            <Link2 className="w-4 h-4 text-smara-400" />
            Memory Graph
            <span className="text-xs text-gray-500 font-normal">{stats}</span>
          </div>

          <div className="ml-4 flex items-center gap-1 bg-gray-800 border border-gray-700 rounded-md px-2">
            <Search className="w-3 h-3 text-gray-500" />
            <input
              value={filter}
              onChange={e => setFilter(e.target.value)}
              placeholder="Filter node…"
              className="bg-transparent text-xs px-1 py-1.5 w-44 focus:outline-none"
            />
          </div>

          <div className="ml-auto flex items-center gap-2">
            <div className="flex items-center gap-1 text-xs text-gray-500">
              threshold
              <input
                type="number" step="0.01" min="0" max="1"
                value={threshold}
                onChange={e => setThreshold(parseFloat(e.target.value) || 0.78)}
                className="w-14 bg-gray-800 border border-gray-700 rounded px-1.5 py-1 text-gray-300 focus:outline-none focus:border-smara-500"
              />
            </div>
            <div className="flex items-center gap-1 text-xs text-gray-500">
              top-k
              <input
                type="number" min="1" max="20"
                value={topK}
                onChange={e => setTopK(parseInt(e.target.value) || 5)}
                className="w-12 bg-gray-800 border border-gray-700 rounded px-1.5 py-1 text-gray-300 focus:outline-none focus:border-smara-500"
              />
            </div>
            <button
              onClick={runAutolink}
              disabled={autolinking}
              className="px-2.5 py-1.5 bg-smara-700 hover:bg-smara-600 disabled:opacity-50 rounded text-xs text-white flex items-center gap-1"
              title="Bangun link otomatis dari similarity"
            >
              <Sparkles className="w-3 h-3" />
              {autolinking ? 'Linking…' : 'Auto-link'}
            </button>
            <button
              onClick={load}
              disabled={loading}
              className="px-2 py-1.5 bg-gray-800 hover:bg-gray-700 disabled:opacity-50 rounded text-xs text-gray-300 flex items-center gap-1"
            >
              <RefreshCw className={`w-3 h-3 ${loading ? 'animate-spin' : ''}`} />
              Reload
            </button>
          </div>
        </div>

        {/* Graph canvas */}
        <div className="flex-1 relative bg-gray-950">
          {data.nodes.length === 0 && !loading && (
            <div className="absolute inset-0 flex items-center justify-center">
              <div className="text-center text-gray-500">
                <Link2 className="w-8 h-8 mx-auto mb-2 opacity-50" />
                <div className="text-sm">Belum ada memori untuk divisualisasi.</div>
                <div className="text-xs mt-1">Tambah memori dulu, atau klik Auto-link untuk koneksi otomatis.</div>
              </div>
            </div>
          )}
          <ReactFlow
            nodes={flowNodes}
            edges={flowEdges}
            onNodesChange={() => {}}
            onEdgesChange={() => {}}
            onNodeClick={(_, node) => {
              const n = data.nodes.find(x => String(x.id) === node.id)
              setSelected(n || null)
            }}
            onPaneClick={() => setSelected(null)}
            fitView
            minZoom={0.2}
            maxZoom={2}
          >
            <Background gap={18} size={1} color="#1f2419" />
            <Controls position="bottom-right" />
            <MiniMap
              nodeColor={(n) => (n.style?.background as string) || '#3a4a25'}
              maskColor="rgba(0,0,0,0.6)"
              style={{ background: '#11140f', border: '1px solid #1f2419' }}
            />
          </ReactFlow>

          {/* Legend */}
          <div className="absolute bottom-3 left-3 bg-gray-900/85 backdrop-blur border border-gray-800 rounded-lg px-3 py-2 text-[10px] text-gray-400 flex gap-3">
            <span className="flex items-center gap-1.5">
              <span className="w-4 h-0.5 bg-smara-400 inline-block" />
              manual
            </span>
            <span className="flex items-center gap-1.5">
              <span className="w-4 h-0.5 inline-block" style={{ borderTop: '1.5px dashed #6a8a3a' }} />
              auto (similarity)
            </span>
          </div>
        </div>
      </div>

      {/* Side panel */}
      <aside className="w-80 border-l border-gray-800 bg-gray-900/40 flex flex-col overflow-hidden shrink-0">
        <div className="p-3 border-b border-gray-800 flex items-center gap-2 text-xs text-gray-400 uppercase tracking-wider">
          <Link2 className="w-3 h-3" />
          Detail
        </div>
        <div className="flex-1 overflow-y-auto p-3">
          {!selected && (
            <div className="text-gray-600 text-xs text-center py-12">
              Klik sebuah node untuk melihat detail dan link-nya.
            </div>
          )}
          {selected && (
            <div className="space-y-3">
              <div className="text-[10px] text-gray-500 uppercase tracking-wider">
                Memory #{selected.id}{selected.source ? ` · ${selected.source}` : ''}
              </div>
              <div className="flex flex-wrap gap-1">
                {(selected.tags || []).map((t, i) => (
                  <span key={i} className="text-[10px] bg-smara-700/30 text-smara-300 px-2 py-0.5 rounded">
                    <Tag className="w-2.5 h-2.5 inline mr-0.5" />
                    {t}
                  </span>
                ))}
                {(!selected.tags || selected.tags.length === 0) && (
                  <span className="text-[10px] text-gray-600">no tags</span>
                )}
              </div>
              <div className="bg-gray-950 border border-gray-800 rounded-lg p-3 text-xs leading-relaxed text-gray-200 whitespace-pre-wrap max-h-72 overflow-y-auto">
                {selected.content}
              </div>
              <div className="flex items-center gap-3 text-[10px] text-gray-500">
                <span className="flex items-center gap-1">
                  <Clock className="w-2.5 h-2.5" />
                  degree: {selected.degree}
                </span>
              </div>

              <div className="pt-2 border-t border-gray-800">
                <div className="text-[10px] text-gray-500 uppercase tracking-wider mb-2">
                  Links · {linksOfSelected.length}
                </div>
                {linksOfSelected.length === 0 && (
                  <div className="text-xs text-gray-600">Belum ada link.</div>
                )}
                <div className="space-y-1">
                  {linksOfSelected.map(l => {
                    const otherId = l.source_id === selected.id ? l.target_id : l.source_id
                    const other = data.nodes.find(n => n.id === otherId)
                    return (
                      <div key={l.id} className="flex items-center gap-2 text-xs bg-gray-900/60 border border-gray-800 rounded px-2 py-1.5 group">
                        <span className={`text-[10px] px-1.5 py-0.5 rounded ${l.auto_linked ? 'bg-emerald-900/40 text-emerald-400' : 'bg-smara-700/30 text-smara-300'}`}>
                          {l.relation}
                        </span>
                        <span className="text-gray-500 text-[10px]">w={l.weight.toFixed(2)}</span>
                        <button
                          className="flex-1 text-left text-gray-300 hover:text-smara-300 truncate"
                          onClick={() => other && setSelected(other)}
                          title={other?.content || `Memory #${otherId}`}
                        >
                          {other ? `[${other.id}] ${other.label}` : `[${otherId}] —`}
                        </button>
                        <button
                          onClick={() => removeLink(l.id)}
                          className="opacity-0 group-hover:opacity-100 text-gray-600 hover:text-red-400 transition-opacity"
                          title="Hapus link"
                        >
                          <Trash2 className="w-3 h-3" />
                        </button>
                      </div>
                    )
                  })}
                </div>
              </div>

              <div className="flex justify-end pt-2">
                <button
                  onClick={() => setSelected(null)}
                  className="text-[10px] text-gray-500 hover:text-gray-300 flex items-center gap-1"
                >
                  <X className="w-3 h-3" /> Tutup
                </button>
              </div>
            </div>
          )}
        </div>
      </aside>

      {toast && (
        <div className="absolute bottom-6 left-1/2 -translate-x-1/2 bg-gray-800 border border-smara-700/40 text-smara-200 text-xs px-3 py-2 rounded-lg shadow-lg z-50">
          {toast}
        </div>
      )}
    </div>
  )
}
