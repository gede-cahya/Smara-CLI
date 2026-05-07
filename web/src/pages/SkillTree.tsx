import { useState, useEffect } from 'react'
import { fetchJSON, type SkillItem } from '../api'
import { ChevronRight, ChevronDown, Folder, FileCode, TreePine, Sparkles } from 'lucide-react'
import SkillConstellation from './SkillConstellation'

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
type ViewMode = 'tree' | 'constellation'

const VIEW_KEY = 'smara_skilltree_view'

export default function SkillTree() {
  const [skills, setSkills] = useState<SkillItem[]>([])
  const [loading, setLoading] = useState(false)
  const [view, setView] = useState<ViewMode>(() => {
    try {
      const saved = localStorage.getItem(VIEW_KEY)
      if (saved === 'tree' || saved === 'constellation') return saved
    } catch {}
    return 'constellation'
  })

  useEffect(() => {
    setLoading(true)
    fetchJSON<{ skills: SkillItem[] }>('/api/skills')
      .then(d => setSkills(d.skills || []))
      .finally(() => setLoading(false))
  }, [])

  const switchView = (v: ViewMode) => {
    setView(v)
    try { localStorage.setItem(VIEW_KEY, v) } catch {}
  }

  const tree = buildCategoryTree(skills)

  return (
    <div className="flex flex-col h-full overflow-hidden">
      {/* Header with view toggle */}
      <div className="flex items-center justify-between px-4 py-3 border-b border-gray-800 shrink-0">
        <h3 className="text-sm font-medium text-gray-300">Skill Tree</h3>
        <div className="flex items-center gap-1 bg-gray-800/60 rounded-lg p-0.5">
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

      {!loading && view === 'constellation' && (
        <div className="flex-1 overflow-hidden">
          <SkillConstellation skills={skills} />
        </div>
      )}
    </div>
  )
}
