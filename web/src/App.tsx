import { useState, useEffect, lazy, Suspense, Component, useRef } from 'react'
import type { ReactNode } from 'react'
import { Atom, MessageSquare, Database, Layers, Settings, BarChart3, Terminal, GitBranch, TreePine, LineChart, Network, AlertTriangle, MousePointer2, Mic, Bot, Monitor, Sparkles, Image as ImageIcon, Zap } from 'lucide-react'
import { loadSmaraConfig } from './configStore'
import Chat, { type ChatHandle } from './pages/Chat'
import Memory from './pages/Memory'
import Workspace from './pages/Workspace'
import Config from './pages/Config'
import Dashboard from './pages/Dashboard'

import Workflow from './pages/Workflow'
import MagicPointer from './pages/MagicPointer'
import VoiceAssistant from './pages/VoiceAssistant'
import AvatarAssistant from './pages/AvatarAssistant'
import RemoteDesktop from './pages/RemoteDesktop'
import ParallelTasks from './pages/ParallelTasks'
class PageErrorBoundary extends Component<{ children: ReactNode; label: string }, { error: Error | null }> {
  state: { error: Error | null } = { error: null }

  static getDerivedStateFromError(error: Error) {
    return { error }
  }
  componentDidCatch(error: Error) {
    console.warn('[smara] page error:', error)
  }
  resetAll = () => {
    try {
      Object.keys(localStorage).forEach(k => {
        if (k.startsWith('smara_')) localStorage.removeItem(k)
      })
    } catch { /* ignore */ }
    this.setState({ error: null })
    window.location.reload()
  }
  render() {
    if (this.state.error) {
      return (
        <div className="h-full flex items-center justify-center p-8">
          <div className="max-w-md bg-slate-950/70 border border-red-500/30 rounded-3xl p-6 space-y-4 shadow-2xl backdrop-blur-xl">
            <div className="flex items-center gap-2 text-red-300">
              <AlertTriangle className="w-5 h-5" />
              <span className="font-semibold">{this.props.label} crashed</span>
            </div>
            <p className="text-sm text-gray-400">
              {this.state.error.name === 'QuotaExceededError'
                ? 'Penyimpanan browser penuh. Reset riwayat untuk lanjut.'
                : this.state.error.message}
            </p>
            <div className="flex gap-2">
              <button onClick={this.resetAll} className="px-3 py-1.5 bg-smara-600 hover:bg-smara-500 text-white text-sm rounded-xl transition-colors">
                Reset penyimpanan & reload
              </button>
              <button onClick={() => this.setState({ error: null })} className="px-3 py-1.5 bg-white/10 hover:bg-white/15 text-gray-300 text-sm rounded-xl transition-colors">
                Coba lagi
              </button>
            </div>
          </div>
        </div>
      )
    }
    return this.props.children
  }
}

const SkillTree = lazy(() => import('./pages/SkillTree'))
const SkillDashboard = lazy(() => import('./pages/SkillDashboard'))
const Graphify = lazy(() => import('./pages/Graphify'))
const CustomWorkflow = lazy(() => import('./pages/CustomWorkflow'))
const ImageFlow = lazy(() => import('./pages/ImageFlow'))

const TAB_KEY = 'smara_active_tab'

const navItems = [
  { id: 'chat', label: 'Chat', icon: MessageSquare, group: 'Core' },
  { id: 'dashboard', label: 'Dashboard', icon: BarChart3, group: 'Core' },
  { id: 'config', label: 'Config', icon: Settings, group: 'Core' },
  { id: 'workflow', label: 'Workflow', icon: GitBranch, group: 'Build' },
  { id: 'custom-workflow', label: 'Custom Workflow', icon: GitBranch, group: 'Build' },
  { id: 'parallel-tasks', label: 'Parallel Tasks', icon: Zap, group: 'Build' },
  { id: 'skilltree', label: 'Skill Tree', icon: TreePine, group: 'Build' },
  { id: 'skilldash', label: 'Analytics', icon: LineChart, group: 'Build' },
  { id: 'graphify', label: 'Graphify', icon: Network, group: 'Build' },
  { id: 'image-flow', label: 'Image Flow', icon: ImageIcon, group: 'Media' },
  { id: 'voice', label: 'Voice', icon: Mic, group: 'Media' },
  { id: 'avatar', label: 'Avatar', icon: Bot, group: 'Media' },
  { id: 'magic-pointer', label: 'Magic Pointer', icon: MousePointer2, group: 'System' },
  { id: 'remote-desktop', label: 'Remote Desktop', icon: Monitor, group: 'System' },
  { id: 'memory', label: 'Memory', icon: Database, group: 'System' },
  { id: 'workspace', label: 'Workspace', icon: Layers, group: 'System' },
]

