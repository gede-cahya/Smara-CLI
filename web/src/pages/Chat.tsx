import { useState, useRef, useEffect, useCallback } from 'react'
import {
  Send, Bot, User, RefreshCw, Plus, Trash2, MessageSquare, Clock,
  Zap, ClipboardList, FlaskConical, ArrowRightLeft, MessageCircle,
  CheckCircle2, BrainCircuit,
} from 'lucide-react'
import type { ChatMessage } from '../api'

const spinnerFrames = ['\u280B','\u2819','\u2839','\u2838','\u283C','\u2834','\u2826','\u2827','\u2807','\u280F']

const MODES: Array<{ id: string; label: string; emoji: string; icon: typeof MessageCircle; bg: string; border: string; text: string }> = [
  { id: 'ask', label: 'Ask', emoji: '\uD83D\uDCAC', icon: MessageCircle, bg: 'bg-cyan-600', border: 'border-cyan-500', text: 'text-cyan-400' },
  { id: 'rush', label: 'Rush', emoji: '\u26A1', icon: Zap, bg: 'bg-yellow-600', border: 'border-yellow-500', text: 'text-yellow-400' },
  { id: 'plan', label: 'Plan', emoji: '\uD83D\uDCCB', icon: ClipboardList, bg: 'bg-fuchsia-600', border: 'border-fuchsia-500', text: 'text-fuchsia-400' },
  { id: 'test', label: 'Test', emoji: '\uD83E\uDDEA', icon: FlaskConical, bg: 'bg-green-600', border: 'border-green-500', text: 'text-green-400' },
  { id: 'workflow', label: 'Workflow', emoji: '\uD83D\uDD04', icon: ArrowRightLeft, bg: 'bg-blue-600', border: 'border-blue-500', text: 'text-blue-400' },
]

const SESSION_META_KEY = 'smara_chat_sessions'
const CURRENT_SESSION_KEY = 'smara_current_session'

interface ChatSession {
  id: string
  name: string
  messages: ChatMessage[]
  updatedAt: string
}

function getAllSessions(): ChatSession[] {
  try {
    const raw = localStorage.getItem(SESSION_META_KEY)
    if (!raw) return []
    return JSON.parse(raw)
  } catch { return [] }
}

function saveAllSessions(sessions: ChatSession[]) {
  localStorage.setItem(SESSION_META_KEY, JSON.stringify(sessions))
}

function createSession(): ChatSession {
  const id = 'sess_' + Date.now()
  const session: ChatSession = {
    id,
    name: 'Chat ' + new Date().toLocaleString('id-ID', { hour: '2-digit', minute: '2-digit', day: 'numeric', month: 'short' }),
    messages: [{ role: 'assistant', content: 'Halo! Saya Smara. Ada yang bisa saya bantu?', timestamp: new Date().toISOString() }],
    updatedAt: new Date().toISOString(),
  }
  const sessions = [session, ...getAllSessions()]
  saveAllSessions(sessions)
  localStorage.setItem(CURRENT_SESSION_KEY, id)
  return session
}

function loadCurrentSession(): ChatSession {
  const currentId = localStorage.getItem(CURRENT_SESSION_KEY)
  const sessions = getAllSessions()
  if (currentId) {
    const found = sessions.find(s => s.id === currentId)
    if (found) return {
      ...found,
      messages: found.messages.map(m => ({ ...m, timestamp: new Date(m.timestamp) }))
    }
  }
  if (sessions.length > 0) {
    localStorage.setItem(CURRENT_SESSION_KEY, sessions[0].id)
    return {
      ...sessions[0],
      messages: sessions[0].messages.map(m => ({ ...m, timestamp: new Date(m.timestamp) }))
    }
  }
  return createSession()
}

function saveSession(id: string, messages: ChatMessage[]) {
  const sessions = getAllSessions()
  const idx = sessions.findIndex(s => s.id === id)
  const updated: ChatSession = {
    id,
    name: idx >= 0 ? sessions[idx].name : ('Chat ' + new Date().toLocaleString('id-ID', { hour: '2-digit', minute: '2-digit', day: 'numeric', month: 'short' })),
    messages: messages.map(m => ({ ...m, timestamp: m.timestamp instanceof Date ? m.timestamp.toISOString() : m.timestamp })),
    updatedAt: new Date().toISOString(),
  }
  if (idx >= 0) sessions[idx] = updated
  else sessions.unshift(updated)
  saveAllSessions(sessions)
}

