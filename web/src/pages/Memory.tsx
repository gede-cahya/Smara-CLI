import { useState, useEffect } from 'react'
import { Database, Search, Tag, Clock, Folder, ChevronDown, ChevronRight, Layers, Settings, Plus, Trash2, X } from 'lucide-react'
import { fetchJSON } from '../api'
import type { MemoryItem, WorkspaceItem } from '../api'

const CAT_CONFIG_KEY = 'smara_memory_categories'

interface CategoryConfig {
  name: string
  keywords: string[]
  color: string
}

const defaultCategories: CategoryConfig[] = [
  { name: 'Developer', keywords: ['code','coding','program','bug','git','api','function','variable','deploy','debug','error','compile','dev','developer','terminal','bash','script','backend','frontend','test','unit test','testing'], color: 'text-blue-400' },
  { name: 'Desain', keywords: ['design','desain','css','figma','ui','ux','layout','color','font','style','mockup','prototype','tailwind','gradient','icon','responsive','theme','svg','png','jpg'], color: 'text-purple-400' },
  { name: 'Hukum', keywords: ['lawyer','hukum','legal','contract','kontrak','sue','gugat','pengadilan','notaris','advokat','undang-undang','peraturan','hak','kewajiban'], color: 'text-yellow-400' },
  { name: 'Bisnis', keywords: ['business','bisnis','market','marketing','sales','jual','beli','profit','revenue','customer','client','strategi','management'], color: 'text-green-400' },
  { name: 'Umum', keywords: [], color: 'text-gray-400' },
]

function loadCategories(): CategoryConfig[] {
  try {
    const raw = localStorage.getItem(CAT_CONFIG_KEY)
    if (raw) return JSON.parse(raw)
  } catch {}
  return defaultCategories
}

function saveCategories(cats: CategoryConfig[]) {
  localStorage.setItem(CAT_CONFIG_KEY, JSON.stringify(cats))
}

