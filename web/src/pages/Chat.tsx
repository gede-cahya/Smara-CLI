import { useState, useRef, useEffect, useCallback } from 'react'
import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import mermaid from 'mermaid'
import { BarChart, Bar, LineChart, Line, XAxis, YAxis, CartesianGrid, Tooltip, ResponsiveContainer, PieChart, Pie, Cell } from 'recharts'
import {
  Send, Bot, User, RefreshCw, Plus, Trash2, MessageSquare, Clock,
  Zap, ClipboardList, FlaskConical, ArrowRightLeft, MessageCircle,
  CheckCircle2, BrainCircuit, Copy, Check, X,
  Paperclip, FileText, FileCode, FileJson, File as FileIcon, Upload,
  Terminal, ChevronDown, ChevronRight, Loader2, AlertCircle, Wrench,
} from 'lucide-react'
import type { ChatMessage } from '../api'
import { uploadClipboardImage, uploadAttachment } from '../api'
type Attachment = {
  path: string
  size: number
  kind: 'image' | 'file'
  name: string
  preview?: string // dataURL only for images
  mime?: string
}

function attachmentIcon(name: string, mime?: string) {
  const ext = name.split('.').pop()?.toLowerCase() || ''
  if (mime?.startsWith('text/') || ['txt', 'md', 'log', 'rst'].includes(ext)) return FileText
  if (['json', 'yaml', 'yml', 'toml', 'xml'].includes(ext)) return FileJson
  if (['go', 'ts', 'tsx', 'js', 'jsx', 'py', 'rs', 'java', 'c', 'cpp', 'h', 'sh'].includes(ext)) return FileCode
  if (mime === 'application/pdf' || ext === 'pdf') return FileText
  return FileIcon
}

function generatedImageUrls(text?: string): string[] {
  if (!text) return []
  const urls = new Set<string>()
  const re = /!\[[^\]]*\]\((\/api\/generated-image\?path=[^)]+)\)/g
  let match: RegExpExecArray | null
  while ((match = re.exec(text)) !== null) urls.add(match[1])
  return [...urls]
}

const CHART_COLORS = ['#22d3ee', '#a78bfa', '#34d399', '#fbbf24', '#fb7185', '#60a5fa']

function MermaidBlock({ code }: { code: string }) {
  const [svg, setSvg] = useState('')
  const [error, setError] = useState('')

  useEffect(() => {
    let cancelled = false
    mermaid.initialize({ startOnLoad: false, theme: 'dark', securityLevel: 'loose' })
    const id = `smara-mermaid-${Math.random().toString(36).slice(2)}`
    mermaid.render(id, code)
      .then(({ svg }) => {
        if (!cancelled) setSvg(svg)
      })
      .catch((err) => {
        if (!cancelled) setError(err?.message || 'Gagal render diagram')
      })
    return () => { cancelled = true }
  }, [code])

  if (error) {
    return <div className="my-2 rounded-xl border border-red-700/50 bg-red-950/20 p-3 text-xs text-red-200">Mermaid error: {error}</div>
  }
  return (
    <div className="my-3 overflow-x-auto rounded-2xl border border-smara-500/30 bg-gray-950/70 p-4 shadow-lg shadow-black/25">
      {svg ? <div className="min-w-fit" dangerouslySetInnerHTML={{ __html: svg }} /> : <div className="text-xs text-gray-400">Rendering diagram...</div>}
    </div>
  )
}

function SmartChart({ data }: { data: any[] }) {
  const keys = Object.keys(data[0] || {})
  const labelKey = keys.find(k => typeof data[0]?.[k] === 'string') || keys[0]
  const numericKeys = keys.filter(k => data.some(row => typeof row?.[k] === 'number'))
  if (data.length < 2 || numericKeys.length === 0) return null
  const valueKey = numericKeys[0]

  return (
    <div className="my-3 grid gap-3 lg:grid-cols-2">
      <div className="h-72 rounded-2xl border border-cyan-500/20 bg-gray-950/60 p-3">
        <div className="mb-2 text-xs font-semibold uppercase tracking-wide text-cyan-300">Auto Bar Chart</div>
        <ResponsiveContainer width="100%" height="88%">
          <BarChart data={data}><CartesianGrid strokeDasharray="3 3" stroke="#334155" /><XAxis dataKey={labelKey} stroke="#94a3b8" /><YAxis stroke="#94a3b8" /><Tooltip /><Bar dataKey={valueKey} fill="#22d3ee" radius={[6, 6, 0, 0]} /></BarChart>
        </ResponsiveContainer>
      </div>
      <div className="h-72 rounded-2xl border border-violet-500/20 bg-gray-950/60 p-3">
        <div className="mb-2 text-xs font-semibold uppercase tracking-wide text-violet-300">Auto Line Chart</div>
        <ResponsiveContainer width="100%" height="88%">
          <LineChart data={data}><CartesianGrid strokeDasharray="3 3" stroke="#334155" /><XAxis dataKey={labelKey} stroke="#94a3b8" /><YAxis stroke="#94a3b8" /><Tooltip /><Line type="monotone" dataKey={valueKey} stroke="#a78bfa" strokeWidth={2} /></LineChart>
        </ResponsiveContainer>
      </div>
      {data.length <= 8 && (
        <div className="h-72 rounded-2xl border border-emerald-500/20 bg-gray-950/60 p-3 lg:col-span-2">
          <div className="mb-2 text-xs font-semibold uppercase tracking-wide text-emerald-300">Auto Pie Chart</div>
          <ResponsiveContainer width="100%" height="88%">
            <PieChart><Pie data={data} dataKey={valueKey} nameKey={labelKey} outerRadius={90} label>{data.map((_, i) => <Cell key={i} fill={CHART_COLORS[i % CHART_COLORS.length]} />)}</Pie><Tooltip /></PieChart>
          </ResponsiveContainer>
        </div>
      )}
    </div>
  )
}

function JsonVisualBlock({ code }: { code: string }) {
  let parsed: any
  try { parsed = JSON.parse(code) } catch { return null }
  const rows = Array.isArray(parsed) ? parsed : Array.isArray(parsed?.data) ? parsed.data : null
  if (!rows || !rows.every((r: any) => r && typeof r === 'object' && !Array.isArray(r))) return null
  const columns = Array.from(new Set(rows.flatMap((r: any) => Object.keys(r)))) as string[]
  return (
    <div className="my-3 overflow-hidden rounded-2xl border border-smara-500/25 bg-gray-950/60">
      <div className="border-b border-gray-800 px-3 py-2 text-xs font-semibold uppercase tracking-wide text-smara-300">Auto Visual JSON</div>
      <div className="overflow-x-auto">
        <table className="min-w-full text-left text-xs text-gray-200">
          <thead className="bg-gray-900/80 text-gray-300"><tr>{columns.map(c => <th key={c} className="px-3 py-2 font-semibold">{c}</th>)}</tr></thead>
          <tbody>{rows.map((row: any, i: number) => <tr key={i} className="border-t border-gray-800/80">{columns.map(c => <td key={c} className="px-3 py-2 font-mono">{String(row[c] ?? '')}</td>)}</tr>)}</tbody>
        </table>
      </div>
      <div className="p-3"><SmartChart data={rows} /></div>
    </div>
  )
}

