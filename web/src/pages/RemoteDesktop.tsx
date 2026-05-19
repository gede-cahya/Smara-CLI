import { useEffect, useState } from 'react'
import { Monitor, Link2, Play, Square, RefreshCw, ShieldCheck, Camera } from 'lucide-react'
import { fetchJSON } from '../api'

type Device = { id: string; name: string; url: string; token?: string; created_at: string; updated_at: string }
type ProxyResult = { ok: boolean; result?: any; error?: string }

export default function RemoteDesktop() {
  const [devices, setDevices] = useState<Device[]>([])
  const [name, setName] = useState('local-desktop')
  const [url, setUrl] = useState('http://127.0.0.1:8765')
  const [token, setToken] = useState('')
  const [selected, setSelected] = useState('local-desktop')
  const [instruction, setInstruction] = useState('Buka browser lalu cari dokumentasi Go terbaru')
  const [maxSteps, setMaxSteps] = useState(10)
  const [status, setStatus] = useState<string>('idle')
  const [result, setResult] = useState<ProxyResult | null>(null)
  const [shotTick, setShotTick] = useState(Date.now())

  const load = async () => {
    const r = await fetchJSON<{ devices: Device[] }>('/api/remote-desktop/devices')
    setDevices(r.devices || [])
    if ((r.devices || []).length && !r.devices.find(d => d.id === selected)) setSelected(r.devices[0].id)
  }
  useEffect(() => { load().catch(e => setStatus(e.message)) }, [])

  const pair = async () => {
    setStatus('pairing...')
    const d = await fetchJSON<Device>('/api/remote-desktop/devices', { method: 'POST', body: JSON.stringify({ name, url, token }) })
    setSelected(d.id); setStatus('paired'); await load()
  }
  const proxy = async (action: string, payload?: any) => {
    setStatus(action + '...')
    const r = await fetchJSON<ProxyResult>('/api/remote-desktop/proxy', { method: 'POST', body: JSON.stringify({ device_id: selected, action, payload }) })
    setResult(r); setStatus(r.ok ? 'ok' : (r.error || 'error'))
    if (action === 'observe' || action === 'task') setShotTick(Date.now())
  }

  const screenshotURL = `/api/remote-desktop/screenshot?device_id=${encodeURIComponent(selected)}&t=${shotTick}`

  return <div className="h-full overflow-y-auto p-6 space-y-5">
    <div className="flex items-center justify-between">
      <div><div className="flex items-center gap-2 text-cyan-200 font-semibold"><Monitor className="w-5 h-5" /> Remote Desktop Assistant</div>
      <p className="text-sm text-gray-400 mt-1">Phase 6: pair desktop-agent, live observation, remote Magic Pointer task, audit log, dan emergency stop.</p></div>
      <div className="px-3 py-1 rounded-full text-xs border border-cyan-300/20 bg-cyan-500/10 text-cyan-100">{status}</div>
    </div>

    <div className="grid lg:grid-cols-[.9fr_1.2fr] gap-5">
      <div className="space-y-5">
        <div className="rounded-3xl border border-white/10 bg-black/25 p-5 space-y-3">
          <div className="flex items-center gap-2 text-sm font-semibold"><Link2 className="w-4 h-4 text-fuchsia-300" /> Pair desktop-agent</div>
          <input value={name} onChange={e=>setName(e.target.value)} className="w-full rounded-2xl bg-black/40 border border-white/10 p-3 text-sm outline-none" placeholder="Device name" />
          <input value={url} onChange={e=>setUrl(e.target.value)} className="w-full rounded-2xl bg-black/40 border border-white/10 p-3 text-sm outline-none" placeholder="http://host:8765" />
          <input value={token} onChange={e=>setToken(e.target.value)} className="w-full rounded-2xl bg-black/40 border border-white/10 p-3 text-sm outline-none" placeholder="token opsional" type="password" />
          <button onClick={pair} className="w-full px-4 py-3 rounded-2xl bg-cyan-500/20 hover:bg-cyan-500/30 border border-cyan-300/20 text-cyan-100 text-sm">Pair / Update</button>
          <div className="text-xs text-gray-500 bg-white/[0.03] rounded-2xl p-3">Jalankan target: <code>smara desktop-agent --addr 0.0.0.0:8765 --token TOKEN</code></div>
        </div>
        <div className="rounded-3xl border border-white/10 bg-black/25 p-5 space-y-3">
          <div className="text-sm font-semibold">Devices</div>
          <select value={selected} onChange={e=>setSelected(e.target.value)} className="w-full rounded-2xl bg-black/40 border border-white/10 p-3 text-sm outline-none">
            {devices.map(d => <option key={d.id} value={d.id}>{d.name} — {d.url}</option>)}
          </select>
          <div className="grid grid-cols-2 gap-2">
            <button onClick={()=>proxy('health')} className="px-3 py-2 rounded-xl bg-white/8 border border-white/10 text-sm">Health</button>
            <button onClick={()=>proxy('observe')} className="px-3 py-2 rounded-xl bg-white/8 border border-white/10 text-sm flex items-center justify-center gap-2"><Camera className="w-4 h-4"/>Observe</button>
            <button onClick={()=>proxy('stop')} className="px-3 py-2 rounded-xl bg-red-500/15 border border-red-300/20 text-red-100 text-sm flex items-center justify-center gap-2"><Square className="w-4 h-4"/>Stop</button>
            <button onClick={()=>proxy('resume')} className="px-3 py-2 rounded-xl bg-emerald-500/15 border border-emerald-300/20 text-emerald-100 text-sm flex items-center justify-center gap-2"><RefreshCw className="w-4 h-4"/>Resume</button>
          </div>
        </div>
      </div>

      <div className="space-y-5">
        <div className="rounded-3xl border border-white/10 bg-black/25 p-5 space-y-3">
          <div className="flex items-center gap-2 text-sm font-semibold"><Play className="w-4 h-4 text-cyan-300"/> Remote Magic Pointer task</div>
          <textarea value={instruction} onChange={e=>setInstruction(e.target.value)} className="w-full h-24 rounded-2xl bg-black/40 border border-white/10 p-3 text-sm outline-none" />
          <input type="number" min={1} max={30} value={maxSteps} onChange={e=>setMaxSteps(Number(e.target.value))} className="w-full rounded-2xl bg-black/40 border border-white/10 p-3 text-sm outline-none" />
          <button onClick={()=>proxy('task', { instruction, max_steps: maxSteps, assume_yes: true })} className="w-full px-4 py-3 rounded-2xl bg-gradient-to-r from-cyan-500/25 to-fuchsia-500/25 border border-cyan-300/20 text-cyan-50 text-sm">Run remote autopilot</button>
        </div>
        <div className="rounded-3xl border border-white/10 bg-black/25 p-4">
          <div className="flex items-center gap-2 text-sm font-semibold mb-3"><ShieldCheck className="w-4 h-4 text-emerald-300"/> Live desktop observation</div>
          <img src={screenshotURL} onError={(e)=>{(e.currentTarget.style.display='none')}} onLoad={(e)=>{(e.currentTarget.style.display='block')}} className="w-full rounded-2xl border border-white/10 bg-black/40" />
        </div>
        <pre className="rounded-3xl border border-white/10 bg-black/40 p-4 text-xs text-gray-300 overflow-auto max-h-72">{result ? JSON.stringify(result, null, 2) : 'Belum ada result.'}</pre>
      </div>
    </div>
  </div>
}
