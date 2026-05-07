import { useState, useEffect } from 'react'
import { Layers, Plus, Check, Folder, FolderOpen } from 'lucide-react'
import { fetchJSON } from '../api'
import type { WorkspaceItem } from '../api'
import FolderPicker from '../components/FolderPicker'

export default function Workspace() {
  const [workspaces, setWorkspaces] = useState<WorkspaceItem[]>([])
  const [active, setActive] = useState('')
  const [newName, setNewName] = useState('')
  const [newPath, setNewPath] = useState('')
  const [loading, setLoading] = useState(false)
  const [folderPickerOpen, setFolderPickerOpen] = useState(false)

  const load = async () => {
    setLoading(true)
    try {
      const data = await fetchJSON<{ workspaces: WorkspaceItem[], active: string }>('/api/workspaces')
      setWorkspaces(data.workspaces || [])
      setActive(data.active || '')
    } catch (e) {
      console.error(e)
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => { load() }, [])

  const switchWs = async (name: string) => {
    try {
      await fetchJSON('/api/workspaces/switch', {
        method: 'POST',
        body: JSON.stringify({ name }),
      })
      setActive(name)
    } catch (e) {
      alert('Gagal switch workspace: ' + e)
    }
  }

  const create = async () => {
    if (!newName.trim()) return
    try {
      await fetchJSON('/api/workspaces/create', {
        method: 'POST',
        body: JSON.stringify({ name: newName.trim(), path: newPath.trim() || '.' }),
      })
      setNewName('')
      setNewPath('')
      load()
    } catch (e) {
      alert('Gagal buat workspace: ' + e)
    }
  }

  return (
    <div className="flex flex-col h-full p-4 overflow-y-auto">
      <div className="flex items-center gap-2 mb-4">
        <Layers className="w-5 h-5 text-smara-400" />
        <h2 className="text-lg font-medium">Workspaces</h2>
      </div>

      <div className="flex flex-col gap-2 mb-4">
        <div className="flex gap-2">
          <input
            value={newName}
            onChange={e => setNewName(e.target.value)}
            onKeyDown={e => e.key === 'Enter' && create()}
            placeholder="Nama workspace baru..."
            className="flex-1 bg-gray-800 border border-gray-700 rounded-lg px-3 py-2 text-sm focus:outline-none focus:border-smara-500"
          />
          <button
            onClick={create}
            className="px-3 py-2 bg-smara-700 hover:bg-smara-600 rounded-lg transition-colors flex items-center gap-1"
          >
            <Plus className="w-4 h-4" /> Buat
          </button>
        </div>
        <div className="flex gap-2">
          <input
            value={newPath}
            onChange={e => setNewPath(e.target.value)}
            onKeyDown={e => e.key === 'Enter' && create()}
            placeholder="Path folder (opsional, default: .)"
            className="flex-1 bg-gray-800 border border-gray-700 rounded-lg px-3 py-2 text-sm focus:outline-none focus:border-smara-500"
          />
          <button
            onClick={() => setFolderPickerOpen(true)}
            className="px-3 py-2 bg-gray-700 hover:bg-gray-600 rounded-lg transition-colors flex items-center gap-1 text-xs"
            title="Browse folder"
          >
            <FolderOpen className="w-3.5 h-3.5" />
          </button>
        </div>
      </div>

      {loading && <div className="text-gray-500 text-sm">Loading...</div>}

      <div className="space-y-1">
        {workspaces.map(ws => (
          <div
            key={ws.id}
            className={`flex items-center justify-between p-3 rounded-lg border transition-colors ${
              ws.name === active
                ? 'bg-smara-900/30 border-smara-700/50'
                : 'bg-gray-900/50 border-gray-800 hover:border-gray-700'
            }`}
          >
            <div className="flex items-center gap-3">
              <Folder className="w-4 h-4 text-gray-500" />
              <div>
                <div className="text-sm font-medium">{ws.name}</div>
                <div className="text-xs text-gray-500">{ws.path}</div>
              </div>
            </div>
            {ws.name === active ? (
              <span className="text-xs text-smara-400 flex items-center gap-1">
                <Check className="w-3 h-3" /> Aktif
              </span>
            ) : (
              <button
                onClick={() => switchWs(ws.name)}
                className="text-xs px-2 py-1 bg-gray-800 hover:bg-gray-700 rounded transition-colors"
              >
                Pilih
              </button>
            )}
          </div>
        ))}
      </div>

      <FolderPicker
        open={folderPickerOpen}
        onClose={() => setFolderPickerOpen(false)}
        onSelect={(path: string) => setNewPath(path)}
        title="Pilih Folder Workspace"
      />
    </div>
  )
}