function formatCodeBlock(raw: string, language: string) {
  const text = raw.replace(/\n$/, '')
  if (language.toLowerCase() !== 'json') return text
  try {
    return JSON.stringify(JSON.parse(text), null, 2)
  } catch {
    return text
  }
}

function MarkdownCodeBlock({ children, className, inline, ...props }: any) {
  const [copied, setCopied] = useState(false)
  const match = /language-([\w-]+)/.exec(className || '')
  const language = match?.[1] || ''
  const raw = String(children ?? '')

  if (inline || !language) {
    return <code className="rounded border border-smara-500/20 bg-smara-950/40 px-1 py-0.5 text-[0.85em] text-cyan-200 font-mono" {...props}>{children}</code>
  }

  const code = formatCodeBlock(raw, language)
  const copy = async () => {
    try {
      await navigator.clipboard.writeText(code)
      setCopied(true)
      window.setTimeout(() => setCopied(false), 1200)
    } catch {}
  }

  if (language.toLowerCase() === 'mermaid') return <MermaidBlock code={code} />
  const jsonVisual = language.toLowerCase() === 'json' ? <JsonVisualBlock code={code} /> : null

  return (
    <>
      {jsonVisual}
      <div className="my-2 overflow-hidden rounded-xl border border-gray-700/70 bg-gray-950/85 shadow-inner shadow-black/25">
        <div className="flex items-center gap-2 border-b border-gray-800/80 bg-gray-900/80 px-3 py-1.5">
          <span className="text-[10px] font-semibold uppercase tracking-wide text-smara-300">{language}</span>
          <button onClick={copy} className="ml-auto flex items-center gap-1 rounded-md border border-gray-700 bg-gray-950/80 px-2 py-1 text-[10px] text-gray-300 hover:border-smara-400 hover:text-white transition-colors" title={language.toLowerCase() === 'json' ? 'Copy JSON' : 'Copy code'}>
            {copied ? <Check className="w-3 h-3 text-green-400" /> : <Copy className="w-3 h-3" />}
            {copied ? 'Copied' : language.toLowerCase() === 'json' ? 'Copy JSON' : 'Copy'}
          </button>
        </div>
        <code className="block overflow-x-auto p-3 text-xs leading-5 text-gray-100 font-mono" {...props}>{code}</code>
      </div>
    </>
  )
}

function SmaraMarkdown({ content }: { content: string }) {
  return (
    <ReactMarkdown
      remarkPlugins={[remarkGfm]}
      components={{
        p: ({ children }) => <p className="mb-2 last:mb-0 text-gray-200 leading-6">{children}</p>,
        strong: ({ children }) => <strong className="font-semibold text-white bg-smara-500/10 px-0.5 rounded">{children}</strong>,
        em: ({ children }) => <em className="text-smara-200 not-italic">{children}</em>,
        ul: ({ children }) => <ul className="my-2 space-y-1.5 pl-1">{children}</ul>,
        ol: ({ children }) => <ol className="my-2 space-y-1.5 pl-5 list-decimal marker:text-smara-400">{children}</ol>,
        li: ({ children }) => (
          <li className="relative pl-5 text-gray-200 leading-6 before:content-[''] before:absolute before:left-0 before:top-2.5 before:w-1.5 before:h-1.5 before:rounded-full before:bg-gradient-to-r before:from-smara-400 before:to-cyan-300">
            {children}
          </li>
        ),
        code: MarkdownCodeBlock,
        pre: ({ children }) => <>{children}</>,
        table: ({ children }) => <div className="my-3 overflow-x-auto rounded-2xl border border-smara-500/25 bg-gray-950/60"><table className="min-w-full text-left text-sm text-gray-200">{children}</table></div>,
        thead: ({ children }) => <thead className="bg-gray-900/80 text-gray-100">{children}</thead>,
        tbody: ({ children }) => <tbody className="divide-y divide-gray-800/80">{children}</tbody>,
        tr: ({ children }) => <tr className="hover:bg-smara-500/5">{children}</tr>,
        th: ({ children }) => <th className="px-3 py-2 text-xs font-semibold uppercase tracking-wide text-smara-200">{children}</th>,
        td: ({ children }) => <td className="px-3 py-2 text-sm text-gray-200">{children}</td>,
        blockquote: ({ children }) => <blockquote className="my-2 border-l-2 border-smara-400/70 bg-smara-950/20 px-3 py-2 text-gray-300 rounded-r-xl">{children}</blockquote>,
        a: ({ children, href }) => <a href={href} target="_blank" rel="noreferrer" className="text-cyan-300 underline decoration-cyan-400/40 underline-offset-4 hover:text-cyan-200">{children}</a>,
        img: ({ src, alt }) => <img src={src || ''} alt={alt || 'generated image'} className="my-2 max-h-96 rounded-xl border border-gray-700/70 shadow-lg shadow-black/30" />,
        h1: ({ children }) => <h1 className="mb-2 mt-1 text-xl font-bold text-white tracking-tight">{children}</h1>,
        h2: ({ children }) => <h2 className="mb-2 mt-3 text-lg font-semibold text-white tracking-tight">{children}</h2>,
        h3: ({ children }) => <h3 className="mb-1.5 mt-3 text-base font-semibold text-smara-100">{children}</h3>,
        hr: () => <hr className="my-3 border-gray-700/60" />,
      }}
    >
      {content}
    </ReactMarkdown>
  )
}

// Tools whose primary purpose is to mutate the filesystem or remote state.
// We use this to pick a color and icon for the card chrome.
const WRITE_TOOLS = new Set(['edit_file', 'write_file', 'create_file', 'rm', 'remove_file'])
const SHELL_TOOLS = new Set(['run_command', 'ssh_exec'])

function toolKind(tool?: string): 'shell' | 'write' | 'read' | 'tool' {
  if (!tool) return 'tool'
  if (SHELL_TOOLS.has(tool)) return 'shell'
  if (WRITE_TOOLS.has(tool)) return 'write'
  if (tool.startsWith('read_') || tool === 'list_directory' || tool === 'search_path') return 'read'
  return 'tool'
}

