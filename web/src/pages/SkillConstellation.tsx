import { useState, useRef, useEffect, useCallback, useMemo } from 'react'
import { type SkillItem } from '../api'
import { X, Zap, Tag, GitBranch, Layers } from 'lucide-react'

// --- Color palette for tags/categories ---
const PALETTE = [
  '#818cf8', '#f472b6', '#34d399', '#fbbf24', '#60a5fa',
  '#a78bfa', '#fb923c', '#2dd4bf', '#f87171', '#38bdf8',
]

function hashColor(str: string): string {
  let h = 0
  for (let i = 0; i < str.length; i++) h = ((h << 5) - h + str.charCodeAt(i)) | 0
  return PALETTE[Math.abs(h) % PALETTE.length]
}

// --- Layout: radial placement ---
interface PlacedNode {
  skill: SkillItem
  x: number
  y: number
  radius: number
  color: string
  ring: number
  angle: number
}

function buildConstellation(skills: SkillItem[], cx: number, cy: number): PlacedNode[] {
  if (skills.length === 0) return []

  // Group by first category or first tag
  const groups = new Map<string, SkillItem[]>()
  for (const sk of skills) {
    const group = sk.category_path?.[0] || sk.tags?.[0] || 'core'
    if (!groups.has(group)) groups.set(group, [])
    groups.get(group)!.push(sk)
  }

  const groupKeys = Array.from(groups.keys())
  const nodes: PlacedNode[] = []
  const baseRadius = Math.min(cx, cy) * 0.55

  groupKeys.forEach((gk, gi) => {
    const groupSkills = groups.get(gk)!
    const groupAngle = (2 * Math.PI * gi) / Math.max(groupKeys.length, 1)
    const color = hashColor(gk)

    groupSkills.forEach((sk, si) => {
      // Spread skills within group sector
      const sectorSpan = (2 * Math.PI) / Math.max(groupKeys.length, 1) * 0.7
      const offset = groupSkills.length > 1
        ? (si / (groupSkills.length - 1) - 0.5) * sectorSpan
        : 0
      const angle = groupAngle + offset
      const ring = 1 + (si % 3) * 0.3
      const dist = baseRadius * (0.5 + ring * 0.35)
      const nodeRadius = 6 + Math.min(sk.version, 5) * 2

      nodes.push({
        skill: sk,
        x: cx + Math.cos(angle) * dist,
        y: cy + Math.sin(angle) * dist,
        radius: nodeRadius,
        color,
        ring: Math.floor(ring),
        angle,
      })
    })
  })

  return nodes
}

// --- Star background particles ---
function StarField({ width, height }: { width: number; height: number }) {
  const stars = useMemo(() => {
    const s: { x: number; y: number; r: number; o: number }[] = []
    for (let i = 0; i < 120; i++) {
      s.push({
        x: Math.random() * width,
        y: Math.random() * height,
        r: Math.random() * 1.2 + 0.3,
        o: Math.random() * 0.4 + 0.1,
      })
    }
    return s
  }, [width, height])

  return (
    <g>
      {stars.map((s, i) => (
        <circle key={i} cx={s.x} cy={s.y} r={s.r} fill="#fff" opacity={s.o}>
          <animate
            attributeName="opacity"
            values={`${s.o};${s.o * 0.3};${s.o}`}
            dur={`${3 + Math.random() * 4}s`}
            repeatCount="indefinite"
          />
        </circle>
      ))}
    </g>
  )
}

