import { useMemo, useRef, useState, useEffect, Suspense } from 'react'
import { Canvas, useFrame, useThree, type ThreeEvent } from '@react-three/fiber'
import { OrbitControls, Stars, Html, Line } from '@react-three/drei'
import * as THREE from 'three'
import { fetchJSON, type SkillItem } from '../api'
import { X, Zap, Tag, GitBranch, Layers, History, RefreshCw, Search, Play, Network, Maximize2 } from 'lucide-react'
import { getSkillIcon } from './skillIcons'

// ---- Color palette ---------------------------------------------------------

const PALETTE = [
  '#bef264', '#84cc16', '#34d399', '#fbbf24', '#d9f99d',
  '#65a30d', '#fb923c', '#2dd4bf', '#f87171', '#a3e635',
]

function hashColor(str: string): string {
  let h = 0
  for (let i = 0; i < str.length; i++) h = ((h << 5) - h + str.charCodeAt(i)) | 0
  return PALETTE[Math.abs(h) % PALETTE.length]
}

function uniqueSorted(values: string[]): string[] {
  return [...new Set(values.filter(Boolean))].sort((a, b) => a.localeCompare(b))
}

function matchesSkillQuery(skill: SkillItem, q: string): boolean {
  if (!q.trim()) return true
  const needle = q.toLowerCase()
  const haystack = [
    skill.name,
    skill.description,
    ...(skill.tags || []),
    ...(skill.category_path || []),
    ...(skill.dependencies || []),
  ].join(' ').toLowerCase()
  return haystack.includes(needle)
}

function relatedSkillNames(skills: SkillItem[], center: SkillItem | null, depth: number): Set<string> {
  const result = new Set<string>()
  if (!center) return result
  const byName = new Map(skills.map(s => [s.name, s]))
  const neighbors = (name: string): string[] => {
    const sk = byName.get(name)
    const out = new Set<string>()
    if (sk?.parent_id && byName.has(sk.parent_id)) out.add(sk.parent_id)
    for (const d of sk?.dependencies || []) if (byName.has(d)) out.add(d)
    for (const other of skills) {
      if (other.parent_id === name) out.add(other.name)
      if ((other.dependencies || []).includes(name)) out.add(other.name)
    }
    return [...out]
  }

  let frontier = [center.name]
  result.add(center.name)
  for (let i = 0; i < depth; i++) {
    const next: string[] = []
    for (const name of frontier) {
      for (const n of neighbors(name)) {
        if (!result.has(n)) {
          result.add(n)
          next.push(n)
        }
      }
    }
    frontier = next
  }
  return result
}

// ---- Tree construction (mirrors 2D views) ----------------------------------

interface FractalNode3D {
  name: string
  label: string
  skill: SkillItem | null
  children: FractalNode3D[]
  subtreeSize: number
  depth: number
  colorKey: string
  // Computed during layout
  position: THREE.Vector3
  radius: number
}

function buildFractal3D(skills: SkillItem[]): FractalNode3D {
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

  const sortSkills = (a: string, b: string) => {
    const sa = byName.get(a)!
    const sb = byName.get(b)!
    const scoreA = childrenOf.get(a)?.length ?? 0
    const scoreB = childrenOf.get(b)?.length ?? 0
    if (scoreA !== scoreB) return scoreB - scoreA
    return sa.name.localeCompare(sb.name)
  }

  const buildSkillNode = (name: string, depth: number): FractalNode3D => {
    const sk = byName.get(name)!
    const kids = (childrenOf.get(name) ?? []).sort(sortSkills).map(cn => buildSkillNode(cn, depth + 1))
    const subtreeSize = 1 + kids.reduce((a, k) => a + k.subtreeSize, 0)
    return {
      name: sk.name,
      label: sk.name,
      skill: sk,
      children: kids,
      subtreeSize,
      depth,
      colorKey: sk.tags?.[0] || sk.category_path?.[0] || sk.name,
      position: new THREE.Vector3(),
      radius: 0,
    }
  }

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

  const categoryNodes: FractalNode3D[] = []
  for (const [cat, subMap] of categoryMap) {
    const subNodes: FractalNode3D[] = []
    let catSize = 1
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
        position: new THREE.Vector3(),
        radius: 0,
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
      position: new THREE.Vector3(),
      radius: 0,
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
    position: new THREE.Vector3(0, 0, 0),
    radius: 0,
  }
}

