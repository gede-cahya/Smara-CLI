import { useState, useEffect, useRef, lazy, Suspense } from 'react'
import { fetchJSON, type SkillItem } from '../api'
import { ChevronRight, ChevronDown, Folder, FileCode, TreePine, Sparkles, Network, Download, Upload, AlertCircle, Box } from 'lucide-react'
import SkillConstellation from './SkillConstellation'
import SkillHierarchy from './SkillHierarchy'

// Lazy-load the 3D view because it pulls in three.js (~700 KB gzipped) — we
// only want to pay that cost when the user actually opens the 3D tab.
const SkillTree3D = lazy(() => import('./SkillTree3D'))

// --- Tree View helpers ---
function buildCategoryTree(skills: SkillItem[]): Record<string, any> {
  const tree: Record<string, any> = {}
  for (const sk of skills) {
    const path = sk.category_path?.length ? sk.category_path : ['Uncategorized']
    let node = tree
    for (let i = 0; i < path.length; i++) {
      const part = path[i]
      if (!node[part]) node[part] = { _children: {}, _skills: [] }
      if (i === path.length - 1) {
        node[part]._skills.push(sk)
      }
      node = node[part]._children
    }
  }
  return tree
}

function TreeSection({ name, data, depth = 0 }: { name: string; data: any; depth?: number }) {
  const [open, setOpen] = useState(depth < 2)
  const hasChildren = Object.keys(data._children || {}).length > 0
  const skills: SkillItem[] = data._skills || []

  return (
    <div className="select-none">
      <button
        onClick={() => setOpen(!open)}
        className={`flex items-center gap-1 w-full text-left hover:bg-gray-800/50 rounded px-2 py-1 ${depth === 0 ? 'font-medium text-sm' : 'text-xs'}`}
        style={{ paddingLeft: `${depth * 12 + 8}px` }}
      >
        {hasChildren || skills.length > 0 ? (
          open ? <ChevronDown className="w-3 h-3 text-gray-500" /> : <ChevronRight className="w-3 h-3 text-gray-500" />
        ) : (
          <span className="w-3" />
        )}
        <Folder className="w-3.5 h-3.5 text-smara-400 mr-1" />
        <span className="text-gray-200">{name}</span>
        {skills.length > 0 && (
          <span className="ml-2 text-[10px] text-gray-500 bg-gray-800 px-1.5 rounded">{skills.length}</span>
        )}
      </button>
      {open && (
        <div>
          {Object.entries(data._children || {}).map(([childName, childData]) => (
            <TreeSection key={childName} name={childName} data={childData} depth={depth + 1} />
          ))}
          {skills.map(sk => (
            <div
              key={sk.name}
              className="flex items-center gap-2 px-2 py-1 hover:bg-gray-800/50 rounded text-xs"
              style={{ paddingLeft: `${(depth + 1) * 12 + 8}px` }}
            >
              <FileCode className="w-3.5 h-3.5 text-green-400" />
              <span className="text-gray-300">{sk.name}</span>
              {sk.dependencies && sk.dependencies.length > 0 && (
                <span className="text-[10px] text-gray-500">({sk.dependencies.length} deps)</span>
              )}
              <span className="ml-auto text-[10px] text-gray-600">v{sk.version}</span>
            </div>
          ))}
        </div>
      )}
    </div>
  )
}

// --- View mode type ---
type ViewMode = 'tree' | 'hierarchy' | 'constellation' | '3d'

// Shape of /api/skills/import-tree response.
interface ImportOutcome {
  created: string[]
  overwritten: string[]
  skipped: string[]
  renamed?: Record<string, string>
  patterns_loaded: number
  warnings?: string[]
}

const VIEW_KEY = 'smara_skilltree_view'

