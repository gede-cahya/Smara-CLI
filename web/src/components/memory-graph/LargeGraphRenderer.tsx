import { useEffect, useMemo, useRef, useState } from 'react'
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

type PatternMode = 'nebula' | 'constellation' | 'radial' | 'galaxy' | 'organic'
type DrawNode = { node: MemGraphNode; x: number; y: number; vx: number; vy: number; r: number; fixed?: boolean }
type DragMode = 'camera' | 'node' | 'edge' | null
type DragEdge = { a: DrawNode; b: DrawNode }
type CameraState = {
  x: number
  y: number
  scale: number
  dragging: boolean
  lastX: number
  lastY: number
  moved: boolean
  mode: DragMode
  dragNode: DrawNode | null
  dragEdge: DragEdge | null
}

const patternModes: Array<{ id: PatternMode; label: string }> = [
  { id: 'nebula', label: 'Nebula' },
  { id: 'constellation', label: 'Constellation' },
  { id: 'radial', label: 'Radial' },
  { id: 'galaxy', label: 'Galaxy' },
  { id: 'organic', label: 'Organic' },
]

function nodeCluster(node: MemGraphNode): number {
  const raw = `${node.source || ''}:${(node.tags || []).join(',')}:${node.label || node.id}`
  let h = 0
  for (let i = 0; i < raw.length; i++) h = (h * 31 + raw.charCodeAt(i)) | 0
  return Math.abs(h) % 9
}

function patternTarget(node: DrawNode, mode: PatternMode, index: number, total: number) {
  const degree = node.node.degree || 0
  const cluster = nodeCluster(node.node)
  const t = total <= 1 ? 0 : index / (total - 1)
  const noise = hashNoise(node.node.id + cluster * 101)

  if (mode === 'radial') {
    const angle = cluster / 9 * Math.PI * 2 + (noise - 0.5) * 0.55
    const radius = 70 + (8 - Math.min(cluster, 8)) * 28 + Math.sqrt(Math.max(1, index)) * 7 - Math.min(degree, 24) * 4
    return { x: Math.cos(angle) * radius, y: Math.sin(angle) * radius }
  }

  if (mode === 'galaxy') {
    const arm = cluster % 4
    const angle = arm * Math.PI / 2 + t * Math.PI * 7.5 + noise * 0.7
    const radius = 38 + Math.sqrt(index + 1) * 18 + cluster * 9
    return { x: Math.cos(angle) * radius, y: Math.sin(angle) * radius }
  }

  if (mode === 'constellation') {
    const hubAngle = cluster / 9 * Math.PI * 2
    const hubRadius = 95 + (cluster % 3) * 92
    const localAngle = noise * Math.PI * 2
    const localRadius = 22 + Math.sqrt(index % 80) * 14 + Math.max(0, 10 - degree) * 3
    return {
      x: Math.cos(hubAngle) * hubRadius + Math.cos(localAngle) * localRadius,
      y: Math.sin(hubAngle) * hubRadius + Math.sin(localAngle) * localRadius,
    }
  }

  if (mode === 'organic') {
    const angle = index * 2.399963229728653 + noise * 1.4
    const wave = Math.sin(index * 0.19 + cluster) * 72
    const radius = 45 + Math.sqrt(index + 1) * 27 + wave - Math.min(degree, 20) * 5
    return { x: Math.cos(angle) * radius * 1.12, y: Math.sin(angle) * radius * 0.82 }
  }

  const angle = index * 2.399963229728653 + noise * 0.9
  const radius = 42 + Math.sqrt(index + 1) * (22 + (cluster % 4) * 4) - Math.min(degree, 22) * 5 + hashNoise(node.node.id + 91) * 78
  return { x: Math.cos(angle) * radius, y: Math.sin(angle) * radius }
}

type SimGraph = {
  nodes: DrawNode[]
  byId: Map<number, DrawNode>
  visible: Set<number>
  neighbors: Set<number>
  edgePairs: Array<{ a: DrawNode; b: DrawNode; weight: number; auto: boolean }>
  alpha: number
}

const simCache = new Map<string, DrawNode[]>()

function cacheKey(data: MemGraphData) {
  const first = data.nodes[0]?.id || 0
  const last = data.nodes[data.nodes.length - 1]?.id || 0
  return `${data.meta?.mode || 'overview'}:${data.nodes.length}:${data.edges.length}:${first}:${last}`
}