export default function Chat() {
  const [sessions, setSessions] = useState<ChatSession[]>(getAllSessions)
  const [current, setCurrentRaw] = useState<ChatSession>(loadCurrentSession)
  const [messages, setMessages] = useState<ChatMessage[]>(current.messages)
  const [sessionId, setSessionId] = useState(current.id)
  const [input, setInput] = useState('')
  const [thinking, setThinking] = useState(false)
  const [connected, setConnected] = useState(false)
  const [showSessions, setShowSessions] = useState(false)
  const [mode, setMode] = useState('ask')
  const [spinnerIdx, setSpinnerIdx] = useState(0)
  const [statusStats, setStatusStats] = useState<{ prompts: number; tokens: number; duration: string; cost: number } | null>(null)
  const [activePhases, setActivePhases] = useState<Array<{ phase: string; description: string; status: 'running' | 'done' }>>([])
  const wsRef = useRef<WebSocket | null>(null)
  const bottomRef = useRef<HTMLDivElement>(null)
  const reconnectTimer = useRef<ReturnType<typeof setTimeout> | null>(null)
  const spinnerTimer = useRef<ReturnType<typeof setInterval> | null>(null)
  const sessionIdRef = useRef(sessionId)
  sessionIdRef.current = sessionId

  const setCurrent = (s: ChatSession) => {
    // Save current session first
    saveSession(sessionId, messages)
    setCurrentRaw(s)
    setSessionId(s.id)
    localStorage.setItem(CURRENT_SESSION_KEY, s.id)
    setMessages(s.messages.map(m => ({ ...m, timestamp: new Date(m.timestamp) })))
  }

  useEffect(() => {
    bottomRef.current?.scrollIntoView({ behavior: 'smooth' })
  }, [messages, thinking])

  // Persist messages to localStorage whenever they change
  useEffect(() => {
    saveSession(sessionId, messages)
    setSessions(getAllSessions())
  }, [messages, sessionId])

  const newSession = () => {
    saveSession(sessionId, messages)
    const s = createSession()
    setSessions(getAllSessions())
    setCurrentRaw(s)
    setSessionId(s.id)
    setMessages(s.messages.map(m => ({ ...m, timestamp: new Date(m.timestamp) })))
  }

  const deleteSession = (id: string) => {
    if (id === sessionId) {
      saveSession(id, messages)
    }
    const all = getAllSessions().filter(s => s.id !== id)
    saveAllSessions(all)
    setSessions(all)
    if (current.id === id) {
      if (all.length > 0) {
        const next = all[0]
        setCurrentRaw(next)
        setSessionId(next.id)
        localStorage.setItem(CURRENT_SESSION_KEY, next.id)
        setMessages(next.messages.map(m => ({ ...m, timestamp: new Date(m.timestamp) })))
      }
      else newSession()
    }
  }

  const connectWs = useCallback(() => {
    if (wsRef.current?.readyState === WebSocket.OPEN) return
    const ws = new WebSocket(`${location.protocol === 'https:' ? 'wss' : 'ws'}://${location.host}/ws`)
    wsRef.current = ws

    ws.onopen = () => {
      setConnected(true)
      ws.send(JSON.stringify({ type: 'session', payload: sessionIdRef.current }))
    }
    ws.onclose = () => {
      setConnected(false)
      wsRef.current = null
      reconnectTimer.current = setTimeout(connectWs, 3000)
    }
    ws.onerror = () => {
      setConnected(false)
      wsRef.current = null
    }

    ws.onmessage = (event) => {
      const msg = JSON.parse(event.data)
      switch (msg.type) {
        case 'connected':
          break
        case 'thinking':
          setThinking(msg.payload === 'true')
          if (msg.payload === 'true') {
            spinnerTimer.current = setInterval(() => {
              setSpinnerIdx(i => (i + 1) % spinnerFrames.length)
            }, 80)
          } else {
            if (spinnerTimer.current) clearInterval(spinnerTimer.current)
            spinnerTimer.current = null
          }
          break
        case 'chat':
          setThinking(false)
          if (spinnerTimer.current) { clearInterval(spinnerTimer.current); spinnerTimer.current = null }
          setActivePhases([])
          setMessages(prev => [...prev, { role: 'assistant', content: msg.payload, timestamp: new Date() }])
          break
        case 'error':
          setThinking(false)
          if (spinnerTimer.current) { clearInterval(spinnerTimer.current); spinnerTimer.current = null }
          setActivePhases([])
          setMessages(prev => [...prev, { role: 'error', content: msg.payload, timestamp: new Date() }])
          break
        case 'phase':
          setActivePhases(prev => {
            const next: Array<{ phase: string; description: string; status: 'running' | 'done' }> = prev.map(p => ({ ...p, status: 'done' }))
            const idx = next.findIndex(p => p.phase === msg.phase)
            if (idx >= 0) {
              next[idx] = { phase: msg.phase, description: msg.description || msg.phase, status: 'running' }
            } else {
              next.push({ phase: msg.phase, description: msg.description || msg.phase, status: 'running' })
            }
            return next
          })
          break
        case 'tool_call':
          setMessages(prev => [...prev, { role: 'tool_call', content: `\u25B6 ${msg.server ? `[${msg.server}] ` : ''}${msg.tool}`, tool: msg.tool, server: msg.server, timestamp: new Date() }])
          break
        case 'tool_result':
          setMessages(prev => [...prev, { role: 'tool_result', content: msg.output ? msg.output.slice(0, 300) : '', output: msg.output, timestamp: new Date() }])
          break
        case 'log':
          setMessages(prev => [...prev, { role: 'log', content: msg.payload, timestamp: new Date() }])
          break
        case 'mode':
          setMode(msg.mode || 'ask')
          break
        case 'stats':
          if (msg.stats) {
            setStatusStats({
              prompts: msg.stats.prompt_count,
              tokens: msg.stats.total_tokens,
              duration: msg.stats.duration,
              cost: msg.stats.cost,
            })
          }
          break
      }
    }
  }, [])

  useEffect(() => {
    connectWs()
    return () => {
      if (reconnectTimer.current) clearTimeout(reconnectTimer.current)
      wsRef.current?.close()
    }
  }, [connectWs])

  const send = useCallback(() => {
    const text = input.trim()
    if (!text) return
    if (!wsRef.current || wsRef.current.readyState !== WebSocket.OPEN) {
      setMessages(prev => [...prev, { role: 'user', content: text, timestamp: new Date() }])
      setInput('')
      connectWs()
      return
    }

    setMessages(prev => [...prev, { role: 'user', content: text, timestamp: new Date() }])
    setInput('')
    wsRef.current.send(JSON.stringify({ type: 'chat', payload: text, mode }))
    setThinking(true)
  }, [input, connectWs, mode])

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault()
      send()
    }
  }

  return (
    <div className="flex flex-col h-full">
      {/* Header */}
      <div className="h-14 border-b border-gray-800 flex items-center justify-between px-4 bg-gray-900/50">
        <div className="flex items-center gap-2">
          <Bot className="w-5 h-5 text-smara-400" />
          <button
            onClick={() => setShowSessions(!showSessions)}
            className="font-medium hover:text-smara-300 transition-colors"
          >
            {current.name}
          </button>
          <span className="text-[10px] text-gray-500 font-mono ml-2">{sessionId}</span>
        </div>
        <div className="flex items-center gap-3">
          <button
            onClick={newSession}
            className="flex items-center gap-1 px-2 py-1 text-xs bg-smara-700 hover:bg-smara-600 rounded transition-colors"
          >
            <Plus className="w-3 h-3" /> Sesi Baru
          </button>
          {!connected && (
            <button onClick={connectWs} className="flex items-center gap-1 text-xs text-smara-400 hover:text-smara-300 transition-colors">
              <RefreshCw className="w-3 h-3" /> Reconnect
            </button>
          )}
          <span className={`w-2 h-2 rounded-full ${connected ? 'bg-green-500' : 'bg-red-500'}`} />
          <span className="text-xs text-gray-500">{connected ? 'Online' : 'Offline'}</span>
        </div>
      </div>

      {/* Session dropdown */}
      {showSessions && (
        <div className="absolute top-14 left-64 right-0 z-50 bg-gray-900 border-b border-gray-800 p-2 max-h-48 overflow-y-auto">
          <div className="flex items-center justify-between mb-2 px-2">
            <span className="text-xs text-gray-500 font-medium">Sesi Chat ({sessions.length})</span>
            <button onClick={() => setShowSessions(false)} className="text-xs text-gray-500 hover:text-gray-300">Tutup</button>
          </div>
          <div className="space-y-1">
            {sessions.map(s => (
              <div
                key={s.id}
                onClick={() => { setCurrent(s); setShowSessions(false); }}
                className={`flex items-center justify-between p-2 rounded cursor-pointer text-sm ${
                  s.id === current.id ? 'bg-smara-900/40 border border-smara-700/30' : 'hover:bg-gray-800'
                }`}
              >
                <div className="flex items-center gap-2">
                  <MessageSquare className="w-3 h-3 text-gray-500" />
                  <span className="truncate max-w-[200px]">{s.name}</span>
                  <span className="text-[10px] text-gray-600">{s.messages.length} pesan</span>
                </div>
                <div className="flex items-center gap-2">
                  <span className="text-[10px] text-gray-600 flex items-center gap-1">
                    <Clock className="w-3 h-3" />
                    {new Date(s.updatedAt).toLocaleTimeString()}
                  </span>
                  {sessions.length > 1 && (
                    <button
                      onClick={(e) => { e.stopPropagation(); deleteSession(s.id); }}
                      className="text-gray-600 hover:text-red-400 transition-colors"
                    >
                      <Trash2 className="w-3 h-3" />
                    </button>
                  )}
                </div>
              </div>
            ))}
          </div>
        </div>
      )}

      {/* Messages */}
      <div className="flex-1 overflow-y-auto p-4 space-y-4">
        {messages.map((msg, i) => {
          if (msg.role === 'tool_call') {
            return (
              <div key={i} className="flex items-center gap-2 text-xs text-gray-500 px-2 pl-4 border-l-2 border-gray-700">
                <span className="text-cyan-400">{msg.content}</span>
              </div>
            )
          }
          if (msg.role === 'tool_result') {
            return (
              <div key={i} className="text-xs text-gray-600 px-2 pl-4 border-l-2 border-gray-700 whitespace-pre-wrap max-h-32 overflow-y-auto">
                {msg.content}
              </div>
            )
          }
          if (msg.role === 'log') {
            return (
              <div key={i} className="flex items-center gap-2 text-xs text-gray-500 px-2">
                <span className="text-gray-600">&#9654;</span>
                <span>{msg.content}</span>
              </div>
            )
          }
          return (
            <div key={i} className={`flex gap-3 ${msg.role === 'user' ? 'flex-row-reverse' : ''}`}>
              <div className={`w-8 h-8 rounded-lg flex items-center justify-center shrink-0 ${
                msg.role === 'user' ? 'bg-smara-700' : 'bg-gray-700'
              }`}>
                {msg.role === 'user' ? <User className="w-4 h-4" /> : <Bot className="w-4 h-4 text-smara-300" />}
              </div>
              <div className={`max-w-[80%] rounded-lg px-4 py-2 text-sm leading-relaxed ${
                msg.role === 'user'
                  ? 'bg-smara-700/30 border border-smara-700/50'
                  : msg.role === 'error'
                  ? 'bg-red-900/30 border border-red-700/50 text-red-200'
                  : 'bg-gray-800/50 border border-gray-700/50'
              }`}>
                <div className="whitespace-pre-wrap">{msg.content}</div>
                <div className="text-[10px] text-gray-500 mt-1 text-right">
                  {new Date(msg.timestamp).toLocaleTimeString()}
                </div>
              </div>
            </div>
          )
        })}

        {thinking && (
          <div className="flex gap-3">
            <div className="w-8 h-8 rounded-lg bg-gray-700 flex items-center justify-center shrink-0">
              <Bot className="w-4 h-4 text-smara-300" />
            </div>
            <div className="flex-1 max-w-lg space-y-2">
              {/* Active phase stepper */}
              {activePhases.length > 0 && (
                <div className="bg-gray-900/80 border border-gray-700/60 rounded-lg p-3 space-y-1.5">
                  <div className="flex items-center gap-2 text-[10px] text-gray-500 uppercase tracking-wider font-medium mb-1">
                    <BrainCircuit className="w-3 h-3" />
                    Proses Berjalan
                  </div>
                  {activePhases.map((ph, idx) => (
                    <div key={ph.phase + idx} className="flex items-center gap-2 text-xs">
                      {ph.status === 'running' ? (
                        <span className="text-smara-400 font-mono w-4 text-center">{spinnerFrames[spinnerIdx]}</span>
                      ) : (
                        <CheckCircle2 className="w-3.5 h-3.5 text-green-400 shrink-0" />
                      )}
                      <span className={ph.status === 'running' ? 'text-gray-200 font-medium' : 'text-gray-500'}>
                        {ph.description || ph.phase}
                      </span>
                    </div>
                  ))}
                </div>
              )}
              {activePhases.length === 0 && (
                <div className="bg-gray-800/50 border border-gray-700/50 rounded-lg px-4 py-3 flex items-center gap-2">
                  <span className="text-smara-400 text-sm font-mono">{spinnerFrames[spinnerIdx]}</span>
                  <span className="text-xs text-gray-400">Menghasilkan...</span>
                </div>
              )}
            </div>
          </div>
        )}
        <div ref={bottomRef} />
      </div>

      {/* Status bar */}
      {statusStats && (
        <div className="px-4 py-1 border-t border-gray-800/50 bg-gray-900/30 flex items-center gap-3 text-[10px] text-gray-500">
          <span className={`flex items-center gap-1 ${MODES.find(m => m.id === mode)?.text || 'text-gray-400'}`}>
            {MODES.find(m => m.id === mode)?.emoji} {MODES.find(m => m.id === mode)?.label || mode}
          </span>
          <span>prompts={statusStats.prompts}</span>
          <span>tokens={statusStats.tokens}</span>
          <span>dur={statusStats.duration}</span>
          <span>cost=${statusStats.cost.toFixed(4)}</span>
        </div>
      )}

      {/* Mode switcher + Input */}
      <div className="p-3 border-t border-gray-800 bg-gray-900/50 space-y-2">
        <div className="flex gap-1">
          {MODES.map(m => {
            const Icon = m.icon
            const active = mode === m.id
            return (
              <button
                key={m.id}
                onClick={() => {
                  setMode(m.id)
                  if (wsRef.current?.readyState === WebSocket.OPEN) {
                    wsRef.current.send(JSON.stringify({ type: 'mode_change', mode: m.id }))
                  }
                }}
                className={`flex items-center gap-1 px-2 py-1 text-xs rounded transition-colors ${
                  active
                    ? `${m.bg} text-white border ${m.border}`
                    : 'bg-gray-800 text-gray-400 hover:bg-gray-700 border border-transparent'
                }`}
                title={m.label}
              >
                <Icon className="w-3 h-3" />
                <span className="hidden sm:inline">{m.label}</span>
              </button>
            )
          })}
        </div>
        <div className="flex gap-2">
          <textarea
            value={input}
            onChange={e => setInput(e.target.value)}
            onKeyDown={handleKeyDown}
            placeholder="Ketik pesan... (Enter untuk kirim)"
            className="flex-1 bg-gray-800 border border-gray-700 rounded-lg px-3 py-2 text-sm resize-none focus:outline-none focus:border-smara-500 min-h-[40px] max-h-[120px]"
            rows={1}
          />
          <button
            onClick={send}
            disabled={!input.trim() || thinking}
            className="px-3 py-2 bg-smara-600 hover:bg-smara-500 disabled:opacity-50 disabled:cursor-not-allowed rounded-lg transition-colors"
          >
            <Send className="w-4 h-4" />
          </button>
        </div>
      </div>
    </div>
  )
}
