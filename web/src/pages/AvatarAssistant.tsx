import { Suspense, useEffect, useMemo, useRef, useState } from 'react'
import { Canvas, useFrame } from '@react-three/fiber'
import { OrbitControls, Stars, Text, Sparkles, useGLTF } from '@react-three/drei'
import * as THREE from 'three'
import { Bot, MessageCircle, MousePointer2, Power, RotateCcw, Volume2, Zap } from 'lucide-react'
import { fetchJSON } from '../api'

type AvatarState = 'idle' | 'listening' | 'thinking' | 'speaking' | 'acting' | 'waiting_approval' | 'success' | 'error' | 'emergency_stop'
type AvatarConfig = { enabled: boolean; model: string; style: string; state: AvatarState; expression: string; speech_bubble: string; lip_sync: boolean; gesture: string; light_mode: boolean; voice_reactive: boolean; intensity: number }

type RigBones = {
  hips?: THREE.Object3D
  spine?: THREE.Object3D
  chest?: THREE.Object3D
  neck?: THREE.Object3D
  head?: THREE.Object3D
  leftUpperArm?: THREE.Object3D
  leftLowerArm?: THREE.Object3D
  leftHand?: THREE.Object3D
  rightUpperArm?: THREE.Object3D
  rightLowerArm?: THREE.Object3D
  rightHand?: THREE.Object3D
}

type MorphBinding = {
  mesh: THREE.Mesh
  influences: number[]
  indices: Partial<Record<'a' | 'i' | 'u' | 'e' | 'o' | 'blinkL' | 'blinkR' | 'happy' | 'joy' | 'smile', number>>
}

const defaultAvatar: AvatarConfig = { enabled: true, model: '/api/avatar/model', style: 'custom-vrm-smara-avatar', state: 'idle', expression: 'soft-smile', speech_bubble: 'Halo, saya Smara. Siap membantu.', lip_sync: true, gesture: 'idle-float', light_mode: false, voice_reactive: true, intensity: 0.35 }
const states: AvatarState[] = ['idle','listening','thinking','speaking','acting','waiting_approval','success','error','emergency_stop']

function colorForState(state: AvatarState) {
  if (state === 'error' || state === 'emergency_stop') return '#fb7185'
  if (state === 'success') return '#34d399'
  if (state === 'speaking') return '#f0abfc'
  if (state === 'acting') return '#22d3ee'
  if (state === 'listening') return '#60a5fa'
  if (state === 'waiting_approval') return '#fbbf24'
  return '#a78bfa'
}

function pickObject(root: THREE.Object3D, names: string[]) {
  for (const name of names) {
    const found = root.getObjectByName(name)
    if (found) return found
  }
  let fallback: THREE.Object3D | undefined
  root.traverse(obj => {
    if (fallback) return
    const lower = obj.name.toLowerCase()
    if (names.some(n => lower.includes(n.toLowerCase()))) fallback = obj
  })
  return fallback
}

function collectRigBones(root: THREE.Object3D): RigBones {
  return {
    hips: pickObject(root, ['J_Bip_C_Hips', 'hips']),
    spine: pickObject(root, ['J_Bip_C_Spine', 'spine']),
    chest: pickObject(root, ['J_Bip_C_Chest', 'chest']),
    neck: pickObject(root, ['J_Bip_C_Neck', 'neck']),
    head: pickObject(root, ['J_Bip_C_Head', 'head']),
    leftUpperArm: pickObject(root, ['J_Bip_L_UpperArm', 'leftUpperArm', 'upper_arm.L', 'LeftArm']),
    leftLowerArm: pickObject(root, ['J_Bip_L_LowerArm', 'leftLowerArm', 'lower_arm.L', 'LeftForeArm']),
    leftHand: pickObject(root, ['J_Bip_L_Hand', 'leftHand', 'hand.L', 'LeftHand']),
    rightUpperArm: pickObject(root, ['J_Bip_R_UpperArm', 'rightUpperArm', 'upper_arm.R', 'RightArm']),
    rightLowerArm: pickObject(root, ['J_Bip_R_LowerArm', 'rightLowerArm', 'lower_arm.R', 'RightForeArm']),
    rightHand: pickObject(root, ['J_Bip_R_Hand', 'rightHand', 'hand.R', 'RightHand']),
  }
}

