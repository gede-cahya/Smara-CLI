import { useState, useEffect, lazy, Suspense, Component, useRef } from 'react'
import type { ReactNode } from 'react'
import { Atom, MessageSquare, Database, Layers, Settings, BarChart3, Terminal, GitBranch, TreePine, LineChart, Network, AlertTriangle, MousePointer2, Mic, Bot, Monitor } from 'lucide-react'
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
          <div className="max-w-md bg-black/70 border border-red-500/30 rounded-3xl p-6 space-y-4 shadow-2xl backdrop-blur-xl">
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

const TAB_KEY = 'smara_active_tab'

const navItems = [
  { id: 'chat', label: 'Chat', icon: MessageSquare },
  { id: 'workflow', label: 'Workflow', icon: GitBranch },
  { id: 'custom-workflow', label: 'Custom Workflow', icon: GitBranch },
  { id: 'magic-pointer', label: 'Magic Pointer', icon: MousePointer2 },
  { id: 'voice', label: 'Voice', icon: Mic },
  { id: 'avatar', label: 'Avatar', icon: Bot },
  { id: 'remote-desktop', label: 'Remote Desktop', icon: Monitor },
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
  const chatRef = useRef<ChatHandle>(null)

  const setActive = (id: string) => {
    setActiveRaw(id)
    try { localStorage.setItem(TAB_KEY, id) } catch {}
  }

  useEffect(() => {
    const handler = (e: StorageEvent) => {
      if (e.key === TAB_KEY && e.newValue && navItems.find(n => n.id === e.newValue)) setActiveRaw(e.newValue)
    }
    window.addEventListener('storage', handler)
    return () => window.removeEventListener('storage', handler)
  }, [])

  return (
    <div className="smara-shell flex h-screen w-screen overflow-hidden text-gray-100">
      <div className="smara-orb smara-orb-a" />
      <div className="smara-orb smara-orb-b" />
      <div className="smara-orb smara-orb-c" />

      <aside className="relative z-10 m-4 mr-0 w-72 rounded-[2rem] border border-white/10 bg-black/35 shadow-2xl shadow-black/50 backdrop-blur-2xl flex flex-col overflow-hidden">
        <div className="absolute inset-x-6 top-0 h-px bg-gradient-to-r from-transparent via-cyan-300/60 to-transparent" />
        <div className="p-5 flex items-center gap-3 border-b border-white/10">
          <div className="w-11 h-11 rounded-2xl bg-gradient-to-br from-cyan-400 via-smara-500 to-fuchsia-500 flex items-center justify-center shadow-lg shadow-cyan-500/20">
            <Atom className="w-6 h-6 text-white" />
          </div>
          <div>
            <div className="font-bold text-xl tracking-tight">Smara</div>
            <div className="text-[10px] uppercase tracking-[0.28em] text-cyan-200/70">AI Console</div>
          </div>
        </div>

        <nav className="flex-1 p-3 space-y-1 overflow-y-auto">
          {navItems.map(item => {
            const Icon = item.icon
            const isActive = active === item.id
            return (
              <button
                key={item.id}
                onClick={() => {
                  setActive(item.id)
                  if (item.id === 'chat') chatRef.current?.openSessions()
                }}
                className={`w-full flex items-center gap-3 px-3.5 py-3 rounded-2xl text-sm transition-all border ${
                  isActive
                    ? 'bg-gradient-to-r from-cyan-500/20 via-smara-500/20 to-fuchsia-500/20 border-cyan-300/25 text-white shadow-lg shadow-cyan-950/30'
                    : 'border-transparent text-gray-400 hover:bg-white/7 hover:text-gray-100 hover:border-white/10'
                }`}
              >
                <Icon className={`w-4 h-4 ${isActive ? 'text-cyan-200' : ''}`} />
                <span className="truncate">{item.label}</span>
                {item.id === 'chat' && (
                  <span className="ml-auto rounded-full border border-cyan-400/25 bg-cyan-500/10 px-2 py-0.5 text-[10px] text-cyan-200">
                    Sesi ▼
                  </span>
                )}
              </button>
            )
          })}
        </nav>

        <div className="p-4 border-t border-white/10 text-xs text-gray-500 flex items-center gap-2 bg-white/[0.03]">
          <Terminal className="w-3 h-3 text-cyan-300" />
          Smara Web v1.0
        </div>
      </aside>

      <main className="relative z-10 flex-1 overflow-hidden p-4">
        <div className="h-full overflow-hidden rounded-[2rem] border border-white/10 bg-black/30 shadow-2xl shadow-black/40 backdrop-blur-xl">
          <div className={active === 'chat' ? 'h-full' : 'hidden'}><PageErrorBoundary label="Chat"><Chat ref={chatRef} /></PageErrorBoundary></div>
          <div className={active === 'workflow' ? 'h-full' : 'hidden'}><Workflow /></div>
          <div className={active === 'magic-pointer' ? 'h-full' : 'hidden'}><MagicPointer /></div>
          <div className={active === 'voice' ? 'h-full' : 'hidden'}><VoiceAssistant /></div>
          <div className={active === 'avatar' ? 'h-full' : 'hidden'}><AvatarAssistant /></div>
          <div className={active === 'remote-desktop' ? 'h-full' : 'hidden'}><RemoteDesktop /></div>
          <div className={active === 'custom-workflow' ? 'h-full' : 'hidden'}><Suspense fallback={<div className="p-4 text-gray-500 text-sm">Loading...</div>}><CustomWorkflow /></Suspense></div>
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
