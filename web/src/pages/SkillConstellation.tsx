import { useState, useRef, useEffect, useCallback, useMemo } from 'react'
import { type SkillItem } from '../api'
import { X, Zap, Tag, GitBranch, Layers, Move, RefreshCw, History } from 'lucide-react'
import { getSkillIcon, getCategoryIcon } from './skillIcons'

// ---- Star geometry --------------------------------------------------------

/**
 * Build an SVG path for a 5-pointed (or N-pointed) star centered at origin.
 * `outer` is the distance from center to the tip; `inner` is the inner-vertex
 * radius — a good-looking classic star uses inner ≈ outer × 0.4.
 */
function starPath(outer: number, inner: number, points = 5, rotate = -Math.PI / 2): string {
  const steps = points * 2
  const parts: string[] = []
  for (let i = 0; i < steps; i++) {
    const r = i % 2 === 0 ? outer : inner
    const theta = rotate + (i * Math.PI) / points
    const x = Math.cos(theta) * r
    const y = Math.sin(theta) * r
    parts.push((i === 0 ? 'M' : 'L') + x.toFixed(2) + ',' + y.toFixed(2))
  }
  parts.push('Z')
  return parts.join(' ')
}

// Sensible defaults for different node depths.
const STAR_POINTS_FOR_DEPTH: Record<number, number> = {
  0: 8, // root — 8-point star (big and distinct)
  1: 6, // category — 6-point
  2: 5, // subcategory — classic 5-point
  3: 5, // leaf — 5-point
}

// ---- Palette --------------------------------------------------------------

const PALETTE = [
  '#a3e635', '#84cc16', '#34d399', '#fbbf24', '#d9f99d',
  '#65a30d', '#fb923c', '#2dd4bf', '#f87171', '#a3e635',
]

function hashColor(str: string): string {
  let h = 0
  for (let i = 0; i < str.length; i++) h = ((h << 5) - h + str.charCodeAt(i)) | 0
  return PALETTE[Math.abs(h) % PALETTE.length]
}

// ---- Fractal tree construction -------------------------------------------
//
// Every skill becomes a node. Each node's children orbit around it at a
// scaled radius, and their children orbit around *them* — recursively,
// producing a fractal constellation where the path
//    skill root → category → sub-category → area → leaf
// is a chain of concentric orbits.

interface FractalNode {
  name: string
  label: string        // short display label
  skill: SkillItem | null // null for synthetic category nodes
  children: FractalNode[]
  subtreeSize: number  // total nodes incl. self — drives node radius
  depth: number
  colorKey: string     // key used to pick a deterministic color
  // Computed during layout
  cx: number
  cy: number
  radius: number       // sphere radius
  orbitRadius: number  // distance children orbit at
  angle: number        // angle from parent
}

/**
 * Build a fractal tree:
 *   synthetic "Skills" root
 *     ├── category-1  (first segment of category_path or first tag)
 *     │   ├── subcat-1 (second segment)
 *     │   │   └── skill-a
 *     │   └── skill-b (flat under category if no subcat)
 *     └── category-2
 *
 * Plus for each actual skill, its own parent_id / dependencies are used
 * to nest children inside.
 */