function findMorphIndex(dict: Record<string, number>, aliases: string[]) {
  const entries = Object.entries(dict)
  for (const alias of aliases) {
    const exact = dict[alias]
    if (exact !== undefined) return exact
    const found = entries.find(([name]) => name.toLowerCase().includes(alias.toLowerCase()))
    if (found) return found[1]
  }
  return undefined
}

function collectMorphs(root: THREE.Object3D): MorphBinding[] {
  const bindings: MorphBinding[] = []
  root.traverse(obj => {
    const mesh = obj as THREE.Mesh
    if (!mesh.isMesh || !mesh.morphTargetDictionary || !mesh.morphTargetInfluences) return
    const dict = mesh.morphTargetDictionary
    bindings.push({
      mesh,
      influences: mesh.morphTargetInfluences,
      indices: {
        a: findMorphIndex(dict, ['Fcl_MTH_A', 'Mouth_A', 'aa', 'viseme_aa']),
        i: findMorphIndex(dict, ['Fcl_MTH_I', 'Mouth_I', 'ih', 'viseme_ih']),
        u: findMorphIndex(dict, ['Fcl_MTH_U', 'Mouth_U', 'ou', 'viseme_ou']),
        e: findMorphIndex(dict, ['Fcl_MTH_E', 'Mouth_E', 'ee', 'viseme_ee']),
        o: findMorphIndex(dict, ['Fcl_MTH_O', 'Mouth_O', 'oh', 'viseme_oh']),
        blinkL: findMorphIndex(dict, ['Fcl_EYE_Close_L', 'Blink_L', 'blinkLeft']),
        blinkR: findMorphIndex(dict, ['Fcl_EYE_Close_R', 'Blink_R', 'blinkRight']),
        happy: findMorphIndex(dict, ['Fcl_ALL_Joy', 'Joy', 'Happy']),
        joy: findMorphIndex(dict, ['Fcl_BRW_Joy', 'Fcl_EYE_Joy', 'joy']),
        smile: findMorphIndex(dict, ['Fcl_MTH_Smile', 'Mouth_Smile', 'smile']),
      },
    })
  })
  return bindings
}

function rememberBasePose(bones: RigBones) {
  const base = new Map<THREE.Object3D, THREE.Euler>()
  Object.values(bones).forEach(bone => {
    if (bone && !base.has(bone)) base.set(bone, bone.rotation.clone())
  })
  return base
}

function setBone(base: Map<THREE.Object3D, THREE.Euler>, bone: THREE.Object3D | undefined, x = 0, y = 0, z = 0) {
  if (!bone) return
  const b = base.get(bone)
  if (!b) return
  bone.rotation.set(b.x + x, b.y + y, b.z + z)
}

function setMorph(binding: MorphBinding, key: keyof MorphBinding['indices'], value: number) {
  const index = binding.indices[key]
  if (index !== undefined) binding.influences[index] = THREE.MathUtils.clamp(value, 0, 1)
}

