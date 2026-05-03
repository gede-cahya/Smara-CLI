import { useState, useEffect } from 'react'
import { Settings, Save } from 'lucide-react'
import { fetchJSON } from '../api'

export default function Config() {
  const [config, setConfig] = useState<Record<string, unknown>>({})
  const [editKey, setEditKey] = useState('')
  const [editValue, setEditValue] = useState('')
  const [loading, setLoading] = useState(false)

  const load = async () => {
    setLoading(true)
    try {
      const data = await fetchJSON<Record<string, unknown>>('/api/config')
      setConfig(data || {})
    } catch (e) {
      console.error(e)
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => { load() }, [])

  const update = async () => {
    if (!editKey.trim()) return
    try {
      await fetchJSON('/api/config', {
        method: 'POST',
        body: JSON.stringify({ key: editKey.trim(), value: editValue }),
      })
      setEditKey('')
      setEditValue('')
      load()
    } catch (e) {
      alert('Gagal update: ' + e)
    }
  }

  const entries = Object.entries(config).sort((a, b) => a[0].localeCompare(b[0]))

  return (
    <div className="flex flex-col h-full p-4 overflow-y-auto">
      <div className="flex items-center gap-2 mb-4">
        <Settings className="w-5 h-5 text-smara-400" />
        <h2 className="text-lg font-medium">Configuration</h2>
      </div>

      <div className="flex gap-2 mb-4">
        <input
          value={editKey}
          onChange={e => setEditKey(e.target.value)}
          placeholder="Key"
          className="w-48 bg-gray-800 border border-gray-700 rounded-lg px-3 py-2 text-sm focus:outline-none focus:border-smara-500"
        />
        <input
          value={editValue}
          onChange={e => setEditValue(e.target.value)}
          placeholder="Value"
          className="flex-1 bg-gray-800 border border-gray-700 rounded-lg px-3 py-2 text-sm focus:outline-none focus:border-smara-500"
        />
        <button
          onClick={update}
          className="px-3 py-2 bg-smara-700 hover:bg-smara-600 rounded-lg transition-colors flex items-center gap-1"
        >
          <Save className="w-4 h-4" /> Update
        </button>
      </div>

      {loading && <div className="text-gray-500 text-sm">Loading...</div>}

      <div className="space-y-1">
        {entries.map(([key, value]) => (
          <div key={key} className="flex items-center justify-between p-2 bg-gray-900/50 border border-gray-800 rounded-lg">
            <span className="text-sm font-mono text-smara-300">{key}</span>
            <span className="text-sm text-gray-400 truncate max-w-[50%]">
              {typeof value === 'string' ? value : JSON.stringify(value)}
            </span>
          </div>
        ))}
      </div>
    </div>
  )
}