const navGroups = ['Core', 'Build', 'Media', 'System']

function loadTab(): string {
  try {
    const saved = localStorage.getItem(TAB_KEY)
    if (saved && navItems.find(n => n.id === saved)) return saved
  } catch {}
  return 'chat'
}

export default function App() {
  const [active, setActiveRaw] = useState(loadTab)
  const chatRef = useRef<ChatHandle>(null)
  const setActive = (id: string) => {
    setActiveRaw(id)
    try { localStorage.setItem(TAB_KEY, id) } catch {}
  }

  useEffect(() => {
    loadSmaraConfig().catch(err => console.warn('[smara] failed to auto-load config:', err))

    const storageHandler = (e: StorageEvent) => {
      if (e.key === TAB_KEY && e.newValue && navItems.find(n => n.id === e.newValue)) setActiveRaw(e.newValue)
    }

    window.addEventListener('storage', storageHandler)
    return () => window.removeEventListener('storage', storageHandler)
  }, [])

  return (
    <div className="smara-shell flex h-screen w-screen overflow-hidden text-gray-100">
      <div className="smara-orb smara-orb-a" />
      <div className="smara-orb smara-orb-b" />
      <div className="smara-orb smara-orb-c" />

      <aside className="relative z-10 m-4 mr-0 h-[calc(100vh-2rem)] w-[286px] shrink-0 rounded-[1.65rem] border border-neutral-900/80 bg-[#151b12]/92 shadow-2xl shadow-black/45 backdrop-blur-2xl flex flex-col overflow-hidden">
        <div className="absolute inset-x-7 top-0 h-px bg-gradient-to-r from-transparent via-smara-300/22 to-transparent" />
        <div className="p-4 border-b border-neutral-900/80">
          <div className="flex items-center gap-3">
            <div className="relative">
              <div className="absolute inset-0 rounded-2xl bg-smara-300/18 blur-lg" />
              <div className="relative w-11 h-11 rounded-2xl bg-gradient-to-br from-smara-200 via-smara-400 to-smara-700 flex items-center justify-center shadow-lg shadow-smara-950/40 ring-1 ring-smara-300/16">
                <Atom className="w-6 h-6 text-black" />
              </div>
            </div>
            <div className="min-w-0">
              <div className="font-semibold text-xl tracking-tight text-white">Smara</div>
              <div className="text-[10px] uppercase tracking-[0.24em] text-smara-200/70">Autonomous Console</div>
            </div>
          </div>
          <div className="mt-4 rounded-2xl border border-neutral-900/70 bg-neutral-900/35 px-3 py-2 shadow-inner shadow-black/20">
            <div className="flex items-center gap-2 text-[11px] text-smara-100"><Sparkles className="h-3.5 w-3.5 text-smara-300" /> Ready for work</div>
            <div className="mt-1 text-[10px] text-neutral-500">CLI · Web · Skills · Memory</div>
          </div>
        </div>

        <nav className="flex-1 p-3 overflow-y-auto">
          <div className="space-y-4">
            {navGroups.map(group => (
              <div key={group}>
                <div className="mb-1.5 px-2 text-[10px] font-semibold uppercase tracking-[0.18em] text-neutral-600">{group}</div>
                <div className="space-y-1">
                  {navItems.filter(item => item.group === group).map(item => {
                    const Icon = item.icon
                    const isActive = active === item.id
                    return (
                      <button
                        key={item.id}
                        onClick={() => {
                          setActive(item.id)
                          if (item.id === 'chat') chatRef.current?.openSessions()
                        }}
                        className={`group relative w-full flex items-center gap-3 px-3 py-2.5 rounded-xl text-sm transition-all ${
                          isActive
                            ? 'bg-smara-300/10 text-white shadow-sm shadow-smara-950/20'
                            : 'text-neutral-500 hover:bg-neutral-900/60 hover:text-neutral-100'
                        }`}
                      >
                        {isActive && <span className="absolute left-0 top-2 bottom-2 w-1 rounded-r-full bg-smara-300/80 shadow-[0_0_14px_rgba(190,242,100,.45)]" />}
                        <span className={`grid h-8 w-8 place-items-center rounded-lg transition-colors ${isActive ? 'bg-smara-300/12 text-smara-200' : 'bg-neutral-900/50 text-neutral-500 group-hover:text-smara-200'}`}>
                          <Icon className="w-4 h-4" />
                        </span>
                        <span className="truncate text-left">{item.label}</span>
                        {item.id === 'chat' && (
                          <span className="ml-auto rounded-full bg-smara-300/10 px-2 py-0.5 text-[10px] text-smara-200">
                            Sesi
                          </span>
                        )}
                      </button>
                    )
                  })}
                </div>
              </div>
            ))}
          </div>
        </nav>

        <div className="m-3 rounded-2xl bg-neutral-900/35 p-3 text-xs text-neutral-500 shadow-inner shadow-black/20">
          <div className="flex items-center gap-2"><Terminal className="w-3.5 h-3.5 text-smara-300" /> Smara Web</div>
          <div className="mt-1 text-[10px] text-neutral-600">Local AI control center</div>
        </div>
      </aside>

      <main className="relative z-10 flex-1 overflow-hidden p-4">
        <div className="h-full overflow-hidden rounded-[1.65rem] bg-[#151d10]/96 shadow-2xl shadow-black/22 backdrop-blur-xl ring-1 ring-black/35">
          <div className={active === 'chat' ? 'h-full' : 'hidden'}><PageErrorBoundary label="Chat"><Chat ref={chatRef} /></PageErrorBoundary></div>
          <div className={active === 'image-flow' ? 'h-full' : 'hidden'}><Suspense fallback={<div className="p-4 text-gray-500 text-sm">Loading...</div>}><ImageFlow /></Suspense></div>
          <div className={active === 'workflow' ? 'h-full' : 'hidden'}><Workflow /></div>
          <div className={active === 'magic-pointer' ? 'h-full' : 'hidden'}><MagicPointer /></div>
          <div className={active === 'voice' ? 'h-full' : 'hidden'}><VoiceAssistant /></div>
          <div className={active === 'avatar' ? 'h-full' : 'hidden'}><AvatarAssistant /></div>
          <div className={active === 'remote-desktop' ? 'h-full' : 'hidden'}><RemoteDesktop /></div>
          <div className={active === 'custom-workflow' ? 'h-full' : 'hidden'}><Suspense fallback={<div className="p-4 text-gray-500 text-sm">Loading...</div>}><CustomWorkflow /></Suspense></div>
          <div className={active === 'parallel-tasks' ? 'h-full' : 'hidden'}><ParallelTasks /></div>
          <div className={active === 'skilltree' ? 'h-full' : 'hidden'}><Suspense fallback={<div className="p-4 text-gray-500 text-sm">Loading...</div>}><SkillTree /></Suspense></div>
          <div className={active === 'skilldash' ? 'h-full' : 'hidden'}><Suspense fallback={<div className="p-4 text-gray-500 text-sm">Loading...</div>}><SkillDashboard /></Suspense></div>
          <div className={active === 'graphify' ? 'h-full' : 'hidden'}><Suspense fallback={<div className="p-4 text-gray-500 text-sm">Loading...</div>}><Graphify /></Suspense></div>
          <div className={active === 'memory' ? 'h-full' : 'hidden'}><Memory /></div>
          <div className={active === 'workspace' ? 'h-full' : 'hidden'}><Workspace /></div>
          <div className={active === 'config' ? 'h-full' : 'hidden'}><Config /></div>
          <div className={active === 'dashboard' ? 'h-full' : 'hidden'}><Dashboard /></div>
        </div>
      </main>
    </div>
  )
}