function buildFractal(skills: SkillItem[]): FractalNode {
  const byName = new Map<string, SkillItem>()
  for (const s of skills) byName.set(s.name, s)

  // Parent resolution identical to the hierarchy view so both stay in sync.
  const parentOf = new Map<string, string | null>()
  for (const s of skills) {
    let parent: string | null = null
    if (s.parent_id && byName.has(s.parent_id)) {
      parent = s.parent_id
    } else if (s.dependencies && s.dependencies.length > 0) {
      const firstDep = s.dependencies.find(d => byName.has(d))
      if (firstDep) parent = firstDep
    }
    parentOf.set(s.name, parent)
  }
  // Break cycles
  for (const [name] of parentOf) {
    const seen = new Set<string>()
    let cur: string | null | undefined = name
    while (cur != null) {
      if (seen.has(cur)) {
        parentOf.set(name, null)
        break
      }
      seen.add(cur)
      cur = parentOf.get(cur) ?? null
    }
  }

  const childrenOf = new Map<string, string[]>()
  for (const s of skills) childrenOf.set(s.name, [])
  for (const [c, p] of parentOf) {
    if (p && childrenOf.has(p)) childrenOf.get(p)!.push(c)
  }

  // Sort helper
  const sortSkills = (a: string, b: string) => {
    const sa = byName.get(a)!
    const sb = byName.get(b)!
    const scoreA = (childrenOf.get(a)?.length ?? 0)
    const scoreB = (childrenOf.get(b)?.length ?? 0)
    if (scoreA !== scoreB) return scoreB - scoreA
    return sa.name.localeCompare(sb.name)
  }

  // Build the skill subtree recursively.
  const buildSkillNode = (name: string, depth: number): FractalNode => {
    const sk = byName.get(name)!
    const kids = (childrenOf.get(name) ?? []).sort(sortSkills).map(cn => buildSkillNode(cn, depth + 1))
    const subtreeSize = 1 + kids.reduce((acc, k) => acc + k.subtreeSize, 0)
    return {
      name: sk.name,
      label: sk.name,
      skill: sk,
      children: kids,
      subtreeSize,
      depth,
      colorKey: sk.tags?.[0] || sk.category_path?.[0] || sk.name,
      cx: 0, cy: 0, radius: 0, orbitRadius: 0, angle: 0,
    }
  }

  // Top-level grouping: by first segment of category_path or first tag.
  const roots = skills.filter(s => !parentOf.get(s.name))
  const categoryMap = new Map<string, Map<string, SkillItem[]>>()
  for (const r of roots) {
    const cat = r.category_path?.[0] || r.tags?.[0] || 'Uncategorized'
    const subcat = r.category_path?.[1] || '__direct'
    if (!categoryMap.has(cat)) categoryMap.set(cat, new Map())
    const subMap = categoryMap.get(cat)!
    if (!subMap.has(subcat)) subMap.set(subcat, [])
    subMap.get(subcat)!.push(r)
  }

  // Build category → subcategory → skill structure.
  const categoryNodes: FractalNode[] = []
  for (const [cat, subMap] of categoryMap) {
    const subNodes: FractalNode[] = []
    let catSize = 1

    // Keep skills that had no second segment ("__direct") as direct
    // children of the category to avoid creating a pointless "direct" sub.
    const directSkills = subMap.get('__direct') ?? []
    const subcatKeys = [...subMap.keys()].filter(k => k !== '__direct').sort()

    for (const sc of subcatKeys) {
      const skillNodes = subMap.get(sc)!.map(s => buildSkillNode(s.name, 3))
      const size = 1 + skillNodes.reduce((a, n) => a + n.subtreeSize, 0)
      catSize += size
      subNodes.push({
        name: `${cat}/${sc}`,
        label: sc,
        skill: null,
        children: skillNodes,
        subtreeSize: size,
        depth: 2,
        colorKey: cat,
        cx: 0, cy: 0, radius: 0, orbitRadius: 0, angle: 0,
      })
    }

    const directNodes = directSkills.map(s => buildSkillNode(s.name, 2))
    catSize += directNodes.reduce((a, n) => a + n.subtreeSize, 0)

    categoryNodes.push({
      name: cat,
      label: cat,
      skill: null,
      children: [...subNodes, ...directNodes].sort((a, b) => b.subtreeSize - a.subtreeSize),
      subtreeSize: catSize,
      depth: 1,
      colorKey: cat,
      cx: 0, cy: 0, radius: 0, orbitRadius: 0, angle: 0,
    })
  }

  categoryNodes.sort((a, b) => b.subtreeSize - a.subtreeSize)

  const totalSize = 1 + categoryNodes.reduce((a, n) => a + n.subtreeSize, 0)

  return {
    name: '__root',
    label: 'Skills',
    skill: null,
    children: categoryNodes,
    subtreeSize: totalSize,
    depth: 0,
    colorKey: 'root',
    cx: 0, cy: 0, radius: 0, orbitRadius: 0, angle: 0,
  }
}

// ---- Fractal layout -------------------------------------------------------
//
// Each level's orbit radius shrinks by a depth-dependent factor. Children
// are spread around the parent on evenly-spaced angles. The radius of each
// node scales with log(subtreeSize) so big subtrees are visually dominant.

