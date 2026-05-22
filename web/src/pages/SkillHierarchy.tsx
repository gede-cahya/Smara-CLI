import { useMemo, useState, useRef, useEffect, useCallback } from 'react'
import { type SkillItem } from '../api'
import { X, Zap, GitBranch, Tag, Layers, Minimize2, Maximize2, Search, RefreshCw, History, Move } from 'lucide-react'
import { getSkillIcon, getCategoryIcon } from './skillIcons'

// Palette reused across views for consistent color coding.
const PALETTE = [
  '#bef264', '#84cc16', '#34d399', '#fbbf24', '#d9f99d',
  '#65a30d', '#fb923c', '#2dd4bf', '#f87171', '#a3e635',
]

function hashColor(str: string): string {
  let h = 0
  for (let i = 0; i < str.length; i++) h = ((h << 5) - h + str.charCodeAt(i)) | 0
  return PALETTE[Math.abs(h) % PALETTE.length]
}

// ---- Tree construction ----------------------------------------------------

interface TreeNode {
  skill: SkillItem
  children: TreeNode[]
  depth: number
  subtreeSize: number
  path: string[]
}

function buildHierarchy(skills: SkillItem[]): TreeNode[] {
  if (skills.length === 0) return []

  const byName = new Map<string, SkillItem>()
  for (const s of skills) byName.set(s.name, s)

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
    const visited = new Set<string>()
    let cur: string | null | undefined = name
    while (cur != null) {
      if (visited.has(cur)) {
        parentOf.set(name, null)
        break
      }
      visited.add(cur)
      cur = parentOf.get(cur) ?? null
    }
  }

  const childrenOf = new Map<string, string[]>()
  for (const s of skills) childrenOf.set(s.name, [])
  for (const [child, parent] of parentOf) {
    if (parent && childrenOf.has(parent)) {
      childrenOf.get(parent)!.push(child)
    }
  }

  const roots = skills.filter(s => !parentOf.get(s.name))
  const categoryGroups = new Map<string, SkillItem[]>()
  for (const r of roots) {
    const cat = r.category_path?.[0] || r.tags?.[0] || 'Uncategorized'
    if (!categoryGroups.has(cat)) categoryGroups.set(cat, [])
    categoryGroups.get(cat)!.push(r)
  }

  const build = (name: string, depth: number, path: string[]): TreeNode => {
    const sk = byName.get(name)!
    const childNames = childrenOf.get(name) ?? []
    const children = childNames.map(cn => build(cn, depth + 1, [...path, name]))
    children.sort((a, b) => b.subtreeSize - a.subtreeSize || a.skill.name.localeCompare(b.skill.name))
    const subtreeSize = 1 + children.reduce((acc, c) => acc + c.subtreeSize, 0)
    return { skill: sk, children, depth, subtreeSize, path }
  }

  const forest: TreeNode[] = []
  for (const [cat, members] of categoryGroups) {
    const nodes = members.map(m => build(m.name, 1, [cat]))
    nodes.sort((a, b) => b.subtreeSize - a.subtreeSize || a.skill.name.localeCompare(b.skill.name))

    const syntheticSkill: SkillItem = {
      name: cat,
      description: `Kategori dengan ${members.length} skill top-level.`,
      version: 0,
      tags: [cat],
    }
    const size = 1 + nodes.reduce((a, c) => a + c.subtreeSize, 0)
    forest.push({
      skill: syntheticSkill,
      children: nodes,
      depth: 0,
      subtreeSize: size,
      path: [],
    })
  }
  forest.sort((a, b) => b.subtreeSize - a.subtreeSize || a.skill.name.localeCompare(b.skill.name))
  return forest
}

// ---- Layout ---------------------------------------------------------------

interface LaidOutNode extends TreeNode {
  baseX: number // original layout position (before user drag)
  baseY: number
  synthetic: boolean
  parentName: string | null
}

interface Layout {
  nodes: LaidOutNode[]
  edges: { parentName: string; childName: string }[]
  width: number
  height: number
}

const LEVEL_HEIGHT = 150
const NODE_SPACING = 210
const SIBLING_GAP = 40      // extra gap between sibling subtrees
const SUBTREE_GAP = 70      // extra gap between separate root trees

