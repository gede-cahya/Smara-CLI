import { useState, useEffect, lazy, Suspense } from 'react'
import { Atom, MessageSquare, Database, Layers, Settings, BarChart3, Terminal, Wrench, GitBranch, TreePine, LineChart, Network, FolderTree } from 'lucide-react'
import Chat from './pages/Chat'
import Memory from './pages/Memory'
import Workspace from './pages/Workspace'
import Config from './pages/Config'
import Dashboard from './pages/Dashboard'
import Skills from './pages/Skills'
import Workflow from './pages/Workflow'

const SkillTree = lazy(() => import('./pages/SkillTree'))
const SkillDashboard = lazy(() => import('./pages/SkillDashboard'))
const Graphify = lazy(() => import('./pages/Graphify'))
const CustomWorkflow = lazy(() => import('./pages/CustomWorkflow'))

const TAB_KEY = 'smara_active_tab'

const navItems = [
  { id: 'chat', label: 'Chat', icon: MessageSquare },
  { id: 'workflow', label: 'Workflow', icon: GitBranch },
  { id: 'custom-workflow', label: 'Custom Workflow', icon: FolderTree },
  { id: 'skills', label: 'Skills', icon: Wrench },
  { id: 'skilltree', label: 'Skill Tree', icon: TreePine },
  { id: 'skilldash', label: 'Analytics', icon: LineChart },
  { id: 'graphify', label: 'Graphify', icon: Network },
  { id: 'memory', label: 'Memory', icon: Database },
  { id: 'workspace', label: 'Workspace', icon: Layers },
  { id: 'config', label: 'Config', icon: Settings },
  { id: 'dashboard', label: 'Dashboard', icon: BarChart3 },
]

function loadTab(): string {
  try {
    const saved = localStorage.getItem(TAB_KEY)
    if (saved && navItems.find(n => n.id === saved)) return saved
  } catch {}
  return 'chat'
}

export default function App() {
  const [active, setActiveRaw] = useState(loadTab)

  const setActive = (id: string) => {
    setActiveRaw(id)
    try { localStorage.setItem(TAB_KEY, id) } catch {}
  }

  // Also sync from storage events (other tabs)
  useEffect(() => {
    const handler = (e: StorageEvent) => {
      if (e.key === TAB_KEY && e.newValue && navItems.find(n => n.id === e.newValue)) {
        setActiveRaw(e.newValue)
      }
    }
    window.addEventListener('storage', handler)
    return () => window.removeEventListener('storage', handler)
  }, [])

  return (
    <div className="flex h-screen w-screen overflow-hidden bg-gray-950 text-gray-100">
      {/* Sidebar */}
      <aside className="w-64 bg-gray-900 border-r border-gray-800 flex flex-col">
        <div className="p-4 flex items-center gap-3 border-b border-gray-800">
          <div className="w-8 h-8 rounded-lg bg-smara-600 flex items-center justify-center">
            <Atom className="w-5 h-5 text-white" />
          </div>
          <span className="font-semibold text-lg">Smara</span>
        </div>

        <nav className="flex-1 p-2 space-y-1">
          {navItems.map(item => {
            const Icon = item.icon
            return (
              <button
                key={item.id}
                onClick={() => setActive(item.id)}
                className={`w-full flex items-center gap-3 px-3 py-2.5 rounded-lg text-sm transition-colors ${
                  active === item.id
                    ? 'bg-smara-700/20 text-smara-300'
                    : 'text-gray-400 hover:bg-gray-800 hover:text-gray-200'
                }`}
              >
                <Icon className="w-4 h-4" />
                {item.label}
              </button>
            )
          })}
        </nav>

        <div className="p-3 border-t border-gray-800 text-xs text-gray-500 flex items-center gap-2">
          <Terminal className="w-3 h-3" />
          Smara Web v1.0
        </div>
      </aside>

      {/* Main content */}
      <main className="flex-1 overflow-hidden">
        <div className={active === 'chat' ? 'h-full' : 'hidden'}><Chat /></div>
        <div className={active === 'workflow' ? 'h-full' : 'hidden'}><Workflow /></div>
        <div className={active === 'custom-workflow' ? 'h-full' : 'hidden'}><Suspense fallback={<div className="p-4 text-gray-500 text-sm">Loading...</div>}><CustomWorkflow /></Suspense></div>
        <div className={active === 'skills' ? 'h-full' : 'hidden'}><Skills /></div>
        <div className={active === 'skilltree' ? 'h-full' : 'hidden'}><Suspense fallback={<div className="p-4 text-gray-500 text-sm">Loading...</div>}><SkillTree /></Suspense></div>
        <div className={active === 'skilldash' ? 'h-full' : 'hidden'}><Suspense fallback={<div className="p-4 text-gray-500 text-sm">Loading...</div>}><SkillDashboard /></Suspense></div>
        <div className={active === 'graphify' ? 'h-full' : 'hidden'}><Suspense fallback={<div className="p-4 text-gray-500 text-sm">Loading...</div>}><Graphify /></Suspense></div>
        <div className={active === 'memory' ? 'h-full' : 'hidden'}><Memory /></div>
        <div className={active === 'workspace' ? 'h-full' : 'hidden'}><Workspace /></div>
        <div className={active === 'config' ? 'h-full' : 'hidden'}><Config /></div>
        <div className={active === 'dashboard' ? 'h-full' : 'hidden'}><Dashboard /></div>
      </main>
    </div>
  )
}