function LoadedVRMAvatar({ cfg }: { cfg: AvatarConfig }) {
  const group = useRef<THREE.Group>(null)
  const gltf = useGLTF(cfg.model || '/api/avatar/model')
  const color = colorForState(cfg.state)
  const rig = useMemo(() => collectRigBones(gltf.scene), [gltf.scene])
  const morphs = useMemo(() => collectMorphs(gltf.scene), [gltf.scene])
  const basePose = useMemo(() => rememberBasePose(rig), [rig])

  useFrame(({ clock }) => {
    const t = clock.elapsedTime
    const intensity = Math.max(0.15, cfg.intensity || 0.35)
    const isSpeaking = cfg.state === 'speaking' && cfg.lip_sync
    const isActing = cfg.state === 'acting'

    if (group.current) {
      group.current.position.y = Math.sin(t * 1.5) * 0.045 - 0.9
      group.current.rotation.y = Math.sin(t * 0.45) * 0.045
      group.current.rotation.z = cfg.state === 'emergency_stop' ? Math.sin(t * 14) * 0.018 : 0
    }

    const breath = Math.sin(t * 1.6)
    const sway = Math.sin(t * 0.9)
    const soft = Math.sin(t * 1.15)
    const wristL = Math.sin(t * 1.7 + 0.4) * 0.035 + Math.sin(t * 3.2) * 0.012
    const wristR = Math.sin(t * 1.6 + 1.1) * 0.035 + Math.sin(t * 3.0 + 0.6) * 0.012
    const gestureBoost = isActing ? 0.22 : 0

    // Natural idle runtime pose.
    // Penting: jangan terlalu banyak twist di wrist/hand. Pada VRM/VRoid, tangan sering terlihat
    // aneh kalau lower-arm/hand diberi rotasi besar. Jadi pose dibuat dari shoulder/upper-arm,
    // elbow hanya sedikit, dan wrist hanya micro motion.
    setBone(basePose, rig.hips, 0, sway * 0.025, sway * 0.018)
    setBone(basePose, rig.spine, breath * 0.018, 0, -sway * 0.018)
    setBone(basePose, rig.chest, breath * 0.028, 0, -sway * 0.028)
    setBone(basePose, rig.neck, -breath * 0.008, 0, sway * 0.018)
    setBone(basePose, rig.head, Math.sin(t * 1.1) * 0.018, Math.sin(t * 0.7) * 0.034, -sway * 0.022)

    // Dari T-pose: upper arm turun ke samping badan, elbow sedikit relax, hand hampir netral.
    // Nilai dibuat konservatif supaya tidak terbalik/melintir di berbagai avatar VRoid.
    const leftArmDown = -1.22 - soft * 0.025 - gestureBoost * Math.abs(Math.sin(t * 1.9))
    const rightArmDown = 1.22 + soft * 0.025 + gestureBoost * Math.abs(Math.sin(t * 1.85 + 0.5))
    const leftElbowRelax = 0.16 + Math.sin(t * 1.25 + 0.7) * 0.035 + gestureBoost * 0.08
    const rightElbowRelax = -0.16 - Math.sin(t * 1.2 + 1.1) * 0.035 - gestureBoost * 0.08

    setBone(basePose, rig.leftUpperArm, 0.035 + breath * 0.012, 0.0 + sway * 0.018, leftArmDown)
    setBone(basePose, rig.leftLowerArm, 0.025 + breath * 0.008, 0.0, leftElbowRelax)
    setBone(basePose, rig.leftHand, wristL, 0.018 + Math.sin(t * 2.1) * 0.018, -0.025 + Math.sin(t * 1.6) * 0.018)

    setBone(basePose, rig.rightUpperArm, 0.035 + breath * 0.012, 0.0 + sway * 0.018, rightArmDown)
    setBone(basePose, rig.rightLowerArm, 0.025 + breath * 0.008, 0.0, rightElbowRelax)
    setBone(basePose, rig.rightHand, wristR, -0.018 + Math.sin(t * 2.0 + 0.6) * 0.018, 0.025 + Math.sin(t * 1.55 + 0.5) * 0.018)

    const blinkPhase = t % 4.2
    const blink = blinkPhase > 3.95 ? Math.sin((blinkPhase - 3.95) / 0.25 * Math.PI) : 0
    const mouthCycle = ['a', 'i', 'u', 'e', 'o'] as const
    const mouthKey = mouthCycle[Math.floor(t * 6) % mouthCycle.length]
    const mouthValue = isSpeaking ? (0.35 + Math.abs(Math.sin(t * 13)) * 0.65) * intensity : 0

    morphs.forEach(binding => {
      ;(['a', 'i', 'u', 'e', 'o'] as const).forEach(k => setMorph(binding, k, 0))
      setMorph(binding, 'blinkL', blink)
      setMorph(binding, 'blinkR', blink)
      setMorph(binding, 'happy', cfg.state === 'success' ? 0.55 : 0.16)
      setMorph(binding, 'joy', cfg.state === 'success' ? 0.45 : 0.12)
      setMorph(binding, 'smile', cfg.state === 'error' || cfg.state === 'emergency_stop' ? 0 : 0.22)
      if (isSpeaking) setMorph(binding, mouthKey, mouthValue)
    })
  })

  return (
    <group>
      <pointLight color={color} intensity={2.1} position={[0, 2.4, 2]} />
      <primitive ref={group} object={gltf.scene} scale={1.05} position={[0, -0.9, 0]} />
      <Text position={[0, -1.25, 0]} fontSize={0.13} color={color} anchorX="center">{cfg.expression}</Text>
      <Sparkles count={48} scale={3.2} size={2.2} speed={0.35} color={color} />
    </group>
  )
}