// ---- Fractal 3D layout -----------------------------------------------------

function layoutFractal3D(root: FractalNode3D, baseOrbit: number) {
  const radiusFor = (n: FractalNode3D) => {
    if (n.depth === 0) return 4
    if (n.depth === 1) return 2 + Math.min(Math.log2(n.subtreeSize + 1) * 0.5, 2.5)
    if (n.depth === 2) return 1.2 + Math.min(Math.log2(n.subtreeSize + 1) * 0.4, 1.4)
    return 0.7 + Math.min(Math.log2(n.subtreeSize + 1) * 0.3, 0.8)
  }

  const orbitDefault = (depth: number, parentOrbit: number) => {
    if (depth === 0) return baseOrbit
    if (depth === 1) return parentOrbit * 0.5
    if (depth === 2) return parentOrbit * 0.55
    return parentOrbit * 0.6
  }

  const minOrbitForKids = (kids: FractalNode3D[], gap: number) => {
    if (kids.length === 0) return 0
    const maxR = Math.max(...kids.map(radiusFor))
    const slot = 2 * maxR + gap
    return Math.sqrt((kids.length * slot * slot) / (4 * Math.PI))
  }

  const gapForDepth = (depth: number) => {
    if (depth === 0) return 4
    if (depth === 1) return 2
    if (depth === 2) return 1.2
    return 0.6
  }

  const walk = (node: FractalNode3D, center: THREE.Vector3, depth: number, parentOrbit: number) => {
    node.position.copy(center)
    node.radius = radiusFor(node)
    const kids = node.children
    if (kids.length === 0) return

    const gap = gapForDepth(depth)
    const orbit = Math.max(orbitDefault(depth, parentOrbit), minOrbitForKids(kids, gap))
    const N = kids.length
    const phi = Math.PI * (3 - Math.sqrt(5))
    kids.forEach((kid, i) => {
      const y = 1 - (i / Math.max(N - 1, 1)) * 1.6
      const radiusAtY = Math.sqrt(Math.max(0.001, 1 - y * y))
      const theta = phi * i
      const dir = new THREE.Vector3(
        Math.cos(theta) * radiusAtY,
        y * 0.85,
        Math.sin(theta) * radiusAtY,
      )
      const childCenter = center.clone().addScaledVector(dir, orbit)
      walk(kid, childCenter, depth + 1, orbit)
    })
  }

  walk(root, new THREE.Vector3(0, 0, 0), 0, baseOrbit)
}

function flattenTree(root: FractalNode3D): {
  nodes: FractalNode3D[]
  edges: { from: FractalNode3D; to: FractalNode3D }[]
} {
  const nodes: FractalNode3D[] = []
  const edges: { from: FractalNode3D; to: FractalNode3D }[] = []
  const walk = (n: FractalNode3D) => {
    nodes.push(n)
    for (const c of n.children) {
      edges.push({ from: n, to: c })
      walk(c)
    }
  }
  walk(root)
  return { nodes, edges }
}

// ---- 3D node components ----------------------------------------------------

interface NodeProps {
  node: FractalNode3D
  hovered: boolean
  selected: boolean
  active: boolean
  dimmed: boolean
  onPointerOver: (e: ThreeEvent<PointerEvent>) => void
  onPointerOut: () => void
  onClick: () => void
}