export default function SkillTree() {
  const [skills, setSkills] = useState<SkillItem[]>([])
  const [loading, setLoading] = useState(false)
  const [view, setView] = useState<ViewMode>(() => {
    try {
      const saved = localStorage.getItem(VIEW_KEY)
      if (saved === 'tree' || saved === 'hierarchy' || saved === 'constellation' || saved === '3d') return saved
    } catch {}
    return 'hierarchy'
  })

  // Import dialog state
  const fileInputRef = useRef<HTMLInputElement>(null)
  const [importOpen, setImportOpen] = useState(false)
  const [importFile, setImportFile] = useState<File | null>(null)
  const [importMode, setImportMode] = useState<'overwrite' | 'skip' | 'rename'>('overwrite')
  const [importDryRun, setImportDryRun] = useState(true)
  const [importBusy, setImportBusy] = useState(false)
  const [importResult, setImportResult] = useState<ImportOutcome | null>(null)
  const [importError, setImportError] = useState<string | null>(null)

  const reload = async () => {
    setLoading(true)
    try {
      const d = await fetchJSON<{ skills: SkillItem[] }>('/api/skills')
      setSkills(d.skills || [])
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => { reload() }, [])

  const switchView = (v: ViewMode) => {
    setView(v)
    try { localStorage.setItem(VIEW_KEY, v) } catch {}
  }

  // Export: call the server endpoint and trigger a browser download.
  const handleExport = () => {
    // Direct link with attachment headers — simplest and avoids blob copy.
    const ts = new Date().toISOString().slice(0, 10)
    const source = encodeURIComponent(`smara-web-${new Date().toISOString().slice(11, 16)}`)
    const url = `/api/skills/export?source=${source}&t=${ts}`
    // Use an anchor element to respect Content-Disposition.
    const a = document.createElement('a')
    a.href = url
    a.style.display = 'none'
    document.body.appendChild(a)
    a.click()
    document.body.removeChild(a)
  }

  // Open file picker for import.
  const handlePickImport = () => {
    setImportError(null)
    setImportResult(null)
    fileInputRef.current?.click()
  }

  const handleFileSelected = (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0]
    if (!file) return
    setImportFile(file)
    setImportOpen(true)
    setImportDryRun(true)
    // Reset the input so picking the same file twice still fires change.
    e.target.value = ''
  }

  const runImport = async (dryRun: boolean) => {
    if (!importFile) return
    setImportBusy(true)
    setImportError(null)
    try {
      const text = await importFile.text()
      const envelope = JSON.parse(text)
      const res = await fetch('/api/skills/import-tree', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ mode: importMode, dry_run: dryRun, envelope }),
      })
      if (!res.ok) {
        const raw = await res.text()
        throw new Error(raw || res.statusText)
      }
      const out = await res.json() as ImportOutcome
      setImportResult(out)
      if (!dryRun) {
        // Real import — reload the tree so new skills show up.
        await reload()
      }
    } catch (err) {
      setImportError(err instanceof Error ? err.message : String(err))
    } finally {
      setImportBusy(false)
    }
  }

  const closeImportDialog = () => {
    if (importBusy) return
    setImportOpen(false)
    setImportFile(null)
    setImportResult(null)
    setImportError(null)
  }

  const tree = buildCategoryTree(skills)

  return (
    <div className="relative flex flex-col h-full overflow-hidden">
      {/* Header with view toggle + export/import actions */}
      <div className="flex items-center justify-between px-4 py-3 border-b border-gray-800 shrink-0 gap-2">
        <h3 className="text-sm font-medium text-gray-300 flex items-center gap-2">
          Skill Tree
          <span className="text-[10px] text-gray-500 bg-gray-800/60 px-1.5 py-0.5 rounded">
            {skills.length}
          </span>
        </h3>

        <div className="flex items-center gap-2">
          <button
            onClick={handleExport}
            className="flex items-center gap-1.5 px-3 py-1.5 rounded-md text-xs font-medium bg-gray-800/60 hover:bg-gray-700 text-gray-300 transition-all"
            title="Unduh seluruh skill tree sebagai JSON"
          >
            <Download className="w-3 h-3" />
            Export
          </button>
          <button
            onClick={handlePickImport}
            className="flex items-center gap-1.5 px-3 py-1.5 rounded-md text-xs font-medium bg-gray-800/60 hover:bg-gray-700 text-gray-300 transition-all"
            title="Muat skill tree dari file JSON"
          >
            <Upload className="w-3 h-3" />
            Import
          </button>
          <input
            ref={fileInputRef}
            type="file"
            accept="application/json,.json"
            className="hidden"
            onChange={handleFileSelected}
          />

          <div className="flex items-center gap-1 bg-gray-800/60 rounded-lg p-0.5 ml-2">
            <button
              onClick={() => switchView('tree')}
              className={`flex items-center gap-1.5 px-3 py-1.5 rounded-md text-xs font-medium transition-all ${
                view === 'tree'
                  ? 'bg-gray-700 text-gray-100 shadow-sm'
                  : 'text-gray-500 hover:text-gray-300'
              }`}
            >
              <TreePine className="w-3 h-3" />
              Tree
            </button>
            <button
              onClick={() => switchView('hierarchy')}
              className={`flex items-center gap-1.5 px-3 py-1.5 rounded-md text-xs font-medium transition-all ${
                view === 'hierarchy'
                  ? 'bg-indigo-700/40 text-indigo-300 shadow-sm'
                  : 'text-gray-500 hover:text-gray-300'
              }`}
            >
              <Network className="w-3 h-3" />
              Hierarchy
            </button>
            <button
              onClick={() => switchView('constellation')}
              className={`flex items-center gap-1.5 px-3 py-1.5 rounded-md text-xs font-medium transition-all ${
                view === 'constellation'
                  ? 'bg-smara-700/40 text-smara-300 shadow-sm'
                  : 'text-gray-500 hover:text-gray-300'
              }`}
            >
              <Sparkles className="w-3 h-3" />
              Constellation
            </button>
            <button
              onClick={() => switchView('3d')}
              className={`flex items-center gap-1.5 px-3 py-1.5 rounded-md text-xs font-medium transition-all ${
                view === '3d'
                  ? 'bg-fuchsia-700/40 text-fuchsia-300 shadow-sm'
                  : 'text-gray-500 hover:text-gray-300'
              }`}
              title="Visual 3D fractal — butuh GPU sederhana"
            >
              <Box className="w-3 h-3" />
              3D
            </button>
          </div>
        </div>
      </div>

      {/* Loading */}
      {loading && (
        <div className="flex-1 flex items-center justify-center">
          <div className="text-gray-500 text-xs">Loading skills...</div>
        </div>
      )}

      {/* Content */}
      {!loading && view === 'tree' && (
        <div className="flex-1 overflow-y-auto p-4">
          {skills.length === 0 && (
            <div className="text-gray-600 text-xs">No skills found.</div>
          )}
          <div className="space-y-1">
            {Object.entries(tree).map(([name, data]) => (
              <TreeSection key={name} name={name} data={data} depth={0} />
            ))}
          </div>
        </div>
      )}

      {!loading && view === 'hierarchy' && (
        <div className="flex-1 overflow-hidden">
          <SkillHierarchy skills={skills} />
        </div>
      )}

      {!loading && view === 'constellation' && (
        <div className="flex-1 overflow-hidden">
          <SkillConstellation skills={skills} />
        </div>
      )}

      {!loading && view === '3d' && (
        <div className="flex-1 overflow-hidden">
          <Suspense fallback={
            <div className="w-full h-full flex items-center justify-center text-gray-500 text-xs">
              Memuat WebGL renderer…
            </div>
          }>
            <SkillTree3D skills={skills} />
          </Suspense>
        </div>
      )}

      {/* Import dialog */}
      {importOpen && (
        <div className="absolute inset-0 bg-black/60 backdrop-blur-sm flex items-center justify-center z-50 p-4" onClick={closeImportDialog}>
          <div
            className="bg-gray-900 border border-gray-700 rounded-xl shadow-2xl w-full max-w-lg overflow-hidden"
            onClick={(e) => e.stopPropagation()}
          >
            <div className="flex items-center justify-between px-5 py-4 border-b border-gray-800">
              <div className="flex items-center gap-2">
                <Upload className="w-4 h-4 text-smara-400" />
                <h3 className="text-sm font-semibold text-gray-100">Import Skill Tree</h3>
              </div>
              <button
                onClick={closeImportDialog}
                disabled={importBusy}
                className="text-gray-500 hover:text-gray-300 disabled:opacity-40"
              >
                ✕
              </button>
            </div>

            <div className="p-5 space-y-4">
              {importFile && (
                <div className="bg-gray-800/60 border border-gray-700 rounded-lg p-3">
                  <div className="text-[10px] text-gray-500 uppercase tracking-wider mb-1">File</div>
                  <div className="text-xs text-gray-200 font-mono break-all">{importFile.name}</div>
                  <div className="text-[10px] text-gray-500 mt-1">
                    {(importFile.size / 1024).toFixed(1)} KB · {importFile.type || 'application/json'}
                  </div>
                </div>
              )}

              <div>
                <label className="text-[10px] text-gray-500 uppercase tracking-wider block mb-1.5">
                  Conflict mode
                </label>
                <div className="grid grid-cols-3 gap-1 bg-gray-800/60 p-0.5 rounded-lg">
                  {(['overwrite', 'skip', 'rename'] as const).map(m => (
                    <button
                      key={m}
                      onClick={() => setImportMode(m)}
                      className={`px-2 py-1.5 rounded text-xs font-medium transition-all ${
                        importMode === m ? 'bg-smara-700/40 text-smara-200' : 'text-gray-400 hover:text-gray-200'
                      }`}
                    >
                      {m.charAt(0).toUpperCase() + m.slice(1)}
                    </button>
                  ))}
                </div>
                <p className="text-[10px] text-gray-500 mt-1.5 leading-relaxed">
                  {importMode === 'overwrite' && 'Skill yang sudah ada akan ditimpa. Versi lama masuk ke lineage history.'}
                  {importMode === 'skip' && 'Skill yang sudah ada dibiarkan. Hanya skill baru yang ditambahkan.'}
                  {importMode === 'rename' && 'Skill yang konflik diimport dengan suffix -2, -3, dst. Keduanya coexist.'}
                </p>
              </div>

              {importError && (
                <div className="bg-red-900/30 border border-red-800 rounded-lg p-3 flex items-start gap-2">
                  <AlertCircle className="w-4 h-4 text-red-400 shrink-0 mt-0.5" />
                  <div className="text-xs text-red-300 font-mono break-all">{importError}</div>
                </div>
              )}

              {importResult && (
                <div className="bg-gray-800/60 border border-gray-700 rounded-lg p-3">
                  <div className="text-[10px] text-gray-500 uppercase tracking-wider mb-2">
                    {importDryRun ? 'Preview (dry-run)' : 'Hasil import'}
                  </div>
                  <div className="grid grid-cols-2 gap-x-4 gap-y-1 text-xs">
                    <div className="text-gray-400">✓ Created</div>
                    <div className="text-emerald-300 font-mono">{importResult.created.length}</div>
                    <div className="text-gray-400">↺ Overwritten</div>
                    <div className="text-amber-300 font-mono">{importResult.overwritten.length}</div>
                    <div className="text-gray-400">⇝ Renamed</div>
                    <div className="text-indigo-300 font-mono">{Object.keys(importResult.renamed || {}).length}</div>
                    <div className="text-gray-400">∅ Skipped</div>
                    <div className="text-gray-400 font-mono">{importResult.skipped.length}</div>
                    <div className="text-gray-400">📌 Patterns</div>
                    <div className="text-gray-300 font-mono">{importResult.patterns_loaded}</div>
                  </div>
                  {importResult.warnings && importResult.warnings.length > 0 && (
                    <div className="mt-2 pt-2 border-t border-gray-700 text-[10px] text-amber-400 space-y-0.5">
                      {importResult.warnings.slice(0, 5).map((w, i) => (
                        <div key={i}>⚠ {w}</div>
                      ))}
                      {importResult.warnings.length > 5 && (
                        <div className="text-gray-500">… dan {importResult.warnings.length - 5} warning lagi</div>
                      )}
                    </div>
                  )}
                </div>
              )}
            </div>

            <div className="flex items-center justify-end gap-2 px-5 py-3 border-t border-gray-800 bg-gray-900/50">
              <button
                onClick={closeImportDialog}
                disabled={importBusy}
                className="px-3 py-1.5 text-xs text-gray-400 hover:text-gray-200 disabled:opacity-40"
              >
                Tutup
              </button>
              <button
                onClick={() => { setImportDryRun(true); runImport(true) }}
                disabled={importBusy || !importFile}
                className="px-3 py-1.5 text-xs bg-gray-800 border border-gray-700 hover:bg-gray-700 text-gray-200 rounded disabled:opacity-40"
              >
                {importBusy && importDryRun ? 'Preview…' : 'Preview (dry-run)'}
              </button>
              <button
                onClick={() => { setImportDryRun(false); runImport(false) }}
                disabled={importBusy || !importFile}
                className="px-4 py-1.5 text-xs bg-smara-600 hover:bg-smara-500 text-white rounded font-medium disabled:opacity-40"
              >
                {importBusy && !importDryRun ? 'Importing…' : 'Apply import'}
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}
