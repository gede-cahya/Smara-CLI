import { useEffect, useMemo, useRef, useState } from 'react'
import { Mic, MicOff, Volume2, Square, Send, Settings2, MousePointer2 } from 'lucide-react'
import { fetchJSON } from '../api'

type SpeechRecognitionCtor = new () => SpeechRecognitionLike
type SpeechRecognitionLike = {
  lang: string
  interimResults: boolean
  continuous: boolean
  onresult: ((event: { results: ArrayLike<{ 0: { transcript: string }, isFinal: boolean }> }) => void) | null
  onend: (() => void) | null
  start: () => void
  stop: () => void
}

type VoiceSettings = {
  provider: string
  language: string
  voice_character: string
  model_id: string
  speed: number
  volume: number
  streaming: boolean
  base_url?: string
}
type VoicePlan = { transcript: string; intent: string; magic_pointer_args?: string[]; needs_guardrail: boolean; warnings?: string[] }

const defaultSettings: VoiceSettings = { provider: 'browser', language: 'id-ID', voice_character: 'ngvNHfiCrXLPAHcTrZK1', model_id: 'eleven_multilingual_v2', speed: 1, volume: 1, streaming: true, base_url: 'https://api.elevenlabs.io' }

export default function VoiceAssistant() {
  const [settings, setSettings] = useState<VoiceSettings>(defaultSettings)
  const [listening, setListening] = useState(false)
  const [speaking, setSpeaking] = useState(false)
  const [transcript, setTranscript] = useState('Smara, buka browser dan cari dokumentasi React')
  const [response, setResponse] = useState('Siap, saya akan bantu menjalankan perintah suara dengan guardrail autopilot.')
  const [plan, setPlan] = useState<VoicePlan | null>(null)
  const [error, setError] = useState('')
  const recognitionRef = useRef<SpeechRecognitionLike | null>(null)

  useEffect(() => { fetchJSON<VoiceSettings>('/api/voice/settings').then(cfg => setSettings({ ...defaultSettings, ...cfg })).catch(() => {}) }, [])

  const supported = useMemo(() => typeof window !== 'undefined' && Boolean((window as unknown as { SpeechRecognition?: SpeechRecognitionCtor; webkitSpeechRecognition?: SpeechRecognitionCtor }).SpeechRecognition || (window as unknown as { webkitSpeechRecognition?: SpeechRecognitionCtor }).webkitSpeechRecognition), [])

  const startListening = () => {
    setError('')
    const w = window as unknown as { SpeechRecognition?: SpeechRecognitionCtor; webkitSpeechRecognition?: SpeechRecognitionCtor }
    const Ctor = w.SpeechRecognition || w.webkitSpeechRecognition
    if (!Ctor) { setError('Browser Web Speech API tidak tersedia. Gunakan Chrome/Edge atau CLI smara voice transcribe.'); return }
    const rec = new Ctor()
    rec.lang = settings.language || 'id-ID'
    rec.interimResults = true
    rec.continuous = false
    rec.onresult = event => {
      let text = ''
      for (let i = 0; i < event.results.length; i++) text += event.results[i][0].transcript
      setTranscript(text.trim())
    }
    rec.onend = () => setListening(false)
    recognitionRef.current = rec
    setListening(true)
    rec.start()
  }

  const stopListening = () => { recognitionRef.current?.stop(); setListening(false) }

  const submitCommand = async () => {
    setError('')
    try {
      const p = await fetchJSON<VoicePlan>('/api/voice/command', { method: 'POST', body: JSON.stringify({ transcript, language: settings.language, autopilot: true, max_steps: 10, source: 'web' }) })
      setPlan(p)
      setResponse(p.intent === 'magic_pointer' ? 'Saya mengenali ini sebagai perintah desktop. Magic Pointer siap menjalankan dengan guardrail dan emergency stop.' : 'Saya menerima perintah suara dan akan menjawab di sesi chat.')
    } catch (e) { setError(e instanceof Error ? e.message : String(e)) }
  }

  const speakBrowser = (text: string) => {
    if (!('speechSynthesis' in window)) { setError('speechSynthesis tidak tersedia di browser ini.'); return }
    window.speechSynthesis.cancel()
    const utter = new SpeechSynthesisUtterance(text)
    utter.lang = settings.language
    utter.rate = settings.speed
    utter.volume = settings.volume
    utter.onend = () => setSpeaking(false)
    setSpeaking(true)
    window.speechSynthesis.speak(utter)
  }

  const speak = async () => {
    setError('')
    try {
      window.speechSynthesis?.cancel()
      if (settings.provider === 'elevenlabs') {
        setSpeaking(true)
        const res = await fetch('/api/voice/speak', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ text: response, settings })
        })
        if (!res.ok) {
          const msg = await res.text()
          setError(`${msg} — fallback ke browser voice.`)
          speakBrowser(response)
          return
        }
        const contentType = res.headers.get('Content-Type') || ''
        if (!contentType.toLowerCase().startsWith('audio/')) {
          const msg = await res.text().catch(() => `Response bukan audio (${contentType || 'unknown content-type'})`)
          setError(`${msg} — fallback ke browser voice.`)
          speakBrowser(response)
          return
        }
        const blob = await res.blob()
        const url = URL.createObjectURL(blob)
        const audio = new Audio(url)
        audio.volume = settings.volume
        audio.onended = () => { setSpeaking(false); URL.revokeObjectURL(url) }
        audio.onerror = () => { setSpeaking(false); URL.revokeObjectURL(url); setError('Gagal memutar audio ElevenLabs'); speakBrowser(response) }
        await audio.play()
        return
      }
      speakBrowser(response)
    } catch (e) {
      setSpeaking(false)
      setError(e instanceof Error ? e.message : String(e))
    }
  }
  const interrupt = () => { window.speechSynthesis?.cancel(); setSpeaking(false); stopListening() }

  return (
    <div className="h-full overflow-y-auto p-6 space-y-5">
      <div className="flex items-center justify-between">
        <div>
          <div className="flex items-center gap-2 text-smara-200 font-semibold"><Mic className="w-5 h-5" /> Voice Assistant</div>
          <div className="text-xs text-gray-500">Hands-free command, speech synthesis, dan autopilot desktop.</div>
        </div>
      </div>

      <div className="grid lg:grid-cols-[1fr_.9fr] gap-5">
        <div className="rounded-3xl border border-neutral-800/70 bg-slate-950/25 p-5 space-y-4">
          <div className="flex items-center gap-2 text-sm font-semibold text-gray-200"><Mic className="w-4 h-4 text-smara-300" /> Push-to-talk</div>
          {!supported && <div className="text-xs text-amber-200 bg-amber-500/10 border border-amber-300/20 rounded-2xl p-3">Web Speech API tidak tersedia; fallback CLI tetap ada: <code>smara voice transcribe --audio file.wav</code></div>}
          <textarea value={transcript} onChange={e => setTranscript(e.target.value)} className="w-full h-32 rounded-2xl bg-slate-950/40 border border-neutral-800/70 p-3 text-sm outline-none focus:border-smara-300/40" />
          <div className="flex flex-wrap gap-2">
            <button onClick={listening ? stopListening : startListening} className="px-4 py-3 rounded-2xl bg-smara-500/20 hover:bg-smara-500/30 border border-smara-300/20 text-smara-100 text-sm flex items-center gap-2">{listening ? <MicOff className="w-4 h-4" /> : <Mic className="w-4 h-4" />} {listening ? 'Stop listening' : 'Push to talk'}</button>
            <button onClick={submitCommand} className="px-4 py-3 rounded-2xl bg-emerald-400/15 hover:bg-emerald-400/25 border border-emerald-300/20 text-emerald-100 text-sm flex items-center gap-2"><Send className="w-4 h-4" /> Plan command</button>
            <button onClick={interrupt} className="px-4 py-3 rounded-2xl bg-red-500/15 hover:bg-red-500/25 border border-red-300/20 text-red-100 text-sm flex items-center gap-2"><Square className="w-4 h-4" /> Interrupt</button>
          </div>
          {error && <div className="text-sm text-red-200 bg-red-500/10 border border-red-300/20 rounded-2xl p-3">{error}</div>}
        </div>

        <div className="rounded-3xl border border-neutral-800/70 bg-slate-950/25 p-5 space-y-4">
          <div className="flex items-center gap-2 text-sm font-semibold text-gray-200"><Settings2 className="w-4 h-4 text-lime-300" /> Voice settings</div>
          <div className="grid grid-cols-2 gap-3 text-sm">
            <label className="space-y-1"><span className="text-xs text-gray-500">Provider</span><select value={settings.provider} onChange={e => setSettings({ ...settings, provider: e.target.value })} className="w-full rounded-xl bg-slate-950/40 border border-neutral-800/70 p-2"><option>browser</option><option>elevenlabs</option><option>auto</option><option>piper</option><option>whisper</option><option>mock</option></select></label>
            <label className="space-y-1"><span className="text-xs text-gray-500">Language</span><input value={settings.language} onChange={e => setSettings({ ...settings, language: e.target.value })} className="w-full rounded-xl bg-slate-950/40 border border-neutral-800/70 p-2" /></label>
            <label className="space-y-1 col-span-2"><span className="text-xs text-gray-500">Voice ID / Character</span><input value={settings.voice_character} onChange={e => setSettings({ ...settings, voice_character: e.target.value })} className="w-full rounded-xl bg-slate-950/40 border border-neutral-800/70 p-2 font-mono text-xs" /></label>
            <label className="space-y-1"><span className="text-xs text-gray-500">Model ID</span><select value={settings.model_id} onChange={e => setSettings({ ...settings, model_id: e.target.value })} className="w-full rounded-xl bg-slate-950/40 border border-neutral-800/70 p-2"><option>eleven_multilingual_v2</option><option>eleven_turbo_v2_5</option><option>eleven_flash_v2_5</option><option>eleven_v3</option></select></label>
            <label className="space-y-1"><span className="text-xs text-gray-500">Base URL</span><input value={settings.base_url || ''} onChange={e => setSettings({ ...settings, base_url: e.target.value })} placeholder="https://api.elevenlabs.io" className="w-full rounded-xl bg-slate-950/40 border border-neutral-800/70 p-2 font-mono text-xs" /></label>
            <label className="space-y-1"><span className="text-xs text-gray-500">Speed</span><input type="number" step="0.1" value={settings.speed} onChange={e => setSettings({ ...settings, speed: Number(e.target.value) })} className="w-full rounded-xl bg-slate-950/40 border border-neutral-800/70 p-2" /></label>
            <label className="space-y-1"><span className="text-xs text-gray-500">Volume</span><input type="number" step="0.1" min="0" max="1" value={settings.volume} onChange={e => setSettings({ ...settings, volume: Number(e.target.value) })} className="w-full rounded-xl bg-slate-950/40 border border-neutral-800/70 p-2" /></label>
            <label className="col-span-2 flex items-center gap-2 text-xs text-gray-400"><input type="checkbox" checked={settings.streaming} onChange={e => setSettings({ ...settings, streaming: e.target.checked })} className="rounded border-neutral-700 bg-slate-950/40" /> Streaming mengikuti config</label>
          </div>
          <textarea value={response} onChange={e => setResponse(e.target.value)} className="w-full h-24 rounded-2xl bg-slate-950/40 border border-neutral-800/70 p-3 text-sm outline-none" />
          <button onClick={speak} className="w-full px-4 py-3 rounded-2xl bg-lime-500/15 hover:bg-lime-500/25 border border-lime-300/20 text-lime-100 text-sm flex items-center justify-center gap-2"><Volume2 className="w-4 h-4" /> {speaking ? 'Speaking…' : 'Speak response'}</button>
        </div>
      </div>

      <div className="rounded-3xl border border-neutral-800/70 bg-slate-950/25 p-5 space-y-3">
        <div className="flex items-center gap-2 text-sm font-semibold text-gray-200"><MousePointer2 className="w-4 h-4 text-smara-300" /> Voice → Magic Pointer</div>
        {!plan && <div className="text-sm text-gray-500">Belum ada plan. Tekan “Plan command”.</div>}
        {plan && <div className="space-y-2 text-sm"><div>Intent: <span className="text-smara-200">{plan.intent}</span></div>{plan.magic_pointer_args && <div className="rounded-2xl bg-white/[0.04] border border-neutral-800/70 p-3 font-mono text-xs">smara {plan.magic_pointer_args.join(' ')}</div>}{plan.warnings?.map((w, i) => <div key={i} className="text-amber-200">⚠ {w}</div>)}</div>}
      </div>
    </div>
  )
}