function StarMesh({ node, hovered, selected, active, dimmed, onPointerOver, onPointerOut, onClick }: NodeProps) {
  const meshRef = useRef<THREE.Mesh>(null)
  const color = useMemo(() => new THREE.Color(hashColor(node.colorKey)), [node.colorKey])
  const emphasized = hovered || selected
  const visibleOpacity = dimmed ? 0.18 : 1

  useFrame((_, delta) => {
    if (meshRef.current) {
      meshRef.current.rotation.y += delta * (emphasized ? 0.6 : 0.15)
      meshRef.current.rotation.x += delta * 0.05
    }
  })

  const geo = useMemo(() => new THREE.IcosahedronGeometry(node.radius, 1), [node.radius])
  const scale = emphasized ? 1.28 : active ? 1.12 : 1.0

  return (
    <group position={node.position}>
      {(emphasized || active) && (
        <mesh>
          <sphereGeometry args={[node.radius * (emphasized ? 2.4 : 1.8), 16, 16]} />
          <meshBasicMaterial color={color} transparent opacity={emphasized ? 0.1 : 0.045} depthWrite={false} />
        </mesh>
      )}

      <mesh
        ref={meshRef}
        geometry={geo}
        scale={scale}
        onPointerOver={onPointerOver}
        onPointerOut={onPointerOut}
        onClick={onClick}
      >
        <meshStandardMaterial
          color={color}
          emissive={color}
          emissiveIntensity={dimmed ? 0.08 : emphasized ? 0.95 : active ? 0.7 : 0.5}
          metalness={0.3}
          roughness={0.4}
          transparent
          opacity={visibleOpacity}
        />
      </mesh>

      <mesh>
        <sphereGeometry args={[node.radius * 0.4, 12, 12]} />
        <meshBasicMaterial color="#ffffff" transparent opacity={dimmed ? 0.2 : 1} />
      </mesh>

      {node.skill?.lineage && node.skill.lineage.length > 0 && (
        <mesh>
          <icosahedronGeometry args={[node.radius * 1.4, 0]} />
          <meshBasicMaterial color="#fbbf24" wireframe transparent opacity={dimmed ? 0.08 : emphasized ? 0.7 : 0.35} />
        </mesh>
      )}

      {selected && node.skill && (
        <Html
          center
          position={[0, node.radius + 0.6, 0]}
          style={{
            pointerEvents: 'none',
            fontFamily: 'Inter, sans-serif',
            fontSize: '11px',
            fontWeight: 600,
            color: '#f3f4f6',
            textShadow: '0 1px 4px rgba(0,0,0,0.9)',
            whiteSpace: 'nowrap',
            userSelect: 'none',
          }}
        >
          {getSkillIcon(node.skill)}{' '}
          {node.label.length > 20 ? node.label.slice(0, 19) + '…' : node.label}
          {node.skill.version > 1 && (
            <span style={{ color: '#fbbf24', marginLeft: 4 }}>v{node.skill.version}</span>
          )}
        </Html>
      )}
    </group>
  )
}

function RootStar({ node, dimmed }: { node: FractalNode3D; dimmed: boolean }) {
  const meshRef = useRef<THREE.Mesh>(null)
  useFrame((state, delta) => {
    if (meshRef.current) {
      meshRef.current.rotation.y += delta * 0.1
      const pulse = 1 + Math.sin(state.clock.elapsedTime * 0.8) * 0.05
      meshRef.current.scale.setScalar(pulse)
    }
  })
  return (
    <group position={node.position}>
      <pointLight color="#bef264" intensity={dimmed ? 1.2 : 3} distance={30} />
      <mesh>
        <sphereGeometry args={[node.radius * 1.8, 24, 24]} />
        <meshBasicMaterial color="#6366f1" transparent opacity={dimmed ? 0.025 : 0.08} depthWrite={false} />
      </mesh>
      <mesh ref={meshRef}>
        <icosahedronGeometry args={[node.radius, 2]} />
        <meshStandardMaterial
          color="#6366f1"
          emissive="#bef264"
          emissiveIntensity={dimmed ? 0.25 : 1.2}
          metalness={0.5}
          roughness={0.2}
          transparent
          opacity={dimmed ? 0.22 : 1}
        />
      </mesh>
      <mesh>
        <sphereGeometry args={[node.radius * 0.5, 16, 16]} />
        <meshBasicMaterial color="#ffffff" transparent opacity={dimmed ? 0.25 : 1} />
      </mesh>
    </group>
  )
}