export default function Memory() {
  const [memories, setMemories] = useState<MemoryItem[]>([])
  const [workspaces, setWorkspaces] = useState<WorkspaceItem[]>([])
  const [activeWorkspace, setActiveWorkspace] = useState<string>('')
  const [query, setQuery] = useState('')
  const [loading, setLoading] = useState(false)
  const [searching, setSearching] = useState(false)
  const [expandedCats, setExpandedCats] = useState<Set<number>>(new Set())
  const [categoryConfig, setCategoryConfig] = useState<CategoryConfig[]>(loadCategories)
  const [showCatManager, setShowCatManager] = useState(false)
  const [newCatName, setNewCatName] = useState('')
  const [newCatKeywords, setNewCatKeywords] = useState('')

  const loadWorkspaces = async () => {
    try {
      const data = await fetchJSON<{ workspaces: WorkspaceItem[]; active: string }>('/api/workspaces')
      setWorkspaces(data.workspaces || [])
      setActiveWorkspace(data.active || (data.workspaces?.[0]?.name ?? ''))
    } catch (e) {
      console.error(e)
    }
  }

  const loadMemories = async (wsName?: string) => {
    setLoading(true)
    const ws = wsName ?? activeWorkspace
    try {
      const qs = ws ? `?limit=200&workspace=${encodeURIComponent(ws)}` : '?limit=200'
      const data = await fetchJSON<{ memories: MemoryItem[] }>(`/api/memories${qs}`)
      setMemories(data.memories || [])
    } catch (e) {
      console.error(e)
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    loadWorkspaces()
  }, [])

  useEffect(() => {
    if (!activeWorkspace) return
    loadMemories(activeWorkspace)
    setExpandedCats(new Set())
  }, [activeWorkspace])

  const search = async () => {
    if (!query.trim()) { loadMemories(); return }
    setSearching(true)
    try {
      const data = await fetchJSON<{ results: MemoryItem[] }>('/api/memories/search', {
        method: 'POST',
        body: JSON.stringify({ query, limit: 50 }),
      })
      setMemories(data.results || [])
    } catch (e) {
      console.error(e)
    } finally {
      setSearching(false)
    }
  }

  const [activeTag, setActiveTag] = useState<string | null>(null)

  const toggleCat = (name: string) => {
    setExpandedCats(prev => {
      const n = new Set(prev)
      // Use string hash for Set since keys are now strings
      const hash = name.split('').reduce((a,b)=>a+b.charCodeAt(0),0)
      if (n.has(hash)) n.delete(hash)
      else n.add(hash)
      return n
    })
  }

  function getGroup(m: MemoryItem): string {
    const tags = (m.tags || []).map(t => t.toLowerCase())
    const content = (m.content || '').toLowerCase()

    for (const cat of categoryConfig) {
      if (cat.keywords.length === 0) continue // "Umum" handled last
      for (const kw of cat.keywords) {
        if (content.includes(kw.toLowerCase()) || tags.includes(kw.toLowerCase())) return cat.name
      }
    }
    return 'Umum'
  }

  const visibleMemories = activeTag
    ? memories.filter(m => (m.tags || []).map(t => t.toLowerCase()).includes(activeTag.toLowerCase()))
    : memories

  const grouped = new Map<string, MemoryItem[]>()
  visibleMemories.forEach(m => {
    const key = getGroup(m)
    const arr = grouped.get(key) || []
    arr.push(m)
    grouped.set(key, arr)
  })

  const sortedKeys = categoryConfig.map(c => c.name).filter(g => grouped.has(g))

  const hash = (s: string) => s.split('').reduce((a,b)=>a+b.charCodeAt(0),0)

  const addCategory = () => {
    if (!newCatName.trim()) return
    const keywords = newCatKeywords.split(',').map(s => s.trim()).filter(Boolean)
    const colors = ['text-blue-400','text-purple-400','text-yellow-400','text-green-400','text-red-400','text-pink-400','text-cyan-400','text-orange-400']
    const color = colors[categoryConfig.length % colors.length]
    const updated = [...categoryConfig.slice(0, -1), { name: newCatName.trim(), keywords, color }, categoryConfig[categoryConfig.length -1]]
    setCategoryConfig(updated)
    saveCategories(updated)
    setNewCatName('')
    setNewCatKeywords('')
  }

  const removeCategory = (name: string) => {
    if (name === 'Umum') return
    const updated = categoryConfig.filter(c => c.name !== name)
    setCategoryConfig(updated)
    saveCategories(updated)
  }

  return (
    <div className="flex h-full">
      {/* Workspace sidebar */}
      <div className="w-48 border-r border-gray-800 bg-gray-900/40 flex flex-col shrink-0">
        <div className="p-3 border-b border-gray-800 flex items-center gap-2 text-xs text-gray-400 uppercase tracking-wider font-medium">
          <Layers className="w-3 h-3" />
          Workspace
        </div>
        <div className="flex-1 overflow-y-auto p-2 space-y-1">
          {workspaces.map(ws => (
            <button
              key={ws.id}
              onClick={() => { setActiveWorkspace(ws.name); setActiveTag(null); }}
              className={`w-full text-left px-3 py-2 rounded-lg text-sm transition-colors flex items-center gap-2 ${
                ws.name === activeWorkspace
                  ? 'bg-smara-900/40 text-smara-300 border border-smara-700/30'
                  : 'hover:bg-gray-800 text-gray-300'
              }`}
            >
              <Folder className="w-3 h-3 shrink-0" />
              <span className="truncate">{ws.name}</span>
            </button>
          ))}
        </div>
      </div>

      {/* Main content */}
      <div className="flex-1 flex flex-col overflow-y-auto p-4">
        <div className="flex items-center gap-2 mb-4">
          <Database className="w-5 h-5 text-smara-400" />
          <h2 className="text-lg font-medium">Memory Store</h2>
          <span className="text-xs text-gray-500 ml-2">({visibleMemories.length})</span>
          {activeWorkspace && (
            <span className="text-xs bg-gray-800 px-2 py-0.5 rounded text-gray-400">
              {activeWorkspace}
            </span>
          )}
          <button
            onClick={() => setShowCatManager(!showCatManager)}
            className="ml-auto text-xs flex items-center gap-1 text-gray-500 hover:text-smara-300 transition-colors"
            title="Kelola Kategori"
          >
            <Settings className="w-3 h-3" /> Kategori
          </button>
        </div>

        {showCatManager && (
          <div className="mb-4 bg-gray-900/60 border border-gray-800 rounded-lg p-3 space-y-2">
            <div className="flex items-center justify-between">
              <span className="text-sm font-medium text-gray-300">Kelompok Kategori</span>
              <button onClick={() => setShowCatManager(false)} className="text-gray-500 hover:text-gray-300">
                <X className="w-3 h-3" />
              </button>
            </div>
            <div className="space-y-1">
              {categoryConfig.map(cat => (
                <div key={cat.name} className="flex items-center justify-between text-xs">
                  <span className={`${cat.color}`}>{cat.name}</span>
                  <span className="text-gray-600">{cat.keywords.join(', ') || 'fallback'}</span>
                  {cat.name !== 'Umum' && (
                    <button onClick={() => removeCategory(cat.name)} className="text-gray-600 hover:text-red-400">
                      <Trash2 className="w-3 h-3" />
                    </button>
                  )}
                </div>
              ))}
            </div>
            <div className="flex gap-2 pt-2 border-t border-gray-800">
              <input
                value={newCatName}
                onChange={e => setNewCatName(e.target.value)}
                placeholder="Nama kategori"
                className="flex-1 bg-gray-800 border border-gray-700 rounded px-2 py-1 text-xs focus:outline-none focus:border-smara-500"
              />
              <input
                value={newCatKeywords}
                onChange={e => setNewCatKeywords(e.target.value)}
                placeholder="Keyword, dipisah koma"
                className="flex-1 bg-gray-800 border border-gray-700 rounded px-2 py-1 text-xs focus:outline-none focus:border-smara-500"
              />
              <button
                onClick={addCategory}
                disabled={!newCatName.trim()}
                className="px-2 py-1 bg-smara-700 hover:bg-smara-600 disabled:opacity-50 rounded text-xs flex items-center gap-1"
              >
                <Plus className="w-3 h-3" /> Tambah
              </button>
            </div>
          </div>
        )}

        <div className="flex gap-2 mb-4">
          <input
            value={query}
            onChange={e => setQuery(e.target.value)}
            onKeyDown={e => e.key === 'Enter' && search()}
            placeholder="Cari memori..."
            className="flex-1 bg-gray-800 border border-gray-700 rounded-lg px-3 py-2 text-sm focus:outline-none focus:border-smara-500"
          />
          <button
            onClick={search}
            disabled={searching}
            className="px-3 py-2 bg-smara-700 hover:bg-smara-600 rounded-lg transition-colors flex items-center gap-1"
          >
            <Search className="w-4 h-4" />
            {searching ? '...' : 'Cari'}
          </button>
        </div>

        {activeTag && (
          <div className="flex items-center gap-2 mb-3 text-xs">
            <span className="bg-smara-700/40 text-smara-300 px-2 py-1 rounded border border-smara-700/30">
              Filter: #{activeTag}
            </span>
            <button onClick={() => setActiveTag(null)} className="text-gray-500 hover:text-gray-300">× Hapus filter</button>
          </div>
        )}

        {loading && <div className="text-gray-500 text-sm">Loading...</div>}

        {/* Grouped memories */}
        <div className="space-y-3">
          {sortedKeys.map(key => {
            const items = grouped.get(key) || []
            if (items.length === 0) return null
            const isExpanded = expandedCats.has(hash(key))
            return (
              <div key={key} className="border border-gray-800 rounded-lg overflow-hidden">
                <button
                  onClick={() => toggleCat(key)}
                  className="w-full flex items-center gap-2 px-3 py-2 bg-gray-900/60 hover:bg-gray-900 text-sm font-medium text-gray-300"
                >
                  {isExpanded ? <ChevronDown className="w-3 h-3" /> : <ChevronRight className="w-3 h-3" />}
                  <Folder className="w-3 h-3 text-smara-400" />
                  <span>{key}</span>
                  <span className="text-xs text-gray-500 ml-auto">{items.length}</span>
                </button>
                {isExpanded && (
                  <div className="p-2 space-y-2">
                    {items.map(m => (
                      <div key={m.id} className="bg-gray-900/30 border border-gray-800/50 rounded-lg p-3 hover:border-gray-700 transition-colors">
                        <div className="text-sm leading-relaxed mb-2 line-clamp-3">{m.content}</div>
                        <div className="flex items-center justify-between text-xs text-gray-500">
                          <div className="flex items-center gap-3 flex-wrap">
                            <span className="flex items-center gap-1">
                              <Tag className="w-3 h-3" />
                              {(m.tags || []).map((t, i) => (
                                <button
                                  key={i}
                                  onClick={() => setActiveTag(t)}
                                  className="hover:text-smara-300 transition-colors cursor-pointer"
                                >
                                  #{t}
                                </button>
                              ))}
                              {(!m.tags || m.tags.length === 0) && 'no tags'}
                            </span>
                            <span className="flex items-center gap-1">
                              <Clock className="w-3 h-3" />
                              {new Date(m.created_at).toLocaleString()}
                            </span>
                          </div>
                          <span className="text-smara-400">#{m.id}</span>
                        </div>
                      </div>
                    ))}
                  </div>
                )}
              </div>
            )
          })}
        </div>

        {!loading && visibleMemories.length === 0 && (
          <div className="text-center text-gray-600 py-12">
            <Database className="w-8 h-8 mx-auto mb-2 opacity-50" />
            <p className="text-sm">Belum ada memori tersimpan di workspace ini.</p>
          </div>
        )}
      </div>
    </div>
  )
}
