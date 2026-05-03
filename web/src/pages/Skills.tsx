import { useState, useEffect } from 'react'
import { Wrench, Play, Trash2, Download, FileJson, FileText, Plus, X } from 'lucide-react'
import { fetchJSON, type SkillItem } from '../api'

export default function Skills() {
  const [skills, setSkills] = useState<SkillItem[]>([])
  const [loading, setLoading] = useState(false)
  const [importOpen, setImportOpen] = useState(false)
  const [importName, setImportName] = useState('')
  const [importFormat, setImportFormat] = useState<'json' | 'md'>('json')
  const [importData, setImportData] = useState('')
  const [running, setRunning] = useState<string | null>(null)
  const [runResult, setRunResult] = useState<string | null>(null)

  const load = async () => {
    setLoading(true)
    try {
      const data = await fetchJSON<{ skills: SkillItem[] }>('/api/skills')
      setSkills(data.skills || [])
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

  const runSkill = async (name: string) => {
    setRunning(name)
    setRunResult(null)
    try {
      const res = await fetchJSON('/api/skills/run', {
        method: 'POST',
        body: JSON.stringify({ name }),
        headers: { 'Content-Type': 'application/json' },
      })
      setRunResult(JSON.stringify(res, null, 2))
    } catch (e: any) {
      setRunResult('Error: ' + (e.message || e))
    } finally {
      setRunning(null)
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

      {loading && <div className="text-gray-500 text-sm">Loading...</div>}

      <div className="space-y-2">
        {skills.length === 0 && !loading && (
          <div className="text-gray-600 text-sm p-4 bg-gray-900/50 rounded-lg">
            Belum ada skill tersimpan. Import atau buat dari workflow.
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
              <div className="flex gap-1 mt-1">
                {sk.tags?.map(t => (
                  <span key={t} className="text-[10px] px-1.5 py-0.5 bg-gray-800 rounded text-gray-400">{t}</span>
                ))}
              </div>
            </div>
            <div className="flex items-center gap-2 ml-3">
              <button
                onClick={() => runSkill(sk.name)}
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
