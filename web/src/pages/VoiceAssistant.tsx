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

type VoiceSettings = { provider: string; language: string; voice_character: string; speed: number; volume: number; streaming: boolean }
type VoicePlan = { transcript: string; intent: string; magic_pointer_args?: string[]; needs_guardrail: boolean; warnings?: string[] }

const defaultSettings: VoiceSettings = { provider: 'browser', language: 'id-ID', voice_character: 'Smara', speed: 1, volume: 1, streaming: true }

export default function VoiceAssistant() {
  const [settings, setSettings] = useState<VoiceSettings>(defaultSettings)
  const [listening, setListening] = useState(false)
  const [speaking, setSpeaking] = useState(false)
  const [transcript, setTranscript] = useState('Smara, buka browser dan cari dokumentasi React')
  const [response, setResponse] = useState('Siap, saya akan bantu menjalankan perintah suara dengan guardrail autopilot.')
  const [plan, setPlan] = useState<VoicePlan | null>(null)
  const [error, setError] = useState('')
  const recognitionRef = useRef<SpeechRecognitionLike | null>(null)

  useEffect(() => { fetchJSON<VoiceSettings>('/api/voice/settings').then(setSettings).catch(() => {}) }, [])

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

  const speak = () => {
    setError('')
    if (!('speechSynthesis' in window)) { setError('speechSynthesis tidak tersedia di browser ini.'); return }
    window.speechSynthesis.cancel()
    const utter = new SpeechSynthesisUtterance(response)
    utter.lang = settings.language
    utter.rate = settings.speed
    utter.volume = settings.volume
    utter.onend = () => setSpeaking(false)
    setSpeaking(true)
    window.speechSynthesis.speak(utter)
  }
  const interrupt = () => { window.speechSynthesis?.cancel(); setSpeaking(false); stopListening() }

  return (
    <div className="h-full overflow-y-auto p-6 space-y-5">
      <div className="flex items-center justify-between">
        <div>
          <div className="flex items-center gap-2 text-cyan-200 font-semibold"><Mic className="w-5 h-5" /> Voice Assistant</div>
          <p className="text-sm text-gray-400 mt-1">Phase 4 MVP: push-to-talk, STT Bahasa Indonesia via browser, TTS, interrupt/barge-in, settings, dan routing ke Magic Pointer.</p>
        </div>
        <div className={`px-3 py-1 rounded-full text-xs border ${listening ? 'border-cyan-400/40 text-cyan-100 bg-cyan-500/10' : speaking ? 'border-fuchsia-400/40 text-fuchsia-100 bg-fuchsia-500/10' : 'border-white/10 text-gray-400 bg-white/5'}`}>{listening ? 'listening' : speaking ? 'speaking' : 'idle'}</div>
      </div>

      <div className="grid lg:grid-cols-[1fr_.9fr] gap-5">
        <div className="rounded-3xl border border-white/10 bg-black/25 p-5 space-y-4">
          <div className="flex items-center gap-2 text-sm font-semibold text-gray-200"><Mic className="w-4 h-4 text-cyan-300" /> Push-to-talk</div>
          {!supported && <div className="text-xs text-amber-200 bg-amber-500/10 border border-amber-300/20 rounded-2xl p-3">Web Speech API tidak tersedia; fallback CLI tetap ada: <code>smara voice transcribe --audio file.wav</code></div>}
          <textarea value={transcript} onChange={e => setTranscript(e.target.value)} className="w-full h-32 rounded-2xl bg-black/40 border border-white/10 p-3 text-sm outline-none focus:border-cyan-300/40" />
          <div className="flex flex-wrap gap-2">
            <button onClick={listening ? stopListening : startListening} className="px-4 py-3 rounded-2xl bg-cyan-500/20 hover:bg-cyan-500/30 border border-cyan-300/20 text-cyan-100 text-sm flex items-center gap-2">{listening ? <MicOff className="w-4 h-4" /> : <Mic className="w-4 h-4" />} {listening ? 'Stop listening' : 'Push to talk'}</button>
            <button onClick={submitCommand} className="px-4 py-3 rounded-2xl bg-emerald-500/15 hover:bg-emerald-500/25 border border-emerald-300/20 text-emerald-100 text-sm flex items-center gap-2"><Send className="w-4 h-4" /> Plan command</button>
            <button onClick={interrupt} className="px-4 py-3 rounded-2xl bg-red-500/15 hover:bg-red-500/25 border border-red-300/20 text-red-100 text-sm flex items-center gap-2"><Square className="w-4 h-4" /> Interrupt</button>
          </div>
          {error && <div className="text-sm text-red-200 bg-red-500/10 border border-red-300/20 rounded-2xl p-3">{error}</div>}
        </div>

        <div className="rounded-3xl border border-white/10 bg-black/25 p-5 space-y-4">
          <div className="flex items-center gap-2 text-sm font-semibold text-gray-200"><Settings2 className="w-4 h-4 text-fuchsia-300" /> Voice settings</div>
          <div className="grid grid-cols-2 gap-3 text-sm">
            <label className="space-y-1"><span className="text-xs text-gray-500">Provider</span><select value={settings.provider} onChange={e => setSettings({ ...settings, provider: e.target.value })} className="w-full rounded-xl bg-black/40 border border-white/10 p-2"><option>browser</option><option>auto</option><option>piper</option></select></label>
            <label className="space-y-1"><span className="text-xs text-gray-500">Language</span><input value={settings.language} onChange={e => setSettings({ ...settings, language: e.target.value })} className="w-full rounded-xl bg-black/40 border border-white/10 p-2" /></label>
            <label className="space-y-1"><span className="text-xs text-gray-500">Speed</span><input type="number" step="0.1" value={settings.speed} onChange={e => setSettings({ ...settings, speed: Number(e.target.value) })} className="w-full rounded-xl bg-black/40 border border-white/10 p-2" /></label>
            <label className="space-y-1"><span className="text-xs text-gray-500">Volume</span><input type="number" step="0.1" min="0" max="1" value={settings.volume} onChange={e => setSettings({ ...settings, volume: Number(e.target.value) })} className="w-full rounded-xl bg-black/40 border border-white/10 p-2" /></label>
          </div>
          <textarea value={response} onChange={e => setResponse(e.target.value)} className="w-full h-24 rounded-2xl bg-black/40 border border-white/10 p-3 text-sm outline-none" />
          <button onClick={speak} className="w-full px-4 py-3 rounded-2xl bg-fuchsia-500/15 hover:bg-fuchsia-500/25 border border-fuchsia-300/20 text-fuchsia-100 text-sm flex items-center justify-center gap-2"><Volume2 className="w-4 h-4" /> Speak response</button>
        </div>
      </div>

      <div className="rounded-3xl border border-white/10 bg-black/25 p-5 space-y-3">
        <div className="flex items-center gap-2 text-sm font-semibold text-gray-200"><MousePointer2 className="w-4 h-4 text-cyan-300" /> Voice → Magic Pointer</div>
        {!plan && <div className="text-sm text-gray-500">Belum ada plan. Tekan “Plan command”.</div>}
        {plan && <div className="space-y-2 text-sm"><div>Intent: <span className="text-cyan-200">{plan.intent}</span></div>{plan.magic_pointer_args && <div className="rounded-2xl bg-white/[0.04] border border-white/10 p-3 font-mono text-xs">smara {plan.magic_pointer_args.join(' ')}</div>}{plan.warnings?.map((w, i) => <div key={i} className="text-amber-200">⚠ {w}</div>)}</div>}
      </div>
    </div>
  )
}