function initialNodes(data: MemGraphData): DrawNode[] {
  const key = cacheKey(data)
  const cached = simCache.get(key)
  if (cached) return cached.map(n => ({ ...n, node: data.nodes.find(x => x.id === n.node.id) || n.node }))

  const degreeRank = [...data.nodes].sort((a, b) => (b.degree || 0) - (a.degree || 0))
  const rank = new Map(degreeRank.map((n, i) => [n.id, i]))
  const nodes = data.nodes.map((n) => {
    const i = rank.get(n.id) || 0
    const hubBias = Math.max(0.35, 1 - Math.min(n.degree || 0, 32) / 42)
    const angle = i * 2.399963229728653 + hashNoise(n.id) * 0.7
    const radius = 35 + Math.sqrt(i + 1) * 31 * hubBias + hashNoise(n.id + 91) * 82
    return {
      node: n,
      x: Math.cos(angle) * radius,
      y: Math.sin(angle) * radius,
      vx: 0,
      vy: 0,
      r: Math.max(3.2, Math.min(13, 3.2 + Math.sqrt(n.degree || 0) * 1.9)),
    }
  })
  simCache.set(key, nodes.map(n => ({ ...n })))
  if (simCache.size > 10) {
    const oldest = simCache.keys().next().value
    if (oldest) simCache.delete(oldest)
  }
  return nodes
}