// --- Main Component ---
export default function SkillConstellation({ skills }: { skills: SkillItem[] }) {
  const containerRef = useRef<HTMLDivElement>(null)
  const [size, setSize] = useState({ w: 800, h: 600 })
  const [selected, setSelected] = useState<SkillItem | null>(null)
  const [hovered, setHovered] = useState<string | null>(null)
  const [zoom, setZoom] = useState(1)
  const [pan, setPan] = useState({ x: 0, y: 0 })
  const [dragging, setDragging] = useState(false)
  const dragStart = useRef({ x: 0, y: 0, px: 0, py: 0 })

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
  const nodes = useMemo(() => buildConstellation(skills, cx, cy), [skills, cx, cy])

  // Build dependency map
  const depLines = useMemo(() => {
    const nameMap = new Map<string, PlacedNode>()
    nodes.forEach(n => nameMap.set(n.skill.name, n))
    const lines: { from: PlacedNode; to: PlacedNode }[] = []
    nodes.forEach(n => {
      n.skill.dependencies?.forEach(dep => {
        const target = nameMap.get(dep)
        if (target) lines.push({ from: n, to: target })
      })
    })
    return lines
  }, [nodes])

  // Pan handlers
  const onMouseDown = useCallback((e: React.MouseEvent) => {
    if (e.button !== 0) return
    setDragging(true)
    dragStart.current = { x: e.clientX, y: e.clientY, px: pan.x, py: pan.y }
  }, [pan])

  const onMouseMove = useCallback((e: React.MouseEvent) => {
    if (!dragging) return
    setPan({
      x: dragStart.current.px + (e.clientX - dragStart.current.x) / zoom,
      y: dragStart.current.py + (e.clientY - dragStart.current.y) / zoom,
    })
  }, [dragging, zoom])

  const onMouseUp = useCallback(() => setDragging(false), [])

  const onWheel = useCallback((e: React.WheelEvent) => {
    e.preventDefault()
    setZoom(z => Math.max(0.3, Math.min(3, z - e.deltaY * 0.001)))
  }, [])

  // Orbit ring radii
  const ringRadii = useMemo(() => {
    const base = Math.min(cx, cy) * 0.55
    return [base * 0.5, base * 0.75, base * 1.0, base * 1.25]
  }, [cx, cy])

  return (
    <div ref={containerRef} className="relative w-full h-full overflow-hidden bg-gray-950">
      {/* SVG Canvas */}
      <svg
        width={size.w}
        height={size.h}
        className="absolute inset-0 cursor-grab active:cursor-grabbing"
        onMouseDown={onMouseDown}
        onMouseMove={onMouseMove}
        onMouseUp={onMouseUp}
        onMouseLeave={onMouseUp}
        onWheel={onWheel}
        style={{ userSelect: 'none' }}
      >
        <defs>
          {/* Glow filter */}
          <filter id="glow" x="-50%" y="-50%" width="200%" height="200%">
            <feGaussianBlur stdDeviation="4" result="blur" />
            <feMerge>
              <feMergeNode in="blur" />
              <feMergeNode in="SourceGraphic" />
            </feMerge>
          </filter>
          <filter id="glow-strong" x="-50%" y="-50%" width="200%" height="200%">
            <feGaussianBlur stdDeviation="8" result="blur" />
            <feMerge>
              <feMergeNode in="blur" />
              <feMergeNode in="blur" />
              <feMergeNode in="SourceGraphic" />
            </feMerge>
          </filter>
          {/* Radial gradient for center core */}
          <radialGradient id="core-glow">
            <stop offset="0%" stopColor="#818cf8" stopOpacity="0.6" />
            <stop offset="50%" stopColor="#4f46e5" stopOpacity="0.2" />
            <stop offset="100%" stopColor="#4f46e5" stopOpacity="0" />
          </radialGradient>
        </defs>

        <g transform={`translate(${pan.x * zoom}, ${pan.y * zoom}) scale(${zoom})`}>
          {/* Star background */}
          <StarField width={size.w} height={size.h} />

          {/* Orbit rings */}
          {ringRadii.map((r, i) => (
            <circle
              key={i}
              cx={cx}
              cy={cy}
              r={r}
              fill="none"
              stroke="#374151"
              strokeWidth={0.5}
              strokeDasharray="4,8"
              opacity={0.4}
            />
          ))}

          {/* Center core */}
          <circle cx={cx} cy={cy} r={40} fill="url(#core-glow)" />
          <circle cx={cx} cy={cy} r={8} fill="#6366f1" filter="url(#glow)" opacity={0.9}>
            <animate attributeName="r" values="8;10;8" dur="3s" repeatCount="indefinite" />
          </circle>
          <text x={cx} y={cy + 28} textAnchor="middle" fill="#9ca3af" fontSize="10" fontFamily="Inter, sans-serif">
            Skills
          </text>

          {/* Connection lines from center to nodes */}
          {nodes.map((n, i) => (
            <line
              key={`conn-${i}`}
              x1={cx}
              y1={cy}
              x2={n.x}
              y2={n.y}
              stroke={n.color}
              strokeWidth={0.5}
              opacity={hovered === n.skill.name ? 0.5 : 0.1}
              style={{ transition: 'opacity 0.3s' }}
            />
          ))}

          {/* Dependency lines */}
          {depLines.map((d, i) => (
            <line
              key={`dep-${i}`}
              x1={d.from.x}
              y1={d.from.y}
              x2={d.to.x}
              y2={d.to.y}
              stroke="#f472b6"
              strokeWidth={1}
              strokeDasharray="4,4"
              opacity={0.4}
              markerEnd="none"
            />
          ))}

          {/* Skill nodes */}
          {nodes.map((n, i) => {
            const isHovered = hovered === n.skill.name
            const isSelected = selected?.name === n.skill.name
            const r = isHovered || isSelected ? n.radius * 1.5 : n.radius

            return (
              <g
                key={i}
                style={{ cursor: 'pointer', transition: 'transform 0.2s' }}
                onMouseEnter={() => setHovered(n.skill.name)}
                onMouseLeave={() => setHovered(null)}
                onClick={(e) => { e.stopPropagation(); setSelected(n.skill) }}
              >
                {/* Outer glow */}
                <circle
                  cx={n.x}
                  cy={n.y}
                  r={r * 2.5}
                  fill={n.color}
                  opacity={isHovered || isSelected ? 0.15 : 0.05}
                  style={{ transition: 'opacity 0.3s' }}
                />
                {/* Core */}
                <circle
                  cx={n.x}
                  cy={n.y}
                  r={r}
                  fill={n.color}
                  filter={isHovered || isSelected ? 'url(#glow-strong)' : 'url(#glow)'}
                  opacity={isHovered || isSelected ? 1 : 0.8}
                  style={{ transition: 'all 0.3s' }}
                >
                  {isHovered && (
                    <animate attributeName="r" values={`${r};${r * 1.15};${r}`} dur="1.5s" repeatCount="indefinite" />
                  )}
                </circle>
                {/* Inner bright spot */}
                <circle
                  cx={n.x - r * 0.2}
                  cy={n.y - r * 0.2}
                  r={r * 0.3}
                  fill="#fff"
                  opacity={0.5}
                />
                {/* Label */}
                <text
                  x={n.x}
                  y={n.y + r + 14}
                  textAnchor="middle"
                  fill={isHovered || isSelected ? '#e5e7eb' : '#9ca3af'}
                  fontSize={isHovered || isSelected ? 11 : 9}
                  fontFamily="Inter, sans-serif"
                  fontWeight={isHovered || isSelected ? 600 : 400}
                  style={{ transition: 'all 0.3s', pointerEvents: 'none' }}
                >
                  {n.skill.name.length > 18 ? n.skill.name.slice(0, 16) + '…' : n.skill.name}
                </text>
              </g>
            )
          })}
        </g>

        {/* Legend */}
        <g transform={`translate(16, ${size.h - 80})`}>
          <rect x={0} y={0} width={140} height={70} rx={8} fill="#111827" stroke="#374151" strokeWidth={0.5} opacity={0.85} />
          <text x={10} y={16} fill="#9ca3af" fontSize="9" fontWeight="600">CONSTELLATION MAP</text>
          <circle cx={16} cy={30} r={4} fill="#818cf8" filter="url(#glow)" />
          <text x={26} y={33} fill="#6b7280" fontSize="8">Skill Node</text>
          <line x1={10} y1={44} x2={30} y2={44} stroke="#f472b6" strokeDasharray="3,3" strokeWidth={1} />
          <text x={36} y={47} fill="#6b7280" fontSize="8">Dependency</text>
          <circle cx={16} cy={58} r={3} fill="none" stroke="#374151" strokeDasharray="2,4" />
          <text x={26} y={61} fill="#6b7280" fontSize="8">Orbit Ring</text>
        </g>

        {/* Node count badge */}
        <g transform="translate(16, 16)">
          <rect x={0} y={0} width={120} height={28} rx={6} fill="#111827" stroke="#374151" strokeWidth={0.5} opacity={0.85} />
          <text x={10} y={18} fill="#9ca3af" fontSize="10">
            {nodes.length} skills · {depLines.length} deps
          </text>
        </g>
      </svg>

      {/* Detail Panel */}
      {selected && (
        <div className="absolute bottom-4 right-4 w-80 bg-gray-900/95 backdrop-blur border border-gray-700 rounded-xl shadow-2xl overflow-hidden z-20 animate-in">
          {/* Header */}
          <div className="flex items-center justify-between px-4 py-3 border-b border-gray-800">
            <div className="flex items-center gap-2 min-w-0">
              <Zap className="w-4 h-4 text-smara-400 shrink-0" />
              <span className="text-sm font-semibold text-gray-100 truncate">{selected.name}</span>
              <span className="text-[10px] px-1.5 py-0.5 rounded bg-gray-800 text-gray-400 shrink-0">
                v{selected.version}
              </span>
            </div>
            <button onClick={() => setSelected(null)} className="text-gray-500 hover:text-gray-300 shrink-0 ml-2">
              <X className="w-4 h-4" />
            </button>
          </div>

          <div className="p-4 space-y-3 max-h-64 overflow-y-auto">
            {/* Description */}
            {selected.description && (
              <div>
                <p className="text-xs text-gray-400 leading-relaxed">{selected.description}</p>
              </div>
            )}

            {/* Tags */}
            {selected.tags && selected.tags.length > 0 && (
              <div>
                <div className="flex items-center gap-1 text-[10px] text-gray-500 mb-1.5">
                  <Tag className="w-3 h-3" />
                  <span>Tags</span>
                </div>
                <div className="flex flex-wrap gap-1">
                  {selected.tags.map(t => (
                    <span
                      key={t}
                      className="px-2 py-0.5 rounded-full text-[10px] font-medium"
                      style={{ background: hashColor(t) + '22', color: hashColor(t), border: `1px solid ${hashColor(t)}44` }}
                    >
                      {t}
                    </span>
                  ))}
                </div>
              </div>
            )}

            {/* Category */}
            {selected.category_path && selected.category_path.length > 0 && (
              <div>
                <div className="flex items-center gap-1 text-[10px] text-gray-500 mb-1">
                  <Layers className="w-3 h-3" />
                  <span>Category</span>
                </div>
                <p className="text-xs text-gray-300 font-mono">{selected.category_path.join(' / ')}</p>
              </div>
            )}

            {/* Dependencies */}
            {selected.dependencies && selected.dependencies.length > 0 && (
              <div>
                <div className="flex items-center gap-1 text-[10px] text-gray-500 mb-1">
                  <GitBranch className="w-3 h-3" />
                  <span>Dependencies</span>
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
          </div>
        </div>
      )}

      {/* Empty state */}
      {skills.length === 0 && (
        <div className="absolute inset-0 flex items-center justify-center">
          <div className="text-center text-gray-500">
            <Zap className="w-10 h-10 mx-auto mb-3 text-gray-600" />
            <p className="text-sm font-medium mb-1">No skills found</p>
            <p className="text-xs">Skills will appear as constellation nodes.</p>
          </div>
        </div>
      )}
    </div>
  )
}
