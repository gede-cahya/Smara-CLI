import { useEffect, useMemo, useRef } from 'react'
import type { MemGraphData, MemGraphNode } from './graphTypes'

function colorForDegree(degree: number) {
  if (degree >= 10) return '#bef264'
  if (degree >= 6) return '#84cc16'
  if (degree >= 3) return '#4ade80'
  if (degree >= 1) return '#22d3ee'
  return '#94a3b8'
}

function hashNoise(seed: number): number {
  const x = Math.sin(seed * 999.13) * 43758.5453
  return x - Math.floor(x)
}

type DrawNode = { node: MemGraphNode; x: number; y: number; r: number }

const layoutCache = new Map<string, DrawNode[]>()

function cacheKey(data: MemGraphData) {
  const first = data.nodes[0]?.id || 0
  const last = data.nodes[data.nodes.length - 1]?.id || 0
  return `${data.meta?.mode || 'overview'}:${data.nodes.length}:${data.edges.length}:${first}:${last}`
}

function buildLayout(data: MemGraphData): DrawNode[] {
  const key = cacheKey(data)
  const cached = layoutCache.get(key)
  if (cached) return cached

  const degreeRank = [...data.nodes].sort((a, b) => (b.degree || 0) - (a.degree || 0))
  const rank = new Map(degreeRank.map((n, i) => [n.id, i]))
  const nodes = data.nodes.map((n) => {
    const i = rank.get(n.id) || 0
    const hubBias = Math.max(0.45, 1 - Math.min(n.degree || 0, 24) / 36)
    const angle = i * 2.399963229728653 + hashNoise(n.id) * 0.55
    const radius = 44 + Math.sqrt(i + 1) * 36 * hubBias + hashNoise(n.id + 91) * 70
    return {
      node: n,
      x: Math.cos(angle) * radius,
      y: Math.sin(angle) * radius,
      r: Math.max(3, Math.min(12, 3 + Math.sqrt(n.degree || 0) * 1.85)),
    }
  })
  layoutCache.set(key, nodes)
  if (layoutCache.size > 12) {
    const oldest = layoutCache.keys().next().value
    if (oldest) layoutCache.delete(oldest)
  }
  return nodes
}
interface Props {
  data: MemGraphData
  highlightedId: number | null
  filter?: string
  onSelect: (node: MemGraphNode | null) => void
  onFocus?: (node: MemGraphNode) => void
}