function layoutFractal(root: FractalNode, cx: number, cy: number, baseOrbit: number) {
  // Root spheres scaled by log of subtree size.
  const radiusFor = (n: FractalNode) => {
    if (n.depth === 0) return 30
    if (n.depth === 1) return 16 + Math.min(Math.log2(n.subtreeSize + 1) * 3.5, 18)
    if (n.depth === 2) return 10 + Math.min(Math.log2(n.subtreeSize + 1) * 2.5, 10)
    return 5 + Math.min(Math.log2(n.subtreeSize + 1) * 1.8, 6)
  }

  // Compute the minimum orbit radius so that child spheres (plus a gap)
  // don't overlap when spread over the given angular span. This is what
  // gives every sub its own "lane" of whitespace around the parent.
  //
  // If children share a full circle and each needs arc-length ≈ 2·r + gap,
  // then circumference ≥ N * (2·r + gap), so orbitRadius ≥ circumference / (2π).
  const minOrbitForChildren = (kids: FractalNode[], span: number, extraGap: number) => {
    if (kids.length === 0) return 0
    // Largest child drives the required arc slot.
    const maxChildR = Math.max(...kids.map(radiusFor))
    const perChildArc = 2 * maxChildR + extraGap
    const requiredCircumference = kids.length * perChildArc
    // Arc length = orbit * span  ⇒  orbit = requiredArc / span.
    return requiredCircumference / Math.max(span, 0.2)
  }

  // Shrink factor per level, but we'll still bump it up whenever the
  // minimum-to-fit orbit is larger than the default.
  const orbitDefault = (depth: number, parentOrbit: number) => {
    if (depth === 0) return baseOrbit
    if (depth === 1) return parentOrbit * 0.55
    if (depth === 2) return parentOrbit * 0.65
    return parentOrbit * 0.7
  }

  // Depth-based visual gap: shrinks with depth but never disappears entirely.
  const gapForDepth = (depth: number) => {
    if (depth === 0) return 70   // between top-level categories
    if (depth === 1) return 40   // between subcategories
    if (depth === 2) return 24   // between leaf skills around a subcategory
    return 16
  }

  const walk = (
    node: FractalNode,
    px: number,
    py: number,
    depth: number,
    parentOrbit: number,
    parentAngle: number,
    spanAngle: number
  ) => {
    node.cx = px
    node.cy = py
    node.radius = radiusFor(node)

    const kids = node.children
    if (kids.length === 0) {
      node.orbitRadius = 0
      return
    }

    // Root gets full circle; children share a fan-shaped span centered on
    // the parent's outward direction so subtrees clearly occupy separate
    // angular regions (making gaps obvious between subs).
    const effectiveSpan = depth === 0
      ? Math.PI * 2
      : Math.min(spanAngle, Math.PI * 1.5)

    const gap = gapForDepth(depth)
    const minOrbit = minOrbitForChildren(kids, effectiveSpan, gap)
    const orbit = Math.max(orbitDefault(depth, parentOrbit), minOrbit)
    node.orbitRadius = orbit

    // Reserve a small margin inside the span so adjacent subtree spans
    // don't touch. Using 85% of the full span on non-root levels gives
    // a visible "break" between neighboring subs.
    const usableSpan = depth === 0 ? effectiveSpan : effectiveSpan * 0.85
    const count = kids.length
    const angleStep = usableSpan / Math.max(count, 1)

    // Centerline of the span is the parent's outward direction (parentAngle).
    // For the root, start at -90° so the first category appears at the top.
    const centerAngle = depth === 0 ? -Math.PI / 2 : parentAngle
    const firstAngle = depth === 0
      ? centerAngle
      : centerAngle - usableSpan / 2 + angleStep / 2

    kids.forEach((kid, i) => {
      const angle = depth === 0
        ? (firstAngle + i * angleStep)
        : (firstAngle + i * angleStep)
      const kx = px + Math.cos(angle) * orbit
      const ky = py + Math.sin(angle) * orbit
      kid.angle = angle

      // Each child gets a narrower cone to spread its own grand-children
      // into — but never smaller than ~60° so deep skills still breathe.
      const childSpan = Math.max(angleStep * 0.9, Math.PI / 3)
      walk(kid, kx, ky, depth + 1, orbit, angle, childSpan)
    })
  }

  walk(root, cx, cy, 0, baseOrbit, 0, Math.PI * 2)
}

// Flatten the tree for rendering. Returns nodes in parent-before-children
// order so draws look correct (edges first, then children, etc.) and also
// returns edges for parent→child lines.
function flattenTree(root: FractalNode): {
  nodes: FractalNode[]
  edges: { from: FractalNode; to: FractalNode }[]
} {
  const nodes: FractalNode[] = []
  const edges: { from: FractalNode; to: FractalNode }[] = []
  const walk = (n: FractalNode) => {
    nodes.push(n)
    for (const c of n.children) {
      edges.push({ from: n, to: c })
      walk(c)
    }
  }
  walk(root)
  return { nodes, edges }
}

// ---- Starfield background -------------------------------------------------

function StarField({ width, height }: { width: number; height: number }) {
  const stars = useMemo(() => {
    const s: { x: number; y: number; r: number; o: number; dur: number }[] = []
    for (let i = 0; i < 150; i++) {
      s.push({
        x: Math.random() * width,
        y: Math.random() * height,
        r: Math.random() * 1.1 + 0.3,
        o: Math.random() * 0.4 + 0.1,
        dur: 3 + Math.random() * 5,
      })
    }
    return s
  }, [width, height])
  return (
    <g>
      {stars.map((s, i) => (
        <circle key={i} cx={s.x} cy={s.y} r={s.r} fill="#fff" opacity={s.o}>
          <animate attributeName="opacity" values={`${s.o};${s.o * 0.3};${s.o}`} dur={`${s.dur}s`} repeatCount="indefinite" />
        </circle>
      ))}
    </g>
  )
}

// ---- Main component -------------------------------------------------------

interface NodeDelta { dx: number; dy: number }

