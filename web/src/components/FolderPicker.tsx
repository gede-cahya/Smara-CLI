import { useState, useEffect } from 'react'
import { Folder, ChevronLeft, Home, X } from 'lucide-react'
import { listDir, getCwd, type FSEntry } from '../api'

interface FolderPickerProps {
  open: boolean
  onClose: () => void
  onSelect: (path: string) => void
  title?: string
}

export default function FolderPicker({ open, onClose, onSelect, title = 'Pilih Folder' }: FolderPickerProps) {
  const [path, setPath] = useState('')
  const [entries, setEntries] = useState<FSEntry[]>([])
  const [loading, setLoading] = useState(false)
  const [err, setErr] = useState<string | null>(null)

  useEffect(() => {
    if (!open) return
    loadPath('')
  }, [open])

  const loadPath = async (p: string) => {
    setLoading(true); setErr(null)
    try {
      let target = p
      if (!target) {
        const cwd = await getCwd()
        target = cwd.path
      }
      const data = await listDir(target)
      setPath(data.path)
      setEntries(data.entries.filter(e => e.is_dir))
    } catch (e: any) {
      setErr(e.message || String(e))
    } finally { setLoading(false) }
  }

  const goUp = () => {
    const parts = path.replace(/\/+$/, '').split(/[\\\/]/)
    if (parts.length <= 1) return
    parts.pop()
    loadPath(parts.join('/'))
  }

  if (!open) return null

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-sm" onClick={onClose}>
      <div className="bg-gray-900 border border-gray-700 rounded-xl w-[480px] max-w-[90vw] shadow-2xl flex flex-col max-h-[70vh]" onClick={e => e.stopPropagation()}>
        <div className="flex items-center justify-between p-3 border-b border-gray-800">
          <h3 className="text-sm font-medium text-gray-200">{title}</h3>
          <button onClick={onClose} className="text-gray-500 hover:text-gray-300"><X className="w-4 h-4" /></button>
        </div>
        <div className="flex items-center gap-2 p-2 border-b border-gray-800 bg-gray-900/80">
          <button onClick={() => loadPath('')} className="p-1 text-gray-400 hover:text-gray-200" title="Home"><Home className="w-3.5 h-3.5" /></button>
          <button onClick={goUp} className="p-1 text-gray-400 hover:text-gray-200" title="Up"><ChevronLeft className="w-3.5 h-3.5" /></button>
          <span className="text-xs text-gray-300 truncate flex-1 font-mono">{path}</span>
        </div>
        {err && <div className="p-2 text-xs text-red-400 bg-red-900/20">{err}</div>}
        <div className="overflow-y-auto flex-1 p-1">
          {loading ? (
            <div className="p-4 text-center text-xs text-gray-500">Memuat...</div>
          ) : entries.length === 0 ? (
            <div className="p-4 text-center text-xs text-gray-500">Tidak ada folder</div>
          ) : (
            entries.map(e => (
              <button key={e.name} onClick={() => loadPath(path + '/' + e.name)}
                className="w-full flex items-center gap-2 px-3 py-2 text-left text-sm text-gray-300 hover:bg-gray-800 rounded-md transition-colors">
                <Folder className="w-4 h-4 text-yellow-500 shrink-0" />
                <span className="truncate">{e.name}</span>
              </button>
            ))
          )}
        </div>
        <div className="flex items-center justify-end gap-2 p-3 border-t border-gray-800">
          <button onClick={onClose} className="px-3 py-1.5 text-xs bg-gray-700 hover:bg-gray-600 rounded-md transition-colors">Batal</button>
          <button onClick={() => { onSelect(path); onClose() }} className="px-3 py-1.5 text-xs bg-smara-700 hover:bg-smara-600 rounded-md transition-colors">Pilih Folder Ini</button>
        </div>
      </div>
    </div>
  )
}
