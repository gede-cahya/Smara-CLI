import React, { useState, useEffect, useRef, memo } from 'react'
import {
  Send,
  Plus,
  Settings,
  Terminal,
  History,
  User,
  Paperclip,
  Mic,
  Search,
  Atom,
  LayoutDashboard,
  Moon,
  Sun,
  ChevronRight,
  Sparkles,
  Archive,
  Trash2,
  RotateCcw,
  Copy,
  Check
} from 'lucide-react'
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { ScrollArea } from "@/components/ui/scroll-area"
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter
} from "@/components/ui/dialog"
import { Card, CardContent } from "@/components/ui/card"
import { cn } from "@/lib/utils"
import CollapsibleMarkdown from './CollapsibleMarkdown'
import VirtualWorkspace from './VirtualWorkspace'
import {
  Ask,
  GetSessions,
  CreateSession,
  GetSessionHistory,
  SwitchSession,
  GetTools,
  GetConfig,
  UpdateConfig,
  ArchiveSession,
  UnarchiveSession,
  GetArchivedSessions,
  DeleteArchivedSession,
  GetMode,
  SetMode
} from "../../wailsjs/go/main/App"
import { EventsOn } from "../../wailsjs/runtime/runtime"
import { config as configModels, llm as llmModels, session as sessionModels } from "../../wailsjs/go/models"

interface Message {
  role: string
  content: string
}

// Memoized Message Item to prevent redundant re-renders and re-parsing
const MessageItem = memo(({ msg, index, copiedIndex, onCopy }: {
  msg: Message
  index: number
  copiedIndex: number | null
  onCopy: (index: number, text: string) => void
}) => {
  const isAssistant = msg.role === 'assistant'
  const copied = copiedIndex === index

  return (
    <div className={cn(
      "flex w-full animate-in-slide group gpu-accelerated optimize-rendering",
      msg.role === 'user' ? "justify-end" : "justify-start"
    )}>
      <div className={cn(
        "relative max-w-[85%] rounded-2xl px-6 py-4 shadow-sm border transition-all duration-300",
        msg.role === 'user'
          ? "bg-smara text-white border-transparent shadow-lg shadow-primary/10"
          : "bg-card text-card-foreground border-border/50 hover:border-primary/20 prose prose-sm dark:prose-invert max-w-full"
      )}>
        {isAssistant ? (
          <CollapsibleMarkdown content={msg.content} />
        ) : (
          <div className="whitespace-pre-wrap font-medium">{msg.content}</div>
        )}
        <button
          type="button"
          onClick={() => onCopy(index, msg.content)}
          title={copied ? 'Tersalin' : 'Salin pesan'}
          aria-label={copied ? 'Pesan tersalin' : 'Salin pesan'}
          className={cn(
            "absolute -top-3 z-20 flex h-8 w-8 items-center justify-center rounded-xl border shadow-lg transition-all duration-200",
            msg.role === 'user' ? "-left-3" : "-right-3",
            copied
              ? "opacity-100 border-emerald-400/40 bg-emerald-500 text-white"
              : "opacity-80 md:opacity-0 border-border/70 bg-background/95 text-muted-foreground hover:border-primary/50 hover:bg-primary hover:text-primary-foreground md:group-hover:opacity-100"
          )}
        >
          {copied ? <Check size={15} /> : <Copy size={15} />}
        </button>
      </div>
    </div>
  )
})