function describeToolCall(msg: ChatMessage): { title: string; subtitle?: string } {
  const tool = msg.tool || 'tool'
  const args = msg.args || {}
  switch (tool) {
    case 'run_command': {
      const cmd = String(args.command ?? '')
      return { title: 'run_command', subtitle: cmd }
    }
    case 'ssh_exec': {
      const host = String(args.host ?? '')
      const cmd = String(args.command ?? '')
      return { title: `ssh ${host}`, subtitle: cmd }
    }
    case 'read_file':
    case 'edit_file':
    case 'write_file': {
      const path = String(args.path ?? '')
      return { title: tool, subtitle: path }
    }
    case 'search_path': {
      const q = String(args.query ?? '')
      return { title: 'search_path', subtitle: q }
    }
    default: {
      // Fall back to first short string arg
      const first = Object.values(args).find(v => typeof v === 'string' && v.length < 200)
      return { title: tool, subtitle: typeof first === 'string' ? first : undefined }
    }
  }
}

function ToolCallCard({
  msg,
  onToggle,
  onCopy,
  copied,
}: {
  msg: ChatMessage
  onToggle: () => void
  onCopy: (text: string) => void
  copied: boolean
}) {
  const kind = toolKind(msg.tool)
  const { title, subtitle } = describeToolCall(msg)
  const status = msg.status || 'running'
  const logs = msg.logs || []
  const hasOutput = !!msg.output && msg.output.trim().length > 0
  const imageUrls = generatedImageUrls(msg.output)
  const collapsed = msg.collapsed ?? false
  const hasBody = logs.length > 0 || hasOutput || imageUrls.length > 0

  const accent =
    kind === 'shell' ? 'border-cyan-700/50 bg-cyan-950/20'
    : kind === 'write' ? 'border-amber-700/50 bg-amber-950/20'
    : kind === 'read' ? 'border-emerald-700/50 bg-emerald-950/20'
    : 'border-gray-700/50 bg-gray-900/40'

  const titleColor =
    kind === 'shell' ? 'text-cyan-300'
    : kind === 'write' ? 'text-amber-300'
    : kind === 'read' ? 'text-emerald-300'
    : 'text-gray-300'

  const Icon = kind === 'shell' ? Terminal : kind === 'write' || kind === 'read' ? FileText : Wrench

  return (
    <div className={`ml-11 max-w-4xl rounded-2xl border ${accent} overflow-hidden shadow-lg shadow-black/20 backdrop-blur-sm`}>
      {/* Header */}
      <div className="flex items-center gap-2 px-3 py-2">
        <button
          onClick={onToggle}
          disabled={!hasBody}
          className="text-gray-400 hover:text-white transition-colors disabled:opacity-40"
          aria-label={collapsed ? 'Expand' : 'Collapse'}
        >
          {!hasBody ? (
            <span className="w-3.5 inline-block" />
          ) : collapsed ? (
            <ChevronRight className="w-3.5 h-3.5" />
          ) : (
            <ChevronDown className="w-3.5 h-3.5" />
          )}
        </button>
        <Icon className={`w-3.5 h-3.5 ${titleColor} shrink-0`} />
        <span className={`text-xs font-medium ${titleColor}`}>{title}</span>
        {msg.server && (
          <span className="text-[10px] text-gray-500 font-mono">[{msg.server}]</span>
        )}
        {subtitle && (
          <span className="text-xs text-gray-300 font-mono truncate flex-1 min-w-0" title={subtitle}>
            {subtitle}
          </span>
        )}
        <div className="flex items-center gap-1 ml-auto shrink-0">
          {status === 'running' && (
            <span className="flex items-center gap-1 text-[10px] text-cyan-400">
              <Loader2 className="w-3 h-3 animate-spin" />
              <span>running</span>
            </span>
          )}
          {status === 'done' && (
            <span className="flex items-center gap-1 text-[10px] text-green-400">
              <CheckCircle2 className="w-3 h-3" />
              <span>done</span>
            </span>
          )}
          {status === 'error' && (
            <span className="flex items-center gap-1 text-[10px] text-red-400">
              <AlertCircle className="w-3 h-3" />
              <span>error</span>
            </span>
          )}
          {subtitle && (
            <button
              onClick={() => onCopy(subtitle)}
              className="ml-1 text-gray-500 hover:text-white transition-colors"
              title="Salin command"
            >
              {copied ? <Check className="w-3 h-3 text-green-400" /> : <Copy className="w-3 h-3" />}
            </button>
          )}
        </div>
      </div>

      {/* Body — terminal-style stream + final result */}
      {!collapsed && hasBody && (
        <div className="border-t border-gray-800 bg-black/40 px-3 py-2 max-h-72 overflow-y-auto font-mono text-[11px] leading-snug">
          {logs.length > 0 && (
            <pre className="whitespace-pre-wrap text-gray-200">
              {logs.join('\n')}
            </pre>
          )}
          {hasOutput && logs.length === 0 && (
            <pre className="whitespace-pre-wrap text-gray-300">
              {(msg.output || '').slice(0, 4000)}
              {(msg.output || '').length > 4000 && (
                <span className="text-gray-500">{`\n[... ${(msg.output || '').length - 4000} chars truncated ...]`}</span>
              )}
            </pre>
          )}
          {hasOutput && logs.length > 0 && msg.output && msg.output.trim() !== logs.join('\n').trim() && (
            <details className="mt-2 text-gray-400">
              <summary className="cursor-pointer text-[10px] text-gray-500 hover:text-gray-300">
                full result ({msg.output.length} chars)
              </summary>
              <pre className="mt-1 whitespace-pre-wrap">{msg.output}</pre>
            </details>
          )}
          {imageUrls.length > 0 && (
            <div className="mt-3 grid gap-2">
              {imageUrls.map(url => (
                <img key={url} src={url} alt="generated image" className="max-h-72 rounded-lg border border-gray-700/70 object-contain shadow-lg shadow-black/30" />
              ))}
            </div>
          )}
        </div>
      )}
    </div>
  )
}

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

// Per-message size caps. Persisting raw data (image base64 previews, full
// command output, multi-thousand-line logs) blows up the localStorage quota
// and crashes the SPA on the next page load. We slim heavy fields here.
const MAX_MESSAGES_PER_SESSION = 200
const MAX_SESSIONS = 20
const MAX_OUTPUT_CHARS = 4000
const MAX_LOG_LINES = 50
// Runtime caps keep the live React tree from growing forever during long
// terminal/tool streams. Persistence caps alone are not enough because the
// lag happens before data is written to localStorage.
const MAX_RUNTIME_MESSAGES = 120
const MAX_RUNTIME_OUTPUT_CHARS = 50_000
const MAX_RUNTIME_LOG_LINES = 300
// Hard ceiling on a single serialized session. If a session would exceed
// this, we keep only the most recent messages until it fits.
const MAX_SESSION_BYTES = 800 * 1024