function SmaraAvatarModel({ cfg }: { cfg: AvatarConfig }) {
  const group = useRef<THREE.Group>(null)
  const mouth = useRef<THREE.Mesh>(null)
  const leftHand = useRef<THREE.Group>(null)
  const rightHand = useRef<THREE.Group>(null)
  const color = colorForState(cfg.state)
  useFrame(({ clock }) => {
    const t = clock.elapsedTime
    const intensity = Math.max(0.15, cfg.intensity || 0.35)
    if (group.current) {
      group.current.position.y = Math.sin(t * 1.5) * 0.08
      group.current.rotation.y = Math.sin(t * 0.55) * 0.12
      if (cfg.state === 'thinking') group.current.rotation.y += Math.sin(t * 4) * 0.035
      if (cfg.state === 'emergency_stop') group.current.rotation.z = Math.sin(t * 14) * 0.025
      else group.current.rotation.z = 0
    }
    if (mouth.current) {
      const speaking = cfg.state === 'speaking' && cfg.lip_sync
      mouth.current.scale.y = speaking ? 0.35 + Math.abs(Math.sin(t * 12)) * intensity : 0.18
    }
    if (leftHand.current && rightHand.current) {
      const acting = cfg.state === 'acting'
      leftHand.current.rotation.z = acting ? Math.sin(t * 5) * 0.7 - 0.5 : Math.sin(t * 1.2) * 0.16 - 0.25
      rightHand.current.rotation.z = acting ? Math.cos(t * 5) * 0.7 + 0.5 : Math.cos(t * 1.2) * 0.16 + 0.25
    }
  })
  return (
    <group ref={group}>
      <pointLight color={color} intensity={2.1} position={[0, 2.4, 2]} />
      <mesh position={[0, 0.25, 0]}><capsuleGeometry args={[0.55, 1.25, 12, 24]} /><meshStandardMaterial color="#312e81" emissive={color} emissiveIntensity={0.1} roughness={0.45} /></mesh>
      <mesh position={[0, 1.3, 0]}><sphereGeometry args={[0.62, 40, 40]} /><meshStandardMaterial color="#ffe4f2" roughness={0.35} /></mesh>
      <mesh position={[-0.35, 1.82, 0]} rotation={[0,0,0.42]}><coneGeometry args={[0.22, 0.55, 4]} /><meshStandardMaterial color="#7c3aed" emissive={color} emissiveIntensity={0.2} /></mesh>
      <mesh position={[0.35, 1.82, 0]} rotation={[0,0,-0.42]}><coneGeometry args={[0.22, 0.55, 4]} /><meshStandardMaterial color="#7c3aed" emissive={color} emissiveIntensity={0.2} /></mesh>
      <mesh position={[-0.2, 1.38, 0.55]}><sphereGeometry args={[0.065, 16, 16]} /><meshStandardMaterial color={color} emissive={color} emissiveIntensity={1.2} /></mesh>
      <mesh position={[0.2, 1.38, 0.55]}><sphereGeometry args={[0.065, 16, 16]} /><meshStandardMaterial color={color} emissive={color} emissiveIntensity={1.2} /></mesh>
      <mesh ref={mouth} position={[0, 1.15, 0.59]}><boxGeometry args={[0.2, 0.05, 0.03]} /><meshStandardMaterial color="#be185d" emissive="#fb7185" emissiveIntensity={0.35} /></mesh>
      <group ref={leftHand} position={[-0.58, 0.55, 0]}><mesh position={[-0.38, -0.12, 0]}><capsuleGeometry args={[0.09, 0.7, 8, 16]} /><meshStandardMaterial color="#ffe4f2" /></mesh></group>
      <group ref={rightHand} position={[0.58, 0.55, 0]}><mesh position={[0.38, -0.12, 0]}><capsuleGeometry args={[0.09, 0.7, 8, 16]} /><meshStandardMaterial color="#ffe4f2" /></mesh></group>
      <mesh position={[0, -0.5, 0]}><torusGeometry args={[0.85, 0.02, 8, 96]} /><meshStandardMaterial color={color} emissive={color} emissiveIntensity={0.75} /></mesh>
      <Text position={[0, -0.95, 0]} fontSize={0.13} color={color} anchorX="center">{cfg.expression}</Text>
      <Sparkles count={48} scale={3.2} size={2.2} speed={0.35} color={color} />
    </group>
  )
}

