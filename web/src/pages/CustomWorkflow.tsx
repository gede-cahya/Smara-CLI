import { useState, useEffect, useRef } from 'react'
import { FolderTree, Plus, Trash2, Save, Play, CheckCircle, AlertCircle, FolderOpen, BrainCircuit } from 'lucide-react'
import { fetchJSON } from '../api'
import type { CustomWorkflowItem, CustomWorkflowAgent, CustomWorkflowTask, CustomWorkflowSummary } from '../api'
import FolderPicker from '../components/FolderPicker'

function defaultAgent(): CustomWorkflowAgent {
  return { role: '', description: '', skills: [], tasks: [{ id: 'main', description: '' }], depends_on: [], inputs_from: {} }
}
function defaultWorkflow(): CustomWorkflowItem {
  return { name: '', description: '', project_dir: '', agents: [defaultAgent()] }
}

const DRAFT_KEY = 'smara_custom_workflow_draft'
const RUN_LOG_KEY = 'smara_custom_workflow_runlog'
const RUN_ERROR_KEY = 'smara_custom_workflow_runerror'
const RUN_SUCCESS_KEY = 'smara_custom_workflow_runsuccess'
const RUN_PHASES_KEY = 'smara_custom_workflow_runphases'
const SELECTED_KEY = 'smara_custom_workflow_selected'

const spinnerFrames = ['\u280B','\u2819','\u2839','\u2838','\u283C','\u2834','\u2826','\u2827','\u2807','\u280F']