export default function SkillConstellation({ skills }: { skills: SkillItem[] }) {
  const containerRef = useRef<HTMLDivElement>(null)
  const [size, setSize] = useState({ w: 800, h: 600 })
  const [selected, setSelected] = useState<SkillItem | null>(null)
  const [hovered, setHovered] = useState<string | null>(null)
  const [zoom, setZoom] = useState(1)
  const [pan, setPan] = useState({ x: 0, y: 0 })
  const [dragging, setDragging] = useState(false)
  const dragStart = useRef({ x: 0, y: 0, px: 0, py: 0, dx: 0, dy: 0 })

  const [draggedNode, setDraggedNode] = useState<string | null>(null)
  const [nodeDeltas, setNodeDeltas] = useState<Map<string, NodeDelta>>(new Map())

  // Resize observer
  useEffect(() => {
    const el = containerRef.current
    if (!el) return
    const obs = new ResizeObserver(entries => {
      const { width, height } = entries[0].contentRect
      setSize({ w: width, h: height })
    })
    obs.observe(el)
    return () => obs.disconnect()
  }, [])

  const cx = size.w / 2
  const cy = size.h / 2
  // Bigger base orbit so top-level categories start far apart and deeper
  // levels (now pushed outward by child-fit calculation) still fit.
  const baseOrbit = Math.min(cx, cy) * 0.9

  // Build tree & compute layout.
  const { nodes, edges, parentOf } = useMemo(() => {
    const root = buildFractal(skills)
    layoutFractal(root, cx, cy, baseOrbit)
    const flat = flattenTree(root)

    const pOf = new Map<string, string | null>()
    const walkParents = (n: FractalNode, parentName: string | null) => {
      pOf.set(n.name, parentName)
      for (const c of n.children) walkParents(c, n.name)
    }
    walkParents(root, null)

    return { nodes: flat.nodes, edges: flat.edges, parentOf: pOf }
  }, [skills, cx, cy, baseOrbit])

  // Effective (post-drag) position: node's base + cumulative ancestor deltas.
  const effectivePos = useCallback((name: string, baseX: number, baseY: number) => {
    let dx = 0
    let dy = 0
    let cur: string | null = name
    const seen = new Set<string>()
    while (cur != null && !seen.has(cur)) {
      seen.add(cur)
      const d = nodeDeltas.get(cur)
      if (d) {
        dx += d.dx
        dy += d.dy
      }
      cur = parentOf.get(cur) ?? null
    }
    return { x: baseX + dx, y: baseY + dy }
  }, [nodeDeltas, parentOf])

  const draggingRef = useRef(false)
  const draggedNodeRef = useRef<string | null>(null)

  useEffect(() => {
    draggingRef.current = dragging
  }, [dragging])

  useEffect(() => {
    draggedNodeRef.current = draggedNode
  }, [draggedNode])

  // ---- Drag handlers ----

  const onCanvasMouseDown = useCallback((e: React.MouseEvent) => {
    if (e.button !== 0 || draggedNodeRef.current) return
    setDragging(true)
    draggingRef.current = true
    dragStart.current = { x: e.clientX, y: e.clientY, px: pan.x, py: pan.y, dx: 0, dy: 0 }
  }, [pan])

  const moveDrag = useCallback((clientX: number, clientY: number) => {
    const activeNode = draggedNodeRef.current
    if (activeNode) {
      const dx = dragStart.current.dx + (clientX - dragStart.current.x) / zoom
      const dy = dragStart.current.dy + (clientY - dragStart.current.y) / zoom
      setNodeDeltas(prev => {
        const next = new Map(prev)
        next.set(activeNode, { dx, dy })
        return next
      })
      return
    }
    if (!draggingRef.current) return
    setPan({
      x: dragStart.current.px + (clientX - dragStart.current.x) / zoom,
      y: dragStart.current.py + (clientY - dragStart.current.y) / zoom,
    })
  }, [zoom])

  const onCanvasMouseMove = useCallback((e: React.MouseEvent) => {
    moveDrag(e.clientX, e.clientY)
  }, [moveDrag])

  const endDrag = useCallback(() => {
    setDragging(false)
    setDraggedNode(null)
    draggingRef.current = false
    draggedNodeRef.current = null
  }, [])

  useEffect(() => {
    const onMove = (e: MouseEvent) => moveDrag(e.clientX, e.clientY)
    const onUp = () => endDrag()
    window.addEventListener('mousemove', onMove)
    window.addEventListener('mouseup', onUp)
    return () => {
      window.removeEventListener('mousemove', onMove)
      window.removeEventListener('mouseup', onUp)
    }
  }, [moveDrag, endDrag])

  const onNodeMouseDown = useCallback((name: string, e: React.MouseEvent) => {
    e.preventDefault()
    e.stopPropagation()
    if (e.button !== 0) return
    setDraggedNode(name)
    draggedNodeRef.current = name
    const existing = nodeDeltas.get(name) ?? { dx: 0, dy: 0 }
    dragStart.current = {
      x: e.clientX,
      y: e.clientY,
      px: 0,
      py: 0,
      dx: existing.dx,
      dy: existing.dy,
    }
  }, [nodeDeltas])

  const onWheel = useCallback((e: React.WheelEvent) => {
    e.preventDefault()
    setZoom(z => Math.max(0.3, Math.min(3, z - e.deltaY * 0.001)))
  }, [])

  const resetDrags = () => setNodeDeltas(new Map())
  const hasMoved = nodeDeltas.size > 0

  // Edge rendering — use effective positions so dragging propagates.
  const nodePos = useMemo(() => {
    const m = new Map<string, { x: number; y: number; node: FractalNode }>()
    for (const n of nodes) m.set(n.name, { x: n.cx, y: n.cy, node: n })
    return m
  }, [nodes])

  return (
    <div ref={containerRef} className="relative w-full h-full overflow-hidden bg-gray-950">
      {/* Toolbar */}
      <div className="absolute top-3 right-3 z-10 flex items-center gap-2">
        {hasMoved && (
          <button
            onClick={resetDrags}
            className="flex items-center gap-1 px-2.5 py-1.5 text-xs bg-amber-900/40 border border-amber-700 rounded-lg text-amber-200 hover:bg-amber-900/60"
            title="Kembalikan semua node yang digeser"
          >
            <Move className="w-3 h-3" /> Reset posisi ({nodeDeltas.size})
          </button>
        )}
      </div>

      <svg
        width={size.w}
        height={size.h}
        className={`absolute inset-0 ${draggedNode ? 'cursor-grabbing' : 'cursor-grab active:cursor-grabbing'}`}
        onMouseDown={onCanvasMouseDown}
        onMouseMove={onCanvasMouseMove}
        onMouseUp={endDrag}
        onMouseLeave={endDrag}
        onWheel={onWheel}
        style={{ userSelect: 'none' }}
      >
        <defs>
          <filter id="glow" x="-50%" y="-50%" width="200%" height="200%">
            <feGaussianBlur stdDeviation="4" result="blur" />
            <feMerge><feMergeNode in="blur" /><feMergeNode in="SourceGraphic" /></feMerge>
          </filter>
          <filter id="glow-strong" x="-50%" y="-50%" width="200%" height="200%">
            <feGaussianBlur stdDeviation="8" result="blur" />
            <feMerge><feMergeNode in="blur" /><feMergeNode in="blur" /><feMergeNode in="SourceGraphic" /></feMerge>
          </filter>
          <radialGradient id="core-glow">
            <stop offset="0%" stopColor="#a3e635" stopOpacity="0.6" />
            <stop offset="50%" stopColor="#4f46e5" stopOpacity="0.2" />
            <stop offset="100%" stopColor="#4f46e5" stopOpacity="0" />
          </radialGradient>
          {/* Star fill gradient — bright at the core, fading toward the tips
              so the star looks radiant instead of flat. */}
          <radialGradient id="star-shine" cx="50%" cy="50%" r="50%">
            <stop offset="0%" stopColor="#ffffff" stopOpacity="0.95" />
            <stop offset="35%" stopColor="currentColor" stopOpacity="0.9" />
            <stop offset="100%" stopColor="currentColor" stopOpacity="0.4" />
          </radialGradient>
          {/* Subtle crystalline highlight for the biggest stars. */}
          <radialGradient id="star-halo" cx="50%" cy="50%" r="50%">
            <stop offset="0%" stopColor="#fff" stopOpacity="0.8" />
            <stop offset="60%" stopColor="#fff" stopOpacity="0" />
          </radialGradient>
        </defs>

        <g transform={`translate(${pan.x * zoom}, ${pan.y * zoom}) scale(${zoom})`}>
          <StarField width={size.w} height={size.h} />

          {/* Orbit rings around each parent — the visual trace of the fractal */}
          {nodes.filter(n => n.orbitRadius > 0 && n.depth <= 2).map((n, i) => {
            const p = effectivePos(n.name, n.cx, n.cy)
            return (
              <circle
                key={`o-${i}`}
                cx={p.x}
                cy={p.y}
                r={n.orbitRadius}
                fill="none"
                stroke="#374151"
                strokeWidth={0.4}
                strokeDasharray={n.depth === 0 ? '5,10' : '3,6'}
                opacity={n.depth === 0 ? 0.45 : 0.25}
              />
            )
          })}

          {/* Edges: parent → child connection lines */}
          {edges.map((e, i) => {
            const pInfo = nodePos.get(e.from.name)
            const cInfo = nodePos.get(e.to.name)
            if (!pInfo || !cInfo) return null
            const p = effectivePos(e.from.name, pInfo.x, pInfo.y)
            const c = effectivePos(e.to.name, cInfo.x, cInfo.y)
            const color = hashColor(e.from.colorKey)
            const active =
              hovered === e.from.name ||
              hovered === e.to.name ||
              selected?.name === e.from.name ||
              selected?.name === e.to.name
            return (
              <line
                key={`e-${i}`}
                x1={p.x}
                y1={p.y}
                x2={c.x}
                y2={c.y}
                stroke={color}
                strokeWidth={active ? 1.5 : (e.from.depth === 0 ? 0.6 : 0.3)}
                opacity={active ? 0.85 : (e.from.depth === 0 ? 0.35 : 0.18)}
                style={{ transition: draggedNode ? 'none' : 'opacity 0.3s' }}
              />
            )
          })}

          {/* Nodes */}
          {nodes.map((n, i) => {
            const p = effectivePos(n.name, n.cx, n.cy)
            const isHovered = hovered === n.name
            const isSelected = selected?.name === n.name
            const isDragging = draggedNode === n.name
            const color = hashColor(n.colorKey)
            const emphasized = isHovered || isSelected || isDragging
            const r = emphasized ? n.radius * 1.25 : n.radius

            // Root "Skills" node gets a distinct core gradient and
            // a prominent 8-point star so it stands out at the center.
            if (n.name === '__root') {
              return (
                <g key={`n-${i}`}
                  transform={`translate(${p.x}, ${p.y})`}
                  onMouseDown={(e) => onNodeMouseDown(n.name, e)}
                  style={{ cursor: 'grab' }}
                >
                  <circle r={60} fill="url(#core-glow)" />
                  <g filter="url(#glow-strong)" style={{ color: '#6366f1' }}>
                    <path
                      d={starPath(r, r * 0.45, 8)}
                      fill="url(#star-shine)"
                      opacity={0.95}
                    >
                      <animateTransform
                        attributeName="transform"
                        type="rotate"
                        from="0"
                        to="360"
                        dur="60s"
                        repeatCount="indefinite"
                      />
                    </path>
                  </g>
                  <circle r={r * 0.4} fill="url(#star-halo)" />
                  <text y={r + 22} textAnchor="middle" fill="#c7d2fe" fontSize={12} fontWeight={700} letterSpacing={1} style={{ pointerEvents: 'none', textTransform: 'uppercase' }}>
                    {n.label}
                  </text>
                </g>
              )
            }

            // Synthetic category / subcategory node rendered as a 6-pointed
            // star with an emoji label — distinctly bigger than leaf skills.
            if (n.skill === null) {
              const label = n.label.length > 18 ? n.label.slice(0, 17) + '…' : n.label
              const points = STAR_POINTS_FOR_DEPTH[n.depth] ?? 6
              return (
                <g key={`n-${i}`}
                  transform={`translate(${p.x}, ${p.y})`}
                  onMouseDown={(e) => onNodeMouseDown(n.name, e)}
                  onMouseEnter={() => setHovered(n.name)}
                  onMouseLeave={() => setHovered(null)}
                  style={{ cursor: 'grab' }}
                >
                  {emphasized && (
                    <path d={starPath(r * 2.4, r * 0.9, points)} fill={color} opacity={0.15} />
                  )}
                  <g style={{ color }} filter={emphasized ? 'url(#glow-strong)' : 'url(#glow)'}>
                    <path
                      d={starPath(r, r * 0.42, points)}
                      fill="url(#star-shine)"
                      opacity={emphasized ? 1 : 0.92}
                    />
                  </g>
                  <circle r={r * 0.35} fill="url(#star-halo)" />
                  {/* Category emoji inside the star */}
                  <text y={3} textAnchor="middle" fontSize={Math.min(r * 0.9, 14)} style={{ pointerEvents: 'none' }}>
                    {getCategoryIcon(n.label)}
                  </text>
                  <text
                    y={r + 16}
                    textAnchor="middle"
                    fill={emphasized ? '#f3f4f6' : '#d1d5db'}
                    fontSize={n.depth === 1 ? 11 : 10}
                    fontWeight={n.depth === 1 ? 700 : 600}
                    letterSpacing={n.depth === 1 ? 0.5 : 0.2}
                    style={{ pointerEvents: 'none', textTransform: n.depth === 1 ? 'uppercase' : 'none' }}
                  >
                    {label}
                  </text>
                  {n.children.length > 0 && (
                    <g transform={`translate(${r * 0.85}, ${-r * 0.85})`} style={{ pointerEvents: 'none' }}>
                      <circle r={7} fill="#111827" stroke={color} strokeWidth={0.8} />
                      <text y={3} textAnchor="middle" fill={color} fontSize={9} fontWeight={800}>
                        {n.children.length}
                      </text>
                    </g>
                  )}
                </g>
              )
            }

            // Real skill = classic 5-point star. The icon sits inside so
            // each skill type is visually recognizable at a glance.
            const points = STAR_POINTS_FOR_DEPTH[n.depth] ?? 5
            return (
              <g key={`n-${i}`}
                transform={`translate(${p.x}, ${p.y})`}
                onMouseDown={(e) => onNodeMouseDown(n.name, e)}
                onMouseEnter={() => setHovered(n.name)}
                onMouseLeave={() => setHovered(null)}
                onClick={(e) => {
                  e.stopPropagation()
                  if (Math.abs(dragStart.current.dx) < 3 && Math.abs(dragStart.current.dy) < 3) {
                    setSelected(n.skill)
                  }
                }}
                style={{
                  cursor: isDragging ? 'grabbing' : 'grab',
                  transition: isDragging || draggedNode ? 'none' : 'transform 0.2s',
                }}
              >
                {emphasized && (
                  <path d={starPath(r * 2.6, r * 1.0, points)} fill={color} opacity={0.18} />
                )}
                {/* Lineage aureole */}
                {n.skill.lineage && n.skill.lineage.length > 0 && (
                  <path
                    d={starPath(r * 1.3, r * 0.55, points)}
                    fill="none"
                    stroke="#fbbf24"
                    strokeWidth={0.8}
                    strokeDasharray="2,3"
                    opacity={emphasized ? 0.95 : 0.55}
                    style={{ pointerEvents: 'none' }}
                  />
                )}
                <g style={{ color }} filter={emphasized ? 'url(#glow-strong)' : 'url(#glow)'}>
                  <path
                    d={starPath(r, r * 0.4, points)}
                    fill="url(#star-shine)"
                    opacity={emphasized ? 1 : 0.88}
                    style={{ transition: draggedNode ? 'none' : 'all 0.3s' }}
                  >
                    {isHovered && (
                      <animateTransform
                        attributeName="transform"
                        type="rotate"
                        from="0"
                        to="360"
                        dur="10s"
                        repeatCount="indefinite"
                      />
                    )}
                  </path>
                </g>
                {/* Bright inner core */}
                <circle r={r * 0.3} fill="url(#star-halo)" />

                {/* Skill icon inside the star — size scales with node radius */}
                {r > 7 && (
                  <text
                    y={r * 0.25}
                    textAnchor="middle"
                    fontSize={Math.min(r * 0.9, 14)}
                    style={{ pointerEvents: 'none' }}
                  >
                    {getSkillIcon(n.skill)}
                  </text>
                )}

                {/* Version badge (top-right) */}
                {n.skill.version > 1 && r > 8 && (
                  <g transform={`translate(${r * 0.9}, ${-r * 0.9})`} style={{ pointerEvents: 'none' }}>
                    <circle r={6} fill="#111827" stroke={color} strokeWidth={0.8} />
                    <text y={2} textAnchor="middle" fill={color} fontSize={7} fontWeight={800}>
                      v{n.skill.version}
                    </text>
                  </g>
                )}

                {/* Lineage count badge (top-left) */}
                {n.skill.lineage && n.skill.lineage.length > 0 && r > 8 && (
                  <g transform={`translate(${-r * 0.9}, ${-r * 0.9})`} style={{ pointerEvents: 'none' }}>
                    <circle r={6} fill="#fbbf24" stroke="#111827" strokeWidth={0.8} />
                    <text y={2} textAnchor="middle" fill="#111827" fontSize={7} fontWeight={800}>
                      {n.skill.lineage.length + 1}
                    </text>
                  </g>
                )}

                {r > 10 && (
                  <text
                    y={r + 14}
                    textAnchor="middle"
                    fill={emphasized ? '#e5e7eb' : '#9ca3af'}
                    fontSize={emphasized ? 10 : 9}
                    fontWeight={emphasized ? 600 : 400}
                    style={{ pointerEvents: 'none', transition: draggedNode ? 'none' : 'all 0.2s' }}
                  >
                    {n.label.length > 18 ? n.label.slice(0, 16) + '…' : n.label}
                  </text>
                )}
              </g>
            )
          })}
        </g>

        {/* Legend */}
        <g transform={`translate(16, ${size.h - 110})`}>
          <rect x={0} y={0} width={180} height={100} rx={8} fill="#111827" stroke="#374151" strokeWidth={0.5} opacity={0.9} />
          <text x={12} y={18} fill="#c7d2fe" fontSize="10" fontWeight="700" letterSpacing={0.5}>FRACTAL STAR MAP</text>

          <g transform="translate(18, 36)" style={{ color: '#a3e635' }}>
            <path d={starPath(6, 2.5, 5)} fill="url(#star-shine)" />
          </g>
          <text x={32} y={39} fill="#9ca3af" fontSize="9">Skill star (5-point)</text>

          <g transform="translate(18, 54)" style={{ color: '#84cc16' }}>
            <path d={starPath(7, 3, 6)} fill="url(#star-shine)" />
          </g>
          <text x={32} y={57} fill="#9ca3af" fontSize="9">Category (6-point)</text>

          <path d={starPath(5, 2, 5)} transform="translate(18, 72)" fill="none" stroke="#fbbf24" strokeDasharray="2,3" />
          <text x={32} y={75} fill="#9ca3af" fontSize="9">Refined (lineage)</text>

          <text x={12} y={92} fill="#6b7280" fontSize="8" fontStyle="italic">drag untuk geser subtree</text>
        </g>

        <g transform="translate(16, 16)">
          <rect x={0} y={0} width={160} height={28} rx={6} fill="#111827" stroke="#374151" strokeWidth={0.5} opacity={0.88} />
          <text x={10} y={18} fill="#9ca3af" fontSize="10">
            {nodes.filter(n => n.skill).length} skills · {edges.length} edges
          </text>
        </g>
      </svg>

      {/* Detail Panel */}
      {selected && (
        <div className="absolute bottom-4 right-4 w-80 bg-gray-900/95 backdrop-blur ring-1 ring-black/35 rounded-xl shadow-2xl overflow-hidden z-20">
          <div className="flex items-center justify-between px-4 py-3 border-b border-smara-300/12">
            <div className="flex items-center gap-2 min-w-0">
              <Zap className="w-4 h-4 text-smara-400 shrink-0" />
              <span className="text-sm font-semibold text-gray-100 truncate">{selected.name}</span>
              <span className="text-[10px] px-1.5 py-0.5 rounded bg-gray-800 text-gray-400 shrink-0">v{selected.version}</span>
            </div>
            <button onClick={() => setSelected(null)} className="text-gray-500 hover:text-gray-300 shrink-0 ml-2">
              <X className="w-4 h-4" />
            </button>
          </div>

          <div className="p-4 space-y-3 max-h-64 overflow-y-auto">
            {selected.description && <p className="text-xs text-gray-400 leading-relaxed">{selected.description}</p>}

            {selected.tags && selected.tags.length > 0 && (
              <div>
                <div className="flex items-center gap-1 text-[10px] text-gray-500 mb-1.5"><Tag className="w-3 h-3" /><span>Tags</span></div>
                <div className="flex flex-wrap gap-1">
                  {selected.tags.map(t => (
                    <span key={t} className="px-2 py-0.5 rounded-full text-[10px] font-medium" style={{ background: hashColor(t) + '22', color: hashColor(t), border: `1px solid ${hashColor(t)}44` }}>
                      {t}
                    </span>
                  ))}
                </div>
              </div>
            )}

            {selected.category_path && selected.category_path.length > 0 && (
              <div>
                <div className="flex items-center gap-1 text-[10px] text-gray-500 mb-1"><Layers className="w-3 h-3" /><span>Category</span></div>
                <p className="text-xs text-gray-300 font-mono">{selected.category_path.join(' / ')}</p>
              </div>
            )}

            {selected.dependencies && selected.dependencies.length > 0 && (
              <div>
                <div className="flex items-center gap-1 text-[10px] text-gray-500 mb-1"><GitBranch className="w-3 h-3" /><span>Dependencies</span></div>
                <div className="flex flex-wrap gap-1">
                  {selected.dependencies.map(d => (
                    <span key={d} className="px-2 py-0.5 rounded bg-pink-900/30 border border-pink-800/40 text-pink-300 text-[10px]">{d}</span>
                  ))}
                </div>
              </div>
            )}

            {selected.lineage && selected.lineage.length > 0 && (
              <div>
                <div className="flex items-center gap-1 text-[10px] text-gray-500 mb-1">
                  <History className="w-3 h-3" /><span>Riwayat Refinement ({selected.lineage.length + 1} versi)</span>
                </div>
                <ol className="space-y-1 mt-1">
                  <li className="flex items-center gap-2 text-[11px] bg-amber-900/20 border border-amber-800/40 rounded px-2 py-1">
                    <span className="inline-flex items-center justify-center w-5 h-5 rounded-full bg-amber-400/90 text-[10px] font-bold text-amber-950">
                      v{selected.version}
                    </span>
                    <span className="text-amber-200 truncate flex-1">sekarang</span>
                    <RefreshCw className="w-3 h-3 text-amber-400" />
                  </li>
                  {[...selected.lineage].reverse().map((l, idx) => (
                    <li key={`${l.version}-${idx}`} className="flex items-center gap-2 text-[11px] bg-gray-800/40 ring-1 ring-black/30 rounded px-2 py-1">
                      <span className="inline-flex items-center justify-center w-5 h-5 rounded-full bg-gray-700 text-[10px] font-bold text-gray-300">
                        v{l.version}
                      </span>
                      <span className="text-gray-400 truncate flex-1" title={l.description}>
                        {l.refined_from ? (
                          <>
                            <span className="text-gray-500">{l.refined_from}</span>
                            {l.refined_at && <span className="text-gray-600"> · {l.refined_at}</span>}
                          </>
                        ) : (
                          l.refined_at || 'older version'
                        )}
                      </span>
                      <span className="text-gray-600 text-[10px]">{l.step_count} steps</span>
                    </li>
                  ))}
                </ol>
              </div>
            )}
          </div>
        </div>
      )}

      {skills.length === 0 && (
        <div className="absolute inset-0 flex items-center justify-center">
          <div className="text-center text-gray-500">
            <Zap className="w-10 h-10 mx-auto mb-3 text-gray-600" />
            <p className="text-sm font-medium mb-1">No skills found</p>
            <p className="text-xs">Skills will appear as fractal spheres.</p>
          </div>
        </div>
      )}
    </div>
  )
}