function layoutHierarchy(forest: TreeNode[]): Layout {
  const laidOut: LaidOutNode[] = []
  const edges: { parentName: string; childName: string }[] = []

  let cursorX = 0

  const walk = (node: TreeNode, depth: number, parentName: string | null): LaidOutNode => {
    const synthetic = node.depth === 0 && node.skill.version === 0
    let x: number
    if (node.children.length === 0) {
      x = cursorX
      cursorX += NODE_SPACING
    } else {
      const childNodes = node.children.map((c, idx) => {
        const laid = walk(c, depth + 1, node.skill.name)
        // Add extra breathing room between sibling subtrees so the tree
        // does not look cramped when a node has many children.
        if (idx < node.children.length - 1) {
          cursorX += SIBLING_GAP
        }
        return laid
      })
      x = (childNodes[0].baseX + childNodes[childNodes.length - 1].baseX) / 2
      const laidOutCopy: LaidOutNode = { ...node, baseX: x, baseY: depth * LEVEL_HEIGHT, synthetic, parentName }
      laidOut.push(laidOutCopy)
      for (const c of childNodes) {
        edges.push({ parentName: node.skill.name, childName: c.skill.name })
      }
      return laidOutCopy
    }
    const laidOutCopy: LaidOutNode = { ...node, baseX: x, baseY: depth * LEVEL_HEIGHT, synthetic, parentName }
    laidOut.push(laidOutCopy)
    return laidOutCopy
  }

  for (const tree of forest) {
    walk(tree, 0, null)
    cursorX += SUBTREE_GAP // gap between separate root trees
  }

  const maxX = laidOut.reduce((m, n) => Math.max(m, n.baseX), 0)
  const maxY = laidOut.reduce((m, n) => Math.max(m, n.baseY), 0)

  return {
    nodes: laidOut,
    edges,
    width: Math.max(maxX + NODE_SPACING, 400),
    height: Math.max(maxY + LEVEL_HEIGHT, 300),
  }
}

// ---- Rendering ------------------------------------------------------------

interface Props {
  skills: SkillItem[]
}

interface NodeDelta { dx: number; dy: number }