// slimMessage strips fields that don't need to survive a refresh.
// The runtime versions in React state still hold the full data.
function slimMessage(m: ChatMessage): ChatMessage {
  const out: ChatMessage = {
    ...m,
    timestamp: m.timestamp instanceof Date ? m.timestamp.toISOString() : m.timestamp,
  }
  if (out.attachments && out.attachments.length > 0) {
    out.attachments = out.attachments.map(a => ({
      path: a.path,
      size: a.size,
      kind: a.kind,
      name: a.name,
      // Drop the base64 preview — it can be hundreds of KB per image.
    }))
  }
  if (out.output && out.output.length > MAX_OUTPUT_CHARS) {
    out.output = out.output.slice(0, MAX_OUTPUT_CHARS) + `\n[... ${out.output.length - MAX_OUTPUT_CHARS} chars truncated for storage ...]`
  }
  if (out.logs && out.logs.length > MAX_LOG_LINES) {
    const dropped = out.logs.length - MAX_LOG_LINES
    out.logs = [`[... ${dropped} earlier lines truncated for storage ...]`, ...out.logs.slice(-MAX_LOG_LINES)]
  }
  return out
}

function capRuntimeMessages(messages: ChatMessage[]): ChatMessage[] {
  if (messages.length <= MAX_RUNTIME_MESSAGES) return messages
  return messages.slice(-MAX_RUNTIME_MESSAGES)
}

function capRuntimeOutput(output?: string): string | undefined {
  if (!output || output.length <= MAX_RUNTIME_OUTPUT_CHARS) return output
  return output.slice(-MAX_RUNTIME_OUTPUT_CHARS)
}

function capRuntimeLogs(logs: string[]): string[] {
  if (logs.length <= MAX_RUNTIME_LOG_LINES) return logs
  const dropped = logs.length - MAX_RUNTIME_LOG_LINES
  return [`[... ${dropped} earlier live lines truncated ...]`, ...logs.slice(-MAX_RUNTIME_LOG_LINES)]
}

function rawSetItem(key: string, value: string): boolean {
  try {
    localStorage.setItem(key, value)
    return true
  } catch {
    return false
  }
}

// setItemSafe writes to localStorage with progressive fallback when the
// browser quota fills up. Strategy:
//   1. Try the write. If it succeeds, done.
//   2. If quota is exceeded, drop the oldest session and retry.
//   3. Repeat until either it fits or we have only the current session left.
//   4. As a last resort, clear our keys and warn — never crash the SPA.
function setItemSafe(key: string, value: string): boolean {
  if (rawSetItem(key, value)) return true

  // Drop oldest sessions one at a time and retry.
  for (let attempt = 0; attempt < 30; attempt++) {
    let sessions: ChatSession[] = []
    try {
      const raw = localStorage.getItem(SESSION_META_KEY)
      if (raw) sessions = JSON.parse(raw)
    } catch {
      sessions = []
    }
    if (sessions.length <= 1) break
    sessions.sort((a, b) => new Date(b.updatedAt).getTime() - new Date(a.updatedAt).getTime())
    sessions.pop() // drop the oldest
    if (!rawSetItem(SESSION_META_KEY, JSON.stringify(sessions))) {
      // The metadata write itself failed — delete it outright.
      try { localStorage.removeItem(SESSION_META_KEY) } catch { /* ignore */ }
    }
    if (rawSetItem(key, value)) return true
  }

  // Last resort: nuke our keys and accept losing chat history rather
  // than crash the React tree.
  try { localStorage.removeItem(SESSION_META_KEY) } catch { /* ignore */ }
  try { localStorage.removeItem(CURRENT_SESSION_KEY) } catch { /* ignore */ }
  if (rawSetItem(key, value)) return true

  console.warn('[smara] localStorage penuh — tidak bisa menyimpan riwayat. Buka /?reset untuk membersihkan.')
  return false
}

// trimSessionToFit aggressively drops oldest messages from a session until
// the serialized form is below MAX_SESSION_BYTES. Guarantees we don't try
// to persist a single session that's larger than the entire quota.
function trimSessionToFit(s: ChatSession): ChatSession {
  let messages = s.messages
  let serialized = JSON.stringify({ ...s, messages })
  while (serialized.length > MAX_SESSION_BYTES && messages.length > 5) {
    // Drop in chunks of 10 to make this fast for very large sessions.
    const drop = Math.min(10, Math.max(1, messages.length - 5))
    messages = messages.slice(drop)
    serialized = JSON.stringify({ ...s, messages })
  }
  return { ...s, messages }
}

