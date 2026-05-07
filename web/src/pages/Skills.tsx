import { useState, useEffect } from 'react'
import { Wrench, Play, Trash2, Download, FileJson, FileText, Plus, X, Box, Settings } from 'lucide-react'
import { fetchJSON, installBundledSkill, type SkillItem, type SkillParam, type BundledSkillItem } from '../api'

function defaultParamValue(p: SkillParam): string {
  if (p.default !== undefined) return String(p.default)
  return ''
}

export default function Skills() {
  const [skills, setSkills] = useState<SkillItem[]>([])
  const [bundled, setBundled] = useState<BundledSkillItem[]>([])
  const [loading, setLoading] = useState(false)
  const [importOpen, setImportOpen] = useState(false)
  const [importName, setImportName] = useState('')
  const [importFormat, setImportFormat] = useState<'json' | 'md'>('json')
  const [importData, setImportData] = useState('')
  const [running, setRunning] = useState<string | null>(null)
  const [runResult, setRunResult] = useState<string | null>(null)
  const [runModal, setRunModal] = useState<SkillItem | null>(null)
  const [runArgs, setRunArgs] = useState<Record<string, string>>({})

  const load = async () => {
    setLoading(true)
    try {
      const [saved, bundle] = await Promise.all([
        fetchJSON<{ skills: SkillItem[] }>('/api/skills').catch(() => ({ skills: [] })),
        fetchJSON<{ skills: BundledSkillItem[] }>('/api/skills/bundled').catch(() => ({ skills: [] })),
      ])
      setSkills(saved.skills || [])
      setBundled(bundle.skills || [])
    } catch (e) {
      console.error(e)
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => { load() }, [])

  const deleteSkill = async (name: string) => {
    if (!confirm(`Hapus skill '${name}'?`)) return
    try {
      await fetch(`/api/skills?name=${encodeURIComponent(name)}`, { method: 'DELETE' })
      load()
    } catch (e) {
      alert('Gagal hapus: ' + e)
    }
  }

  const openRunModal = (sk: SkillItem) => {
    if (sk.params && sk.params.length > 0) {
      const defaults: Record<string, string> = {}
      for (const p of sk.params) {
        defaults[p.name] = defaultParamValue(p)
      }
      setRunArgs(defaults)
      setRunModal(sk)
    } else {
      executeRun(sk.name, {})
    }
  }

  const executeRun = async (name: string, args: Record<string, string>) => {
    setRunning(name)
    setRunResult(null)
    try {
      const payload: Record<string, unknown> = { name }
      const nonEmptyArgs: Record<string, string> = {}
      for (const [k, v] of Object.entries(args)) {
        if (v.trim() !== '') nonEmptyArgs[k] = v
      }
      if (Object.keys(nonEmptyArgs).length > 0) {
        payload.args = nonEmptyArgs
      }
      const res = await fetchJSON('/api/skills/run', {
        method: 'POST',
        body: JSON.stringify(payload),
        headers: { 'Content-Type': 'application/json' },
      })
      setRunResult(JSON.stringify(res, null, 2))
    } catch (e: any) {
      setRunResult('Error: ' + (e.message || e))
    } finally {
      setRunning(null)
      setRunModal(null)
    }
  }

  const doImport = async () => {
    if (!importName.trim() || !importData.trim()) return
    try {
      await fetchJSON('/api/skills/import', {
        method: 'POST',
        body: JSON.stringify({ name: importName.trim(), format: importFormat, data: importData }),
        headers: { 'Content-Type': 'application/json' },
      })
      setImportOpen(false)
      setImportName('')
      setImportData('')
      load()
    } catch (e: any) {
      alert('Gagal import: ' + (e.message || e))
    }
  }

  const installBundled = async (name: string) => {
    try {
      await installBundledSkill(name)
      load()
    } catch (e: any) {
      alert('Gagal install: ' + (e.message || e))
    }
  }

  const savedNames = new Set(skills.map(s => s.name))

  return (
    <div className="flex flex-col h-full p-4 overflow-y-auto">
      <div className="flex items-center justify-between mb-4">
        <div className="flex items-center gap-2">
          <Wrench className="w-5 h-5 text-smara-400" />
          <h2 className="text-lg font-medium">Skills</h2>
        </div>
        <button
          onClick={() => setImportOpen(!importOpen)}
          className="flex items-center gap-1 px-3 py-1.5 bg-smara-700 hover:bg-smara-600 rounded-lg text-xs transition-colors"
        >
          {importOpen ? <X className="w-3 h-3" /> : <Plus className="w-3 h-3" />}
          {importOpen ? 'Batal' : 'Import'}
        </button>
      </div>

      {importOpen && (
        <div className="mb-4 p-3 bg-gray-900/50 border border-gray-800 rounded-lg space-y-2">
          <div className="flex gap-2">
            <input
              value={importName}
              onChange={e => setImportName(e.target.value)}
              placeholder="Nama skill..."
              className="flex-1 bg-gray-800 border border-gray-700 rounded px-3 py-1.5 text-sm focus:outline-none focus:border-smara-500"
            />
            <div className="flex bg-gray-800 rounded border border-gray-700 overflow-hidden">
              <button
                onClick={() => setImportFormat('json')}
                className={`px-3 py-1.5 text-xs ${importFormat === 'json' ? 'bg-smara-700 text-white' : 'text-gray-400'}`}
              >
                <FileJson className="w-3 h-3 inline mr-1" />JSON
              </button>
              <button
                onClick={() => setImportFormat('md')}
                className={`px-3 py-1.5 text-xs ${importFormat === 'md' ? 'bg-smara-700 text-white' : 'text-gray-400'}`}
              >
                <FileText className="w-3 h-3 inline mr-1" />MD
              </button>
            </div>
          </div>
          <textarea
            value={importData}
            onChange={e => setImportData(e.target.value)}
            placeholder={`Paste ${importFormat.toUpperCase()} skill di sini...`}
            className="w-full h-32 bg-gray-800 border border-gray-700 rounded px-3 py-2 text-xs font-mono resize-none focus:outline-none focus:border-smara-500"
          />
          <div className="flex justify-end">
            <button
              onClick={doImport}
              disabled={!importName.trim() || !importData.trim()}
              className="px-3 py-1.5 bg-smara-600 hover:bg-smara-500 disabled:opacity-50 rounded text-xs transition-colors"
            >
              <Download className="w-3 h-3 inline mr-1" /> Import
            </button>
          </div>
        </div>
      )}

      {loading && <div className="text-gray-500 text-sm mb-4">Loading...</div>}

      {/* Saved Skills */}
      <div className="mb-2 text-[10px] text-gray-500 uppercase tracking-wider font-medium">
        Tersimpan ({skills.length})
      </div>
      <div className="space-y-2 mb-6">
        {skills.length === 0 && !loading && (
          <div className="text-gray-600 text-sm p-4 bg-gray-900/50 rounded-lg">
            Belum ada skill tersimpan. Import dari bundled atau buat dari workflow.
          </div>
        )}
        {skills.map(sk => (
          <div
            key={sk.name}
            className="flex items-center justify-between p-3 bg-gray-900/50 border border-gray-800 rounded-lg hover:border-gray-700 transition-colors"
          >
            <div className="flex-1 min-w-0">
              <div className="text-sm font-medium truncate">{sk.name}</div>
              <div className="text-xs text-gray-500 truncate">{sk.description || 'No description'}</div>
              <div className="flex gap-1 mt-1 flex-wrap">
                {sk.tags?.map(t => (
                  <span key={t} className="text-[10px] px-1.5 py-0.5 bg-gray-800 rounded text-gray-400">{t}</span>
                ))}
                {sk.params && sk.params.length > 0 && (
                  <span className="text-[10px] px-1.5 py-0.5 bg-smara-900/30 rounded text-smara-400 flex items-center gap-1">
                    <Settings className="w-3 h-3" />{sk.params.length} param
                  </span>
                )}
              </div>
            </div>
            <div className="flex items-center gap-2 ml-3">
              <button
                onClick={() => openRunModal(sk)}
                disabled={running === sk.name}
                className="p-1.5 bg-green-900/30 hover:bg-green-900/50 text-green-400 rounded transition-colors disabled:opacity-50"
                title="Jalankan skill"
              >
                <Play className="w-3.5 h-3.5" />
              </button>
              <button
                onClick={() => deleteSkill(sk.name)}
                className="p-1.5 bg-red-900/30 hover:bg-red-900/50 text-red-400 rounded transition-colors"
                title="Hapus skill"
              >
                <Trash2 className="w-3.5 h-3.5" />
              </button>
            </div>
          </div>
        ))}
      </div>

      {/* Bundled Skills */}
      {bundled.length > 0 && (
        <>
          <div className="mb-2 text-[10px] text-gray-500 uppercase tracking-wider font-medium">
            Bundled Skills ({bundled.length})
          </div>
          <div className="space-y-2">
            {bundled.map(b => {
              const installed = savedNames.has(b.name)
              return (
                <div
                  key={b.name}
                  className="flex items-center justify-between p-3 bg-gray-900/30 border border-gray-800/60 rounded-lg"
                >
                  <div className="flex-1 min-w-0">
                    <div className="text-sm font-medium truncate flex items-center gap-2">
                      <Box className="w-3.5 h-3.5 text-gray-500" />
                      {b.name}
                      {installed && <span className="text-[10px] px-1.5 py-0.5 bg-green-900/20 text-green-400 rounded">installed</span>}
                    </div>
                    <div className="text-xs text-gray-500 truncate">{b.description || 'No description'}</div>
                    <div className="flex gap-1 mt-1">
                      {b.tags?.map(t => (
                        <span key={t} className="text-[10px] px-1.5 py-0.5 bg-gray-800/60 rounded text-gray-400">{t}</span>
                      ))}
                    </div>
                  </div>
                  <button
                    onClick={() => installBundled(b.name)}
                    disabled={installed}
                    className="px-3 py-1.5 bg-smara-700 hover:bg-smara-600 disabled:opacity-40 disabled:cursor-not-allowed rounded text-xs transition-colors"
                  >
                    {installed ? 'Installed' : 'Install'}
                  </button>
                </div>
              )
            })}
          </div>
        </>
      )}

      {/* Run Modal */}
      {runModal && (
        <div className="fixed inset-0 bg-black/60 flex items-center justify-center z-50 p-4">
          <div className="bg-gray-900 border border-gray-700 rounded-xl p-5 w-full max-w-md space-y-4 shadow-2xl">
            <div className="flex items-center justify-between">
              <h3 className="text-sm font-medium">Run: {runModal.name}</h3>
              <button onClick={() => setRunModal(null)} className="text-gray-500 hover:text-gray-300">
                <X className="w-4 h-4" />
              </button>
            </div>
            <div className="space-y-3">
              {runModal.params?.map(p => (
                <div key={p.name}>
                  <label className="text-xs text-gray-500 mb-1 block">
                    {p.name} {p.required && <span className="text-red-400">*</span>}
                  </label>
                  <input
                    value={runArgs[p.name] ?? ''}
                    onChange={e => setRunArgs(prev => ({ ...prev, [p.name]: e.target.value }))}
                    placeholder={p.description}
                    className="w-full bg-gray-800 border border-gray-700 rounded-lg px-3 py-2 text-sm focus:outline-none focus:border-smara-500"
                  />
                </div>
              ))}
            </div>
            <div className="flex justify-end gap-2">
              <button
                onClick={() => setRunModal(null)}
                className="px-3 py-2 bg-gray-700 hover:bg-gray-600 rounded-lg text-xs transition-colors"
              >
                Batal
              </button>
              <button
                onClick={() => executeRun(runModal.name, runArgs)}
                disabled={running === runModal.name || (runModal.params?.some(p => p.required && !runArgs[p.name]?.trim()) ?? false)}
                className="px-3 py-2 bg-green-700 hover:bg-green-600 disabled:opacity-50 rounded-lg text-xs transition-colors flex items-center gap-1"
              >
                <Play className="w-3.5 h-3.5" /> {running === runModal.name ? 'Menjalankan...' : 'Jalankan'}
              </button>
            </div>
          </div>
        </div>
      )}

      {/* Run Result */}
      {runResult && (
        <div className="mt-4 p-3 bg-gray-900/50 border border-gray-800 rounded-lg">
          <div className="flex items-center justify-between mb-2">
            <span className="text-xs font-medium text-gray-400">Hasil Eksekusi</span>
            <button onClick={() => setRunResult(null)} className="text-xs text-gray-500 hover:text-gray-300"><X className="w-3 h-3" /></button>
          </div>
          <pre className="text-xs font-mono text-gray-300 whitespace-pre-wrap max-h-64 overflow-y-auto">{runResult}</pre>
        </div>
      )}
    </div>
  )
}