export default function SkillHierarchy({ skills }: Props) {
  const containerRef = useRef<HTMLDivElement>(null)
  const [viewport, setViewport] = useState({ w: 800, h: 600 })
  const [selected, setSelected] = useState<SkillItem | null>(null)
  const [hovered, setHovered] = useState<string | null>(null)
  const [collapsed, setCollapsed] = useState<Set<string>>(new Set())
  const [zoom, setZoom] = useState(1)
  const [pan, setPan] = useState({ x: 40, y: 40 })
  const [dragging, setDragging] = useState(false)
  const [query, setQuery] = useState('')

  // Per-node drag displacement (applied on top of layout base).
  // Dragging a parent propagates its delta to all descendants so the
  // subtree keeps its shape; the delta is stored only on the dragged
  // node itself and computed on render by walking the parent chain.
  const [nodeDeltas, setNodeDeltas] = useState<Map<string, NodeDelta>>(new Map())
  const [draggedNode, setDraggedNode] = useState<string | null>(null)

  const dragStart = useRef({ x: 0, y: 0, px: 0, py: 0, dx: 0, dy: 0 })

  useEffect(() => {
    const el = containerRef.current
    if (!el) return
    const obs = new ResizeObserver(entries => {
      const { width, height } = entries[0].contentRect
      setViewport({ w: width, h: height })
    })
    obs.observe(el)
    return () => obs.disconnect()
  }, [])

  const filteredSkills = useMemo(() => {
    const q = query.trim().toLowerCase()
    if (!q) return skills
    return skills.filter(s =>
      s.name.toLowerCase().includes(q) ||
      (s.description || '').toLowerCase().includes(q) ||
      (s.tags || []).some(t => t.toLowerCase().includes(q))
    )
  }, [skills, query])

  const visibleTree = useMemo(() => {
    const forest = buildHierarchy(filteredSkills)
    const prune = (node: TreeNode): TreeNode => {
      if (collapsed.has(node.skill.name)) {
        return { ...node, children: [] }
      }
      return { ...node, children: node.children.map(prune) }
    }
    return forest.map(prune)
  }, [filteredSkills, collapsed])

  const layout = useMemo(() => layoutHierarchy(visibleTree), [visibleTree])

  // parentOf map for delta propagation.
  const parentOf = useMemo(() => {
    const m = new Map<string, string | null>()
    for (const n of layout.nodes) m.set(n.skill.name, n.parentName)
    return m
  }, [layout.nodes])

  // Resolve effective (x, y) for a node after applying drag deltas from
  // itself plus all ancestors. This is why dragging a parent moves the
  // whole subtree visually.
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

  const nameToBase = useMemo(() => {
    const m = new Map<string, { x: number; y: number; node: LaidOutNode }>()
    for (const n of layout.nodes) m.set(n.skill.name, { x: n.baseX, y: n.baseY, node: n })
    return m
  }, [layout.nodes])

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
    if (e.button !== 0) return
    if (draggedNodeRef.current) return // node drag takes precedence
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
      x: dragStart.current.px + (clientX - dragStart.current.x),
      y: dragStart.current.py + (clientY - dragStart.current.y),
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
    setZoom(z => Math.max(0.3, Math.min(2.5, z - e.deltaY * 0.001)))
  }, [])

  const fit = useCallback(() => {
    if (layout.width === 0 || layout.height === 0) return
    const padding = 60
    const zx = (viewport.w - padding * 2) / layout.width
    const zy = (viewport.h - padding * 2) / layout.height
    const z = Math.min(zx, zy, 1.2)
    setZoom(z)
    setPan({ x: (viewport.w - layout.width * z) / 2, y: padding })
  }, [layout.width, layout.height, viewport.w, viewport.h])

  useEffect(() => {
    if (layout.nodes.length > 0) fit()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [layout.width, layout.height])

  const toggleCollapsed = (name: string) => {
    setCollapsed(prev => {
      const next = new Set(prev)
      if (next.has(name)) next.delete(name)
      else next.add(name)
      return next
    })
  }

  const collapseAll = () => {
    const all = new Set<string>()
    for (const n of layout.nodes) {
      if (n.subtreeSize > 1) all.add(n.skill.name)
    }
    setCollapsed(all)
  }
  const expandAll = () => setCollapsed(new Set())
  const resetDrags = () => setNodeDeltas(new Map())

  const linkPath = (px: number, py: number, cx: number, cy: number) => {
    const midY = (py + cy) / 2
    return `M ${px} ${py + 22} C ${px} ${midY}, ${cx} ${midY}, ${cx} ${cy - 22}`
  }

  const hasMoved = nodeDeltas.size > 0

  return (
    <div ref={containerRef} className="relative w-full h-full overflow-hidden bg-gray-950">
      {/* Toolbar */}
      <div className="absolute top-3 left-3 right-3 z-10 flex items-center gap-2 flex-wrap">
        <div className="flex-1 min-w-[200px] max-w-md relative">
          <Search className="absolute left-2.5 top-1/2 -translate-y-1/2 w-3.5 h-3.5 text-gray-500" />
          <input
            value={query}
            onChange={e => setQuery(e.target.value)}
            placeholder="Cari skill, tag, atau deskripsi…"
            className="w-full pl-8 pr-3 py-1.5 text-xs bg-gray-900/80 ring-1 ring-black/35 rounded-lg text-gray-200 placeholder-gray-500 focus:outline-none focus:ring-1 focus:ring-smara-500"
          />
        </div>
        <button onClick={expandAll} className="flex items-center gap-1 px-2.5 py-1.5 text-xs bg-gray-900/80 ring-1 ring-black/35 rounded-lg text-gray-300 hover:bg-gray-800" title="Expand semua">
          <Maximize2 className="w-3 h-3" /> Expand
        </button>
        <button onClick={collapseAll} className="flex items-center gap-1 px-2.5 py-1.5 text-xs bg-gray-900/80 ring-1 ring-black/35 rounded-lg text-gray-300 hover:bg-gray-800" title="Collapse semua">
          <Minimize2 className="w-3 h-3" /> Collapse
        </button>
        <button onClick={fit} className="px-2.5 py-1.5 text-xs bg-gray-900/80 ring-1 ring-black/35 rounded-lg text-gray-300 hover:bg-gray-800">
          Fit
        </button>
        {hasMoved && (
          <button
            onClick={resetDrags}
            className="flex items-center gap-1 px-2.5 py-1.5 text-xs bg-amber-900/40 border border-amber-700 rounded-lg text-amber-200 hover:bg-amber-900/60"
            title="Reset semua node yang digeser ke posisi auto"
          >
            <Move className="w-3 h-3" /> Reset posisi
          </button>
        )}
        <div className="text-xs text-gray-500 ml-auto bg-gray-900/70 ring-1 ring-black/30 rounded px-2 py-1">
          {layout.nodes.filter(n => !n.synthetic).length} skills · {layout.edges.length} links
          {hasMoved && <span className="ml-2 text-amber-400">· {nodeDeltas.size} digeser</span>}
        </div>
      </div>

      {/* SVG canvas */}
      <svg
        width={viewport.w}
        height={viewport.h}
        className={`absolute inset-0 ${draggedNode ? 'cursor-grabbing' : 'cursor-grab active:cursor-grabbing'}`}
        onMouseDown={onCanvasMouseDown}
        onMouseMove={onCanvasMouseMove}
        onMouseUp={endDrag}
        onMouseLeave={endDrag}
        onWheel={onWheel}
        style={{ userSelect: 'none' }}
      >
        <defs>
          <filter id="tree-glow" x="-50%" y="-50%" width="200%" height="200%">
            <feGaussianBlur stdDeviation="3" result="blur" />
            <feMerge>
              <feMergeNode in="blur" />
              <feMergeNode in="SourceGraphic" />
            </feMerge>
          </filter>
          <linearGradient id="link-grad" x1="0%" y1="0%" x2="0%" y2="100%">
            <stop offset="0%" stopColor="#4b5563" stopOpacity="0.7" />
            <stop offset="100%" stopColor="#4b5563" stopOpacity="0.25" />
          </linearGradient>
        </defs>

        <g transform={`translate(${pan.x}, ${pan.y}) scale(${zoom})`}>
          {/* Edges — use effective positions so they follow dragged nodes */}
          {layout.edges.map((e, i) => {
            const pBase = nameToBase.get(e.parentName)
            const cBase = nameToBase.get(e.childName)
            if (!pBase || !cBase) return null
            const p = effectivePos(e.parentName, pBase.x, pBase.y)
            const c = effectivePos(e.childName, cBase.x, cBase.y)
            const active =
              hovered === e.parentName ||
              hovered === e.childName ||
              selected?.name === e.parentName ||
              selected?.name === e.childName
            return (
              <path
                key={`e-${i}`}
                d={linkPath(p.x, p.y, c.x, c.y)}
                fill="none"
                stroke={active ? '#bef264' : 'url(#link-grad)'}
                strokeWidth={active ? 2 : 1.2}
                opacity={active ? 0.95 : 0.55}
                style={{ transition: draggedNode ? 'none' : 'all 0.2s' }}
              />
            )
          })}

          {/* Nodes */}
          {layout.nodes.map((n, i) => {
            const isHovered = hovered === n.skill.name
            const isSelected = selected?.name === n.skill.name
            const isDragging = draggedNode === n.skill.name
            const color = hashColor(n.skill.tags?.[0] || n.skill.category_path?.[0] || n.skill.name)
            const base = n.synthetic ? 26 : 16 + Math.min(Math.log2(n.subtreeSize + 1) * 4, 14)
            const r = isHovered || isSelected || isDragging ? base * 1.15 : base
            const hasChildren = n.subtreeSize > 1
            const isCollapsed = collapsed.has(n.skill.name)
            const pos = effectivePos(n.skill.name, n.baseX, n.baseY)

            return (
              <g
                key={`n-${i}`}
                transform={`translate(${pos.x}, ${pos.y})`}
                style={{
                  cursor: isDragging ? 'grabbing' : 'grab',
                  transition: isDragging || draggedNode ? 'none' : 'transform 0.2s',
                }}
                onMouseDown={(e) => onNodeMouseDown(n.skill.name, e)}
                onMouseEnter={() => setHovered(n.skill.name)}
                onMouseLeave={() => setHovered(null)}
                onClick={(e) => {
                  e.stopPropagation()
                  if (n.synthetic) return
                  // Suppress selection if the user was actually dragging.
                  if (Math.abs(dragStart.current.dx) < 3 && Math.abs(dragStart.current.dy) < 3) {
                    setSelected(n.skill)
                  }
                }}
              >
                {(isHovered || isSelected || isDragging) && (
                  <circle r={r * 1.8} fill={color} opacity={0.12} />
                )}

                {n.synthetic ? (
                  <>
                    <rect
                      x={-r - 6}
                      y={-16}
                      width={(r + 6) * 2}
                      height={32}
                      rx={16}
                      fill={color}
                      opacity={0.18}
                      stroke={color}
                      strokeWidth={1.2}
                    />
                    <text
                      y={-1}
                      textAnchor="middle"
                      fontSize={13}
                      style={{ pointerEvents: 'none' }}
                    >
                      {getCategoryIcon(n.skill.name)}
                    </text>
                    <text
                      y={14}
                      textAnchor="middle"
                      fill={color}
                      fontSize={10}
                      fontFamily="Inter, sans-serif"
                      fontWeight={700}
                      letterSpacing={0.3}
                      style={{ textTransform: 'uppercase', pointerEvents: 'none' }}
                    >
                      {n.skill.name.length > 14 ? n.skill.name.slice(0, 13) + '…' : n.skill.name}
                    </text>
                  </>
                ) : (
                  <>
                    <circle
                      r={r}
                      fill={color}
                      opacity={isHovered || isSelected || isDragging ? 1 : 0.88}
                      filter="url(#tree-glow)"
                      style={{ transition: draggedNode ? 'none' : 'all 0.2s' }}
                    />
                    <circle r={r * 0.35} cx={-r * 0.25} cy={-r * 0.25} fill="#fff" opacity={0.45} />

                    {/* Skill-type emoji sits inside the sphere. Scales with node radius. */}
                    <text
                      y={r > 18 ? 4 : 3}
                      textAnchor="middle"
                      fontSize={Math.min(r * 1.0, 18)}
                      style={{ pointerEvents: 'none' }}
                    >
                      {getSkillIcon(n.skill)}
                    </text>

                    {/* Version badge moved to a small corner chip so it doesn't
                        overlap the icon. */}
                    {n.skill.version > 1 && (
                      <g transform={`translate(${r * 0.75}, ${r * 0.75})`} style={{ pointerEvents: 'none' }}>
                        <circle r={7} fill="#111827" stroke={color} strokeWidth={1} />
                        <text y={3} textAnchor="middle" fill={color} fontSize={8} fontWeight={800}>
                          v{n.skill.version}
                        </text>
                      </g>
                    )}
                    {n.skill.lineage && n.skill.lineage.length > 0 && (
                      <g style={{ pointerEvents: 'none' }}>
                        <circle r={r + 3} fill="none" stroke="#fbbf24" strokeWidth={1} strokeDasharray="2,3" opacity={isHovered || isSelected ? 0.95 : 0.6} />
                        <g transform={`translate(${-r * 0.75}, ${-r * 0.75})`}>
                          <circle r={6} fill="#fbbf24" stroke="#111827" strokeWidth={1} />
                          <text y={3} textAnchor="middle" fill="#111827" fontSize={8} fontWeight={800}>
                            {n.skill.lineage.length + 1}
                          </text>
                        </g>
                      </g>
                    )}
                    <text
                      y={r + 14}
                      textAnchor="middle"
                      fill={isHovered || isSelected ? '#e5e7eb' : '#9ca3af'}
                      fontSize={isHovered || isSelected ? 11 : 10}
                      fontFamily="Inter, sans-serif"
                      fontWeight={isHovered || isSelected ? 600 : 400}
                      style={{ pointerEvents: 'none', transition: draggedNode ? 'none' : 'all 0.2s' }}
                    >
                      {n.skill.name.length > 20 ? n.skill.name.slice(0, 19) + '…' : n.skill.name}
                    </text>

                    {hasChildren && !isCollapsed && (
                      <g
                        transform={`translate(0, ${r + 26})`}
                        onClick={(ev) => { ev.stopPropagation(); toggleCollapsed(n.skill.name) }}
                        onMouseDown={(ev) => ev.stopPropagation()}
                        style={{ cursor: 'pointer' }}
                      >
                        <circle r={7} fill="#1f2937" stroke="#4b5563" strokeWidth={0.8} />
                        <text y={3} textAnchor="middle" fill="#9ca3af" fontSize={10} fontWeight={700}>−</text>
                      </g>
                    )}
                    {isCollapsed && (
                      <g
                        transform={`translate(0, ${r + 26})`}
                        onClick={(ev) => { ev.stopPropagation(); toggleCollapsed(n.skill.name) }}
                        onMouseDown={(ev) => ev.stopPropagation()}
                        style={{ cursor: 'pointer' }}
                      >
                        <circle r={7} fill="#1f2937" stroke={color} strokeWidth={1.2} />
                        <text y={3} textAnchor="middle" fill={color} fontSize={10} fontWeight={700}>+</text>
                      </g>
                    )}
                  </>
                )}
              </g>
            )
          })}
        </g>

        {layout.nodes.length === 0 && (
          <g transform={`translate(${viewport.w / 2}, ${viewport.h / 2})`}>
            <text textAnchor="middle" fill="#6b7280" fontSize={12}>
              {query ? `Tidak ada skill yang cocok dengan "${query}"` : 'Belum ada skill untuk divisualisasikan'}
            </text>
          </g>
        )}
      </svg>

      {/* Hint for drag */}
      <div className="absolute bottom-3 left-3 bg-gray-900/80 ring-1 ring-black/30 rounded px-2.5 py-1.5 text-[10px] text-gray-500 pointer-events-none">
        💡 Drag node untuk geser sub-tree · drag area kosong untuk pan · scroll untuk zoom
      </div>

      {/* Detail panel */}
      {selected && (
        <div className="absolute bottom-4 right-4 w-80 bg-gray-900/95 backdrop-blur ring-1 ring-black/35 rounded-xl shadow-2xl overflow-hidden z-20">
          <div className="flex items-center justify-between px-4 py-3 border-b border-smara-300/12">
            <div className="flex items-center gap-2 min-w-0">
              <Zap className="w-4 h-4 text-smara-400 shrink-0" />
              <span className="text-sm font-semibold text-gray-100 truncate">{selected.name}</span>
              {selected.version > 0 && (
                <span className="text-[10px] px-1.5 py-0.5 rounded bg-gray-800 text-gray-400 shrink-0">
                  v{selected.version}
                </span>
              )}
            </div>
            <button onClick={() => setSelected(null)} className="text-gray-500 hover:text-gray-300 shrink-0 ml-2">
              <X className="w-4 h-4" />
            </button>
          </div>

          <div className="p-4 space-y-3 max-h-64 overflow-y-auto">
            {selected.description && (
              <p className="text-xs text-gray-400 leading-relaxed">{selected.description}</p>
            )}

            {selected.tags && selected.tags.length > 0 && (
              <div>
                <div className="flex items-center gap-1 text-[10px] text-gray-500 mb-1.5">
                  <Tag className="w-3 h-3" /><span>Tags</span>
                </div>
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
                <div className="flex items-center gap-1 text-[10px] text-gray-500 mb-1">
                  <Layers className="w-3 h-3" /><span>Category</span>
                </div>
                <p className="text-xs text-gray-300 font-mono">{selected.category_path.join(' / ')}</p>
              </div>
            )}

            {selected.dependencies && selected.dependencies.length > 0 && (
              <div>
                <div className="flex items-center gap-1 text-[10px] text-gray-500 mb-1">
                  <GitBranch className="w-3 h-3" /><span>Dependencies</span>
                </div>
                <div className="flex flex-wrap gap-1">
                  {selected.dependencies.map(d => (
                    <span key={d} className="px-2 py-0.5 rounded bg-pink-900/30 border border-pink-800/40 text-pink-300 text-[10px]">
                      {d}
                    </span>
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
    </div>
  )
}