export default function CustomWorkflow() {
  const [workflows, setWorkflows] = useState<CustomWorkflowSummary[]>([])
  const [selected, setSelected] = useState<CustomWorkflowItem | null>(null)
  const [editing, setEditing] = useState<CustomWorkflowItem | null>(null)
  const [loading, setLoading] = useState(false)
  const [saving, setSaving] = useState(false)
  const [running, setRunning] = useState(false)
  const [runLog, setRunLog] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [success, setSuccess] = useState<string | null>(null)
  const [folderPickerOpen, setFolderPickerOpen] = useState(false)
  const [phases, setPhases] = useState<Array<{phase:string;description:string;status:'running'|'done'}>>([])
  const [spinnerIdx, setSpinnerIdx] = useState(0)
  const timer = useRef<ReturnType<typeof setInterval>|null>(null)

  const load = async () => {
    setLoading(true)
    try {
      const data = await fetchJSON<{ workflows: CustomWorkflowSummary[] }>('/api/custom-workflow/list')
      setWorkflows(data.workflows || [])
    } catch (e: any) {
      setError('Gagal load workflows: ' + (e.message || e))
    } finally { setLoading(false) }
  }

  // Restore draft + transient state from localStorage on mount
  useEffect(() => {
    const saved = localStorage.getItem(DRAFT_KEY)
    if (saved) {
      try {
        const parsed = JSON.parse(saved)
        if (parsed && typeof parsed === 'object') {
          setEditing(parsed)
        }
      } catch { /* ignore */ }
    }
    const savedSelected = localStorage.getItem(SELECTED_KEY)
    if (savedSelected) {
      // Restore selection by name and load full data
      selectWorkflow(savedSelected)
    }
    const savedRunLog = localStorage.getItem(RUN_LOG_KEY)
    if (savedRunLog) setRunLog(savedRunLog)
    const savedError = localStorage.getItem(RUN_ERROR_KEY)
    if (savedError) setError(savedError)
    const savedSuccess = localStorage.getItem(RUN_SUCCESS_KEY)
    if (savedSuccess) setSuccess(savedSuccess)
    const savedPhases = localStorage.getItem(RUN_PHASES_KEY)
    if (savedPhases) {
      try {
        const parsed = JSON.parse(savedPhases)
        // Mark all as done since we can't reconnect to in-flight request
        const restored = parsed.map((p: any) => ({ ...p, status: 'done' as const }))
        setPhases(restored)
      } catch { /* ignore */ }
    }
  }, [])

  // Persist draft to localStorage whenever it changes
  useEffect(() => {
    if (editing) {
      localStorage.setItem(DRAFT_KEY, JSON.stringify(editing))
    }
  }, [editing])

  // Persist transient run state
  useEffect(() => {
    if (runLog) localStorage.setItem(RUN_LOG_KEY, runLog)
    else localStorage.removeItem(RUN_LOG_KEY)
  }, [runLog])

  useEffect(() => {
    if (error) localStorage.setItem(RUN_ERROR_KEY, error)
    else localStorage.removeItem(RUN_ERROR_KEY)
  }, [error])

  useEffect(() => {
    if (success) localStorage.setItem(RUN_SUCCESS_KEY, success)
    else localStorage.removeItem(RUN_SUCCESS_KEY)
  }, [success])

  useEffect(() => {
    if (phases.length > 0) localStorage.setItem(RUN_PHASES_KEY, JSON.stringify(phases))
    else localStorage.removeItem(RUN_PHASES_KEY)
  }, [phases])

  useEffect(() => {
    if (selected?.name) localStorage.setItem(SELECTED_KEY, selected.name)
    else localStorage.removeItem(SELECTED_KEY)
  }, [selected])

  useEffect(() => { load() }, [])

  // Spinner animation when running
  useEffect(() => {
    if (running) {
      timer.current = setInterval(() => setSpinnerIdx(i => (i+1)%spinnerFrames.length), 80)
    } else if (timer.current) {
      clearInterval(timer.current); timer.current = null
    }
    return () => { if (timer.current) clearInterval(timer.current) }
  }, [running])

  const selectWorkflow = async (name: string) => {
    try {
      const data = await fetchJSON<CustomWorkflowItem>(`/api/custom-workflow/get?name=${encodeURIComponent(name)}`)
      setSelected(data); setEditing(JSON.parse(JSON.stringify(data))); setError(null); setSuccess(null); setRunLog(null); setPhases([])
    } catch (e: any) { setError('Gagal load workflow: ' + (e.message || e)) }
  }

  const addPhase = (phase: string, description: string) => {
    setPhases(prev => {
      const next: Array<{ phase: string; description: string; status: 'running' | 'done' }> = prev.map(p => ({ ...p, status: 'done' as const }))
      const idx = next.findIndex(p => p.phase === phase)
      if (idx >= 0) next[idx] = { phase, description, status: 'running' }
      else next.push({ phase, description, status: 'running' })
      return next
    })
  }

  const clearPhases = () => setPhases([])

  const save = async () => {
    if (!editing) return
    if (!editing.name.trim()) { setError('Nama workflow wajib diisi'); return }
    setSaving(true); setError(null); setSuccess(null)
    try {
      await fetchJSON('/api/custom-workflow/save', { method: 'POST', body: JSON.stringify(editing) })
      setSuccess('Workflow disimpan!'); load()
      if (selected?.name === editing.name) setSelected(JSON.parse(JSON.stringify(editing)))
    } catch (e: any) { setError('Gagal simpan: ' + (e.message || e)) }
    finally { setSaving(false) }
  }

  const del = async (name: string) => {
    if (!confirm(`Hapus workflow '${name}'?`)) return
    setError(null)
    try {
      await fetchJSON('/api/custom-workflow/delete', { method: 'POST', body: JSON.stringify({ name }) })
      if (selected?.name === name) { setSelected(null); setEditing(null) }
      load()
    } catch (e: any) { setError('Gagal hapus: ' + (e.message || e)) }
  }

  const run = async () => {
    if (!editing) return
    setRunning(true); setError(null); setSuccess(null); setRunLog(null); clearPhases()
    const steps = [
      { phase:'prepare', desc:'Mempersiapkan workspace & agents...' },
      { phase:'execute', desc:'Menjalankan custom workflow agents...' },
      { phase:'compile', desc:'Mengompilasi hasil eksekusi...' },
      { phase:'finalize', desc:'Menyelesaikan workflow...' }
    ]
    let stepIdx = 0
    addPhase(steps[0].phase, steps[0].desc)
    const stepTimer = setInterval(() => {
      stepIdx++
      if (stepIdx < steps.length) addPhase(steps[stepIdx].phase, steps[stepIdx].desc)
    }, 2000)

    try {
      const result = await fetchJSON('/api/custom-workflow/run', {
        method: 'POST',
        body: JSON.stringify({ name: editing.name, project_dir: editing.project_dir || undefined })
      })
      setSuccess('Workflow berhasil dijalankan!')
      setRunLog(JSON.stringify(result, null, 2))
      addPhase('done', 'Workflow selesai!')
    } catch (e: any) {
      setError('Gagal run: ' + (e.message || e))
    } finally {
      clearInterval(stepTimer)
      setRunning(false)
    }
  }

  const updateAgent = (idx: number, patch: Partial<CustomWorkflowAgent>) => {
    if (!editing) return
    const next = { ...editing, agents: [...editing.agents] }
    next.agents[idx] = { ...next.agents[idx], ...patch }
    setEditing(next)
  }

  const addAgent = () => {
    if (!editing) return
    setEditing({ ...editing, agents: [...editing.agents, defaultAgent()] })
  }

  const removeAgent = (idx: number) => {
    if (!editing) return
    const next = [...editing.agents]; next.splice(idx, 1)
    setEditing({ ...editing, agents: next })
  }

  const updateTask = (agentIdx: number, taskIdx: number, patch: Partial<CustomWorkflowTask>) => {
    if (!editing) return
    const next = { ...editing, agents: [...editing.agents] }
    const tasks = [...next.agents[agentIdx].tasks]
    tasks[taskIdx] = { ...tasks[taskIdx], ...patch }
    next.agents[agentIdx] = { ...next.agents[agentIdx], tasks }
    setEditing(next)
  }

  const addTask = (agentIdx: number) => {
    if (!editing) return
    const next = { ...editing, agents: [...editing.agents] }
    const tasks = [...next.agents[agentIdx].tasks]
    tasks.push({ id: `t${tasks.length+1}`, description: '' })
    next.agents[agentIdx] = { ...next.agents[agentIdx], tasks }
    setEditing(next)
  }

  const removeTask = (agentIdx: number, taskIdx: number) => {
    if (!editing) return
    const next = { ...editing, agents: [...editing.agents] }
    const tasks = [...next.agents[agentIdx].tasks]
    tasks.splice(taskIdx, 1)
    next.agents[agentIdx] = { ...next.agents[agentIdx], tasks }
    setEditing(next)
  }

  const strArrInput = (value: string[]) => value.join(', ')
  const parseStrArr = (value: string) => value.split(',').map(s => s.trim()).filter(Boolean)

  const fileRef = { current: null as HTMLInputElement | null }
  const [importName, setImportName] = useState('')

  const handleImportFile = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0]
    if (!file) return
    const text = await file.text()
    const name = importName.trim() || file.name.replace(/\.json$/i, '')
    if (!name) { setError('Nama workflow wajib diisi untuk import'); return }
    setError(null); setSuccess(null)
    try {
      await fetchJSON('/api/custom-workflow/import', { method: 'POST', body: JSON.stringify({ name, json: text }) })
      setSuccess(`Workflow '${name}' berhasil di-import!`)
      setImportName('')
      load()
    } catch (err: any) { setError('Gagal import: ' + (err.message || err)) }
    if (fileRef.current) fileRef.current.value = ''
  }

  return (
    <div className="flex flex-col h-full p-4 overflow-y-auto">
      <div className="flex items-center gap-2 mb-4">
        <FolderTree className="w-5 h-5 text-smara-400" />
        <h2 className="text-lg font-medium">Custom Workflow</h2>
      </div>
      <div className="flex gap-2 mb-4">
        <button onClick={() => { setSelected(null); setEditing(defaultWorkflow()); setError(null); setSuccess(null) }}
          className="px-3 py-2 bg-smara-700 hover:bg-smara-600 rounded-lg transition-colors flex items-center gap-1 text-sm">
          <Plus className="w-4 h-4" /> Baru
        </button>
        <button onClick={load} disabled={loading}
          className="px-3 py-2 bg-gray-700 hover:bg-gray-600 rounded-lg transition-colors text-sm">
          {loading ? 'Loading...' : 'Refresh'}
        </button>
        <div className="flex items-center gap-2 ml-auto">
          <input
            value={importName}
            onChange={e => setImportName(e.target.value)}
            placeholder="Nama untuk import..."
            className="bg-gray-800 border border-gray-700 rounded-lg px-2 py-1.5 text-xs focus:outline-none focus:border-smara-500 w-40"
          />
          <input
            ref={el => fileRef.current = el}
            type="file"
            accept=".json"
            className="hidden"
            onChange={handleImportFile}
          />
          <button
            onClick={() => fileRef.current?.click()}
            className="px-3 py-2 bg-blue-700 hover:bg-blue-600 rounded-lg transition-colors text-xs flex items-center gap-1"
          >
            Import JSON
          </button>
        </div>
      </div>

      {error && (
        <div className="mb-4 p-3 bg-red-900/20 border border-red-800 rounded-lg text-sm text-red-200 flex items-center gap-2">
          <AlertCircle className="w-4 h-4 shrink-0" /><span>{error}</span>
        </div>
      )}
      {success && (
        <div className="mb-4 p-3 bg-green-900/20 border border-green-800 rounded-lg text-sm text-green-200 flex items-center gap-2">
          <CheckCircle className="w-4 h-4 shrink-0" /><span>{success}</span>
        </div>
      )}

      {(phases.length > 0 || running) && (
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

      <div className="flex gap-4 flex-1 min-h-0 overflow-hidden">
        <div className="w-56 shrink-0 bg-gray-900/50 border border-gray-800 rounded-lg overflow-y-auto">
          <div className="p-2 text-[10px] text-gray-500 uppercase tracking-wider font-medium border-b border-gray-800">
            Workflows ({workflows.length})
          </div>
          {workflows.map(w => (
            <div key={w.name} className="flex items-center justify-between p-2 border-b border-gray-800/50">
              <button onClick={() => selectWorkflow(w.name)}
                className={`text-left text-xs transition-colors flex-1 ${selected?.name === w.name ? 'text-gray-200 font-medium' : 'text-gray-400 hover:text-gray-200'}`}>
                <div className="truncate">{w.name}</div>
                <div className="text-[10px] text-gray-500">{w.agents} agent</div>
              </button>
              <button onClick={() => del(w.name)} className="text-gray-500 hover:text-red-400 p-1"><Trash2 className="w-3 h-3" /></button>
            </div>
          ))}
        </div>

        <div className="flex-1 overflow-y-auto">
          {editing ? (
            <div className="space-y-4">
              <div className="bg-gray-900/50 border border-gray-800 rounded-lg p-4 space-y-3">
                <div>
                  <label className="text-xs text-gray-500 mb-1 block">Nama Workflow</label>
                  <input value={editing.name} onChange={e => setEditing({ ...editing, name: e.target.value })}
                    className="w-full bg-gray-800 border border-gray-700 rounded-lg px-3 py-2 text-sm focus:outline-none focus:border-smara-500" placeholder="my-custom-flow" />
                </div>
                <div>
                  <label className="text-xs text-gray-500 mb-1 block">Deskripsi</label>
                  <input value={editing.description} onChange={e => setEditing({ ...editing, description: e.target.value })}
                    className="w-full bg-gray-800 border border-gray-700 rounded-lg px-3 py-2 text-sm focus:outline-none focus:border-smara-500" placeholder="Deskripsi singkat..." />
                </div>
                <div>
                  <label className="text-xs text-gray-500 mb-1 block">Project Dir (opsional)</label>
                  <div className="flex gap-2">
                    <input value={editing.project_dir || ''} onChange={e => setEditing({ ...editing, project_dir: e.target.value })}
                      className="flex-1 bg-gray-800 border border-gray-700 rounded-lg px-3 py-2 text-sm focus:outline-none focus:border-smara-500" placeholder="/path/to/project atau kosongkan" />
                    <button onClick={() => setFolderPickerOpen(true)} className="px-3 py-2 bg-gray-700 hover:bg-gray-600 rounded-lg transition-colors flex items-center gap-1 text-xs" title="Browse folder">
                      <FolderOpen className="w-3.5 h-3.5" />
                    </button>
                  </div>
                </div>
              </div>

              <div className="space-y-3">
                <div className="flex items-center justify-between">
                  <h3 className="text-sm font-medium text-gray-300">Agents ({editing.agents.length})</h3>
                  <button onClick={addAgent} className="text-xs px-2 py-1 bg-smara-700 hover:bg-smara-600 rounded flex items-center gap-1">
                    <Plus className="w-3 h-3" /> Tambah Agent
                  </button>
                </div>

                {editing.agents.map((agent, aIdx) => (
                  <div key={aIdx} className="bg-gray-900/50 border border-gray-800 rounded-lg p-4 space-y-3">
                    <div className="flex items-center justify-between">
                      <span className="text-xs font-medium text-gray-400">Agent #{aIdx + 1}</span>
                      {editing.agents.length > 1 && (
                        <button onClick={() => removeAgent(aIdx)} className="text-gray-500 hover:text-red-400"><Trash2 className="w-3 h-3" /></button>
                      )}
                    </div>
                    <div className="grid grid-cols-2 gap-3">
                      <input value={agent.role} onChange={e => updateAgent(aIdx, { role: e.target.value })}
                        placeholder="Role (contoh: frontend)"
                        className="bg-gray-800 border border-gray-700 rounded-lg px-3 py-2 text-sm focus:outline-none focus:border-smara-500" />
                      <input value={strArrInput(agent.depends_on || [])} onChange={e => updateAgent(aIdx, { depends_on: parseStrArr(e.target.value) })}
                        placeholder="Depends on (pisah koma)"
                        className="bg-gray-800 border border-gray-700 rounded-lg px-3 py-2 text-sm focus:outline-none focus:border-smara-500" />
                    </div>
                    <input value={agent.description} onChange={e => updateAgent(aIdx, { description: e.target.value })}
                      placeholder="Deskripsi agent"
                      className="w-full bg-gray-800 border border-gray-700 rounded-lg px-3 py-2 text-sm focus:outline-none focus:border-smara-500" />
                    <input value={strArrInput(agent.skills || [])} onChange={e => updateAgent(aIdx, { skills: parseStrArr(e.target.value) })}
                      placeholder="Skills (pisah koma)"
                      className="w-full bg-gray-800 border border-gray-700 rounded-lg px-3 py-2 text-sm focus:outline-none focus:border-smara-500" />

                    <div className="space-y-2">
                      <div className="flex items-center justify-between">
                        <span className="text-xs text-gray-500">Tasks</span>
                        <button onClick={() => addTask(aIdx)} className="text-[10px] px-2 py-0.5 bg-gray-800 hover:bg-gray-700 rounded flex items-center gap-1">
                          <Plus className="w-3 h-3" /> Task
                        </button>
                      </div>
                      {agent.tasks.map((task, tIdx) => (
                        <div key={tIdx} className="flex gap-2">
                          <input value={task.id} onChange={e => updateTask(aIdx, tIdx, { id: e.target.value })}
                            placeholder="ID" className="w-24 bg-gray-800 border border-gray-700 rounded-lg px-2 py-1.5 text-xs focus:outline-none focus:border-smara-500" />
                          <input value={task.description} onChange={e => updateTask(aIdx, tIdx, { description: e.target.value })}
                            placeholder="Deskripsi task" className="flex-1 bg-gray-800 border border-gray-700 rounded-lg px-2 py-1.5 text-xs focus:outline-none focus:border-smara-500" />
                          {agent.tasks.length > 1 && (
                            <button onClick={() => removeTask(aIdx, tIdx)} className="text-gray-500 hover:text-red-400 px-1"><Trash2 className="w-3 h-3" /></button>
                          )}
                        </div>
                      ))}
                    </div>
                  </div>
                ))}
              </div>

              <div className="flex items-center gap-2">
                <button onClick={save} disabled={saving}
                  className="px-4 py-2 bg-smara-700 hover:bg-smara-600 disabled:opacity-50 rounded-lg transition-colors flex items-center gap-1 text-sm">
                  <Save className="w-4 h-4" /> {saving ? 'Menyimpan...' : 'Simpan'}
                </button>
                <button onClick={run} disabled={running || saving}
                  className="px-4 py-2 bg-green-700 hover:bg-green-600 disabled:opacity-50 rounded-lg transition-colors flex items-center gap-1 text-sm">
                  <Play className="w-4 h-4" /> {running ? 'Menjalankan...' : 'Jalankan'}
                </button>
                <button onClick={() => {
                  localStorage.removeItem(DRAFT_KEY)
                  localStorage.removeItem(RUN_LOG_KEY)
                  localStorage.removeItem(RUN_ERROR_KEY)
                  localStorage.removeItem(RUN_SUCCESS_KEY)
                  localStorage.removeItem(RUN_PHASES_KEY)
                  localStorage.removeItem(SELECTED_KEY)
                  setEditing(defaultWorkflow()); setSelected(null); setError(null); setSuccess(null); setRunLog(null); setPhases([])
                }}
                  className="px-3 py-2 bg-gray-700 hover:bg-gray-600 rounded-lg transition-colors text-xs ml-auto">
                  Clear Draft
                </button>
              </div>

              {runLog && (
                <div className="mt-2 bg-gray-950 border border-gray-800 rounded-lg p-3">
                  <div className="text-xs text-gray-500 mb-1 font-medium">Hasil Eksekusi</div>
                  <pre className="text-[11px] text-gray-300 font-mono whitespace-pre-wrap overflow-y-auto max-h-48">{runLog}</pre>
                </div>
              )}
            </div>
          ) : (
            <div className="text-gray-500 text-sm">Pilih workflow dari sidebar atau buat baru.</div>
          )}
        </div>
      </div>
      <FolderPicker
        open={folderPickerOpen}
        onClose={() => setFolderPickerOpen(false)}
        onSelect={(path: string) => {
          if (editing) setEditing({ ...editing, project_dir: path })
        }}
        title="Pilih Project Directory"
      />
    </div>
  )
}
