import { useMemo, useRef, useState, useEffect, Suspense } from 'react'
import { Canvas, useFrame, type ThreeEvent } from '@react-three/fiber'
import { OrbitControls, Stars, Html, Line } from '@react-three/drei'
import * as THREE from 'three'
import { type SkillItem } from '../api'
import { X, Zap, Tag, GitBranch, Layers, History, RefreshCw } from 'lucide-react'
import { getSkillIcon, getCategoryIcon } from './skillIcons'

// ---- Color palette ---------------------------------------------------------

const PALETTE = [
  '#818cf8', '#f472b6', '#34d399', '#fbbf24', '#60a5fa',
  '#a78bfa', '#fb923c', '#2dd4bf', '#f87171', '#38bdf8',
]

function hashColor(str: string): string {
  let h = 0
  for (let i = 0; i < str.length; i++) h = ((h << 5) - h + str.charCodeAt(i)) | 0
  return PALETTE[Math.abs(h) % PALETTE.length]
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
//
// Each node distributes its children around a sphere using a Fibonacci
// lattice (golden-angle distribution). The orbit radius scales by depth
// and is also bumped up when a parent has many children so they don't
// collide. This produces a recursive 3D fractal where every subtree gets
// its own little spherical cluster.

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

  // Minimum orbit so children don't intersect when distributed on a sphere.
  // Surface area scales with R²; we want each child to get an area roughly
  // (kid_radius + gap)² wide.
  const minOrbitForKids = (kids: FractalNode3D[], gap: number) => {
    if (kids.length === 0) return 0
    const maxR = Math.max(...kids.map(radiusFor))
    const slot = 2 * maxR + gap
    // Sphere surface area per child: 4πR² / N >= slot²
    const requiredR = Math.sqrt((kids.length * slot * slot) / (4 * Math.PI))
    return requiredR
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
    // Fibonacci sphere distribution gives well-separated points on a sphere.
    const N = kids.length
    const phi = Math.PI * (3 - Math.sqrt(5)) // golden angle
    kids.forEach((kid, i) => {
      // y in [-1..1], slightly compressed so children don't pile on poles.
      const y = 1 - (i / Math.max(N - 1, 1)) * 1.6
      const radiusAtY = Math.sqrt(Math.max(0.001, 1 - y * y))
      const theta = phi * i
      const dir = new THREE.Vector3(
        Math.cos(theta) * radiusAtY,
        y * 0.85, // slightly compressed vertically so the visual reads as a "tree"
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
  onPointerOver: (e: ThreeEvent<PointerEvent>) => void
  onPointerOut: () => void
  onClick: () => void
}

function StarMesh({ node, hovered, selected, onPointerOver, onPointerOut, onClick }: NodeProps) {
  const meshRef = useRef<THREE.Mesh>(null)
  const color = useMemo(() => new THREE.Color(hashColor(node.colorKey)), [node.colorKey])
  const emphasized = hovered || selected

  // Slow rotation for ambient liveliness.
  useFrame((_, delta) => {
    if (meshRef.current) {
      meshRef.current.rotation.y += delta * (emphasized ? 0.6 : 0.15)
      meshRef.current.rotation.x += delta * 0.05
    }
  })

  // Build star geometry: a torusKnot or sphere look — we use icosahedron
  // for a faceted "crystal star" look that reads well in 3D.
  const geo = useMemo(() => {
    return new THREE.IcosahedronGeometry(node.radius, 1)
  }, [node.radius])

  const scale = emphasized ? 1.25 : 1.0

  return (
    <group position={node.position}>
      {/* Outer halo (glow) */}
      {emphasized && (
        <mesh>
          <sphereGeometry args={[node.radius * 2.2, 16, 16]} />
          <meshBasicMaterial color={color} transparent opacity={0.08} depthWrite={false} />
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
          emissiveIntensity={emphasized ? 0.9 : 0.5}
          metalness={0.3}
          roughness={0.4}
        />
      </mesh>

      {/* Inner bright core */}
      <mesh>
        <sphereGeometry args={[node.radius * 0.4, 12, 12]} />
        <meshBasicMaterial color="#ffffff" />
      </mesh>

      {/* Lineage aureole as wireframe icosahedron */}
      {node.skill?.lineage && node.skill.lineage.length > 0 && (
        <mesh>
          <icosahedronGeometry args={[node.radius * 1.4, 0]} />
          <meshBasicMaterial color="#fbbf24" wireframe transparent opacity={emphasized ? 0.7 : 0.35} />
        </mesh>
      )}

      {/* Floating label always faces camera */}
      <Html
        center
        position={[0, node.radius + 0.6, 0]}
        style={{
          pointerEvents: 'none',
          fontFamily: 'Inter, sans-serif',
          fontSize: emphasized ? '11px' : '10px',
          fontWeight: emphasized ? 600 : 400,
          color: emphasized ? '#f3f4f6' : '#9ca3af',
          textShadow: '0 1px 4px rgba(0,0,0,0.9)',
          whiteSpace: 'nowrap',
          userSelect: 'none',
        }}
      >
        {node.skill ? getSkillIcon(node.skill) : getCategoryIcon(node.label)}{' '}
        {node.label.length > 20 ? node.label.slice(0, 19) + '…' : node.label}
        {node.skill && node.skill.version > 1 && (
          <span style={{ color: '#fbbf24', marginLeft: 4 }}>v{node.skill.version}</span>
        )}
      </Html>
    </group>
  )
}

// Root sphere is bigger and pulses.
function RootStar({ node }: { node: FractalNode3D }) {
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
      <pointLight color="#818cf8" intensity={3} distance={30} />
      <mesh>
        <sphereGeometry args={[node.radius * 1.8, 24, 24]} />
        <meshBasicMaterial color="#6366f1" transparent opacity={0.08} depthWrite={false} />
      </mesh>
      <mesh ref={meshRef}>
        <icosahedronGeometry args={[node.radius, 2]} />
        <meshStandardMaterial
          color="#6366f1"
          emissive="#818cf8"
          emissiveIntensity={1.2}
          metalness={0.5}
          roughness={0.2}
        />
      </mesh>
      <mesh>
        <sphereGeometry args={[node.radius * 0.5, 16, 16]} />
        <meshBasicMaterial color="#ffffff" />
      </mesh>
      <Html
        center
        position={[0, node.radius + 1.5, 0]}
        style={{
          pointerEvents: 'none',
          fontFamily: 'Inter, sans-serif',
          fontSize: '12px',
          fontWeight: 700,
          letterSpacing: '1px',
          textTransform: 'uppercase',
          color: '#c7d2fe',
          textShadow: '0 0 8px #818cf8',
        }}
      >
        ✦ Skills
      </Html>
    </group>
  )
}

// ---- Edge as bezier line ---------------------------------------------------

function Edge({ from, to, active }: { from: FractalNode3D; to: FractalNode3D; active: boolean }) {
  const points = useMemo(() => {
    // Build a slightly curved line between parent and child by adding a
    // midpoint pulled toward the origin direction so links arc inward.
    const start = from.position
    const end = to.position
    const mid = new THREE.Vector3()
      .addVectors(start, end)
      .multiplyScalar(0.5)
    // Pull mid toward origin for curve effect (smaller for short edges)
    const dist = start.distanceTo(end)
    const curve = Math.min(dist * 0.15, 1.5)
    const dirToCenter = new THREE.Vector3().subVectors(new THREE.Vector3(0, 0, 0), mid).normalize()
    mid.addScaledVector(dirToCenter, curve)

    const curveLine = new THREE.QuadraticBezierCurve3(start, mid, end)
    return curveLine.getPoints(20)
  }, [from.position, to.position])

  const color = useMemo(() => hashColor(from.colorKey), [from.colorKey])

  return (
    <Line
      points={points}
      color={active ? '#818cf8' : color}
      lineWidth={active ? 1.6 : 0.6}
      opacity={active ? 0.95 : (from.depth === 0 ? 0.5 : 0.25)}
      transparent
    />
  )
}

// ---- Scene -----------------------------------------------------------------

interface SceneProps {
  skills: SkillItem[]
  selected: SkillItem | null
  setSelected: (s: SkillItem | null) => void
  hovered: string | null
  setHovered: (n: string | null) => void
}

function Scene({ skills, selected, setSelected, hovered, setHovered }: SceneProps) {
  const { nodes, edges } = useMemo(() => {
    const root = buildFractal3D(skills)
    layoutFractal3D(root, 14)
    const flat = flattenTree(root)
    return flat
  }, [skills])

  return (
    <>
      <ambientLight intensity={0.25} />
      <pointLight position={[20, 20, 20]} intensity={0.8} />
      <pointLight position={[-20, -10, -20]} intensity={0.4} color="#a78bfa" />

      <Stars
        radius={120}
        depth={60}
        count={3000}
        factor={4}
        saturation={0}
        fade
        speed={0.3}
      />

      {edges.map((e, i) => (
        <Edge
          key={`e-${i}`}
          from={e.from}
          to={e.to}
          active={
            hovered === e.from.name ||
            hovered === e.to.name ||
            selected?.name === e.from.name ||
            selected?.name === e.to.name
          }
        />
      ))}

      {nodes.map(n => {
        if (n.name === '__root') {
          return <RootStar key={n.name} node={n} />
        }
        return (
          <StarMesh
            key={n.name}
            node={n}
            hovered={hovered === n.name}
            selected={selected?.name === n.name}
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

function DetailPanel({ skill, onClose }: { skill: SkillItem; onClose: () => void }) {
  return (
    <div className="absolute bottom-4 right-4 w-80 bg-gray-900/95 backdrop-blur border border-gray-700 rounded-xl shadow-2xl overflow-hidden z-20">
      <div className="flex items-center justify-between px-4 py-3 border-b border-gray-800">
        <div className="flex items-center gap-2 min-w-0">
          <Zap className="w-4 h-4 text-smara-400 shrink-0" />
          <span className="text-sm font-semibold text-gray-100 truncate">{skill.name}</span>
          {skill.version > 0 && (
            <span className="text-[10px] px-1.5 py-0.5 rounded bg-gray-800 text-gray-400 shrink-0">
              v{skill.version}
            </span>
          )}
        </div>
        <button onClick={onClose} className="text-gray-500 hover:text-gray-300 shrink-0 ml-2">
          <X className="w-4 h-4" />
        </button>
      </div>

      <div className="p-4 space-y-3 max-h-64 overflow-y-auto">
        {skill.description && (
          <p className="text-xs text-gray-400 leading-relaxed">{skill.description}</p>
        )}

        {skill.tags && skill.tags.length > 0 && (
          <div>
            <div className="flex items-center gap-1 text-[10px] text-gray-500 mb-1.5">
              <Tag className="w-3 h-3" /><span>Tags</span>
            </div>
            <div className="flex flex-wrap gap-1">
              {skill.tags.map(t => (
                <span key={t} className="px-2 py-0.5 rounded-full text-[10px] font-medium" style={{ background: hashColor(t) + '22', color: hashColor(t), border: `1px solid ${hashColor(t)}44` }}>
                  {t}
                </span>
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
                <span className="inline-flex items-center justify-center w-5 h-5 rounded-full bg-amber-600/90 text-[10px] font-bold text-gray-900">v{skill.version}</span>
                <span className="text-amber-200 truncate flex-1">sekarang</span>
                <RefreshCw className="w-3 h-3 text-amber-400" />
              </li>
              {[...skill.lineage].reverse().map((l, idx) => (
                <li key={`${l.version}-${idx}`} className="flex items-center gap-2 text-[11px] bg-gray-800/40 border border-gray-700/60 rounded px-2 py-1">
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

  // Suppress autoRotate when user is interacting.
  useEffect(() => {
    if (selected !== null || hovered !== null) setAutoRotate(false)
  }, [selected, hovered])

  return (
    <div className="relative w-full h-full bg-gray-950">
      <Canvas
        camera={{ position: [0, 8, 28], fov: 60 }}
        gl={{ antialias: true, alpha: false }}
        style={{ background: '#030712' }}
      >
        <Suspense fallback={null}>
          <Scene
            skills={skills}
            selected={selected}
            setSelected={setSelected}
            hovered={hovered}
            setHovered={setHovered}
          />
        </Suspense>
      </Canvas>

      {/* Hint overlay */}
      <div className="absolute bottom-3 left-3 bg-gray-900/80 border border-gray-800 rounded px-3 py-2 text-[10px] text-gray-400 pointer-events-none space-y-0.5">
        <div>🖱 Drag = rotate · Wheel = zoom · Right drag = pan</div>
        <div>✨ Klik node untuk detail · Auto-rotate aktif saat idle</div>
      </div>

      {/* Stats badge */}
      <div className="absolute top-3 left-3 bg-gray-900/80 border border-gray-800 rounded px-2.5 py-1.5 text-[10px] text-gray-400 pointer-events-none">
        🌌 3D Fractal — {skills.length} skills
        {autoRotate && <span className="ml-2 text-indigo-400">⟳</span>}
      </div>

      {selected && <DetailPanel skill={selected} onClose={() => setSelected(null)} />}

      {skills.length === 0 && (
        <div className="absolute inset-0 flex items-center justify-center pointer-events-none">
          <div className="text-center text-gray-500">
            <Zap className="w-10 h-10 mx-auto mb-3 text-gray-600" />
            <p className="text-sm font-medium mb-1">No skills found</p>
            <p className="text-xs">Skills will appear as 3D stars in space.</p>
          </div>
        </div>
      )}
    </div>
  )
}