export default function LargeGraphRenderer({ data, highlightedId, filter, onSelect, onFocus }: Props) {
  const canvasRef = useRef<HTMLCanvasElement | null>(null)
  const wrapRef = useRef<HTMLDivElement | null>(null)
  const camera = useRef({ x: 0, y: 0, scale: 1, dragging: false, lastX: 0, lastY: 0, moved: false })
  const drawRef = useRef<() => void>(() => {})

  const graph = useMemo(() => {
    const nodes = buildLayout(data)
    const byId = new Map(nodes.map(n => [n.node.id, n]))
    const q = (filter || '').trim().toLowerCase()
    const visible = new Set<number>()
    if (q) data.nodes.forEach(n => { if ((n.label + ' ' + n.content + ' ' + (n.tags || []).join(' ') + ' ' + n.source).toLowerCase().includes(q)) visible.add(n.id) })
    const neighbors = new Set<number>()
    if (highlightedId !== null) {
      neighbors.add(highlightedId)
      data.edges.forEach(e => { if (e.from === highlightedId) neighbors.add(e.to); if (e.to === highlightedId) neighbors.add(e.from) })
    }
    return { nodes, byId, visible, neighbors }
  }, [data, highlightedId, filter])

  useEffect(() => {
    let raf = 0
    const drawNow = () => {
      const canvas = canvasRef.current
      const wrap = wrapRef.current
      if (!canvas || !wrap) return
      const rect = wrap.getBoundingClientRect()
      if (rect.width <= 0 || rect.height <= 0) return
      const dpr = window.devicePixelRatio || 1
      const nextW = Math.max(1, Math.floor(rect.width * dpr))
      const nextH = Math.max(1, Math.floor(rect.height * dpr))
      if (canvas.width !== nextW || canvas.height !== nextH) {
        canvas.width = nextW
        canvas.height = nextH
        canvas.style.width = `${rect.width}px`
        canvas.style.height = `${rect.height}px`
      }
      const ctx = canvas.getContext('2d')!
      ctx.setTransform(dpr, 0, 0, dpr, 0, 0)
      ctx.clearRect(0, 0, rect.width, rect.height)
      const cam = camera.current
      const sx = (x: number) => rect.width / 2 + (x + cam.x) * cam.scale
      const sy = (y: number) => rect.height / 2 + (y + cam.y) * cam.scale
      const dense = data.edges.length > 2500 || data.nodes.length > 1200
      const zoomedOut = cam.scale < 0.45

      // Edge LOD: for dense/zoomed-out views draw only manual, strong, or selected-neighborhood edges.
      ctx.lineCap = 'round'
      for (const e of data.edges) {
        const a = graph.byId.get(e.from); const b = graph.byId.get(e.to)
        if (!a || !b) continue
        const active = highlightedId !== null && (e.from === highlightedId || e.to === highlightedId)
        if (!active) {
          if (highlightedId !== null && dense) continue
          if (zoomedOut && e.auto && e.weight < 0.86) continue
          if (dense && e.auto && e.weight < 0.8) continue
        }
        const ax = sx(a.x); const ay = sy(a.y); const bx = sx(b.x); const by = sy(b.y)
        if ((ax < -80 && bx < -80) || (ay < -80 && by < -80) || (ax > rect.width + 80 && bx > rect.width + 80) || (ay > rect.height + 80 && by > rect.height + 80)) continue
        ctx.globalAlpha = active ? 1 : zoomedOut ? 0.38 : 0.72
        ctx.strokeStyle = active ? 'rgba(250,204,21,0.9)' : e.auto ? 'rgba(148,163,184,0.15)' : 'rgba(132,204,22,0.30)'
        ctx.lineWidth = active ? 2.2 : Math.max(0.25, Math.min(1.35, e.weight * 1.45 * cam.scale))
        ctx.beginPath(); ctx.moveTo(ax, ay); ctx.lineTo(bx, by); ctx.stroke()
      }

      for (const { node, x, y, r } of graph.nodes) {
        const px = sx(x); const py = sy(y)
        const rr = Math.max(1.8, r * Math.max(0.55, Math.min(1.6, cam.scale)))
        if (px < -rr || py < -rr || px > rect.width + rr || py > rect.height + rr) continue
        const dimFilter = graph.visible.size > 0 && !graph.visible.has(node.id)
        const dimHighlight = highlightedId !== null && !graph.neighbors.has(node.id)
        ctx.globalAlpha = dimFilter || dimHighlight ? 0.12 : 1
        ctx.fillStyle = highlightedId === node.id ? '#facc15' : graph.neighbors.has(node.id) && highlightedId !== null ? '#67e8f9' : colorForDegree(node.degree)
        ctx.beginPath(); ctx.arc(px, py, rr, 0, Math.PI * 2); ctx.fill()
        if (highlightedId === node.id) {
          ctx.globalAlpha = 0.32
          ctx.strokeStyle = '#fde68a'
          ctx.lineWidth = 3
          ctx.beginPath(); ctx.arc(px, py, rr + 7, 0, Math.PI * 2); ctx.stroke()
        }
        const showLabel = cam.scale > 0.72 && (!dense || node.degree >= 5 || highlightedId === node.id || graph.neighbors.has(node.id))
        if (showLabel) {
          ctx.globalAlpha = dimFilter || dimHighlight ? 0.18 : 0.88
          ctx.fillStyle = '#e5e7eb'; ctx.font = '11px system-ui, sans-serif'; ctx.fillText(node.label || `#${node.id}`, px + rr + 4, py + 3)
        }
        ctx.globalAlpha = 1
      }
    }
    drawRef.current = () => {
      cancelAnimationFrame(raf)
      raf = requestAnimationFrame(drawNow)
    }
    drawRef.current()
    const ro = new ResizeObserver(() => drawRef.current())
    if (wrapRef.current) ro.observe(wrapRef.current)
    return () => { cancelAnimationFrame(raf); ro.disconnect() }
  }, [data, graph, highlightedId])

  const pickNode = (clientX: number, clientY: number): MemGraphNode | null => {
    const wrap = wrapRef.current
    if (!wrap) return null
    const rect = wrap.getBoundingClientRect()
    const cam = camera.current
    const wx = (clientX - rect.left - rect.width / 2) / cam.scale - cam.x
    const wy = (clientY - rect.top - rect.height / 2) / cam.scale - cam.y
    let bestNode: MemGraphNode | null = null
    let bestD = Infinity
    for (const n of graph.nodes) {
      const d = Math.hypot(n.x - wx, n.y - wy)
      if (d < n.r + 10 / cam.scale && d < bestD) { bestD = d; bestNode = n.node }
    }
    return bestNode
  }

  return <div
    ref={wrapRef}
    className="absolute inset-0 cursor-grab active:cursor-grabbing"
    onWheel={e => {
      e.preventDefault()
      const before = camera.current.scale
      camera.current.scale = Math.max(0.12, Math.min(4, camera.current.scale * (e.deltaY > 0 ? 0.9 : 1.1)))
      if (before !== camera.current.scale) drawRef.current()
    }}
    onMouseDown={e => { camera.current.dragging = true; camera.current.moved = false; camera.current.lastX = e.clientX; camera.current.lastY = e.clientY }}
    onMouseMove={e => {
      const cam = camera.current
      if (!cam.dragging) return
      const dx = e.clientX - cam.lastX; const dy = e.clientY - cam.lastY
      if (Math.abs(dx) + Math.abs(dy) > 2) cam.moved = true
      cam.x += dx / cam.scale; cam.y += dy / cam.scale; cam.lastX = e.clientX; cam.lastY = e.clientY
      drawRef.current()
    }}
    onMouseUp={e => {
      const moved = camera.current.moved
      camera.current.dragging = false
      if (!moved) onSelect(pickNode(e.clientX, e.clientY))
    }}
    onMouseLeave={() => { camera.current.dragging = false }}
    onDoubleClick={e => {
      const n = pickNode(e.clientX, e.clientY)
      if (n && onFocus) onFocus(n)
    }}
  >
    <canvas ref={canvasRef} className="w-full h-full" />
    <div className="absolute top-3 left-3 text-[10px] text-cyan-100 bg-slate-950/70 border border-cyan-400/20 rounded-lg px-2 py-1">Fast Canvas LOD · pan/zoom/click · double-click node untuk focus neighborhood</div>
  </div>
}