// ---- Edge as bezier line ---------------------------------------------------

function Edge({ from, to, active, dimmed }: { from: FractalNode3D; to: FractalNode3D; active: boolean; dimmed: boolean }) {
  const points = useMemo(() => {
    const start = from.position
    const end = to.position
    const mid = new THREE.Vector3().addVectors(start, end).multiplyScalar(0.5)
    const dist = start.distanceTo(end)
    const curve = Math.min(dist * 0.15, 1.5)
    const dirToCenter = new THREE.Vector3().subVectors(new THREE.Vector3(0, 0, 0), mid).normalize()
    mid.addScaledVector(dirToCenter, curve)
    return new THREE.QuadraticBezierCurve3(start, mid, end).getPoints(20)
  }, [from.position, to.position])

  const color = useMemo(() => hashColor(from.colorKey), [from.colorKey])

  return (
    <Line
      points={points}
      color={active ? '#a5b4fc' : color}
      lineWidth={active ? 2 : 0.55}
      opacity={active ? 0.98 : dimmed ? 0.035 : (from.depth === 0 ? 0.42 : 0.2)}
      transparent
    />
  )
}

function CameraFocus({ target }: { target: THREE.Vector3 | null }) {
  const { camera } = useThree()
  useFrame(() => {
    if (!target) return
    const desired = target.clone().add(new THREE.Vector3(0, 4, 14))
    camera.position.lerp(desired, 0.045)
    camera.lookAt(target)
  })
  return null
}

// ---- Scene -----------------------------------------------------------------

interface SceneProps {
  skills: SkillItem[]
  selected: SkillItem | null
  setSelected: (s: SkillItem | null) => void
  hovered: string | null
  setHovered: (n: string | null) => void
  focusToken: number
}

function Scene({ skills, selected, setSelected, hovered, setHovered, focusToken }: SceneProps) {
  const { nodes, edges } = useMemo(() => {
    const root = buildFractal3D(skills)
    layoutFractal3D(root, 14)
    return flattenTree(root)
  }, [skills])

  const selectedNode = useMemo(
    () => selected ? nodes.find(n => n.skill?.name === selected.name) || null : null,
    [nodes, selected],
  )
  const hoverNodeName = hovered
  const activeNames = useMemo(() => {
    const active = new Set<string>()
    const center = selectedNode?.name || hoverNodeName
    if (!center) return active
    active.add(center)
    for (const e of edges) {
      if (e.from.name === center || e.to.name === center) {
        active.add(e.from.name)
        active.add(e.to.name)
      }
    }
    return active
  }, [edges, selectedNode, hoverNodeName])
  const hasFocus = activeNames.size > 0

  return (
    <>
      <ambientLight intensity={0.25} />
      <pointLight position={[20, 20, 20]} intensity={0.8} />
      <pointLight position={[-20, -10, -20]} intensity={0.4} color="#84cc16" />

      <Stars radius={120} depth={60} count={3000} factor={4} saturation={0} fade speed={0.3} />

      {edges.map((e, i) => {
        const active = activeNames.has(e.from.name) && activeNames.has(e.to.name)
        return (
          <Edge
            key={`e-${i}`}
            from={e.from}
            to={e.to}
            active={active}
            dimmed={hasFocus && !active}
          />
        )
      })}

      {nodes.map(n => {
        const active = activeNames.has(n.name)
        const dimmed = hasFocus && !active
        if (n.name === '__root') return <RootStar key={n.name} node={n} dimmed={dimmed} />
        return (
          <StarMesh
            key={n.name}
            node={n}
            hovered={hovered === n.name}
            selected={selected?.name === n.name}
            active={active}
            dimmed={dimmed}
            onPointerOver={(e) => {
              e.stopPropagation()
              setHovered(n.name)
              document.body.style.cursor = n.skill ? 'pointer' : 'default'
            }}
            onPointerOut={() => {
              setHovered(null)
              document.body.style.cursor = 'auto'
            }}
            onClick={() => {
              if (n.skill) setSelected(n.skill)
            }}
          />
        )
      })}

      <CameraFocus target={focusToken > 0 ? selectedNode?.position || null : null} />
      <OrbitControls
        enablePan
        enableZoom
        enableRotate
        autoRotate={selected === null && hovered === null}
        autoRotateSpeed={0.25}
        minDistance={6}
        maxDistance={120}
      />
    </>
  )
}

