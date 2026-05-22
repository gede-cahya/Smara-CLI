import { useEffect, useMemo, useState } from 'react'
import { MousePointer2, Play, Square, Eye, Activity } from 'lucide-react'

type PointerEvent = {
  type: string
  target?: string
  x?: number
  y?: number
  value?: string
  success?: boolean
  error?: string
  timestamp?: string
}

const demoEvents: PointerEvent[] = [
  { type: 'observe', target: 'desktop', success: true },
  { type: 'open_app', target: 'browser', success: true },
  { type: 'key', value: 'ctrl+l', success: true },
  { type: 'type', value: '[TYPED_TEXT_REDACTED]', success: true },
  { type: 'key', value: 'Return', success: true },
]

export default function MagicPointer() {
  const [running, setRunning] = useState(false)
  const [step, setStep] = useState(0)
  const [maxSteps, setMaxSteps] = useState(10)
  const [instruction, setInstruction] = useState('Buka browser lalu cari dokumentasi Go terbaru')
  const [events, setEvents] = useState<PointerEvent[]>([])

  useEffect(() => {
    if (!running) return
    const id = window.setInterval(() => {
      setStep(s => {
        const next = s + 1
        const ev = demoEvents[Math.min(next - 1, demoEvents.length - 1)]
        setEvents(prev => [...prev, { ...ev, timestamp: new Date().toISOString() }])
        if (next >= Math.min(maxSteps, demoEvents.length)) setRunning(false)
        return next
      })
    }, 650)
    return () => window.clearInterval(id)
  }, [running, maxSteps])

  const pos = useMemo(() => ({ left: `${18 + (step * 13) % 68}%`, top: `${28 + (step * 9) % 46}%` }), [step])

  const startPreview = () => {
    setEvents([])
    setStep(0)
    setRunning(true)
  }
  const stop = () => setRunning(false)

  return (
    <div className="h-full overflow-y-auto p-6 space-y-5">
      <div className="flex items-center justify-between">
        <div>
          <div className="flex items-center gap-2 text-smara-200 font-semibold"><MousePointer2 className="w-5 h-5" /> Magic Pointer Autopilot</div>
          <p className="text-sm text-gray-400 mt-1">Visual pointer preview, action replay log, max steps, dan stop condition untuk Phase 3.</p>
        </div>
        <div className={`px-3 py-1 rounded-full text-xs border ${running ? 'border-emerald-400/40 text-emerald-200 bg-emerald-400/10' : 'border-neutral-800/70 text-gray-400 bg-white/5'}`}>
          {running ? 'acting' : 'idle'}
        </div>
      </div>

      <div className="grid lg:grid-cols-[1.3fr_.9fr] gap-5">
        <div className="relative min-h-[420px] rounded-3xl border border-neutral-800/70 bg-gradient-to-br from-slate-950 via-slate-900 to-black overflow-hidden shadow-2xl">
          <div className="absolute inset-4 rounded-2xl border border-smara-300/10 bg-slate-950/25">
            <div className="h-9 border-b border-neutral-800/70 flex items-center gap-2 px-4 text-xs text-gray-400">
              <Eye className="w-3 h-3 text-smara-300" /> Desktop observation preview
            </div>
            <div className="p-5 grid grid-cols-2 gap-4 opacity-80">
              <div className="h-24 rounded-2xl bg-white/8 border border-neutral-800/70" />
              <div className="h-24 rounded-2xl bg-white/8 border border-neutral-800/70" />
              <div className="col-span-2 h-40 rounded-2xl bg-smara-500/8 border border-smara-300/10 flex items-center justify-center text-gray-500 text-sm">OCR / UI targets</div>
            </div>
          </div>
          <div className="absolute transition-all duration-500 ease-out drop-shadow-[0_0_18px_rgba(34,211,238,.9)]" style={pos}>
            <MousePointer2 className="w-9 h-9 text-smara-200 fill-smara-300/20" />
            <div className="absolute -left-2 -top-2 w-14 h-14 rounded-full border border-smara-300/30 animate-ping" />
          </div>
        </div>

        <div className="rounded-3xl border border-neutral-800/70 bg-slate-950/25 p-5 space-y-4">
          <label className="text-xs uppercase tracking-widest text-gray-500">Instruksi natural language</label>
          <textarea value={instruction} onChange={e => setInstruction(e.target.value)} className="w-full h-24 rounded-2xl bg-slate-950/40 border border-neutral-800/70 p-3 text-sm outline-none focus:border-smara-300/40" />
          <div>
            <label className="text-xs uppercase tracking-widest text-gray-500">Max steps</label>
            <input type="number" min={1} max={20} value={maxSteps} onChange={e => setMaxSteps(Number(e.target.value))} className="mt-2 w-full rounded-2xl bg-slate-950/40 border border-neutral-800/70 p-3 text-sm outline-none" />
          </div>
          <div className="flex gap-2">
            <button onClick={startPreview} className="flex-1 px-4 py-3 rounded-2xl bg-smara-500/20 hover:bg-smara-500/30 border border-smara-300/20 text-smara-100 text-sm flex items-center justify-center gap-2"><Play className="w-4 h-4" /> Preview</button>
            <button onClick={stop} className="px-4 py-3 rounded-2xl bg-red-500/15 hover:bg-red-500/25 border border-red-300/20 text-red-100 text-sm"><Square className="w-4 h-4" /></button>
          </div>
          <div className="text-xs text-gray-500 bg-white/[0.03] rounded-2xl p-3">CLI nyata: <code>smara magic-pointer --ask "{instruction}" --autopilot --execute --yes --max-steps {maxSteps}</code></div>
        </div>
      </div>

      <div className="rounded-3xl border border-neutral-800/70 bg-slate-950/25 p-5">
        <div className="flex items-center gap-2 text-sm font-semibold text-gray-200 mb-3"><Activity className="w-4 h-4 text-lime-300" /> Action replay log</div>
        <div className="space-y-2">
          {events.length === 0 && <div className="text-sm text-gray-500">Belum ada event. Klik Preview untuk melihat simulasi pointer movement.</div>}
          {events.map((e, i) => <div key={i} className="flex items-center justify-between rounded-2xl bg-white/[0.04] border border-neutral-800/70 px-4 py-3 text-sm"><span>{i + 1}. {e.type} {e.target || e.value || ''}</span><span className={e.success === false ? 'text-red-300' : 'text-emerald-300'}>{e.success === false ? e.error || 'fail' : 'ok'}</span></div>)}
        </div>
      </div>
    </div>
  )
}
