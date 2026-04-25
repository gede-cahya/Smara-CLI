import React, { useState, useEffect, useRef } from 'react'
import { Ask, GetWorkspaces, GetSessions, SwitchSession, GetSessionHistory, CreateSession, SetWorkspace, GetConfig, UpdateConfig, GetTools } from '../../wailsjs/go/main/App'
import { EventsOn } from '../../wailsjs/runtime'
import { marked } from 'marked'
import { cn } from "@/lib/utils"
import { 
  Plus, 
  Settings, 
  FolderOpen, 
  MessageSquare, 
  Send, 
  Sun, 
  Moon, 
  Paperclip, 
  Mic, 
  Atom,
  Terminal
} from 'lucide-react'

// ShadCN UI Components
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { ScrollArea } from "@/components/ui/scroll-area"
import { Card, CardContent } from "@/components/ui/card"
import { Badge } from "@/components/ui/badge"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"

interface Workspace {
  id: number
  name: string
}

interface Session {
  id: string
  name: string
}

interface Tool {
  name: string
  description: string
  source: string
}

interface Message {
  role: 'user' | 'assistant'
  content: string
}

const App: React.FC = () => {
  const [theme, setTheme] = useState<'dark' | 'light'>('light')
  const [workspaces, setWorkspaces] = useState<Workspace[]>([])
  const [sessions, setSessions] = useState<Session[]>([])
  const [messages, setMessages] = useState<Message[]>([])
  const [input, setInput] = useState('')
  const [isThinking, setIsThinking] = useState(false)
  const [currentWorkspace, setCurrentWorkspace] = useState('')
  const [tools, setTools] = useState<Tool[]>([])
  const [showSettings, setShowSettings] = useState(false)
  const [config, setConfig] = useState<any>(null)

  const chatEndRef = useRef<HTMLDivElement>(null)
  const inputRef = useRef<HTMLTextAreaElement>(null)

  useEffect(() => {
    marked.setOptions({ breaks: true })
    document.documentElement.classList.remove('dark', 'light')
    document.documentElement.classList.add(theme)
  }, [theme])

  useEffect(() => {
    const init = async () => {
      await loadWorkspaces()
      await loadSessions()
      await loadHistory()
      await loadTools()
      await loadConfig()
    }
    
    init()

    if (typeof EventsOn === 'function') {
      EventsOn('stream_chunk', (data: { chunk: string; is_thinking: boolean }) => {
        if (data.is_thinking) return
        setMessages(prev => {
          const last = prev[prev.length - 1]
          if (last && last.role === 'assistant') {
            return [...prev.slice(0, -1), { ...last, content: last.content + data.chunk }]
          }
          return [...prev, { role: 'assistant', content: data.chunk }]
        })
      })
    }
  }, [])

  useEffect(() => {
    chatEndRef.current?.scrollIntoView({ behavior: 'smooth' })
  }, [messages])

  const loadWorkspaces = async () => {
    try {
      if (typeof GetWorkspaces !== 'function') return
      const ws = await GetWorkspaces()
      if (ws) setWorkspaces(ws as Workspace[])
    } catch (err) { console.error('loadWorkspaces error:', err) }
  }

  const loadSessions = async () => {
    try {
      if (typeof GetSessions !== 'function') return
      const s = await GetSessions()
      if (s) setSessions(s as Session[])
    } catch (err) { console.error('loadSessions error:', err) }
  }

  const loadHistory = async () => {
    try {
      if (typeof GetSessionHistory !== 'function') return
      const history = await GetSessionHistory()
      if (history) {
        setMessages((history as Message[]).filter((m: any) => m.role === 'user' || m.role === 'assistant'))
      } else {
        setMessages([])
      }
    } catch (err) { console.error('loadHistory error:', err) }
  }

  const loadTools = async () => {
    try {
      if (typeof GetTools !== 'function') return
      const t = await GetTools()
      if (t) setTools(t as Tool[])
    } catch (err) { console.error('loadTools error:', err) }
  }

  const loadConfig = async () => {
    try {
      if (typeof GetConfig !== 'function') return
      const cfg = await GetConfig()
      if (cfg) setConfig(cfg)
    } catch (err) { console.error('loadConfig error:', err) }
  }

  const handleWorkspaceSelect = async (id: number, name: string) => {
    try {
      if (typeof SetWorkspace !== 'function') return
      await SetWorkspace(id)
      setCurrentWorkspace(name)
      await loadSessions()
      await loadHistory()
    } catch (err) { console.error('handleWorkspaceSelect error:', err) }
  }

  const handleSessionSelect = async (id: string) => {
    try {
      if (typeof SwitchSession !== 'function') return
      await SwitchSession(id)
      await loadHistory()
    } catch (err) { console.error('handleSessionSelect error:', err) }
  }

  const handleNewSession = async () => {
    const name = prompt('Nama Session Baru:', 'Sesi Baru')
    if (name) {
      try {
        if (typeof CreateSession !== 'function') return
        await CreateSession(name)
        setMessages([])
        await loadSessions()
      } catch (err) { console.error('handleNewSession error:', err) }
    }
  }

  const handleSend = async () => {
    if (!input.trim() || isThinking) return
    const text = input.trim()
    setInput('')
    setIsThinking(true)
    setMessages(prev => [...prev, { role: 'user', content: text }])

    try {
      if (typeof Ask !== 'function') {
        setMessages(prev => [...prev, { role: 'assistant', content: 'Wails bridge not found. Are you running in browser?' }])
        return
      }
      const response = await Ask(text)
      setMessages(prev => [...prev, { role: 'assistant', content: response }])
      await loadSessions()
    } catch (err) {
      console.error('handleSend error:', err)
      setMessages(prev => [...prev, { role: 'assistant', content: 'Maaf, terjadi kesalahan.' }])
    } finally {
      setIsThinking(false)
    }
  }

  const handleKeyPress = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault()
      handleSend()
    }
  }

  const toggleTheme = () => {
    setTheme(prev => prev === 'dark' ? 'light' : 'dark')
  }

  const handleSaveConfig = async () => {
    if (!config) return
    try {
      if (typeof UpdateConfig !== 'function') return
      await UpdateConfig(config)
      setShowSettings(false)
      loadTools()
    } catch (err) {
      alert('Gagal menyimpan: ' + err)
    }
  }

  return (
    <div className="flex h-screen w-full bg-background overflow-hidden font-sans text-foreground select-none">
      {/* Sidebar */}
      <aside className="w-72 glass-sidebar flex flex-col z-20">
        <div className="p-6 flex items-center gap-3">
          <div className="w-9 h-9 bg-smara rounded-xl flex items-center justify-center text-white shadow-lg shadow-primary/20">
            <Atom size={22} />
          </div>
          <span className="font-bold text-2xl tracking-tighter bg-clip-text text-transparent bg-gradient-to-r from-foreground to-foreground/70">Smara</span>
        </div>

        <ScrollArea className="flex-1 px-4">
          <div className="space-y-6">
            <div>
              <div className="px-2 mb-2 text-xs font-semibold text-muted-foreground uppercase tracking-wider">
                Workspaces
              </div>
              <div className="space-y-1">
                {workspaces.map(w => (
                  <button
                    key={w.id}
                    onClick={() => handleWorkspaceSelect(w.id, w.name)}
                    className={cn(
                      "w-full flex items-center gap-3 px-3 py-2.5 text-sm rounded-xl transition-all duration-200 group",
                      currentWorkspace === w.name 
                        ? "bg-primary/10 text-primary font-semibold shadow-sm" 
                        : "hover:bg-muted/50 text-muted-foreground hover:text-foreground hover:translate-x-1"
                    )}
                  >
                    <FolderOpen size={17} className={cn(
                      "transition-colors",
                      currentWorkspace === w.name ? "text-primary" : "text-muted-foreground group-hover:text-foreground"
                    )} />
                    {w.name}
                  </button>
                ))}
              </div>
            </div>

            <div>
              <div className="px-2 mb-2 flex items-center justify-between">
                <span className="text-xs font-semibold text-muted-foreground uppercase tracking-wider">
                  Sessions
                </span>
                <Button variant="ghost" size="icon" className="h-6 w-6" onClick={handleNewSession}>
                  <Plus size={14} />
                </Button>
              </div>
              <div className="space-y-1">
                {sessions.map(s => (
                  <button
                    key={s.id}
                    onClick={() => handleSessionSelect(s.id)}
                    className="w-full flex items-center gap-3 px-3 py-2.5 text-sm rounded-xl hover:bg-muted/50 text-muted-foreground hover:text-foreground transition-all truncate text-left group hover:translate-x-1"
                    title={s.name}
                  >
                    <MessageSquare size={17} className="shrink-0 text-muted-foreground group-hover:text-foreground transition-colors" />
                    <span className="truncate">{s.name || 'Untitled Session'}</span>
                  </button>
                ))}
              </div>
            </div>

            <div>
              <div className="px-2 mb-2 text-xs font-semibold text-muted-foreground uppercase tracking-wider">
                Connected Tools
              </div>
              <div className="flex flex-wrap gap-2 px-2">
                {tools.map((t, i) => (
                  <Badge key={i} variant="secondary" className="px-2 py-0 h-6 gap-1 font-normal bg-background/50 border-muted">
                    <div className="w-1.5 h-1.5 rounded-full bg-green-500 animate-pulse" />
                    {t.name}
                  </Badge>
                ))}
              </div>
            </div>
          </div>
        </ScrollArea>

        <div className="p-4 border-t bg-muted/50 mt-auto">
          <Button variant="ghost" className="w-full justify-start gap-3" onClick={() => { loadConfig(); setShowSettings(true) }}>
            <Settings size={18} />
            Settings
          </Button>
        </div>
      </aside>

      {/* Main Content */}
      <main className="flex-1 flex flex-col bg-background relative overflow-hidden">
        <header className="h-16 border-b flex items-center justify-between px-8 glass sticky top-0 z-10">
          <div className="flex items-center gap-4">
            <div className="flex flex-col text-left">
              <h2 className="font-bold text-sm tracking-tight text-foreground/90 uppercase">{currentWorkspace || 'SMARA DESKTOP'}</h2>
              <div className="flex items-center gap-2">
                <div className="w-2 h-2 rounded-full bg-green-500 shadow-[0_0_8px_rgba(34,197,94,0.5)]" />
                <span className="text-[10px] text-muted-foreground uppercase tracking-widest font-bold">System Ready</span>
              </div>
            </div>
          </div>

          <div className="flex items-center gap-4">
            <Button variant="ghost" size="icon" onClick={toggleTheme} className="rounded-full hover:bg-muted/50">
              {theme === 'dark' ? <Sun size={19} className="text-yellow-400" /> : <Moon size={19} className="text-slate-700" />}
            </Button>
            <div className="h-6 w-[1px] bg-border mx-2" />
            <Button variant="outline" size="sm" className="gap-2 text-xs font-bold uppercase tracking-widest px-4 rounded-full border-primary/20 hover:border-primary/50 transition-colors">
              <Terminal size={14} className="text-primary" />
              Ask Mode
            </Button>
          </div>
        </header>

        <ScrollArea className="flex-1 p-8">
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
              <div key={i} className={cn(
                "flex w-full animate-in-slide",
                msg.role === 'user' ? "justify-end" : "justify-start"
              )}>
                <div className={cn(
                  "max-w-[85%] rounded-2xl px-6 py-4 shadow-sm border transition-all duration-300",
                  msg.role === 'user' 
                    ? "bg-smara text-white border-transparent shadow-lg shadow-primary/10" 
                    : "bg-card/50 backdrop-blur-sm text-card-foreground border-border/50 hover:border-primary/20 prose prose-sm dark:prose-invert max-w-full"
                )}>
                  {msg.role === 'assistant' ? (
                    <div className="markdown-content leading-relaxed" dangerouslySetInnerHTML={{ __html: marked.parse(msg.content) as string }} />
                  ) : (
                    <div className="whitespace-pre-wrap font-medium">{msg.content}</div>
                  )}
                </div>
              </div>
            ))}

            {isThinking && (
              <div className="flex justify-start animate-in fade-in slide-in-from-bottom-1">
                <Card className="bg-muted/50 border-none shadow-none">
                  <CardContent className="p-3 flex items-center gap-3 text-sm text-muted-foreground">
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

        <div className="absolute bottom-0 left-0 right-0 p-8 bg-gradient-to-t from-background via-background/95 to-transparent pointer-events-none">
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
            <div className="space-y-4 py-4">
              <div className="space-y-2">
                <label className="text-sm font-medium">Provider</label>
                <select 
                  className="w-full h-10 px-3 rounded-md border border-input bg-background"
                  value={config.Provider} 
                  onChange={e => setConfig({ ...config, Provider: e.target.value })}
                >
                  <option value="ollama">Ollama</option>
                  <option value="openai">OpenAI</option>
                  <option value="openrouter">OpenRouter</option>
                  <option value="anthropic">Anthropic</option>
                  <option value="custom">Custom</option>
                </select>
              </div>

              <div className="space-y-2">
                <label className="text-sm font-medium">Model</label>
                <Input
                  value={config.Model || ''}
                  onChange={e => setConfig({ ...config, Model: e.target.value })}
                  placeholder="e.g. gpt-4o, claude-3-5-sonnet"
                />
              </div>

              <div className="space-y-2">
                <label className="text-sm font-medium">API Key</label>
                <Input
                  type="password"
                  value={config.OpenAIAPIKey || config.AnthropicAPIKey || config.OpenRouterAPIKey || config.CustomAPIKey || ''}
                  onChange={e => {
                    const provider = config.Provider
                    if (provider === 'openai') setConfig({ ...config, OpenAIAPIKey: e.target.value })
                    else if (provider === 'anthropic') setConfig({ ...config, AnthropicAPIKey: e.target.value })
                    else if (provider === 'openrouter') setConfig({ ...config, OpenRouterAPIKey: e.target.value })
                    else if (provider === 'custom') setConfig({ ...config, CustomAPIKey: e.target.value })
                  }}
                  placeholder="sk-..."
                />
              </div>

              <div className="space-y-2">
                <label className="text-sm font-medium">Base URL</label>
                <Input
                  value={config.OllamaHost || config.OpenAIBaseURL || config.CustomBaseURL || ''}
                  onChange={e => {
                    const provider = config.Provider
                    if (provider === 'ollama') setConfig({ ...config, OllamaHost: e.target.value })
                    else if (provider === 'openai') setConfig({ ...config, OpenAIBaseURL: e.target.value })
                    else if (provider === 'custom') setConfig({ ...config, CustomBaseURL: e.target.value })
                  }}
                  placeholder="e.g. http://localhost:11434"
                />
              </div>
            </div>
          )}

          <DialogFooter>
            <Button variant="outline" onClick={() => setShowSettings(false)}>Batal</Button>
            <Button onClick={handleSaveConfig}>Simpan Perubahan</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}

export default App