export default function App() {
  const [messages, setMessages] = useState<Message[]>([])
  const [input, setInput] = useState('')
  const [sessions, setSessions] = useState<sessionModels.Session[]>([])
  const [archivedSessions, setArchivedSessions] = useState<sessionModels.Session[]>([])
  const [isThinking, setIsThinking] = useState(false)
  const [activeSession, setActiveSession] = useState<string | null>(null)
  const [showSettings, setShowSettings] = useState(false)
  const [archiveTab, setArchiveTab] = useState<'active' | 'archived'>('active')
  const [config, setConfig] = useState<configModels.SmaraConfig | null>(null)
  const [theme, setTheme] = useState<'light' | 'dark' | 'system'>('dark')
  const [activeView, setActiveView] = useState<'chat' | 'workspace'>('chat')
  const [currentMode, setCurrentMode] = useState<string>('ask')
  const [copyToast, setCopyToast] = useState<string | null>(null)
  const [copiedMessageIndex, setCopiedMessageIndex] = useState<number | null>(null)

  const chatEndRef = useRef<HTMLDivElement>(null)
  const inputRef = useRef<HTMLTextAreaElement>(null)
  const isAutoScrolling = useRef(true)
  const copyDebounce = useRef<number | null>(null)

  useEffect(() => {
    const init = async () => {
      await loadSessions()
      await loadHistory()
      await loadTools()
      await loadConfig()
      try {
        if (typeof GetMode === 'function') {
          const mode = await GetMode()
          if (mode) setCurrentMode(mode)
        }
      } catch (err) { console.error('loadMode error:', err) }
    }

    init()

    let off: any = null
    // Handle streaming chunks
    if (typeof EventsOn === 'function') {
      off = EventsOn('stream_chunk', (data: { chunk: string; is_thinking: boolean }) => {
        if (data.is_thinking) return
        setMessages(prev => {
          const last = prev[prev.length - 1]
          if (last && last.role === 'assistant') {
            // Append to existing assistant message
            return [
              ...prev.slice(0, -1),
              { ...last, content: last.content + data.chunk }
            ]
          }
          // Create new assistant message
          return [...prev, { role: 'assistant', content: data.chunk }]
        })
      })
    }

    return () => {
      if (typeof off === 'function') off()
    }
  }, [])

  // Watch for active session changes and reload history
  useEffect(() => {
    if (activeSession) {
      loadHistory()
    }
  }, [activeSession])

  // CapsLock toggles view (Chat / Workspace)
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'CapsLock') {
        e.preventDefault()
        setActiveView(prev => prev === 'chat' ? 'workspace' : 'chat')
      }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [])

  // Auto-copy on text selection release in chat area
  useEffect(() => {
    const onSelectionChange = () => {
      if (copyDebounce.current) {
        window.clearTimeout(copyDebounce.current)
      }
      copyDebounce.current = window.setTimeout(() => {
        const sel = window.getSelection()
        if (!sel || sel.rangeCount === 0) return
        const text = sel.toString().trim()
        if (!text) return
        // Only auto-copy if selection is inside the chat scroll area, not input
        const anchor = sel.anchorNode as Node | null
        if (!anchor) return
        const el = anchor.nodeType === Node.ELEMENT_NODE ? (anchor as Element) : anchor.parentElement
        const inChat = el?.closest('[data-chat-area]') != null
        const inInput = el?.closest('textarea') != null || el?.closest('[data-input-area]') != null
        if (inChat && !inInput && text.length > 0) {
          navigator.clipboard.writeText(text).then(() => {
            setCopyToast('Copied to clipboard')
            window.setTimeout(() => setCopyToast(null), 1500)
          }).catch(() => {})
        }
      }, 200)
    }
    document.addEventListener('selectionchange', onSelectionChange)
    return () => document.removeEventListener('selectionchange', onSelectionChange)
  }, [])

  // Robust auto-scroll logic
  useEffect(() => {
    if (isAutoScrolling.current) {
      chatEndRef.current?.scrollIntoView({ behavior: 'auto' })
    }
  }, [messages])

  const loadSessions = async () => {
    try {
      if (typeof GetSessions !== 'function') return
      const s = await GetSessions()
      if (s) setSessions(s)
    } catch (err) { console.error('loadSessions error:', err) }
  }

  const loadHistory = async () => {
    try {
      if (typeof GetSessionHistory !== 'function') return
      const history = await GetSessionHistory()
      if (history) {
        setMessages((history as llmModels.Message[]).filter((m: any) => m.role === 'user' || m.role === 'assistant'))
      } else {
        setMessages([])
      }
    } catch (err) { console.error('loadHistory error:', err) }
  }

  const loadTools = async () => {
    try {
      if (typeof GetTools !== 'function') return
      await GetTools()
    } catch (err) { console.error('loadTools error:', err) }
  }

  const loadConfig = async () => {
    try {
      if (typeof GetConfig !== 'function') return
      const cfg = await GetConfig()
      if (cfg) setConfig(cfg)
    } catch (err) { console.error('loadConfig error:', err) }
  }

  const handleSaveConfig = async () => {
    if (!config) return
    try {
      await UpdateConfig(config)
      setShowSettings(false)
    } catch (err) { console.error('UpdateConfig error:', err) }
  }

  const handleNewSession = async () => {
    const name = prompt('Nama sesi baru:')
    if (name) {
      try {
        if (typeof CreateSession !== 'function') return
        const newID = await CreateSession(name)
        if (newID) {
          await SwitchSession(newID)
          setActiveSession(newID)
          setMessages([]) // Clear UI immediately
        }
        await loadSessions()
      } catch (err) { console.error('handleNewSession error:', err) }
    }
  }


  const copyMessage = async (index: number, text: string) => {
    try {
      await navigator.clipboard.writeText(text)
      setCopiedMessageIndex(index)
      setCopyToast('Pesan disalin')
      window.setTimeout(() => setCopiedMessageIndex(current => (current === index ? null : current)), 1500)
      window.setTimeout(() => setCopyToast(null), 1500)
    } catch (err) {
      console.error('copyMessage error:', err)
      setCopyToast('Gagal menyalin pesan')
      window.setTimeout(() => setCopyToast(null), 1500)
    }
  }

  const handleSend = async () => {
    if (!input.trim() || isThinking) return
    const text = input.trim()
    setInput('')
    setIsThinking(true)

    // Add user message
    setMessages(prev => [...prev, { role: 'user', content: text }])
    isAutoScrolling.current = true

    try {
      if (typeof Ask !== 'function') {
        setMessages(prev => [...prev, { role: 'assistant', content: 'Wails bridge not found. Are you running in browser?' }])
        return
      }

      // We DON'T add the response manually here to avoid double-response bug.
      await Ask(text)
      await loadSessions()
    } catch (err) {
      console.error('handleSend error:', err)
      setMessages(prev => [...prev, { role: 'assistant', content: 'Maaf, terjadi kesalahan saat menghubungi asisten.' }])
    } finally {
      setIsThinking(false)
    }
  }

  const cycleMode = async () => {
    const modes = ['ask', 'rush', 'plan', 'test']
    const idx = modes.indexOf(currentMode)
    const next = modes[(idx + 1) % modes.length]
    try {
      if (typeof SetMode === 'function') {
        await SetMode(next)
        setCurrentMode(next)
      }
    } catch (err) { console.error('setMode error:', err) }
  }

  const handleKeyPress = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault()
      handleSend()
    }
    if (e.key === 'Tab') {
      e.preventDefault()
      cycleMode()
    }
  }

  const toggleTheme = () => {
    const newTheme = theme === 'dark' ? 'light' : 'dark'
    setTheme(newTheme)
    document.documentElement.classList.toggle('dark')
  }

  const updateConfig = (key: keyof configModels.SmaraConfig, value: any) => {
    if (!config) return
    const newConfig = configModels.SmaraConfig.createFrom({
      ...config,
      [key]: value
    })
    setConfig(newConfig)
  }

  const handleArchiveSession = async (id: string, e: React.MouseEvent) => {
    e.stopPropagation()
    try {
      if (typeof ArchiveSession !== 'function') return
      await ArchiveSession(id)
      await loadSessions()
      if (activeSession === id) setActiveSession(null)
    } catch (err) { console.error('archiveSession error:', err) }
  }

  const handleUnarchiveSession = async (id: string) => {
    try {
      if (typeof UnarchiveSession !== 'function') return
      await UnarchiveSession(id)
      await loadArchivedSessions()
      await loadSessions()
    } catch (err) { console.error('unarchiveSession error:', err) }
  }

  const loadArchivedSessions = async () => {
    try {
      if (typeof GetArchivedSessions !== 'function') return
      const s = await GetArchivedSessions()
      if (s) setArchivedSessions(s)
    } catch (err) { console.error('loadArchivedSessions error:', err) }
  }

  const handleDeleteArchivedSession = async (id: string) => {
    if (!confirm('Hapus permanen session ini? Data tidak bisa dikembalikan.')) return
    try {
      if (typeof DeleteArchivedSession !== 'function') return
      await DeleteArchivedSession(id)
      await loadArchivedSessions()
    } catch (err) { console.error('deleteArchivedSession error:', err) }
  }

  return (
    <div className={cn(
      "flex h-screen w-full bg-background text-foreground overflow-hidden font-sans selection:bg-primary/20",
      theme === 'dark' ? 'dark' : ''
    )}>
      {/* Premium Sidebar */}
      <aside className="w-80 flex flex-col glass-sidebar z-20">
        <div className="p-8">
          <div className="flex items-center gap-3 group cursor-default">
            <div className="w-10 h-10 bg-smara rounded-xl flex items-center justify-center shadow-lg shadow-primary/20 group-hover:scale-110 transition-transform duration-500">
              <Atom size={24} className="text-white animate-pulse" />
            </div>
            <div>
              <h2 className="text-xl font-bold tracking-tight">Smara</h2>
              <p className="text-[10px] text-muted-foreground uppercase tracking-widest font-bold">Intelligence v2.0</p>
            </div>
          </div>
        </div>

        <div className="px-4 mb-6">
          <Button
            onClick={handleNewSession}
            className="w-full justify-start gap-3 bg-smara text-white hover:opacity-90 shadow-lg shadow-primary/10 rounded-xl py-6 font-semibold transition-all active:scale-[0.98]"
          >
            <Plus size={20} />
            New Mission
          </Button>
        </div>

        <ScrollArea className="flex-1 px-4">
          <div className="space-y-2 pb-8">
            {/* Tab Switcher */}
            <div className="flex items-center gap-1 px-2 mb-3">
              <button
                onClick={() => setArchiveTab('active')}
                className={cn(
                  "flex-1 px-3 py-1.5 rounded-lg text-[10px] font-bold uppercase tracking-wider transition-all",
                  archiveTab === 'active'
                    ? "bg-primary/10 text-primary border border-primary/20"
                    : "text-muted-foreground hover:bg-muted/30"
                )}
              >
                Active
              </button>
              <button
                onClick={() => setArchiveTab('archived')}
                className={cn(
                  "flex-1 px-3 py-1.5 rounded-lg text-[10px] font-bold uppercase tracking-wider transition-all",
                  archiveTab === 'archived'
                    ? "bg-primary/10 text-primary border border-primary/20"
                    : "text-muted-foreground hover:bg-muted/30"
                )}
              >
                Archived
              </button>
            </div>

            <div className="flex items-center justify-between px-4 py-2 text-[10px] font-bold text-muted-foreground uppercase tracking-[0.2em]">
              <div className="flex items-center gap-2">
                <History size={12} />
                {archiveTab === 'active' ? 'Recent Sessions' : 'Archived Sessions'}
              </div>
              {archiveTab === 'archived' && archivedSessions.length > 0 && (
                <span className="text-[9px] bg-muted px-2 py-0.5 rounded-full">{archivedSessions.length}</span>
              )}
            </div>

            {archiveTab === 'active' && sessions.map(s => (
              <button
                key={s.id}
                onClick={async () => {
                  await SwitchSession(s.id)
                  setActiveSession(s.id)
                }}
                className={cn(
                  "w-full flex items-center gap-3 px-4 py-3 rounded-xl text-sm transition-all duration-300 group relative overflow-hidden",
                  activeSession === s.id
                    ? "bg-primary/10 text-primary font-bold border border-primary/20"
                    : "text-muted-foreground hover:bg-muted/50 hover:text-foreground"
                )}
              >
                <div className={cn(
                  "w-2 h-2 rounded-full transition-all duration-500",
                  activeSession === s.id ? "bg-primary scale-125 shadow-[0_0_8px_rgba(var(--primary),0.5)]" : "bg-muted-foreground/30"
                )} />
                <span className="truncate flex-1 text-left">{s.name}</span>
                {/* Archive button on hover */}
                <div
                  onClick={(e) => handleArchiveSession(s.id, e)}
                  className="opacity-0 group-hover:opacity-100 transition-opacity p-1 rounded-lg hover:bg-destructive/10 hover:text-destructive"
                  title="Archive session"
                >
                  <Archive size={14} />
                </div>
                <ChevronRight size={14} className={cn(
                  "transition-transform duration-300",
                  activeSession === s.id ? "translate-x-0 opacity-100" : "-translate-x-2 opacity-0"
                )} />
              </button>
            ))}

            {archiveTab === 'active' && sessions.length === 0 && (
              <div className="px-4 py-6 text-center text-xs text-muted-foreground/60">
                Tidak ada sesi aktif. Buat sesi baru untuk memulai.
              </div>
            )}

            {archiveTab === 'archived' && archivedSessions.map(s => (
              <div
                key={s.id}
                className="w-full flex items-center gap-3 px-4 py-3 rounded-xl text-sm transition-all duration-300 group relative overflow-hidden text-muted-foreground bg-muted/20 border border-border/20"
              >
                <Archive size={14} className="text-muted-foreground/50" />
                <span className="truncate flex-1 text-left">{s.name}</span>
                <div className="flex items-center gap-1 opacity-0 group-hover:opacity-100 transition-opacity">
                  <button
                    onClick={() => handleUnarchiveSession(s.id)}
                    className="p-1.5 rounded-lg hover:bg-primary/10 hover:text-primary transition-colors"
                    title="Restore session"
                  >
                    <RotateCcw size={14} />
                  </button>
                  <button
                    onClick={() => handleDeleteArchivedSession(s.id)}
                    className="p-1.5 rounded-lg hover:bg-destructive/10 hover:text-destructive transition-colors"
                    title="Delete permanently"
                  >
                    <Trash2 size={14} />
                  </button>
                </div>
              </div>
            ))}

            {archiveTab === 'archived' && archivedSessions.length === 0 && (
              <div className="px-4 py-6 text-center text-xs text-muted-foreground/60">
                Tidak ada sesi yang diarsipkan.
              </div>
            )}
          </div>
        </ScrollArea>

        <div className="p-4 mt-auto border-t border-border/30 bg-muted/5">
          <div className="flex items-center justify-between gap-2 p-2 rounded-2xl bg-background/50 border border-border/20 shadow-sm mb-4">
            <div className="flex items-center gap-3">
              <div className="w-9 h-9 rounded-xl bg-muted flex items-center justify-center text-muted-foreground">
                <User size={18} />
              </div>
              <div className="flex flex-col">
                <span className="text-xs font-bold">Cahya</span>
                <span className="text-[9px] text-muted-foreground uppercase font-black">Developer</span>
              </div>
            </div>
            <Button variant="ghost" size="icon" onClick={toggleTheme} className="h-9 w-9 rounded-xl">
              {theme === 'dark' ? <Sun size={18} /> : <Moon size={18} />}
            </Button>
          </div>

          <div className="grid grid-cols-2 gap-2">
            <Button
              variant="outline"
              size="sm"
              onClick={() => setShowSettings(true)}
              className="rounded-xl border-border/50 hover:bg-muted/50 transition-all text-xs h-10 gap-2"
            >
              <Settings size={14} />
              Setup
            </Button>
            <Button
              variant="outline"
              size="sm"
              className="rounded-xl border-border/50 hover:bg-muted/50 transition-all text-xs h-10 gap-2"
            >
              <LayoutDashboard size={14} />
              Fleet
            </Button>
          </div>
        </div>
      </aside>

      {/* Main Interface */}
      <main className="flex-1 flex flex-col relative bg-[radial-gradient(ellipse_at_top_right,_var(--tw-gradient-stops))] from-primary/5 via-background to-background">
        <header className="h-20 flex items-center justify-between px-8 border-b border-border/30 glass sticky top-0 z-10">
          <div className="flex items-center gap-4">
            <div className="flex flex-col">
              <h3 className="text-sm font-bold tracking-tight flex items-center gap-2">
                {activeSession ? sessions.find(s => s.id === activeSession)?.name : "General Intelligence"}
                <Sparkles size={14} className="text-primary animate-pulse" />
              </h3>
              <div className="flex items-center gap-1.5 mt-0.5">
                <div className="w-1.5 h-1.5 bg-green-500 rounded-full shadow-[0_0_4px_rgba(34,197,94,0.5)]" />
                <span className="text-[10px] font-bold text-muted-foreground uppercase tracking-widest">Active Core</span>
              </div>
            </div>
          </div>

          <div className="flex items-center gap-3">
            <div className="hidden md:flex items-center px-3 py-1.5 bg-muted/40 rounded-xl border border-border/50 gap-2">
              <span
                className="text-[10px] font-bold uppercase tracking-wider px-2 py-0.5 rounded-md bg-primary/10 text-primary border border-primary/20 cursor-pointer select-none"
                onClick={cycleMode}
                title="Click or press Tab to cycle mode"
              >
                {currentMode}
              </span>
            </div>
            <div className="hidden md:flex items-center px-4 py-2 bg-muted/40 rounded-xl border border-border/50 gap-3">
              <Search size={14} className="text-muted-foreground" />
              <input
                type="text"
                placeholder="Search history..."
                className="bg-transparent border-none text-xs focus:ring-0 w-32 placeholder:text-muted-foreground/50"
              />
            </div>
            <div className="flex items-center gap-2">
              <button
                onClick={() => setActiveView('chat')}
                className={cn(
                  "flex items-center gap-2 px-4 py-2 rounded-full text-xs font-bold uppercase tracking-widest transition-all border",
                  activeView === 'chat'
                    ? 'border-primary/40 bg-primary/10 text-primary'
                    : 'border-border/30 text-muted-foreground hover:text-foreground hover:border-primary/20'
                )}
              >
                <Terminal size={14} />
                Chat
              </button>
              <button
                onClick={() => setActiveView('workspace')}
                className={cn(
                  "flex items-center gap-2 px-4 py-2 rounded-full text-xs font-bold uppercase tracking-widest transition-all border",
                  activeView === 'workspace'
                    ? 'border-primary/40 bg-primary/10 text-primary'
                    : 'border-border/30 text-muted-foreground hover:text-foreground hover:border-primary/20'
                )}
              >
                <LayoutDashboard size={14} />
                Workspace
              </button>
            </div>
          </div>
        </header>

        {copyToast && (
          <div className="absolute top-24 left-1/2 -translate-x-1/2 z-50 px-4 py-2 bg-primary text-primary-foreground rounded-xl text-xs font-bold shadow-lg animate-in-fade pointer-events-none">
            {copyToast}
          </div>
        )}
        {activeView === 'chat' && (
        <>
        <ScrollArea
          className="flex-1 p-8 gpu-accelerated"
          onWheel={() => { isAutoScrolling.current = false }}
          data-chat-area
        >
          <div className="max-w-3xl mx-auto space-y-8 pb-32">
            {messages.length === 0 && (
              <div className="flex flex-col items-center justify-center py-20 text-center space-y-4">
                <div className="w-16 h-16 bg-muted rounded-full flex items-center justify-center text-muted-foreground">
                  <Atom size={32} />
                </div>
                <div>
                  <h1 className="text-2xl font-bold tracking-tight">Selamat datang di Smara</h1>
                  <p className="text-muted-foreground max-w-sm mx-auto">
                    Asisten AI yang siap membantu Anda dalam pengembangan perangkat lunak dan manajemen proyek.
                  </p>
                </div>
              </div>
            )}

            {messages.map((msg, i) => (
              <MessageItem key={i} msg={msg} index={i} copiedIndex={copiedMessageIndex} onCopy={copyMessage} />
            ))}

            {isThinking && messages[messages.length - 1]?.role === 'user' && (
              <div className="flex justify-start animate-in-fade">
                <Card className="max-w-[85%] bg-card/50 border-dashed border-border/50 rounded-2xl shadow-none">
                  <CardContent className="px-6 py-4 flex items-center gap-3 text-sm text-muted-foreground font-medium italic">
                    <div className="flex gap-1">
                      <div className="w-1.5 h-1.5 bg-muted-foreground/40 rounded-full animate-bounce [animation-delay:-0.3s]" />
                      <div className="w-1.5 h-1.5 bg-muted-foreground/40 rounded-full animate-bounce [animation-delay:-0.15s]" />
                      <div className="w-1.5 h-1.5 bg-muted-foreground/40 rounded-full animate-bounce" />
                    </div>
                    Smara sedang mengetik...
                  </CardContent>
                </Card>
              </div>
            )}
            <div ref={chatEndRef} />
          </div>
        </ScrollArea>

        <div className="absolute bottom-0 left-0 right-0 p-8 bg-gradient-to-t from-background via-background/95 to-transparent pointer-events-none" data-input-area>
          <div className="max-w-3xl mx-auto relative pointer-events-auto">
            <div className="relative group">
              <div className="absolute inset-0 bg-primary/5 rounded-2xl blur-xl group-focus-within:bg-primary/10 transition-all duration-500" />
              <div className="relative flex items-end gap-2 glass-card p-3 rounded-2xl shadow-2xl group-focus-within:border-primary/40 group-focus-within:ring-4 group-focus-within:ring-primary/5 transition-all duration-500 border-border/50">
                <Button variant="ghost" size="icon" className="shrink-0 h-10 w-10 text-muted-foreground hover:text-primary hover:bg-primary/5 rounded-xl transition-colors">
                  <Paperclip size={20} />
                </Button>
                <textarea
                  ref={inputRef}
                  placeholder="Ask anything or give a command..."
                  className="flex-1 min-h-[44px] max-h-48 py-3 px-2 bg-transparent border-none focus:ring-0 resize-none text-sm placeholder:text-muted-foreground/60 font-medium"
                  rows={1}
                  value={input}
                  onChange={e => setInput(e.target.value)}
                  onKeyDown={handleKeyPress}
                  onInput={e => {
                    const target = e.target as HTMLTextAreaElement
                    target.style.height = 'auto'
                    target.style.height = `${target.scrollHeight}px`
                  }}
                />
                <div className="flex items-center gap-1.5 p-1">
                  <Button variant="ghost" size="icon" className="h-10 w-10 text-muted-foreground hover:text-primary hover:bg-primary/5 rounded-xl transition-colors">
                    <Mic size={19} />
                  </Button>
                  <Button
                    onClick={handleSend}
                    disabled={!input.trim() || isThinking}
                    className={cn(
                      "h-10 w-10 bg-smara text-white rounded-xl shadow-lg shadow-primary/20 transition-all duration-300 active:scale-90 disabled:opacity-30 disabled:grayscale",
                    )}
                  >
                    <Send size={19} />
                  </Button>
                </div>
              </div>
            </div>
            <p className="mt-3 text-[10px] text-center text-muted-foreground/50 uppercase tracking-[0.2em] font-medium">
              Powered by Smara Intelligence Framework
            </p>
          </div>
        </div>
        </>
        )}

        {activeView === 'workspace' && <VirtualWorkspace />}
      </main>

      <Dialog open={showSettings} onOpenChange={setShowSettings}>
        <DialogContent className="sm:max-w-[500px]">
          <DialogHeader>
            <DialogTitle className="flex items-center gap-2">
              <Settings size={18} className="text-primary" />
              Smara Configuration
            </DialogTitle>
            <DialogDescription>
              Atur provider AI dan model yang ingin Anda gunakan.
            </DialogDescription>
          </DialogHeader>

          {config && (
            <div className="space-y-4 py-4 max-h-[400px] overflow-y-auto pr-2">
              <div className="space-y-2">
                <label className="text-sm font-bold uppercase tracking-widest text-muted-foreground">LLM Provider</label>
                <Input
                  value={config.Provider}
                  onChange={e => updateConfig('Provider', e.target.value)}
                  className="rounded-xl border-border/50"
                />
              </div>
              <div className="space-y-2">
                <label className="text-sm font-bold uppercase tracking-widest text-muted-foreground">Model</label>
                <Input
                  value={config.Model}
                  onChange={e => updateConfig('Model', e.target.value)}
                  className="rounded-xl border-border/50"
                />
              </div>
              <div className="space-y-2">
                <label className="text-sm font-bold uppercase tracking-widest text-muted-foreground">OpenAI API Key</label>
                <Input
                  type="password"
                  value={config.OpenAIAPIKey}
                  placeholder="sk-..."
                  onChange={e => updateConfig('OpenAIAPIKey', e.target.value)}
                  className="rounded-xl border-border/50"
                />
              </div>
              <div className="space-y-2">
                <label className="text-sm font-bold uppercase tracking-widest text-muted-foreground">Anthropic API Key</label>
                <Input
                  type="password"
                  value={config.AnthropicAPIKey}
                  placeholder="sk-ant-..."
                  onChange={e => updateConfig('AnthropicAPIKey', e.target.value)}
                  className="rounded-xl border-border/50"
                />
              </div>
            </div>
          )}

          <DialogFooter>
            <Button variant="outline" onClick={() => setShowSettings(false)} className="rounded-xl">Cancel</Button>
            <Button onClick={handleSaveConfig} className="bg-smara text-white rounded-xl">Save Configuration</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}