// ---- Detail panel ----------------------------------------------------------

function DetailPanel({ skill, onClose, onRun, running }: { skill: SkillItem; onClose: () => void; onRun: (skill: SkillItem) => void; running: boolean }) {
  return (
    <div className="absolute bottom-4 right-4 w-80 bg-gray-900/95 backdrop-blur ring-1 ring-black/35 rounded-xl shadow-2xl overflow-hidden z-20">
      <div className="flex items-center justify-between px-4 py-3 border-b border-smara-300/12">
        <div className="flex items-center gap-2 min-w-0">
          <Zap className="w-4 h-4 text-smara-400 shrink-0" />
          <span className="text-sm font-semibold text-gray-100 truncate">{skill.name}</span>
          {skill.version > 0 && (
            <span className="text-[10px] px-1.5 py-0.5 rounded bg-gray-800 text-gray-400 shrink-0">v{skill.version}</span>
          )}
        </div>
        <button onClick={onClose} className="text-gray-500 hover:text-gray-300 shrink-0 ml-2">
          <X className="w-4 h-4" />
        </button>
      </div>

      <div className="p-4 space-y-3 max-h-72 overflow-y-auto">
        <div className="grid grid-cols-2 gap-2">
          <button
            onClick={() => onRun(skill)}
            disabled={running}
            className="inline-flex items-center justify-center gap-1.5 px-2.5 py-1.5 rounded-lg bg-smara-500 hover:bg-smara-400 text-black disabled:opacity-50 text-xs text-white transition-colors"
          >
            <Play className="w-3 h-3" /> {running ? 'Running...' : 'Run Skill'}
          </button>
          <button
            onClick={() => navigator.clipboard?.writeText(skill.name)}
            className="inline-flex items-center justify-center gap-1.5 px-2.5 py-1.5 rounded-lg bg-gray-800 hover:bg-gray-700 text-xs text-gray-200 transition-colors"
          >
            Copy Name
          </button>
        </div>

        {skill.description && <p className="text-xs text-gray-400 leading-relaxed">{skill.description}</p>}

        {skill.params && skill.params.length > 0 && (
          <div>
            <div className="flex items-center gap-1 text-[10px] text-gray-500 mb-1.5">
              <Zap className="w-3 h-3" /><span>Params</span>
            </div>
            <div className="space-y-1">
              {skill.params.map(p => (
                <div key={p.name} className="flex items-center justify-between gap-2 rounded bg-gray-800/50 ring-1 ring-black/30 px-2 py-1 text-[10px]">
                  <span className="text-gray-200 font-mono truncate">{p.name}</span>
                  <span className="text-gray-500 shrink-0">{p.required ? 'required' : p.type || 'string'}</span>
                </div>
              ))}
            </div>
          </div>
        )}

        {skill.tags && skill.tags.length > 0 && (
          <div>
            <div className="flex items-center gap-1 text-[10px] text-gray-500 mb-1.5">
              <Tag className="w-3 h-3" /><span>Tags</span>
            </div>
            <div className="flex flex-wrap gap-1">
              {skill.tags.map(t => (
                <span key={t} className="px-2 py-0.5 rounded-full text-[10px] font-medium" style={{ background: hashColor(t) + '22', color: hashColor(t), border: `1px solid ${hashColor(t)}44` }}>{t}</span>
              ))}
            </div>
          </div>
        )}

        {skill.category_path && skill.category_path.length > 0 && (
          <div>
            <div className="flex items-center gap-1 text-[10px] text-gray-500 mb-1">
              <Layers className="w-3 h-3" /><span>Category</span>
            </div>
            <p className="text-xs text-gray-300 font-mono">{skill.category_path.join(' / ')}</p>
          </div>
        )}

        {skill.dependencies && skill.dependencies.length > 0 && (
          <div>
            <div className="flex items-center gap-1 text-[10px] text-gray-500 mb-1">
              <GitBranch className="w-3 h-3" /><span>Dependencies</span>
            </div>
            <div className="flex flex-wrap gap-1">
              {skill.dependencies.map(d => (
                <span key={d} className="px-2 py-0.5 rounded bg-pink-900/30 border border-pink-800/40 text-pink-300 text-[10px]">{d}</span>
              ))}
            </div>
          </div>
        )}

        {skill.lineage && skill.lineage.length > 0 && (
          <div>
            <div className="flex items-center gap-1 text-[10px] text-gray-500 mb-1">
              <History className="w-3 h-3" /><span>Riwayat ({skill.lineage.length + 1} versi)</span>
            </div>
            <ol className="space-y-1 mt-1">
              <li className="flex items-center gap-2 text-[11px] bg-amber-900/20 border border-amber-800/40 rounded px-2 py-1">
                <span className="inline-flex items-center justify-center w-5 h-5 rounded-full bg-amber-400/90 text-[10px] font-bold text-amber-950">v{skill.version}</span>
                <span className="text-amber-200 truncate flex-1">sekarang</span>
                <RefreshCw className="w-3 h-3 text-amber-400" />
              </li>
              {[...skill.lineage].reverse().map((l, idx) => (
                <li key={`${l.version}-${idx}`} className="flex items-center gap-2 text-[11px] bg-gray-800/40 ring-1 ring-black/30 rounded px-2 py-1">
                  <span className="inline-flex items-center justify-center w-5 h-5 rounded-full bg-gray-700 text-[10px] font-bold text-gray-300">v{l.version}</span>
                  <span className="text-gray-400 truncate flex-1" title={l.description}>
                    {l.refined_from && <span className="text-gray-500">{l.refined_from}</span>}
                    {l.refined_at && <span className="text-gray-600"> · {l.refined_at}</span>}
                  </span>
                  <span className="text-gray-600 text-[10px]">{l.step_count} steps</span>
                </li>
              ))}
            </ol>
          </div>
        )}
      </div>
    </div>
  )
}

