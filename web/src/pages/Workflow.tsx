import { useState, useEffect, useRef } from 'react'
import { GitBranch, Sparkles, Play, FileText, Layers, CheckCircle, AlertCircle, BrainCircuit, Terminal, ChevronDown, ChevronUp } from 'lucide-react'
import { fetchJSON, APIError, type Blueprint } from '../api'

const spinnerFrames = ['\u280B','\u2819','\u2839','\u2838','\u283C','\u2834','\u2826','\u2827','\u2807','\u280F']

interface WorkflowItem {
  id: string
  blueprint: Blueprint
  result: string | null
  timestamp: number
}

const STORAGE_KEY = 'smara_workflow_history'
const ACTIVE_ID_KEY = 'smara_workflow_active_id'
const PROMPT_KEY = 'smara_workflow_prompt'
const PHASES_KEY = 'smara_workflow_phases'
const THOUGHTS_KEY = 'smara_workflow_thoughts'
const ERROR_KEY = 'smara_workflow_error'
const LAST_REQ_KEY = 'smara_workflow_last_req'
const LAST_RES_KEY = 'smara_workflow_last_res'
const SHOW_DEBUG_KEY = 'smara_workflow_show_debug'

export default function Workflow() {
  const [prompt, setPrompt] = useState('')
  const [history, setHistory] = useState<WorkflowItem[]>([])
  const [activeId, setActiveId] = useState<string | null>(null)
  const [generating, setGenerating] = useState(false)
  const [executing, setExecuting] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [thoughts, setThoughts] = useState<string[]>([])
  const [phases, setPhases] = useState<Array<{phase:string;description:string;status:'running'|'done'}>>([])
  const [spinnerIdx, setSpinnerIdx] = useState(0)
  const [lastRequest, setLastRequest] = useState<string | null>(null)
  const [lastResponse, setLastResponse] = useState<string | null>(null)
  const [showDebug, setShowDebug] = useState(false)
  const timer = useRef<ReturnType<typeof setInterval>|null>(null)

  const activeItem = history.find(h => h.id === activeId) || null
  const blueprint = activeItem?.blueprint || null
  const result = activeItem?.result || null

  useEffect(() => {
    if (generating || executing) {
      timer.current = setInterval(() => setSpinnerIdx(i => (i+1)%spinnerFrames.length), 80)
    } else if (timer.current) {
      clearInterval(timer.current); timer.current = null
    }
    return () => { if (timer.current) clearInterval(timer.current) }
  }, [generating, executing])

  // Load history + transient state from localStorage on mount
  useEffect(() => {
    const saved = localStorage.getItem(STORAGE_KEY)
    if (saved) {
      try {
        const parsed: WorkflowItem[] = JSON.parse(saved)
        setHistory(parsed)
      } catch { /* ignore */ }
    }
    const savedActive = localStorage.getItem(ACTIVE_ID_KEY)
    if (savedActive) setActiveId(savedActive)
    const savedPrompt = localStorage.getItem(PROMPT_KEY)
    if (savedPrompt) setPrompt(savedPrompt)

    // Restore transient UI state
    const savedPhases = localStorage.getItem(PHASES_KEY)
    if (savedPhases) {
      try {
        const parsed = JSON.parse(savedPhases)
        // If we were interrupted while running, mark all phases as done
        // because we can't reconnect to the in-flight HTTP request
        const restored = parsed.map((p: any) => ({ ...p, status: 'done' as const }))
        setPhases(restored)
      } catch { /* ignore */ }
    }
    const savedThoughts = localStorage.getItem(THOUGHTS_KEY)
    if (savedThoughts) {
      try { setThoughts(JSON.parse(savedThoughts)) } catch { /* ignore */ }
    }
    const savedError = localStorage.getItem(ERROR_KEY)
    if (savedError) setError(savedError)
    const savedLastReq = localStorage.getItem(LAST_REQ_KEY)
    if (savedLastReq) setLastRequest(savedLastReq)
    const savedLastRes = localStorage.getItem(LAST_RES_KEY)
    if (savedLastRes) setLastResponse(savedLastRes)
    const savedShowDebug = localStorage.getItem(SHOW_DEBUG_KEY)
    if (savedShowDebug) setShowDebug(savedShowDebug === 'true')
  }, [])

  // Save history to localStorage whenever it changes
  useEffect(() => {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(history))
  }, [history])

  // Save activeId to localStorage
  useEffect(() => {
    if (activeId) localStorage.setItem(ACTIVE_ID_KEY, activeId)
    else localStorage.removeItem(ACTIVE_ID_KEY)
  }, [activeId])

  // Save prompt to localStorage
  useEffect(() => {
    if (prompt) localStorage.setItem(PROMPT_KEY, prompt)
    else localStorage.removeItem(PROMPT_KEY)
  }, [prompt])

  // Persist transient state
  useEffect(() => {
    if (phases.length > 0) localStorage.setItem(PHASES_KEY, JSON.stringify(phases))
    else localStorage.removeItem(PHASES_KEY)
  }, [phases])

  useEffect(() => {
    if (thoughts.length > 0) localStorage.setItem(THOUGHTS_KEY, JSON.stringify(thoughts))
    else localStorage.removeItem(THOUGHTS_KEY)
  }, [thoughts])

  useEffect(() => {
    if (error) localStorage.setItem(ERROR_KEY, error)
    else localStorage.removeItem(ERROR_KEY)
  }, [error])

  useEffect(() => {
    if (lastRequest) localStorage.setItem(LAST_REQ_KEY, lastRequest)
    else localStorage.removeItem(LAST_REQ_KEY)
  }, [lastRequest])

  useEffect(() => {
    if (lastResponse) localStorage.setItem(LAST_RES_KEY, lastResponse)
    else localStorage.removeItem(LAST_RES_KEY)
  }, [lastResponse])

  useEffect(() => {
    localStorage.setItem(SHOW_DEBUG_KEY, String(showDebug))
  }, [showDebug])

  const addPhase = (phase: string, description: string) => {
    setPhases(prev => {
      const next: Array<{ phase: string; description: string; status: 'running' | 'done' }> = prev.map(p => ({ ...p, status: 'done' }))
      const idx = next.findIndex(p => p.phase === phase)
      if (idx >= 0) next[idx] = { phase, description, status: 'running' }
      else next.push({ phase, description, status: 'running' })
      return next
    })
  }

  const clearPhases = () => setPhases([])

  const generate = async () => {
    if (!prompt.trim()) return
    setGenerating(true); setError(null); setThoughts([]); clearPhases(); setLastRequest(null); setLastResponse(null)
    const steps = [
      { phase:'analyze', desc:'Menganalisis permintaan proyek...' },
      { phase:'design', desc:'Merancang arsitektur sistem...' },
      { phase:'plan', desc:'Menyusun PRD & rencana...' },
      { phase:'generate', desc:'Menghasilkan blueprint & agents...' }
    ]
    let stepIdx = 0
    addPhase(steps[0].phase, steps[0].desc)
    const stepTimer = setInterval(() => {
      stepIdx++
      if (stepIdx < steps.length) addPhase(steps[stepIdx].phase, steps[stepIdx].desc)
    }, 1200)

    const reqBody = JSON.stringify({ prompt: prompt.trim() })
    setLastRequest(`POST /api/blueprint/generate\n${reqBody}`)
    console.log('[Workflow] generate start:', reqBody)
    try {
      const bp = await fetchJSON<Blueprint>('/api/blueprint/generate', {
        method: 'POST', body: reqBody
      })
      console.log('[Workflow] generate success:', bp)
      setLastResponse(JSON.stringify(bp, null, 2))
      const newItem: WorkflowItem = {
        id: Date.now().toString(),
        blueprint: bp,
        result: null,
        timestamp: Date.now()
      }
      setHistory(prev => [newItem, ...prev])
      setActiveId(newItem.id)
      addPhase('complete', 'Blueprint berhasil dibuat!')
      if (bp.thoughts) setThoughts(bp.thoughts)
    } catch (e: any) {
      console.error('[Workflow] generate error:', e)
      const msg = e instanceof APIError ? `${e.message}\n\nRaw response:\n${e.raw}` : (e.message || 'Gagal generate blueprint')
      setLastResponse(msg)
      setError(msg)
    } finally {
      clearInterval(stepTimer)
      setGenerating(false)
    }
  }

  const execute = async () => {
    if (!activeItem) return
    setExecuting(true); setError(null); clearPhases(); setLastRequest(null); setLastResponse(null)
    const steps = [
      { phase:'prepare', desc:'Mempersiapkan workspace...' },
      { phase:'agents', desc:'Menjalankan agents...' },
      { phase:'compile', desc:'Mengompilasi hasil...' },
      { phase:'finalize', desc:'Menyelesaikan workflow...' }
    ]
    let stepIdx = 0
    addPhase(steps[0].phase, steps[0].desc)
    const stepTimer = setInterval(() => {
      stepIdx++
      if (stepIdx < steps.length) addPhase(steps[stepIdx].phase, steps[stepIdx].desc)
    }, 2000)

    const reqBody = JSON.stringify({ prompt: prompt.trim() })
    setLastRequest(`POST /api/blueprint/execute\n${reqBody}`)
    console.log('[Workflow] execute start:', reqBody)
    try {
      const res = await fetchJSON('/api/blueprint/execute', {
        method: 'POST', body: reqBody
      })
      console.log('[Workflow] execute success:', res)
      const resStr = JSON.stringify(res, null, 2)
      setLastResponse(resStr)
      setHistory(prev => prev.map(h => h.id === activeId ? { ...h, result: resStr } : h))
      addPhase('done', 'Workflow selesai!')
    } catch (e: any) {
      console.error('[Workflow] execute error:', e)
      const msg = e instanceof APIError ? `${e.message}\n\nRaw response:\n${e.raw}` : (e.message || 'Gagal eksekusi workflow')
      setLastResponse(msg)
      setError(msg)
    } finally {
      clearInterval(stepTimer)
      setExecuting(false)
    }
  }

  return (
    <div className="flex flex-col h-full p-4 overflow-y-auto">
      <div className="flex items-center gap-2 mb-4">
        <GitBranch className="w-5 h-5 text-smara-400" />
        <h2 className="text-lg font-medium">Workflow</h2>
      </div>

      <div className="flex gap-2 mb-4">
        <input
          value={prompt}
          onChange={e => setPrompt(e.target.value)}
          onKeyDown={e => e.key === 'Enter' && generate()}
          placeholder="Deskripsikan proyek yang ingin dikerjakan... (contoh: buatkan web portfolio dengan React)"
          className="flex-1 bg-gray-800 border border-gray-700 rounded-lg px-3 py-2 text-sm focus:outline-none focus:border-smara-500"
        />
        <button
          onClick={generate}
          disabled={generating || !prompt.trim()}
          className="px-3 py-2 bg-fuchsia-600 hover:bg-fuchsia-500 disabled:opacity-50 rounded-lg transition-colors flex items-center gap-1 text-xs"
        >
          <Sparkles className="w-3.5 h-3.5" />
          {generating ? 'Generate...' : 'Generate'}
        </button>
        <button
          onClick={() => {
            localStorage.removeItem(STORAGE_KEY)
            localStorage.removeItem(ACTIVE_ID_KEY)
            localStorage.removeItem(PROMPT_KEY)
            localStorage.removeItem(PHASES_KEY)
            localStorage.removeItem(THOUGHTS_KEY)
            localStorage.removeItem(ERROR_KEY)
            localStorage.removeItem(LAST_REQ_KEY)
            localStorage.removeItem(LAST_RES_KEY)
            localStorage.removeItem(SHOW_DEBUG_KEY)
            setHistory([])
            setActiveId(null)
            setPrompt('')
            setPhases([])
            setThoughts([])
            setError(null)
            setLastRequest(null)
            setLastResponse(null)
            setShowDebug(false)
          }}
          className="px-3 py-2 bg-gray-700 hover:bg-gray-600 rounded-lg transition-colors text-xs"
        >
          Clear
        </button>
      </div>

      <div className="flex gap-4 flex-1 min-h-0 overflow-hidden">
        {/* Sidebar: Workflow History */}
        {history.length > 0 && (
          <div className="w-56 shrink-0 bg-gray-900/50 border border-gray-800 rounded-lg overflow-y-auto">
            <div className="p-2 text-[10px] text-gray-500 uppercase tracking-wider font-medium border-b border-gray-800">
              Workflow History ({history.length})
            </div>
            {history.map(item => (
              <button
                key={item.id}
                onClick={() => setActiveId(item.id)}
                className={`w-full text-left p-2 text-xs border-b border-gray-800/50 transition-colors ${
                  activeId === item.id ? 'bg-gray-800/80 text-gray-200' : 'text-gray-400 hover:bg-gray-800/40'
                }`}
              >
                <div className="font-medium truncate">{item.blueprint.project_name}</div>
                <div className="text-[10px] text-gray-500 flex items-center gap-1 mt-0.5">
                  <span className="uppercase px-1 bg-gray-800 rounded">{item.blueprint.domain}</span>
                  <span>{new Date(item.timestamp).toLocaleTimeString()}</span>
                  {item.result && <span className="text-green-400">✓</span>}
                </div>
              </button>
            ))}
          </div>
        )}

        {/* Main Content */}
        <div className="flex-1 overflow-y-auto">
      {(phases.length > 0 || generating || executing) && (
        <div className="mb-4 bg-gray-900/80 border border-gray-700/60 rounded-lg p-3 space-y-1.5">
          <div className="flex items-center gap-2 text-[10px] text-gray-500 uppercase tracking-wider font-medium mb-1">
            <BrainCircuit className="w-3 h-3" />
            Proses Berjalan
          </div>
          {phases.map((ph, idx) => (
            <div key={ph.phase + idx} className="flex items-center gap-2 text-xs">
              {ph.status === 'running' ? (
                <span className="text-smara-400 font-mono w-4 text-center">{spinnerFrames[spinnerIdx]}</span>
              ) : (
                <CheckCircle className="w-3.5 h-3.5 text-green-400 shrink-0" />
              )}
              <span className={ph.status === 'running' ? 'text-gray-200 font-medium' : 'text-gray-500'}>
                {ph.description || ph.phase}
              </span>
            </div>
          ))}
        </div>
      )}

      {error && (
        <div className="mb-4 p-3 bg-red-900/20 border border-red-800 rounded-lg text-sm text-red-200">
          <div className="flex items-center gap-2 mb-1">
            <AlertCircle className="w-4 h-4 shrink-0" />
            <span className="font-medium">Error</span>
          </div>
          <pre className="text-xs whitespace-pre-wrap font-mono text-red-300/80 max-h-40 overflow-y-auto">{error}</pre>
        </div>
      )}

      {(lastRequest || lastResponse) && (
        <div className="mb-4 border border-gray-700 rounded-lg overflow-hidden">
          <button
            onClick={() => setShowDebug(v => !v)}
            className="w-full flex items-center justify-between px-3 py-2 bg-gray-800/60 hover:bg-gray-800 text-xs text-gray-400 transition-colors"
          >
            <div className="flex items-center gap-2">
              <Terminal className="w-3.5 h-3.5" />
              <span>Debug Output</span>
            </div>
            {showDebug ? <ChevronUp className="w-3.5 h-3.5" /> : <ChevronDown className="w-3.5 h-3.5" />}
          </button>
          {showDebug && (
            <div className="p-3 bg-gray-900/80 space-y-2">
              {lastRequest && (
                <div>
                  <div className="text-[10px] text-gray-500 uppercase tracking-wider font-medium mb-1">Request</div>
                  <pre className="text-[10px] font-mono text-gray-400 whitespace-pre-wrap">{lastRequest}</pre>
                </div>
              )}
              {lastResponse && (
                <div>
                  <div className="text-[10px] text-gray-500 uppercase tracking-wider font-medium mb-1">Response</div>
                  <pre className="text-[10px] font-mono text-gray-400 whitespace-pre-wrap max-h-64 overflow-y-auto">{lastResponse}</pre>
                </div>
              )}
            </div>
          )}
        </div>
      )}

      {thoughts.length > 0 && (
        <div className="mb-4 bg-gray-900/50 border border-gray-800 rounded-lg p-3">
          <div className="text-[10px] text-gray-500 uppercase tracking-wider font-medium mb-2">Thinking</div>
          <div className="space-y-1">
            {thoughts.map((t, i) => (
              <div key={i} className="text-xs text-gray-400 font-mono whitespace-pre-wrap">{t}</div>
            ))}
          </div>
        </div>
      )}

      {blueprint && (
        <div className="space-y-4">
          {/* Blueprint card */}
          <div className="bg-gray-900/50 border border-gray-800 rounded-lg p-4">
            <div className="flex items-center justify-between mb-3">
              <h3 className="text-sm font-medium text-gray-200">{blueprint.project_name}</h3>
              <span className="text-[10px] px-2 py-0.5 bg-gray-800 rounded text-gray-400 uppercase">{blueprint.domain}</span>
            </div>
            <p className="text-xs text-gray-400 mb-3">{blueprint.description}</p>

            <div className="grid grid-cols-1 lg:grid-cols-2 gap-3 mb-3">
              <div className="bg-gray-800/40 border border-gray-700/50 rounded-lg p-3">
                <div className="flex items-center gap-2 mb-2 text-xs text-gray-400">
                  <FileText className="w-3 h-3" /> PRD
                </div>
                <pre className="text-xs text-gray-300 whitespace-pre-wrap max-h-48 overflow-y-auto font-mono">{blueprint.prd}</pre>
              </div>
              <div className="bg-gray-800/40 border border-gray-700/50 rounded-lg p-3">
                <div className="flex items-center gap-2 mb-2 text-xs text-gray-400">
                  <Layers className="w-3 h-3" /> Architecture
                </div>
                <pre className="text-xs text-gray-300 whitespace-pre-wrap max-h-48 overflow-y-auto font-mono">{blueprint.architecture}</pre>
              </div>
            </div>

            <div className="space-y-2">
              <div className="text-xs text-gray-400 font-medium">Agents ({blueprint.agents.length})</div>
              {blueprint.agents.map((agent, i) => (
                <div key={i} className="flex items-start gap-2 p-2 bg-gray-800/30 rounded border border-gray-700/40">
                  <CheckCircle className="w-3 h-3 text-green-400 mt-0.5 shrink-0" />
                  <div>
                    <div className="text-xs font-medium text-gray-300">{agent.role}</div>
                    <div className="text-[10px] text-gray-500">{agent.description}</div>
                    <div className="flex gap-1 mt-1">
                      {agent.skills?.map(s => (
                        <span key={s} className="text-[10px] px-1 py-0.5 bg-gray-700 rounded text-gray-400">{s}</span>
                      ))}
                    </div>
                  </div>
                </div>
              ))}
            </div>

            <div className="mt-4 flex justify-end">
              <button
                onClick={execute}
                disabled={executing}
                className="px-4 py-2 bg-blue-600 hover:bg-blue-500 disabled:opacity-50 rounded-lg transition-colors flex items-center gap-1 text-sm"
              >
                <Play className="w-4 h-4" />
                {executing ? 'Menjalankan...' : 'Jalankan Workflow'}
              </button>
            </div>
          </div>
        </div>
      )}

      {result && (
        <div className="mt-4 p-3 bg-gray-900/50 border border-gray-800 rounded-lg">
          <div className="text-xs font-medium text-gray-400 mb-2">Workflow Result</div>
          <pre className="text-xs font-mono text-gray-300 whitespace-pre-wrap max-h-96 overflow-y-auto">{result}</pre>
        </div>
      )}
        </div>
      </div>
    </div>
  )
}