export default function AvatarAssistant() {
  const [cfg, setCfg] = useState<AvatarConfig>(defaultAvatar)
  const [bubble, setBubble] = useState(defaultAvatar.speech_bubble)
  const [saved, setSaved] = useState(false)
  useEffect(() => { fetchJSON<AvatarConfig>('/api/avatar/state').then(c => { setCfg(c); setBubble(c.speech_bubble) }).catch(() => {}) }, [])
  const postEvent = async (patch: Partial<AvatarConfig>) => {
    const next = { ...cfg, ...patch }
    setCfg(next); setBubble(next.speech_bubble)
    try { await fetchJSON<AvatarConfig>('/api/avatar/state', { method: 'POST', body: JSON.stringify(next) }); setSaved(true); setTimeout(() => setSaved(false), 900) } catch {}
  }
  const speakDemo = () => {
    postEvent({ state: 'speaking', speech_bubble: bubble || 'Saya sedang berbicara.', intensity: 0.85 })
    if ('speechSynthesis' in window) {
      window.speechSynthesis.cancel(); const u = new SpeechSynthesisUtterance(bubble || 'Saya sedang berbicara.'); u.lang = 'id-ID'; u.onend = () => postEvent({ state: 'idle', intensity: 0.35 }); window.speechSynthesis.speak(u)
    }
  }
  const status = useMemo(() => cfg.enabled ? cfg.state : 'disabled', [cfg])
  return (
    <div className="h-full overflow-y-auto p-6 space-y-5">
      <div className="flex items-center justify-between">
        <div><div className="flex items-center gap-2 text-smara-200 font-semibold"><Bot className="w-5 h-5" /> Anime 3D Character Assistant</div><p className="text-sm text-gray-400 mt-1">Avatar VRM Smara, state/expression sync, lip-sync TTS, gesture feedback, speech bubble, dan light-mode toggle.</p></div>
        <div className="px-3 py-1 rounded-full text-xs border border-smara-300/20 bg-smara-500/10 text-smara-100">{status}</div>
      </div>
      <div className="grid xl:grid-cols-[1.15fr_.85fr] gap-5">
        <div className="relative min-h-[560px] rounded-3xl border border-neutral-800/70 bg-slate-950/30 overflow-hidden">
          {cfg.enabled ? <Canvas camera={{ position: [0, 1.0, 3.2], fov: 35 }}><color attach="background" args={[cfg.light_mode ? '#111827' : '#020617']} /><ambientLight intensity={0.75} /><Stars radius={45} depth={25} count={900} factor={3} fade speed={0.25} /><Suspense fallback={<SmaraAvatarModel cfg={cfg} />}><LoadedVRMAvatar cfg={cfg} /></Suspense><OrbitControls enablePan={false} minDistance={1.8} maxDistance={5} /></Canvas> : <div className="h-[560px] flex items-center justify-center text-gray-500">Avatar dimatikan untuk mode ringan.</div>}
          <div className="absolute left-5 right-5 bottom-5 rounded-3xl border border-neutral-800/70 bg-slate-950/55 backdrop-blur-xl p-4 flex gap-3"><MessageCircle className="w-5 h-5 text-lime-200 shrink-0" /><div><div className="text-xs text-gray-500 mb-1">Speech bubble</div><div className="text-sm text-gray-100">{cfg.speech_bubble}</div></div></div>
        </div>
        <div className="space-y-4">
          <div className="rounded-3xl border border-neutral-800/70 bg-slate-950/25 p-5 space-y-4">
            <div className="text-sm font-semibold text-gray-200">Character state</div>
            <div className="grid grid-cols-2 gap-2">{states.map(s => <button key={s} onClick={() => postEvent({ state: s, speech_bubble: s === 'acting' ? 'Saya sedang mengoperasikan desktop.' : s === 'error' ? 'Saya menemukan masalah.' : s === 'success' ? 'Berhasil selesai.' : cfg.speech_bubble })} className={`px-3 py-2 rounded-2xl text-xs border ${cfg.state === s ? 'border-smara-300/35 bg-smara-500/20 text-smara-100' : 'border-neutral-800/70 bg-white/5 text-gray-400 hover:text-gray-100'}`}>{s}</button>)}</div>
            <label className="flex items-center justify-between text-sm text-gray-300"><span>Avatar enabled</span><input type="checkbox" checked={cfg.enabled} onChange={e => postEvent({ enabled: e.target.checked })} /></label>
            <label className="flex items-center justify-between text-sm text-gray-300"><span>Light mode</span><input type="checkbox" checked={cfg.light_mode} onChange={e => postEvent({ light_mode: e.target.checked })} /></label>
            <label className="flex items-center justify-between text-sm text-gray-300"><span>Lip sync</span><input type="checkbox" checked={cfg.lip_sync} onChange={e => postEvent({ lip_sync: e.target.checked })} /></label>
          </div>
          <div className="rounded-3xl border border-neutral-800/70 bg-slate-950/25 p-5 space-y-3">
            <div className="text-sm font-semibold text-gray-200">Feedback & gesture</div>
            <textarea value={bubble} onChange={e => setBubble(e.target.value)} className="w-full h-24 rounded-2xl bg-slate-950/40 border border-neutral-800/70 p-3 text-sm outline-none focus:border-smara-300/40" />
            <div className="grid grid-cols-2 gap-2"><button onClick={() => postEvent({ speech_bubble: bubble, state: 'thinking' })} className="px-3 py-3 rounded-2xl bg-white/5 hover:bg-white/10 border border-neutral-800/70 text-sm flex items-center justify-center gap-2"><RotateCcw className="w-4 h-4" /> Update bubble</button><button onClick={speakDemo} className="px-3 py-3 rounded-2xl bg-lime-500/15 hover:bg-lime-500/25 border border-lime-300/20 text-sm flex items-center justify-center gap-2"><Volume2 className="w-4 h-4" /> Speak + lip sync</button><button onClick={() => postEvent({ state: 'acting', gesture: 'pointer-guide', speech_bubble: 'Saya mengarahkan Magic Pointer ke target UI.' })} className="px-3 py-3 rounded-2xl bg-smara-500/15 hover:bg-smara-500/25 border border-smara-300/20 text-sm flex items-center justify-center gap-2"><MousePointer2 className="w-4 h-4" /> Pointer gesture</button><button onClick={() => postEvent({ state: 'emergency_stop', speech_bubble: 'Autopilot dihentikan untuk keamanan.' })} className="px-3 py-3 rounded-2xl bg-red-500/15 hover:bg-red-500/25 border border-red-300/20 text-sm flex items-center justify-center gap-2"><Power className="w-4 h-4" /> Emergency</button></div>
            {saved && <div className="text-xs text-emerald-200 flex items-center gap-1"><Zap className="w-3 h-3" /> state tersimpan ke backend</div>}
          </div>
          <div className="rounded-3xl border border-neutral-800/70 bg-slate-950/25 p-4 text-xs text-gray-400">Model aktif: VRM Smara dari <code>/api/avatar/model</code>. Runtime animation aktif: idle tangan natural, blink, smile, dan mouth shape A/I/U/E/O saat speaking.</div>
        </div>
      </div>
    </div>
  )
}