// ---- Main component --------------------------------------------------------

export default function SkillTree3D({ skills }: { skills: SkillItem[] }) {
  const [selected, setSelected] = useState<SkillItem | null>(null)
  const [hovered, setHovered] = useState<string | null>(null)
  const [autoRotate, setAutoRotate] = useState(true)
  const [query, setQuery] = useState('')
  const [tagFilter, setTagFilter] = useState('all')
  const [localGraph, setLocalGraph] = useState(false)
  const [focusToken, setFocusToken] = useState(0)
  const [runningSkill, setRunningSkill] = useState<string | null>(null)
  const [runResult, setRunResult] = useState<string | null>(null)

  const tags = useMemo(() => uniqueSorted(skills.flatMap(s => [...(s.tags || []), ...(s.category_path || []).slice(0, 1)])), [skills])

  const visibleSkills = useMemo(() => {
    let out = skills.filter(sk => matchesSkillQuery(sk, query))
    if (tagFilter !== 'all') {
      out = out.filter(sk => (sk.tags || []).includes(tagFilter) || (sk.category_path || []).includes(tagFilter))
    }
    if (localGraph && selected) {
      const keep = relatedSkillNames(skills, selected, 2)
      out = out.filter(sk => keep.has(sk.name))
    }
    if (selected && !out.some(sk => sk.name === selected.name)) {
      setSelected(null)
    }
    return out
  }, [skills, query, tagFilter, localGraph, selected])

  const searchMatches = useMemo(() => {
    if (!query.trim()) return []
    return skills.filter(sk => matchesSkillQuery(sk, query)).slice(0, 6)
  }, [skills, query])

  useEffect(() => {
    if (selected !== null || hovered !== null) setAutoRotate(false)
  }, [selected, hovered])

  const selectAndFocus = (skill: SkillItem) => {
    setSelected(skill)
    setFocusToken(t => t + 1)
    setAutoRotate(false)
  }

  const resetView = () => {
    setSelected(null)
    setHovered(null)
    setQuery('')
    setTagFilter('all')
    setLocalGraph(false)
    setAutoRotate(true)
    setFocusToken(0)
  }

  const runSkill = async (skill: SkillItem) => {
    if (skill.params && skill.params.some(p => p.required && !p.default)) {
      setRunResult(`Skill "${skill.name}" butuh parameter. Jalankan dari halaman Skills untuk mengisi param.`)
      return
    }
    setRunningSkill(skill.name)
    setRunResult(null)
    try {
      const args: Record<string, string> = {}
      for (const p of skill.params || []) {
        if (p.default !== undefined) args[p.name] = String(p.default)
      }
      const payload: Record<string, unknown> = { name: skill.name }
      if (Object.keys(args).length > 0) payload.args = args
      const res = await fetchJSON('/api/skills/run', {
        method: 'POST',
        body: JSON.stringify(payload),
        headers: { 'Content-Type': 'application/json' },
      })
      setRunResult(JSON.stringify(res, null, 2))
    } catch (e: any) {
      setRunResult('Error: ' + (e.message || e))
    } finally {
      setRunningSkill(null)
    }
  }

  return (
    <div className="relative w-full h-full bg-gray-950">
      <Canvas camera={{ position: [0, 8, 28], fov: 60 }} gl={{ antialias: true, alpha: false }} style={{ background: '#030712' }}>
        <Suspense fallback={null}>
          <Scene
            skills={visibleSkills}
            selected={selected}
            setSelected={(s) => {
              if (s) selectAndFocus(s)
            }}
            hovered={hovered}
            setHovered={setHovered}
            focusToken={focusToken}
          />
        </Suspense>
      </Canvas>

      {/* Obsidian-like command bar */}
      <div className="absolute top-3 left-3 right-3 z-20 flex flex-wrap items-start gap-2 pointer-events-none">
        <div className="relative pointer-events-auto w-72 max-w-[calc(100vw-2rem)]">
          <div className="flex items-center gap-2 rounded-xl ring-1 ring-black/30 bg-gray-900/90 px-3 py-2 shadow-xl backdrop-blur">
            <Search className="w-4 h-4 text-gray-500" />
            <input
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              placeholder="Search skill..."
              className="w-full bg-transparent text-xs text-gray-100 placeholder:text-gray-500 outline-none"
            />
            {query && <button onClick={() => setQuery('')} className="text-gray-500 hover:text-gray-300"><X className="w-3.5 h-3.5" /></button>}
          </div>
          {searchMatches.length > 0 && (
            <div className="mt-1 overflow-hidden rounded-xl ring-1 ring-black/30 bg-gray-900/95 shadow-2xl backdrop-blur">
              {searchMatches.map(sk => (
                <button key={sk.name} onClick={() => selectAndFocus(sk)} className="flex w-full items-center gap-2 px-3 py-2 text-left text-xs text-gray-300 hover:bg-gray-800/80">
                  <span>{getSkillIcon(sk)}</span>
                  <span className="truncate">{sk.name}</span>
                </button>
              ))}
            </div>
          )}
        </div>

        <select
          value={tagFilter}
          onChange={(e) => setTagFilter(e.target.value)}
          className="pointer-events-auto rounded-xl ring-1 ring-black/30 bg-gray-900/90 px-3 py-2 text-xs text-gray-300 outline-none backdrop-blur"
        >
          <option value="all">All tags/categories</option>
          {tags.map(t => <option key={t} value={t}>{t}</option>)}
        </select>

        <button
          onClick={() => setLocalGraph(v => !v)}
          disabled={!selected}
          className={`pointer-events-auto inline-flex items-center gap-1.5 rounded-xl border px-3 py-2 text-xs backdrop-blur transition-colors ${localGraph ? 'border-smara-500/60 bg-smara-600/20 text-smara-200' : 'border-smara-300/12 bg-gray-900/90 text-gray-300 hover:bg-gray-800'} disabled:opacity-40`}
        >
          <Network className="w-3.5 h-3.5" /> Local graph
        </button>

        <button
          onClick={() => selected && setFocusToken(t => t + 1)}
          disabled={!selected}
          className="pointer-events-auto inline-flex items-center gap-1.5 rounded-xl ring-1 ring-black/30 bg-gray-900/90 px-3 py-2 text-xs text-gray-300 backdrop-blur hover:bg-gray-800 disabled:opacity-40"
        >
          <Maximize2 className="w-3.5 h-3.5" /> Focus
        </button>

        <button onClick={resetView} className="pointer-events-auto rounded-xl ring-1 ring-black/30 bg-gray-900/90 px-3 py-2 text-xs text-gray-300 backdrop-blur hover:bg-gray-800">
          Reset
        </button>
      </div>

      {/* Hint overlay */}
      <div className="absolute bottom-3 left-3 bg-gray-900/80 ring-1 ring-black/30 rounded px-3 py-2 text-[10px] text-gray-400 pointer-events-none space-y-0.5">
        <div>🖱 Drag = rotate · Wheel = zoom · Right drag = pan</div>
        <div>✨ Klik node untuk detail · search/focus/local graph seperti Obsidian</div>
      </div>

      {/* Stats badge */}
      <div className="absolute top-[4.25rem] left-3 bg-gray-900/80 ring-1 ring-black/30 rounded px-2.5 py-1.5 text-[10px] text-gray-400 pointer-events-none">
        🌌 3D Fractal — {visibleSkills.length}/{skills.length} skills
        {localGraph && selected && <span className="ml-2 text-smara-300">local</span>}
        {autoRotate && <span className="ml-2 text-smara-400">⟳</span>}
      </div>

      {selected && <DetailPanel skill={selected} onClose={() => setSelected(null)} onRun={runSkill} running={runningSkill === selected.name} />}

      {runResult && (
        <div className="absolute bottom-4 left-4 z-30 w-96 max-w-[calc(100vw-2rem)] overflow-hidden rounded-xl ring-1 ring-black/35 bg-gray-900/95 shadow-2xl backdrop-blur">
          <div className="flex items-center justify-between border-b border-smara-300/12 px-3 py-2 text-xs text-gray-300">
            <span>Skill run result</span>
            <button onClick={() => setRunResult(null)} className="text-gray-500 hover:text-gray-300"><X className="w-4 h-4" /></button>
          </div>
          <pre className="max-h-56 overflow-auto p-3 text-[10px] text-gray-400 whitespace-pre-wrap">{runResult}</pre>
        </div>
      )}

      {visibleSkills.length === 0 && (
        <div className="absolute inset-0 flex items-center justify-center pointer-events-none">
          <div className="text-center text-gray-500">
            <Zap className="w-10 h-10 mx-auto mb-3 text-gray-600" />
            <p className="text-sm font-medium mb-1">No skills found</p>
            <p className="text-xs">Coba reset search/filter atau matikan local graph.</p>
          </div>
        </div>
      )}
    </div>
  )
}