function saveAllSessions(sessions: ChatSession[]) {
  // Cap session count and trim each session before serializing so we don't
  // try to persist a 5 MB blob into a 5 MB quota.
  const capped = sessions.slice(0, MAX_SESSIONS).map(trimSessionToFit)
  setItemSafe(SESSION_META_KEY, JSON.stringify(capped))
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
  setItemSafe(CURRENT_SESSION_KEY, id)
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
    setItemSafe(CURRENT_SESSION_KEY, sessions[0].id)
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
  // Cap to the most recent N messages and slim each before persisting.
  const slim = messages.slice(-MAX_MESSAGES_PER_SESSION).map(slimMessage)
  const updated: ChatSession = {
    id,
    name: idx >= 0 ? sessions[idx].name : ('Chat ' + new Date().toLocaleString('id-ID', { hour: '2-digit', minute: '2-digit', day: 'numeric', month: 'short' })),
    messages: slim.map(m => ({
      ...m,
      timestamp: m.timestamp instanceof Date ? m.timestamp.toISOString() : m.timestamp,
    })),
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
  const [statusStats, setStatusStats] = useState<{ prompts: number; inputTokens: number; outputTokens: number; tokens: number; duration: string; cost: number } | null>(null)
  const [activePhases, setActivePhases] = useState<Array<{ phase: string; description: string; status: 'running' | 'done' }>>([])
  const [attachments, setAttachments] = useState<Attachment[]>([])
  const [uploading, setUploading] = useState(false)
  const [copiedIdx, setCopiedIdx] = useState<number | null>(null)
  const [toast, setToast] = useState<string | null>(null)
  const [dragOver, setDragOver] = useState(false)
  const fileInputRef = useRef<HTMLInputElement>(null)
  const dragCounterRef = useRef(0)
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
    setItemSafe(CURRENT_SESSION_KEY, s.id)
    setMessages(capRuntimeMessages(s.messages.map(m => ({ ...m, timestamp: new Date(m.timestamp) }))))
  }

  useEffect(() => {
    bottomRef.current?.scrollIntoView({ behavior: 'smooth' })
  }, [messages, thinking])

  // Persist messages to localStorage whenever they change.
  // Wrap in try/catch so a stray quota error doesn't unmount the tree.
  useEffect(() => {
    try {
      saveSession(sessionId, messages)
      setSessions(getAllSessions())
    } catch (err) {
      console.warn('[smara] persist session gagal:', err)
    }
  }, [messages, sessionId])

  const newSession = () => {
    saveSession(sessionId, messages)
    const s = createSession()
    setSessions(getAllSessions())
    setCurrentRaw(s)
    setSessionId(s.id)
    setMessages(capRuntimeMessages(s.messages.map(m => ({ ...m, timestamp: new Date(m.timestamp) }))))
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
        setItemSafe(CURRENT_SESSION_KEY, next.id)
        setMessages(capRuntimeMessages(next.messages.map(m => ({ ...m, timestamp: new Date(m.timestamp) }))))
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
            if (!spinnerTimer.current) {
              spinnerTimer.current = setInterval(() => {
                setSpinnerIdx(i => (i + 1) % spinnerFrames.length)
              }, 80)
            }
          } else {
            if (spinnerTimer.current) clearInterval(spinnerTimer.current)
            spinnerTimer.current = null
          }
          break
        case 'chat':
          setThinking(false)
          if (spinnerTimer.current) { clearInterval(spinnerTimer.current); spinnerTimer.current = null }
          setActivePhases([])
          setMessages(prev => capRuntimeMessages([...prev, { role: 'assistant', content: msg.payload, timestamp: new Date() }]))
          break
        case 'error':
          setThinking(false)
          if (spinnerTimer.current) { clearInterval(spinnerTimer.current); spinnerTimer.current = null }
          setActivePhases([])
          setMessages(prev => capRuntimeMessages([...prev, { role: 'error', content: msg.payload, timestamp: new Date() }]))
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
            return next.slice(-12)
          })
          break
        case 'tool_call':
          setMessages(prev => capRuntimeMessages([...prev, {
            role: 'tool_call',
            content: msg.tool || 'tool',
            tool: msg.tool,
            server: msg.server,
            args: msg.args,
            logs: [],
            status: 'running',
            collapsed: false,
            timestamp: new Date(),
          }]))
          break
        case 'tool_result':
          setMessages(prev => {
            // Attach result to the most recent running tool_call card.
            const next = [...prev]
            for (let i = next.length - 1; i >= 0; i--) {
              if (next[i].role === 'tool_call' && next[i].status === 'running') {
                next[i] = {
                  ...next[i],
                  output: capRuntimeOutput(msg.output),
                  status: 'done',
                  // Auto-collapse cards with bulky output to reduce noise.
                  collapsed: (next[i].logs?.length || 0) > 6,
                }
                return capRuntimeMessages(next)
              }
            }
            // No open card — surface as a standalone block (rare).
            next.push({ role: 'tool_result', content: '', output: capRuntimeOutput(msg.output), timestamp: new Date() })
            return capRuntimeMessages(next)
          })
          break
        case 'log':
          setMessages(prev => {
            const next = [...prev]
            // Terminal logs from run_command stream into the most recent
            // running tool_call card. Other logs (system / explore) stay
            // as standalone rows.
            if ((msg.role === 'Terminal' || msg.role === 'terminal') && msg.payload !== undefined) {
              for (let i = next.length - 1; i >= 0; i--) {
                if (next[i].role === 'tool_call' && next[i].status === 'running') {
                  const logs = capRuntimeLogs([...(next[i].logs || []), msg.payload])
                  next[i] = { ...next[i], logs }
                  return capRuntimeMessages(next)
                }
              }
            }
            next.push({ role: 'log', content: msg.payload, timestamp: new Date() })
            return capRuntimeMessages(next)
          })
          break
        case 'mode':
          setMode(msg.mode || 'ask')
          break
        case 'stats':
          if (msg.stats) {
            setStatusStats({
              prompts: msg.stats.prompt_count,
              inputTokens: msg.stats.input_tokens || 0,
              outputTokens: msg.stats.output_tokens || 0,
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
      if (spinnerTimer.current) clearInterval(spinnerTimer.current)
      spinnerTimer.current = null
      wsRef.current?.close()
    }
  }, [connectWs])

  const send = useCallback(() => {
    const text = input.trim()
    if (!text && attachments.length === 0) return

    // Inject [image:/path] or [file:/path] tokens — same convention used by
    // the TUI Ctrl+V flow and the Telegram/Discord adapters. The web backend
    // tacks on a steer hint for the agent to call analyze_image / read_file.
    const refs = attachments
      .map(a => `[${a.kind}:${a.path}]`)
      .join(' ')
    const messageText = [refs, text].filter(Boolean).join(' ')

    if (!wsRef.current || wsRef.current.readyState !== WebSocket.OPEN) {
      setMessages(prev => capRuntimeMessages([...prev, {
        role: 'user',
        content: messageText,
        timestamp: new Date(),
        attachments: attachments.map(a => ({ path: a.path, size: a.size, kind: a.kind, name: a.name, preview: a.preview })),
      }]))
      setInput('')
      setAttachments([])
      connectWs()
      return
    }

    setMessages(prev => capRuntimeMessages([...prev, {
      role: 'user',
      content: messageText,
      timestamp: new Date(),
      attachments: attachments.map(a => ({ path: a.path, size: a.size, kind: a.kind, name: a.name, preview: a.preview })),
    }]))
    setInput('')
    setAttachments([])
    wsRef.current.send(JSON.stringify({ type: 'chat', payload: messageText, mode }))
    setThinking(true)
  }, [input, attachments, connectWs, mode])

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault()
      send()
    }
  }

  const showToast = (msg: string) => {
    setToast(msg)
    window.setTimeout(() => setToast(null), 2500)
  }

  const uploadFile = useCallback(async (file: File) => {
    setUploading(true)
    try {
      if (file.type.startsWith('image/')) {
        // Read as dataURL for inline preview AND for the lightweight
        // /api/clipboard/upload endpoint (handles base64 directly).
        const dataUrl = await new Promise<string>((resolve, reject) => {
          const reader = new FileReader()
          reader.onload = () => resolve(String(reader.result))
          reader.onerror = () => reject(reader.error)
          reader.readAsDataURL(file)
        })
        const res = await uploadClipboardImage(dataUrl)
        setAttachments(prev => [...prev, {
          path: res.path,
          size: res.size,
          kind: 'image',
          name: file.name || 'pasted-image.png',
          preview: dataUrl,
          mime: file.type,
        }])
        showToast(`📎 ${(res.size / 1024).toFixed(0)} KB → ${res.path.split('/').pop()}`)
        return
      }
      // Documents and everything else go through multipart.
      const res = await uploadAttachment(file)
      setAttachments(prev => [...prev, {
        path: res.path,
        size: res.size,
        kind: res.kind,
        name: res.name || file.name,
        mime: res.mime || file.type,
      }])
      showToast(`📎 ${(res.size / 1024).toFixed(0)} KB → ${res.path.split('/').pop()}`)
    } catch (err) {
      showToast(`Upload gagal: ${err instanceof Error ? err.message : 'unknown'}`)
    } finally {
      setUploading(false)
    }
  }, [])

  const uploadFiles = useCallback(async (files: FileList | File[]) => {
    for (const f of Array.from(files)) {
      await uploadFile(f)
    }
  }, [uploadFile])

  const handlePaste = useCallback(async (e: React.ClipboardEvent) => {
    const items = Array.from(e.clipboardData?.items || [])
    const fileItems = items.filter(it => it.kind === 'file')
    if (fileItems.length === 0) return // fall through to default text paste
    e.preventDefault()
    const files: File[] = []
    for (const it of fileItems) {
      const f = it.getAsFile()
      if (f) files.push(f)
    }
    if (files.length > 0) await uploadFiles(files)
  }, [uploadFiles])

  const handleFilePick = (e: React.ChangeEvent<HTMLInputElement>) => {
    const files = e.target.files
    if (files && files.length > 0) uploadFiles(files)
    if (fileInputRef.current) fileInputRef.current.value = ''
  }

  // Drag-drop handlers — counter pattern avoids flicker on child enter/leave.
  const handleDragEnter = (e: React.DragEvent) => {
    if (!Array.from(e.dataTransfer.types || []).includes('Files')) return
    e.preventDefault()
    dragCounterRef.current += 1
    setDragOver(true)
  }
  const handleDragLeave = (e: React.DragEvent) => {
    e.preventDefault()
    dragCounterRef.current -= 1
    if (dragCounterRef.current <= 0) {
      dragCounterRef.current = 0
      setDragOver(false)
    }
  }
  const handleDragOver = (e: React.DragEvent) => {
    if (!Array.from(e.dataTransfer.types || []).includes('Files')) return
    e.preventDefault()
  }
  const handleDrop = async (e: React.DragEvent) => {
    e.preventDefault()
    dragCounterRef.current = 0
    setDragOver(false)
    const files = e.dataTransfer.files
    if (files && files.length > 0) await uploadFiles(files)
  }

  const removeAttachment = (idx: number) => {
    setAttachments(prev => prev.filter((_, i) => i !== idx))
  }

  const copyMessage = async (idx: number, text: string) => {
    try {
      await navigator.clipboard.writeText(text)
      setCopiedIdx(idx)
      showToast('Pesan disalin')
      window.setTimeout(() => setCopiedIdx(c => (c === idx ? null : c)), 1500)
    } catch {
      showToast('Browser menolak akses clipboard')
    }
  }

  const toggleCard = (idx: number) => {
    setMessages(prev => {
      const next = [...prev]
      if (next[idx]) {
        next[idx] = { ...next[idx], collapsed: !next[idx].collapsed }
      }
      return next
    })
  }

  return (
    <div
      className="flex flex-col h-full relative"
      onDragEnter={handleDragEnter}
      onDragOver={handleDragOver}
      onDragLeave={handleDragLeave}
      onDrop={handleDrop}
    >
      {/* Header */}
      <div className="h-16 border-b border-white/10 flex items-center justify-between px-5 bg-gradient-to-r from-gray-950/75 via-gray-900/55 to-smara-950/35 backdrop-blur-xl shadow-lg shadow-black/15">
        <div className="flex items-center gap-3 min-w-0">
          <div className="relative shrink-0">
            <div className="absolute inset-0 rounded-2xl bg-cyan-400/30 blur-md" />
            <div className="relative w-10 h-10 rounded-2xl bg-gradient-to-br from-cyan-400 via-smara-500 to-fuchsia-500 flex items-center justify-center shadow-lg shadow-cyan-950/40 ring-1 ring-white/15">
              <Bot className="w-5 h-5 text-white" />
            </div>
          </div>
          <button
            onClick={() => setShowSessions(!showSessions)}
            className="group flex items-center gap-2 min-w-0 max-w-[360px] px-3 py-1.5 rounded-lg border border-gray-700/70 bg-gray-950/50 hover:border-smara-500/60 hover:bg-smara-950/30 transition-colors"
            title="Pilih sesi chat"
          >
            <MessageSquare className="w-4 h-4 text-gray-500 group-hover:text-smara-300 shrink-0" />
            <span className="font-medium truncate group-hover:text-smara-200">{current.name}</span>
            <span className="text-[10px] text-gray-500 shrink-0">{sessions.length} sesi</span>
            <ChevronDown className={`w-4 h-4 text-gray-500 transition-transform shrink-0 ${showSessions ? 'rotate-180 text-smara-300' : ''}`} />
          </button>
          <span className="hidden md:inline text-[10px] text-gray-500 font-mono truncate max-w-[180px]">{sessionId}</span>
        </div>
        <div className="flex items-center gap-3">
          <button
            onClick={newSession}
            className="flex items-center gap-1.5 px-3 py-2 text-xs font-medium bg-gradient-to-r from-smara-600 to-cyan-600 hover:from-smara-500 hover:to-cyan-500 rounded-xl transition-all shadow-lg shadow-smara-950/30 border border-white/10"
          >
            <Plus className="w-3 h-3" /> Sesi Baru
          </button>
          {!connected && (
            <button onClick={connectWs} className="flex items-center gap-1 text-xs text-smara-400 hover:text-smara-300 transition-colors">
              <RefreshCw className="w-3 h-3" /> Reconnect
            </button>
          )}
          <span className={`inline-flex items-center gap-1.5 rounded-full border px-2.5 py-1 text-xs ${connected ? 'border-emerald-400/20 bg-emerald-500/10 text-emerald-300' : 'border-red-400/20 bg-red-500/10 text-red-300'}`}>
            <span className={`w-1.5 h-1.5 rounded-full ${connected ? 'bg-emerald-400 shadow-[0_0_10px_rgba(52,211,153,.9)]' : 'bg-red-400'}`} />
            {connected ? 'Online' : 'Offline'}
          </span>
        </div>
      </div>

      {/* Session dropdown */}
      {showSessions && (
        <div className="absolute top-14 left-0 right-0 md:left-64 z-50 border-b border-gray-800 bg-gray-950/95 backdrop-blur-xl shadow-2xl shadow-black/40">
          <div className="mx-auto max-w-5xl p-3">
            <div className="flex items-center justify-between mb-3 px-1">
              <div>
                <div className="text-xs text-gray-400 font-semibold uppercase tracking-wider">Sesi dari Smara Web</div>
                <div className="text-[11px] text-gray-600">Pilih sesi untuk ditampilkan di chat</div>
              </div>
              <button onClick={() => setShowSessions(false)} className="text-xs text-gray-500 hover:text-gray-300 px-2 py-1 rounded hover:bg-gray-800">Tutup</button>
            </div>
            <div className="grid gap-2 max-h-80 overflow-y-auto pr-1">
              {sessions.length === 0 && (
                <div className="rounded-xl border border-gray-800 bg-gray-900/60 p-4 text-sm text-gray-500">Belum ada sesi tersimpan.</div>
              )}
              {sessions.map(s => {
                const last = s.messages[s.messages.length - 1]
                return (
                  <div
                    key={s.id}
                    onClick={() => { setCurrent(s); setShowSessions(false); }}
                    className={`group flex items-center justify-between gap-3 p-3 rounded-xl cursor-pointer text-sm border transition-all ${
                      s.id === current.id
                        ? 'bg-smara-900/35 border-smara-500/40 shadow-sm shadow-smara-950/40'
                        : 'bg-gray-900/60 border-gray-800 hover:bg-gray-800 hover:border-gray-700'
                    }`}
                  >
                    <div className="flex items-start gap-3 min-w-0">
                      <div className={`mt-0.5 rounded-lg p-2 ${s.id === current.id ? 'bg-smara-500/15 text-smara-300' : 'bg-gray-800 text-gray-500 group-hover:text-gray-300'}`}>
                        <MessageSquare className="w-4 h-4" />
                      </div>
                      <div className="min-w-0">
                        <div className="flex items-center gap-2 min-w-0">
                          <span className="font-medium text-gray-100 truncate max-w-[360px]">{s.name}</span>
                          {s.id === current.id && <span className="text-[10px] px-1.5 py-0.5 rounded-full bg-smara-500/15 text-smara-200 border border-smara-500/20">aktif</span>}
                        </div>
                        <div className="mt-1 text-xs text-gray-500 truncate max-w-[520px]">
                          {last?.content ? last.content : 'Sesi kosong'}
                        </div>
                      </div>
                    </div>
                    <div className="flex items-center gap-3 shrink-0">
                      <div className="hidden sm:flex flex-col items-end text-[10px] text-gray-600">
                        <span>{s.messages.length} pesan</span>
                        <span className="flex items-center gap-1"><Clock className="w-3 h-3" /> {new Date(s.updatedAt).toLocaleString('id-ID', { day: '2-digit', month: 'short', hour: '2-digit', minute: '2-digit' })}</span>
                      </div>
                      {sessions.length > 1 && (
                        <button
                          onClick={(e) => { e.stopPropagation(); deleteSession(s.id); }}
                          className="text-gray-600 hover:text-red-400 transition-colors p-1.5 rounded hover:bg-red-950/30"
                          title="Hapus sesi"
                        >
                          <Trash2 className="w-4 h-4" />
                        </button>
                      )}
                    </div>
                  </div>
                )
              })}
            </div>
          </div>
        </div>
      )}

      {/* Messages */}
      <div className="flex-1 overflow-y-auto px-4 py-5 md:px-7 md:py-6 space-y-4 bg-[radial-gradient(circle_at_50%_0%,rgba(34,211,238,0.08),transparent_32%),linear-gradient(180deg,rgba(255,255,255,0.02),transparent)]">
        {messages.map((msg, i) => {
          if (msg.role === 'tool_call') {
            return (
              <ToolCallCard
                key={i}
                msg={msg}
                onToggle={() => toggleCard(i)}
                onCopy={(text) => copyMessage(i, text)}
                copied={copiedIdx === i}
              />
            )
          }
          if (msg.role === 'tool_result') {
            // Standalone tool_result (no preceding open card) — render as a
            // compact muted block. The common case is rendered inside the
            // ToolCallCard above.
            return (
              <div key={i} className="ml-11 px-3 py-2 bg-gray-900/40 border border-gray-800 rounded text-xs text-gray-400 font-mono whitespace-pre-wrap max-h-40 overflow-y-auto">
                {msg.output || msg.content}
              </div>
            )
          }
          if (msg.role === 'log') {
            return (
              <div key={i} className="flex items-center gap-2 text-xs text-gray-500 px-2 ml-11">
                <span className="text-gray-600">&#9654;</span>
                <span>{msg.content}</span>
              </div>
            )
          }
          return (
            <div key={i} className={`flex gap-3 group ${msg.role === 'user' ? 'flex-row-reverse' : ''}`}>
              <div className={`w-10 h-10 rounded-2xl flex items-center justify-center shrink-0 shadow-lg ring-1 ${
                msg.role === 'user'
                  ? 'bg-gradient-to-br from-smara-600 to-smara-800 ring-smara-400/30 shadow-smara-950/40'
                  : msg.role === 'error'
                  ? 'bg-gradient-to-br from-red-700 to-red-950 ring-red-400/30 shadow-red-950/40'
                  : 'bg-gradient-to-br from-gray-700 via-gray-800 to-smara-950 ring-smara-400/20 shadow-smara-950/40'
              }`}>
                {msg.role === 'user' ? <User className="w-4 h-4" /> : <Bot className="w-4 h-4 text-smara-200" />}
              </div>
              <div className={`max-w-[88%] md:max-w-[78%] rounded-[1.35rem] px-4 py-3 text-sm leading-relaxed relative shadow-2xl backdrop-blur-md overflow-hidden transition-all duration-200 group-hover:-translate-y-0.5 ${
                msg.role === 'user'
                  ? 'bg-gradient-to-br from-cyan-600/30 via-smara-700/35 to-fuchsia-900/25 border border-cyan-300/25 shadow-cyan-950/25 ring-1 ring-white/10'
                  : msg.role === 'error'
                  ? 'bg-gradient-to-br from-red-950/70 to-gray-950/60 border border-red-600/50 text-red-100 shadow-red-950/20'
                  : 'bg-gradient-to-br from-white/[0.08] via-gray-900/85 to-smara-950/35 border border-white/10 shadow-black/30 ring-1 ring-white/[0.04] before:absolute before:inset-x-0 before:top-0 before:h-px before:bg-gradient-to-r before:from-transparent before:via-cyan-200/60 before:to-transparent'
              }`}>
                {msg.role !== 'user' && msg.role !== 'error' && (
                  <div className="mb-2 flex items-center gap-2 text-[10px] font-semibold uppercase tracking-[0.12em] text-smara-300/90">
                    <span className="h-1.5 w-1.5 rounded-full bg-smara-300 shadow-[0_0_12px_rgba(45,212,191,0.8)]" />
                    Smara Response
                  </div>
                )}
                {msg.attachments && msg.attachments.length > 0 && (
                  <div className="flex flex-wrap gap-2 mb-3">
                    {msg.attachments.map((att, ai) => {
                      const Icon = attachmentIcon(att.name || att.path, undefined)
                      return (
                        <div key={ai} className="relative group/att">
                          {att.kind === 'image' && att.preview ? (
                            <img src={att.preview} alt={att.path} className="max-h-32 rounded-xl border border-gray-700/70 shadow-lg" />
                          ) : (
                            <div className="flex items-center gap-2 px-3 py-2 bg-gray-950/60 border border-gray-700/70 rounded-xl shadow-inner">
                              <Icon className="w-5 h-5 text-smara-300 shrink-0" />
                              <div className="flex flex-col min-w-0">
                                <span className="text-xs text-gray-200 truncate max-w-[200px]">{att.name || att.path.split('/').pop()}</span>
                                <span className="text-[10px] text-gray-500 font-mono">{(att.size / 1024).toFixed(0)} KB</span>
                              </div>
                            </div>
                          )}
                          {att.kind === 'image' && (
                            <div className="text-[10px] text-gray-500 mt-0.5 font-mono truncate max-w-[200px]">{(att.size / 1024).toFixed(0)} KB</div>
                          )}
                        </div>
                      )
                    })}
                  </div>
                )}
                {msg.role === 'user' ? (
                  <div className="whitespace-pre-wrap text-gray-100 leading-6">{msg.content}</div>
                ) : msg.role === 'error' ? (
                  <div className="whitespace-pre-wrap leading-6">{msg.content}</div>
                ) : (
                  <SmaraMarkdown content={msg.content} />
                )}
                <div className="mt-2 flex justify-end border-t border-white/5 pt-1.5 text-[10px] text-gray-500">
                  {new Date(msg.timestamp).toLocaleTimeString()}
                </div>
                <button
                  onClick={() => copyMessage(i, msg.content)}
                  title="Salin pesan"
                  aria-label="Salin pesan"
                  className={`absolute top-2 ${msg.role === 'user' ? 'left-2' : 'right-2'} z-10 inline-flex h-8 w-8 items-center justify-center rounded-xl border border-white/10 bg-gray-950/90 text-gray-300 shadow-lg shadow-black/30 backdrop-blur-md opacity-80 transition-all hover:border-smara-400 hover:bg-gray-900 hover:text-white hover:opacity-100 group-hover:opacity-100 md:opacity-0 md:group-hover:opacity-100`}
                >
                  {copiedIdx === i ? <Check className="w-3.5 h-3.5 text-green-400" /> : <Copy className="w-3.5 h-3.5" />}
                </button>
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
          <span>in={statusStats.inputTokens}</span>
          <span>out={statusStats.outputTokens}</span>
          <span>dur={statusStats.duration}</span>
          <span>cost=${statusStats.cost.toFixed(4)}</span>
        </div>
      )}

      {/* Mode switcher + Input */}
      <div className="p-4 border-t border-white/10 bg-gray-950/70 backdrop-blur-xl space-y-3 shadow-[0_-18px_40px_rgba(0,0,0,0.25)]">
        <div className="flex flex-wrap gap-1.5 rounded-2xl border border-white/10 bg-black/20 p-1.5 w-fit">
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
                className={`flex items-center gap-1.5 px-3 py-1.5 text-xs rounded-xl transition-all ${
                  active
                    ? `${m.bg} text-white border ${m.border} shadow-lg shadow-black/20`
                    : 'bg-white/[0.04] text-gray-400 hover:bg-white/[0.08] hover:text-gray-200 border border-transparent'
                }`}
                title={m.label}
              >
                <Icon className="w-3 h-3" />
                <span className="hidden sm:inline">{m.label}</span>
              </button>
            )
          })}
        </div>
        {attachments.length > 0 && (
          <div className="flex flex-wrap gap-2">
            {attachments.map((att, i) => {
              const Icon = attachmentIcon(att.name, att.mime)
              return (
                <div key={i} className="relative group inline-flex items-center gap-2 pl-1 pr-2 py-1 bg-gray-800/80 border border-gray-700 rounded">
                  {att.kind === 'image' && att.preview ? (
                    <img src={att.preview} alt="" className="h-8 w-8 object-cover rounded" />
                  ) : (
                    <div className="h-8 w-8 flex items-center justify-center bg-gray-900/60 rounded">
                      <Icon className="w-4 h-4 text-smara-300" />
                    </div>
                  )}
                  <span className="text-xs text-gray-300 font-mono truncate max-w-[160px]" title={att.path}>
                    {att.name}
                  </span>
                  <span className="text-[10px] text-gray-500">{(att.size / 1024).toFixed(0)} KB</span>
                  <button
                    onClick={() => removeAttachment(i)}
                    className="ml-1 text-gray-500 hover:text-red-400 transition-colors"
                    title="Hapus lampiran"
                  >
                    <X className="w-3 h-3" />
                  </button>
                </div>
              )
            })}
          </div>
        )}
        <div className="flex gap-2 rounded-3xl border border-white/10 bg-black/25 p-2 shadow-inner shadow-black/30 focus-within:border-cyan-300/35 focus-within:ring-2 focus-within:ring-cyan-400/10 transition-all">
          <input
            ref={fileInputRef}
            type="file"
            multiple
            onChange={handleFilePick}
            className="hidden"
          />
          <button
            onClick={() => fileInputRef.current?.click()}
            disabled={uploading}
            title="Lampirkan file (gambar, PDF, dokumen, kode)"
            className="px-3 py-2 bg-white/[0.06] hover:bg-white/[0.1] border border-white/10 hover:border-smara-400/60 rounded-2xl transition-colors disabled:opacity-50"
          >
            <Paperclip className="w-4 h-4 text-gray-400" />
          </button>
          <textarea
            value={input}
            onChange={e => setInput(e.target.value)}
            onKeyDown={handleKeyDown}
            onPaste={handlePaste}
            placeholder={uploading ? 'Mengunggah...' : 'Ketik pesan... (Enter kirim · Ctrl+V paste · drop file untuk lampirkan)'}
            className="flex-1 bg-transparent border border-transparent rounded-2xl px-3 py-2 text-sm resize-none focus:outline-none min-h-[42px] max-h-[140px] placeholder:text-gray-500"
            rows={1}
          />
          <button
            onClick={send}
            disabled={(!input.trim() && attachments.length === 0) || thinking || uploading}
            className="px-4 py-2 bg-gradient-to-r from-smara-600 to-cyan-600 hover:from-smara-500 hover:to-cyan-500 disabled:opacity-50 disabled:cursor-not-allowed rounded-2xl transition-all shadow-lg shadow-cyan-950/30 border border-white/10"
          >
            <Send className="w-4 h-4" />
          </button>
        </div>
      </div>

      {dragOver && (
        <div className="absolute inset-0 z-50 flex items-center justify-center bg-smara-900/40 backdrop-blur-sm border-4 border-dashed border-smara-500 rounded-lg pointer-events-none">
          <div className="flex flex-col items-center gap-2 text-smara-200">
            <Upload className="w-12 h-12" />
            <span className="text-lg font-semibold">Drop file untuk lampirkan</span>
            <span className="text-xs text-smara-300/80">Gambar, PDF, dokumen, kode — max 25 MB</span>
          </div>
        </div>
      )}

      {toast && (
        <div className="fixed bottom-24 left-1/2 -translate-x-1/2 z-50 px-4 py-2 bg-gray-900 border border-gray-700 rounded-lg shadow-lg text-sm text-gray-200">
          {toast}
        </div>
      )}
    </div>
  )
}