function stepSimulation(sim: SimGraph, dense: boolean, pattern: PatternMode) {
  const nodes = sim.nodes
  const alpha = sim.alpha
  if (alpha < 0.015) return

  for (let i = 0; i < nodes.length; i++) {
    const n = nodes[i]
    const target = patternTarget(n, pattern, i, nodes.length)
    const patternPull = pattern === 'nebula' ? 0.0012 : pattern === 'organic' ? 0.0020 : 0.0028
    n.vx += (target.x - n.x) * patternPull * alpha
    n.vy += (target.y - n.y) * patternPull * alpha
    const hubPull = 0.0016 + Math.min(n.node.degree || 0, 20) * 0.00005
    n.vx += -n.x * hubPull * alpha
    n.vy += -n.y * hubPull * alpha
  }

  const maxEdgesPerTick = dense ? 4200 : sim.edgePairs.length
  for (let i = 0; i < sim.edgePairs.length && i < maxEdgesPerTick; i++) {
    const e = sim.edgePairs[i]
    const dx = e.b.x - e.a.x
    const dy = e.b.y - e.a.y
    const dist = Math.max(1, Math.hypot(dx, dy))
    const target = pattern === 'constellation' ? (e.auto ? 132 : 105) : pattern === 'radial' ? (e.auto ? 118 : 98) : e.auto ? 112 : 92
    const strength = (e.auto ? 0.010 : 0.018) * Math.max(0.35, Math.min(1.35, e.weight)) * alpha
    const f = (dist - target) * strength / dist
    const fx = dx * f
    const fy = dy * f
    e.a.vx += fx; e.a.vy += fy
    e.b.vx -= fx; e.b.vy -= fy
  }

  const repel = pattern === 'constellation' ? 1250 : pattern === 'radial' ? 1020 : dense ? 980 : 1450
  const stride = dense ? Math.max(2, Math.ceil(nodes.length / 620)) : 1
  for (let i = 0; i < nodes.length; i++) {
    const a = nodes[i]
    for (let j = i + stride; j < nodes.length; j += stride) {
      const b = nodes[j]
      let dx = b.x - a.x
      let dy = b.y - a.y
      let d2 = dx * dx + dy * dy
      if (d2 < 0.01) { dx = hashNoise(a.node.id + b.node.id) - 0.5; dy = hashNoise(a.node.id - b.node.id) - 0.5; d2 = dx * dx + dy * dy }
      if (d2 > 620 * 620) continue
      const dist = Math.sqrt(d2)
      const minDist = a.r + b.r + 18
      const force = ((repel / Math.max(d2, 80)) + (dist < minDist ? (minDist - dist) * 0.018 : 0)) * alpha
      const fx = (dx / dist) * force
      const fy = (dy / dist) * force
      a.vx -= fx; a.vy -= fy
      b.vx += fx; b.vy += fy
    }
  }

  for (const n of nodes) {
    if (n.fixed) {
      n.vx = 0
      n.vy = 0
      continue
    }
    n.vx *= 0.84
    n.vy *= 0.84
    n.x += Math.max(-14, Math.min(14, n.vx))
    n.y += Math.max(-14, Math.min(14, n.vy))
  }
  sim.alpha *= 0.982
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
  const camera = useRef<CameraState>({
    x: 0,
    y: 0,
    scale: 1,
    dragging: false,
    lastX: 0,
    lastY: 0,
    moved: false,
    mode: null,
    dragNode: null,
    dragEdge: null,
  })
  const drawRef = useRef<() => void>(() => {})
  const simRef = useRef<SimGraph | null>(null)
  const [pattern, setPattern] = useState<PatternMode>('nebula')

  const graph = useMemo(() => {
    const nodes = initialNodes(data)
    const byId = new Map(nodes.map(n => [n.node.id, n]))
    const q = (filter || '').trim().toLowerCase()
    const visible = new Set<number>()
    if (q) data.nodes.forEach(n => { if ((n.label + ' ' + n.content + ' ' + (n.tags || []).join(' ') + ' ' + n.source).toLowerCase().includes(q)) visible.add(n.id) })
    const neighbors = new Set<number>()
    if (highlightedId !== null) {
      neighbors.add(highlightedId)
      data.edges.forEach(e => { if (e.from === highlightedId) neighbors.add(e.to); if (e.to === highlightedId) neighbors.add(e.from) })
    }
    const edgePairs = data.edges.map(e => {
      const a = byId.get(e.from); const b = byId.get(e.to)
      return a && b ? { a, b, weight: e.weight, auto: e.auto } : null
    }).filter(Boolean) as SimGraph['edgePairs']
    return { nodes, byId, visible, neighbors, edgePairs, alpha: 1 } as SimGraph
  }, [data, highlightedId, filter])

  useEffect(() => {
    simRef.current = graph
  }, [graph])

  useEffect(() => {
    if (simRef.current) simRef.current.alpha = 1
    drawRef.current()
  }, [pattern])

  useEffect(() => {
    let raf = 0
    let running = true
    const drawNow = () => {
      const canvas = canvasRef.current
      const wrap = wrapRef.current
      const sim = simRef.current
      if (!canvas || !wrap || !sim) return
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
      const dense = data.edges.length > 2500 || data.nodes.length > 1200
      stepSimulation(sim, dense, pattern)

      const ctx = canvas.getContext('2d')!
      ctx.setTransform(dpr, 0, 0, dpr, 0, 0)
      ctx.clearRect(0, 0, rect.width, rect.height)
      const cam = camera.current
      const sx = (x: number) => rect.width / 2 + (x + cam.x) * cam.scale
      const sy = (y: number) => rect.height / 2 + (y + cam.y) * cam.scale
      const zoomedOut = cam.scale < 0.45

      const bg = ctx.createRadialGradient(rect.width / 2, rect.height / 2, 0, rect.width / 2, rect.height / 2, Math.max(rect.width, rect.height) * 0.6)
      bg.addColorStop(0, 'rgba(15,23,42,0.06)')
      bg.addColorStop(1, 'rgba(2,6,23,0.0)')
      ctx.fillStyle = bg
      ctx.fillRect(0, 0, rect.width, rect.height)

      ctx.lineCap = 'round'
      for (let i = 0; i < data.edges.length; i++) {
        const e = data.edges[i]
        const a = sim.byId.get(e.from); const b = sim.byId.get(e.to)
        if (!a || !b) continue
        const active = highlightedId !== null && (e.from === highlightedId || e.to === highlightedId)
        if (!active) {
          if (highlightedId !== null && dense) continue
          if (zoomedOut && e.auto && e.weight < 0.86) continue
          if (dense && e.auto && e.weight < 0.8) continue
        }
        const ax = sx(a.x); const ay = sy(a.y); const bx = sx(b.x); const by = sy(b.y)
        if ((ax < -80 && bx < -80) || (ay < -80 && by < -80) || (ax > rect.width + 80 && bx > rect.width + 80) || (ay > rect.height + 80 && by > rect.height + 80)) continue
        ctx.globalAlpha = active ? 0.95 : e.relation === 'hint' ? (zoomedOut ? 0.18 : 0.34) : zoomedOut ? 0.30 : 0.58
        ctx.strokeStyle = active ? 'rgba(250,204,21,0.92)' : e.relation === 'hint' ? 'rgba(34,211,238,0.22)' : e.auto ? 'rgba(148,163,184,0.18)' : 'rgba(132,204,22,0.32)'
        ctx.lineWidth = active ? 2.25 : e.relation === 'hint' ? Math.max(0.25, 0.7 * cam.scale) : Math.max(0.3, Math.min(1.3, e.weight * 1.25 * cam.scale))
        if (e.relation === 'hint') ctx.setLineDash([2.5, 5])
        else ctx.setLineDash([])
        ctx.beginPath(); ctx.moveTo(ax, ay); ctx.lineTo(bx, by); ctx.stroke()
        ctx.setLineDash([])
      }
      for (const { node, x, y, r } of sim.nodes) {
        const px = sx(x); const py = sy(y)
        const rr = Math.max(1.8, r * Math.max(0.55, Math.min(1.7, cam.scale)))
        if (px < -rr - 40 || py < -rr - 40 || px > rect.width + rr + 40 || py > rect.height + rr + 40) continue
        const dimFilter = sim.visible.size > 0 && !sim.visible.has(node.id)
        const dimHighlight = highlightedId !== null && !sim.neighbors.has(node.id)
        const selected = highlightedId === node.id
        const neighbor = sim.neighbors.has(node.id) && highlightedId !== null && !selected
        ctx.globalAlpha = dimFilter || dimHighlight ? 0.12 : 1

        if ((selected || neighbor || node.degree >= 6) && cam.scale > 0.35) {
          ctx.globalAlpha = selected ? 0.32 : neighbor ? 0.18 : 0.10
          ctx.fillStyle = selected ? '#facc15' : neighbor ? '#22d3ee' : colorForDegree(node.degree)
          ctx.beginPath(); ctx.arc(px, py, rr + (selected ? 10 : 6), 0, Math.PI * 2); ctx.fill()
          ctx.globalAlpha = dimFilter || dimHighlight ? 0.12 : 1
        }

        const grad = ctx.createRadialGradient(px - rr * 0.35, py - rr * 0.35, 0, px, py, rr)
        grad.addColorStop(0, '#ffffff')
        grad.addColorStop(0.18, selected ? '#fde68a' : neighbor ? '#a5f3fc' : '#e0f2fe')
        grad.addColorStop(0.52, selected ? '#facc15' : neighbor ? '#22d3ee' : colorForDegree(node.degree))
        grad.addColorStop(1, '#0f172a')
        ctx.fillStyle = grad
        ctx.beginPath(); ctx.arc(px, py, rr, 0, Math.PI * 2); ctx.fill()

        ctx.globalAlpha = selected ? 0.9 : 0.45
        ctx.strokeStyle = selected ? '#fef3c7' : neighbor ? '#cffafe' : 'rgba(226,232,240,0.45)'
        ctx.lineWidth = selected ? 1.8 : 0.75
        ctx.beginPath(); ctx.arc(px, py, rr, 0, Math.PI * 2); ctx.stroke()

        if (selected) {
          ctx.globalAlpha = 0.98
          ctx.fillStyle = '#fef9c3'
          ctx.font = '600 12px system-ui, sans-serif'
          ctx.fillText(node.label || `#${node.id}`, px + rr + 7, py + 4)
        }
        ctx.globalAlpha = 1
      }
    }
    const loop = () => {
      drawNow()
      if (running) raf = requestAnimationFrame(loop)
    }
    drawRef.current = () => drawNow()
    raf = requestAnimationFrame(loop)
    const ro = new ResizeObserver(() => drawRef.current())
    if (wrapRef.current) ro.observe(wrapRef.current)
    return () => { running = false; cancelAnimationFrame(raf); ro.disconnect() }
  }, [data, highlightedId, pattern])

  const screenToWorld = (clientX: number, clientY: number) => {
    const wrap = wrapRef.current
    if (!wrap) return null
    const rect = wrap.getBoundingClientRect()
    const cam = camera.current
    return {
      x: (clientX - rect.left - rect.width / 2) / cam.scale - cam.x,
      y: (clientY - rect.top - rect.height / 2) / cam.scale - cam.y,
    }
  }

  const pickDrawNode = (clientX: number, clientY: number): DrawNode | null => {
    const sim = simRef.current
    const p = screenToWorld(clientX, clientY)
    if (!sim || !p) return null
    let bestNode: DrawNode | null = null
    let bestD = Infinity
    for (const n of sim.nodes) {
      const d = Math.hypot(n.x - p.x, n.y - p.y)
      if (d < n.r + 12 / camera.current.scale && d < bestD) { bestD = d; bestNode = n }
    }
    return bestNode
  }

  const pickNode = (clientX: number, clientY: number): MemGraphNode | null => pickDrawNode(clientX, clientY)?.node || null

  const pickEdge = (clientX: number, clientY: number): DragEdge | null => {
    const sim = simRef.current
    const p = screenToWorld(clientX, clientY)
    if (!sim || !p) return null
    let best: DragEdge | null = null
    let bestD = Infinity
    const limit = data.edges.length > 3000 ? Math.min(1200, sim.edgePairs.length) : sim.edgePairs.length
    for (let i = 0; i < limit; i++) {
      const e = sim.edgePairs[i]
      const dx = e.b.x - e.a.x
      const dy = e.b.y - e.a.y
      const len2 = dx * dx + dy * dy
      if (len2 < 1) continue
      const t = Math.max(0, Math.min(1, ((p.x - e.a.x) * dx + (p.y - e.a.y) * dy) / len2))
      const cx = e.a.x + dx * t
      const cy = e.a.y + dy * t
      const d = Math.hypot(p.x - cx, p.y - cy)
      if (d < Math.max(8, 10 / camera.current.scale) && d < bestD) { bestD = d; best = { a: e.a, b: e.b } }
    }
    return best
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
    onMouseDown={e => {
      const cam = camera.current
      const node = pickDrawNode(e.clientX, e.clientY)
      const edge = node ? null : pickEdge(e.clientX, e.clientY)
      cam.dragging = true
      cam.moved = false
      cam.lastX = e.clientX
      cam.lastY = e.clientY
      cam.mode = node ? 'node' : edge ? 'edge' : 'camera'
      cam.dragNode = node
      cam.dragEdge = edge
      if (node) {
        node.fixed = true
        node.vx = 0
        node.vy = 0
        onSelect(node.node)
      } else if (edge) {
        edge.a.fixed = true
        edge.b.fixed = true
        edge.a.vx = 0
        edge.a.vy = 0
        edge.b.vx = 0
        edge.b.vy = 0
      }
    }}
    onMouseMove={e => {
      const cam = camera.current
      if (!cam.dragging) return
      const dx = e.clientX - cam.lastX; const dy = e.clientY - cam.lastY
      if (Math.abs(dx) + Math.abs(dy) > 2) cam.moved = true
      if (cam.mode === 'node' && cam.dragNode) {
        cam.dragNode.x += dx / cam.scale
        cam.dragNode.y += dy / cam.scale
        cam.dragNode.vx = 0
        cam.dragNode.vy = 0
        const sim = simRef.current
        if (sim) sim.alpha = Math.max(sim.alpha, 0.35)
      } else if (cam.mode === 'edge' && cam.dragEdge) {
        cam.dragEdge.a.x += dx / cam.scale
        cam.dragEdge.a.y += dy / cam.scale
        cam.dragEdge.b.x += dx / cam.scale
        cam.dragEdge.b.y += dy / cam.scale
        cam.dragEdge.a.vx = 0
        cam.dragEdge.a.vy = 0
        cam.dragEdge.b.vx = 0
        cam.dragEdge.b.vy = 0
        const sim = simRef.current
        if (sim) sim.alpha = Math.max(sim.alpha, 0.28)
      } else {
        cam.x += dx / cam.scale; cam.y += dy / cam.scale
      }
      cam.lastX = e.clientX; cam.lastY = e.clientY
      drawRef.current()
    }}
    onMouseUp={e => {
      const cam = camera.current
      const moved = cam.moved
      const mode = cam.mode
      if (cam.dragNode) cam.dragNode.fixed = false
      if (cam.dragEdge) { cam.dragEdge.a.fixed = false; cam.dragEdge.b.fixed = false }
      cam.dragging = false
      cam.mode = null
      cam.dragNode = null
      cam.dragEdge = null
      if (!moved && mode !== 'edge') onSelect(pickNode(e.clientX, e.clientY))
    }}
    onMouseLeave={() => {
      const cam = camera.current
      if (cam.dragNode) cam.dragNode.fixed = false
      if (cam.dragEdge) { cam.dragEdge.a.fixed = false; cam.dragEdge.b.fixed = false }
      cam.dragging = false
      cam.mode = null
      cam.dragNode = null
      cam.dragEdge = null
    }}
    onDoubleClick={e => {
      const n = pickNode(e.clientX, e.clientY)
      if (n && onFocus) onFocus(n)
    }}
  >
    <canvas ref={canvasRef} className="w-full h-full" />
    <div className="absolute top-3 left-3 flex flex-wrap items-center gap-2 text-[10px] text-smara-100 bg-slate-950/70 border border-smara-400/20 rounded-lg px-2 py-1">
      <span>Obsidian-style Fast LOD · draggable nodes/edges</span>
      <select
        value={pattern}
        onChange={e => setPattern(e.target.value as PatternMode)}
        onMouseDown={e => e.stopPropagation()}
        className="bg-slate-900/90 border border-smara-400/30 rounded px-1 py-0.5 text-smara-50 outline-none"
      >
        {patternModes.map(mode => <option key={mode.id} value={mode.id}>{mode.label}</option>)}
      </select>
    </div>
  </div>
}
