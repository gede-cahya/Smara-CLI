import { useState, useRef, useEffect, useCallback, forwardRef, useImperativeHandle } from 'react'
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
  Archive, ArchiveRestore, StopCircle, Pencil, Server,
} from 'lucide-react'
import type { ChatMessage, WebSessionItem, WebSessionStatus } from '../api'
import {
  uploadClipboardImage,
  uploadAttachment,
  fetchWebSessions,
  createWebSession,
  getWebSession,
  renameWebSession,
  deleteWebSession,
  archiveWebSession,
  unarchiveWebSession,
  cancelWebSession,
} from '../api'
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

function formatCostUSD(cost?: number): string {
  if (cost === undefined || cost === null || Number.isNaN(cost)) return 'n/a'
  if (cost === 0) return '$0.00'
  if (cost < 0.000001) return '<$0.000001'
  return `$${cost.toFixed(6)}`
}

const CHART_COLORS = ['#bef264', '#84cc16', '#65a30d', '#d9f99d', '#22c55e', '#facc15']

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
    <div className="my-3 overflow-x-auto rounded-2xl border border-[#223018]/75 bg-[#1d2718]/88 p-4 shadow-lg shadow-black/25">
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
      <div className="h-72 rounded-2xl border border-[#223018]/70 bg-[#1d2718]/84 p-3">
        <div className="mb-2 text-xs font-semibold uppercase tracking-wide text-smara-300">Auto Bar Chart</div>
        <ResponsiveContainer width="100%" height="88%">
          <BarChart data={data}><CartesianGrid strokeDasharray="3 3" stroke="#334155" /><XAxis dataKey={labelKey} stroke="#94a3b8" /><YAxis stroke="#94a3b8" /><Tooltip /><Bar dataKey={valueKey} fill="#84cc16" radius={[6, 6, 0, 0]} /></BarChart>
        </ResponsiveContainer>
      </div>
      <div className="h-72 rounded-2xl border border-[#223018]/70 bg-[#1d2718]/84 p-3">
        <div className="mb-2 text-xs font-semibold uppercase tracking-wide text-smara-300">Auto Line Chart</div>
        <ResponsiveContainer width="100%" height="88%">
          <LineChart data={data}><CartesianGrid strokeDasharray="3 3" stroke="#334155" /><XAxis dataKey={labelKey} stroke="#94a3b8" /><YAxis stroke="#94a3b8" /><Tooltip /><Line type="monotone" dataKey={valueKey} stroke="#bef264" strokeWidth={2} /></LineChart>
        </ResponsiveContainer>
      </div>
      {data.length <= 8 && (
        <div className="h-72 rounded-2xl border border-[#223018]/70 bg-[#1d2718]/84 p-3 lg:col-span-2">
          <div className="mb-2 text-xs font-semibold uppercase tracking-wide text-smara-300">Auto Pie Chart</div>
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
    <div className="my-3 overflow-hidden rounded-2xl border border-[#223018]/70 bg-[#1d2718]/84">
      <div className="border-b border-[#223018]/75 px-3 py-2 text-xs font-semibold uppercase tracking-wide text-smara-300">Auto Visual JSON</div>
      <div className="overflow-x-auto">
        <table className="min-w-full text-left text-xs text-gray-200">
          <thead className="bg-[#27331f]/90 text-gray-300"><tr>{columns.map(c => <th key={c} className="px-3 py-2 font-semibold">{c}</th>)}</tr></thead>
          <tbody>{rows.map((row: any, i: number) => <tr key={i} className="border-t border-[#223018]/75">{columns.map(c => <td key={c} className="px-3 py-2 font-mono">{String(row[c] ?? '')}</td>)}</tr>)}</tbody>
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
    return <code className="rounded border border-[#31421f]/60 bg-[#20291a]/78 px-1 py-0.5 text-[0.85em] text-smara-200 font-mono" {...props}>{children}</code>
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
      <div className="my-2 overflow-hidden rounded-xl border border-[#223018]/75 bg-[#1d2718]/92 shadow-inner shadow-black/25">
        <div className="flex items-center gap-2 border-b border-[#223018]/75 bg-[#27331f]/90 px-3 py-1.5">
          <span className="text-[10px] font-semibold uppercase tracking-wide text-smara-300">{language}</span>
          <button onClick={copy} className="ml-auto flex items-center gap-1 rounded-md border border-[#31421f]/60 bg-[#202b18]/95 px-2 py-1 text-[10px] text-gray-300 hover:border-smara-400 hover:text-white transition-colors" title={language.toLowerCase() === 'json' ? 'Copy JSON' : 'Copy code'}>
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
        p: ({ children }) => <p className="mb-2 last:mb-0 text-gray-50 leading-6">{children}</p>,
        strong: ({ children }) => <strong className="font-semibold text-white bg-smara-500/10 px-0.5 rounded">{children}</strong>,
        em: ({ children }) => <em className="text-smara-200 not-italic">{children}</em>,
        ul: ({ children }) => <ul className="my-2 space-y-1.5 pl-1">{children}</ul>,
        ol: ({ children }) => <ol className="my-2 space-y-1.5 pl-5 list-decimal marker:text-smara-400">{children}</ol>,
        li: ({ children }) => (
          <li className="relative pl-5 text-gray-100 leading-6 before:content-[''] before:absolute before:left-0 before:top-2.5 before:w-1.5 before:h-1.5 before:rounded-full before:bg-gradient-to-r before:from-smara-400 before:to-smara-300">
            {children}
          </li>
        ),
        code: MarkdownCodeBlock,
        pre: ({ children }) => <>{children}</>,
        table: ({ children }) => <div className="my-3 overflow-x-auto rounded-2xl border border-[#223018]/70 bg-[#1d2718]/84"><table className="min-w-full text-left text-sm text-gray-200">{children}</table></div>,
        thead: ({ children }) => <thead className="bg-[#27331f]/90 text-gray-100">{children}</thead>,
        tbody: ({ children }) => <tbody className="divide-y divide-[#31421f]/70">{children}</tbody>,
        tr: ({ children }) => <tr className="hover:bg-smara-500/5">{children}</tr>,
        th: ({ children }) => <th className="px-3 py-2 text-xs font-semibold uppercase tracking-wide text-smara-200">{children}</th>,
        td: ({ children }) => <td className="px-3 py-2 text-sm text-gray-200">{children}</td>,
        blockquote: ({ children }) => <blockquote className="my-2 rounded-xl border border-[#31421f]/60 bg-[#202b18]/58 px-3 py-2 text-gray-200">{children}</blockquote>,
        a: ({ children, href }) => <a href={href} target="_blank" rel="noreferrer" className="text-smara-300 underline decoration-smara-400/40 underline-offset-4 hover:text-smara-200">{children}</a>,
        img: ({ src, alt }) => <img src={src || ''} alt={alt || 'generated image'} className="my-2 max-h-96 rounded-xl border border-[#31421f]/60 shadow-lg shadow-black/30" />,
        h1: ({ children }) => <h1 className="mb-2 mt-1 text-xl font-bold text-white tracking-tight">{children}</h1>,
        h2: ({ children }) => <h2 className="mb-2 mt-3 text-lg font-semibold text-white tracking-tight">{children}</h2>,
        h3: ({ children }) => <h3 className="mb-1.5 mt-3 text-base font-semibold text-smara-100">{children}</h3>,
        hr: () => <hr className="my-3 border-[#31421f]/60" />,
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
    kind === 'shell' ? 'border-neutral-800/75 bg-[#202b18]/58'
    : kind === 'write' ? 'border-neutral-800/75 bg-[#202b18]/58'
    : kind === 'read' ? 'border-neutral-800/75 bg-[#202b18]/58'
    : 'border-neutral-800/75 bg-[#202b18]/58'

  const titleColor =
    kind === 'shell' ? 'text-smara-300'
    : kind === 'write' ? 'text-smara-200'
    : kind === 'read' ? 'text-smara-200'
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
          <span className="text-[10px] text-neutral-400 font-mono">[{msg.server}]</span>
        )}
        {subtitle && (
          <span className="text-xs text-gray-300 font-mono truncate flex-1 min-w-0" title={subtitle}>
            {subtitle}
          </span>
        )}
        <div className="flex items-center gap-1 ml-auto shrink-0">
          {status === 'running' && (
            <span className="flex items-center gap-1 text-[10px] text-smara-400">
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
              className="ml-1 text-neutral-400 hover:text-white transition-colors"
              title="Salin command"
            >
              {copied ? <Check className="w-3 h-3 text-green-400" /> : <Copy className="w-3 h-3" />}
            </button>
          )}
        </div>
      </div>

      {/* Body — terminal-style stream + final result */}
      {!collapsed && hasBody && (
        <div className="border-t border-[#223018]/75 bg-[#1a2314]/65 px-3 py-2 max-h-72 overflow-y-auto font-mono text-[11px] leading-snug">
          {logs.length > 0 && (
            <pre className="whitespace-pre-wrap text-gray-200">
              {logs.join('\n')}
            </pre>
          )}
          {hasOutput && logs.length === 0 && (
            <pre className="whitespace-pre-wrap text-gray-300">
              {(msg.output || '').slice(0, 4000)}
              {(msg.output || '').length > 4000 && (
                <span className="text-neutral-400">{`\n[... ${(msg.output || '').length - 4000} chars truncated ...]`}</span>
              )}
            </pre>
          )}
          {hasOutput && logs.length > 0 && msg.output && msg.output.trim() !== logs.join('\n').trim() && (
            <details className="mt-2 text-gray-400">
              <summary className="cursor-pointer text-[10px] text-neutral-400 hover:text-gray-300">
                full result ({msg.output.length} chars)
              </summary>
              <pre className="mt-1 whitespace-pre-wrap">{msg.output}</pre>
            </details>
          )}
          {imageUrls.length > 0 && (
            <div className="mt-3 grid gap-2">
              {imageUrls.map(url => (
                <img key={url} src={url} alt="generated image" className="max-h-72 rounded-lg border border-[#31421f]/60 object-contain shadow-lg shadow-black/30" />
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
  { id: 'ask', label: 'Ask', emoji: '\uD83D\uDCAC', icon: MessageCircle, bg: 'bg-smara-600', border: 'border-smara-500', text: 'text-smara-400' },
  { id: 'rush', label: 'Rush', emoji: '\u26A1', icon: Zap, bg: 'bg-yellow-600', border: 'border-yellow-500', text: 'text-yellow-400' },
  { id: 'plan', label: 'Plan', emoji: '\uD83D\uDCCB', icon: ClipboardList, bg: 'bg-lime-600', border: 'border-lime-500', text: 'text-lime-400' },
  { id: 'test', label: 'Test', emoji: '\uD83E\uDDEA', icon: FlaskConical, bg: 'bg-green-600', border: 'border-green-500', text: 'text-green-400' },
  { id: 'workflow', label: 'Workflow', emoji: '\uD83D\uDD04', icon: ArrowRightLeft, bg: 'bg-smara-600', border: 'border-smara-500', text: 'text-smara-400' },
]

const SESSION_META_KEY = 'smara_chat_sessions'
const CURRENT_SESSION_KEY = 'smara_current_session'


interface PlanQuest {
  title: string
  options: string[]
  allowCustom: boolean
}

function parsePlanQuest(content: string): { cleanContent: string; quest: PlanQuest | null } {
  const startToken = '[[SMARA_PLAN_QUEST]]'
  const endToken = '[[/SMARA_PLAN_QUEST]]'
  const start = content.indexOf(startToken)
  if (start < 0) return { cleanContent: content, quest: null }
  const afterStart = start + startToken.length
  const end = content.indexOf(endToken, afterStart)
  const rawBlock = end >= 0 ? content.slice(afterStart, end) : content.slice(afterStart)
  const blockWithNewlines = rawBlock
    .replace(/\s+(title:)/i, '\n$1')
    .replace(/\s+(options:)/i, '\n$1')
    .replace(/\s+(allow_custom:)/i, '\n$1')
    .replace(/\s+(-\s+)/g, '\n$1')

  const lines = blockWithNewlines.split(/\r?\n/).map(l => l.trim()).filter(Boolean)
  let title = 'Pilih salah satu opsi'
  const options: string[] = []
  let allowCustom = false
  let inOptions = false

  for (const line of lines) {
    const titleMatch = line.match(/^title\s*:\s*(.+)$/i)
    if (titleMatch) {
      title = titleMatch[1].trim()
      inOptions = false
      continue
    }
    if (/^options\s*:/i.test(line)) {
      inOptions = true
      const inline = line.replace(/^options\s*:\s*/i, '').trim()
      if (inline) {
        inline.split(/\s+-\s+/).map(x => x.replace(/^-\s*/, '').trim()).filter(Boolean).forEach(x => options.push(x))
      }
      continue
    }
    const allowMatch = line.match(/^allow_custom\s*:\s*(true|false|yes|no|1|0)$/i)
    if (allowMatch) {
      allowCustom = /^(true|yes|1)$/i.test(allowMatch[1])
      inOptions = false
      continue
    }
    if (inOptions) {
      const opt = line.replace(/^-\s*/, '').trim()
      if (opt && !/^[a-z_]+\s*:/i.test(opt)) options.push(opt)
    }
  }

  const rawFull = end >= 0 ? content.slice(start, end + endToken.length) : content.slice(start)
  const cleanContent = content.replace(rawFull, '').replace(/\n{3,}/g, '\n\n').trim()
  return { cleanContent, quest: options.length > 0 || title ? { title, options, allowCustom } : null }
}

interface ChatSession {
  id: string
  name: string
  messages: ChatMessage[]
  updatedAt: string
  mode?: string
  status?: WebSessionStatus
  archived?: boolean
  error?: string
  totalHistory?: number
  historyLimit?: number
}
function historyToMessages(history: WebSessionItem['history']): ChatMessage[] {
  return (history || []).map(h => ({
    role: h.role === 'user' ? 'user' : h.role === 'error' ? 'error' : 'assistant',
    content: h.content,
    timestamp: new Date(h.timestamp),
  } as ChatMessage))
}

function webToChatSession(s: WebSessionItem): ChatSession {
  return {
    id: s.id,
    name: s.name,
    messages: historyToMessages(s.history),
    updatedAt: s.updated_at,
    mode: s.mode,
    status: s.status,
    archived: s.archived,
    error: s.error,
    totalHistory: s.total_history ?? s.history?.length ?? 0,
    historyLimit: s.history_limit,
  }
}

function mergeSessionPreview(prev: ChatSession | undefined, incoming: ChatSession): ChatSession {
  const keepMessages = (prev?.messages?.length || 0) > incoming.messages.length
  return {
    ...incoming,
    messages: keepMessages ? prev!.messages : incoming.messages,
    totalHistory: incoming.totalHistory ?? prev?.totalHistory,
    historyLimit: incoming.historyLimit ?? prev?.historyLimit,
  }
}

function statusBadgeClass(status?: WebSessionStatus) {
  switch (status) {
    case 'running': return 'border-smara-400/30 bg-smara-500/10 text-smara-300'
    case 'completed': return 'border-emerald-400/25 bg-emerald-400/10 text-emerald-300'
    case 'error': return 'border-red-400/25 bg-red-500/10 text-red-300'
    case 'cancelled': return 'border-amber-400/25 bg-amber-500/10 text-amber-300'
    case 'archived': return 'border-[#5f7446]/30 bg-[#31421f]/35 text-gray-400'
    default: return 'border-[#5f7446]/24 bg-[#31421f]/24 text-gray-400'
  }
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
// const MAX_MESSAGES_PER_SESSION = 200 // backend persistence now
const MAX_SESSIONS = 20
const MAX_OUTPUT_CHARS = 4000
const MAX_LOG_LINES = 50
// Runtime caps keep the live React tree from growing forever during long
// terminal/tool streams. Persistence caps alone are not enough because the
// lag happens before data is written to localStorage.
const MAX_RUNTIME_MESSAGES = 60
const SESSION_LIST_HISTORY_LIMIT = 1
const SESSION_VIEW_HISTORY_LIMIT = 60
const MAX_RUNTIME_OUTPUT_CHARS = 20_000
const MAX_RUNTIME_LOG_LINES = 120

const MAX_SESSION_BYTES = 800 * 1024

// slimMessage strips fields that don't need to survive a refresh.
// The runtime versions in React state still hold the full data.
export function slimMessage(m: ChatMessage): ChatMessage {
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

export function saveSession(id: string, messages: ChatMessage[]) {
  void id
  void messages
  // Riwayat chat sekarang disimpan oleh backend multi-session.
  // Fungsi ini dipertahankan sebagai no-op untuk fallback kompatibilitas lama.
}

export type ChatHandle = { openSessions: () => void }

function Chat(_props: {}, ref: React.Ref<ChatHandle>) {
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
  const [activePlanQuest, setActivePlanQuest] = useState<PlanQuest | null>(null)
  const [dragOver, setDragOver] = useState(false)
  const fileInputRef = useRef<HTMLInputElement>(null)
  const dragCounterRef = useRef(0)
  const wsRef = useRef<WebSocket | null>(null)
  const bottomRef = useRef<HTMLDivElement>(null)
  const messagesScrollRef = useRef<HTMLDivElement>(null)
  const shouldAutoScrollRef = useRef(true)
  const reconnectTimer = useRef<ReturnType<typeof setTimeout> | null>(null)
  const spinnerTimer = useRef<ReturnType<typeof setInterval> | null>(null)
  const sessionIdRef = useRef(sessionId)
  sessionIdRef.current = sessionId

  useImperativeHandle(ref, () => ({
    openSessions: () => setShowSessions(true),
  }), [])

  const refreshBackendSessions = useCallback(async () => {
    try {
      const res = await fetchWebSessions(true, SESSION_LIST_HISTORY_LIMIT)
      const backend = res.sessions.map(webToChatSession)
      if (backend.length === 0) {
        const created = webToChatSession(await createWebSession(undefined, mode))
        setSessions([created])
        setCurrentRaw(created)
        setSessionId(created.id)
        setMessages(capRuntimeMessages(created.messages.map(m => ({ ...m, timestamp: new Date(m.timestamp) }))))
        setItemSafe(CURRENT_SESSION_KEY, created.id)
        return
      }
      setSessions(prev => backend.map(incoming => mergeSessionPreview(prev.find(p => p.id === incoming.id), incoming)))
      const currentBackend = backend.find(s => s.id === sessionIdRef.current)
      if (currentBackend) {
        setCurrentRaw(prev => mergeSessionPreview(prev.id === currentBackend.id ? prev : undefined, currentBackend))
        // Jangan replace message list aktif dari polling session-list.
        // List endpoint sekarang hanya membawa preview supaya dropdown ringan.
      } else {
        const next = backend.find(s => !s.archived) || backend[0]
        setCurrentRaw(next)
        setSessionId(next.id)
        setItemSafe(CURRENT_SESSION_KEY, next.id)
        try {
          const fresh = webToChatSession(await getWebSession(next.id, SESSION_VIEW_HISTORY_LIMIT))
          setCurrentRaw(fresh)
          setMessages(capRuntimeMessages(fresh.messages.map(m => ({ ...m, timestamp: new Date(m.timestamp) }))))
        } catch {
          setMessages(capRuntimeMessages(next.messages.map(m => ({ ...m, timestamp: new Date(m.timestamp) }))))
        }
      }
    } catch (err) {
      console.warn('[smara] backend sessions unavailable, fallback localStorage:', err)
    }
  }, [mode])

  const setCurrent = async (s: ChatSession) => {
    setCurrentRaw(s)
    setSessionId(s.id)
    setItemSafe(CURRENT_SESSION_KEY, s.id)
    setActivePhases([])
    setActivePlanQuest(null)
    try {
      const fresh = webToChatSession(await getWebSession(s.id, SESSION_VIEW_HISTORY_LIMIT))
      setCurrentRaw(fresh)
      setMessages(capRuntimeMessages(fresh.messages.map(m => ({ ...m, timestamp: new Date(m.timestamp) }))))
    } catch {
      setMessages(capRuntimeMessages(s.messages.map(m => ({ ...m, timestamp: new Date(m.timestamp) }))))
    }
    if (wsRef.current?.readyState === WebSocket.OPEN) wsRef.current.send(JSON.stringify({ type: 'session', payload: s.id, session_id: s.id }))
  }

  const isNearBottom = useCallback(() => {
    const el = messagesScrollRef.current
    if (!el) return true
    return el.scrollHeight - el.scrollTop - el.clientHeight < 120
  }, [])

  const markAutoScrollIfNearBottom = useCallback(() => {
    shouldAutoScrollRef.current = isNearBottom()
  }, [isNearBottom])

  useEffect(() => {
    refreshBackendSessions()
    const timer = window.setInterval(refreshBackendSessions, 5000)
    return () => window.clearInterval(timer)
  }, [refreshBackendSessions])

  useEffect(() => {
    if (!shouldAutoScrollRef.current) return
    bottomRef.current?.scrollIntoView({ behavior: 'smooth' })
  }, [messages, thinking, activePhases, activePlanQuest])

  const newSession = async () => {
    try {
      const s = webToChatSession(await createWebSession(undefined, mode))
      setSessions(prev => [s, ...prev.filter(x => x.id !== s.id)])
      setCurrentRaw(s)
      setSessionId(s.id)
      setItemSafe(CURRENT_SESSION_KEY, s.id)
      shouldAutoScrollRef.current = true
      setMessages(capRuntimeMessages(s.messages.map(m => ({ ...m, timestamp: new Date(m.timestamp) }))))
      setActivePlanQuest(null)
      if (wsRef.current?.readyState === WebSocket.OPEN) wsRef.current.send(JSON.stringify({ type: 'session', payload: s.id, session_id: s.id }))
    } catch (err) {
      showToast(`Gagal membuat sesi: ${err instanceof Error ? err.message : 'unknown'}`)
    }
  }

  const deleteSession = async (id: string) => {
    try {
      await deleteWebSession(id)
      const remaining = sessions.filter(s => s.id !== id)
      setSessions(remaining)
      if (id === sessionId) {
        if (remaining.length > 0) await setCurrent(remaining[0])
        else await newSession()
      }
    } catch (err) {
      showToast(`Gagal hapus sesi: ${err instanceof Error ? err.message : 'unknown'}`)
    }
  }

  const renameSession = async (id: string) => {
    const old = sessions.find(s => s.id === id)?.name || current.name
    const name = window.prompt('Nama sesi baru', old)?.trim()
    if (!name || name === old) return
    try {
      await renameWebSession(id, name)
      await refreshBackendSessions()
    } catch (err) {
      showToast(`Gagal rename: ${err instanceof Error ? err.message : 'unknown'}`)
    }
  }

  const toggleArchiveSession = async (s: ChatSession) => {
    try {
      if (s.archived) await unarchiveWebSession(s.id)
      else await archiveWebSession(s.id)
      await refreshBackendSessions()
    } catch (err) {
      showToast(`Gagal archive: ${err instanceof Error ? err.message : 'unknown'}`)
    }
  }

  const cancelSession = async (id: string) => {
    try {
      await cancelWebSession(id)
      if (wsRef.current?.readyState === WebSocket.OPEN) wsRef.current.send(JSON.stringify({ type: 'cancel', session_id: id }))
      setThinking(false)
      await refreshBackendSessions()
    } catch (err) {
      showToast(`Gagal stop: ${err instanceof Error ? err.message : 'unknown'}`)
    }
  }

  const connectWs = useCallback(() => {
    if (wsRef.current?.readyState === WebSocket.OPEN) return
    const ws = new WebSocket(`${location.protocol === 'https:' ? 'wss' : 'ws'}://${location.host}/ws`)
    wsRef.current = ws

    ws.onopen = () => {
      setConnected(true)
      ws.send(JSON.stringify({ type: 'session', payload: sessionIdRef.current, session_id: sessionIdRef.current }))
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
      if (msg.session_id && msg.session_id !== sessionIdRef.current && msg.type !== 'session_status') return
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
        case 'chat': {
          setThinking(false)
          if (spinnerTimer.current) { clearInterval(spinnerTimer.current); spinnerTimer.current = null }
          setActivePhases([])
          const parsed = parsePlanQuest(String(msg.payload || ''))
          setActivePlanQuest(parsed.quest)
          const content = parsed.cleanContent || (parsed.quest ? '' : msg.payload)
          if (content.trim()) {
            markAutoScrollIfNearBottom()
            setMessages(prev => capRuntimeMessages([...prev, {
              role: 'assistant',
              content,
              timestamp: new Date(),
              requestPrompt: msg.request_prompt,
              provider: msg.provider,
              model: msg.model,
              inputTokens: msg.stats?.input_tokens,
              outputTokens: msg.stats?.output_tokens,
              totalTokens: msg.stats?.total_tokens,
              duration: msg.stats?.duration,
              durationMs: msg.stats?.duration_ms,
              estimatedCostUSD: msg.stats?.estimated_cost_usd ?? msg.stats?.cost,
            }]))
          }
          break
        }
        case 'error':
          setThinking(false)
          if (spinnerTimer.current) { clearInterval(spinnerTimer.current); spinnerTimer.current = null }
          setActivePhases([])
          markAutoScrollIfNearBottom()
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
          markAutoScrollIfNearBottom()
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
          markAutoScrollIfNearBottom()
          setMessages(prev => {
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
          markAutoScrollIfNearBottom()
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
        case 'session_status':
          setSessions(prev => prev.map(s => s.id === msg.session_id ? { ...s, status: msg.payload as WebSessionStatus, updatedAt: new Date().toISOString() } : s))
          if (msg.session_id === sessionIdRef.current) setCurrentRaw(c => ({ ...c, status: msg.payload as WebSessionStatus }))
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
    const userMessage: ChatMessage = {
      role: 'user',
      content: messageText,
      timestamp: new Date(),
      attachments: attachments.map(a => ({ path: a.path, size: a.size, kind: a.kind, name: a.name, preview: a.preview })),
    }

    shouldAutoScrollRef.current = true
    setMessages(prev => capRuntimeMessages([...prev, userMessage]))
    setInput('')
    setAttachments([])
    setActivePlanQuest(null)

    if (!wsRef.current || wsRef.current.readyState !== WebSocket.OPEN) {
      connectWs()
      return
    }

    wsRef.current.send(JSON.stringify({ type: 'chat', payload: messageText, mode, session_id: sessionIdRef.current }))
    setThinking(true)
  }, [input, attachments, connectWs, mode])

  const sendPlanQuestAnswer = useCallback((answer: string) => {
    const text = `Saya pilih: ${answer}`
    setInput('')
    setActivePlanQuest(null)
    shouldAutoScrollRef.current = true
    setMessages(prev => capRuntimeMessages([...prev, { role: 'user', content: text, timestamp: new Date() }]))
    if (!wsRef.current || wsRef.current.readyState !== WebSocket.OPEN) {
      connectWs()
      return
    }
    wsRef.current.send(JSON.stringify({ type: 'chat', payload: text, mode, session_id: sessionIdRef.current }))
    setThinking(true)
  }, [connectWs, mode])

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
      <div className="h-[72px] flex items-center justify-between gap-4 px-5 bg-[#1b2416]/96 backdrop-blur-xl shadow-lg shadow-black/10">
        <div className="flex items-center gap-3 min-w-0">
          <div className="relative shrink-0">
            <div className="absolute inset-0 rounded-2xl bg-smara-400/30 blur-md" />
            <div className="relative w-10 h-10 rounded-2xl bg-gradient-to-br from-smara-200 via-smara-400 to-smara-700 flex items-center justify-center shadow-lg shadow-smara-950/40 ring-1 ring-smara-300/14">
              <Bot className="w-5 h-5 text-white" />
            </div>
          </div>
          <button
            onClick={() => setShowSessions(!showSessions)}
            className="group flex items-center gap-2 min-w-0 max-w-[420px] px-3 py-2 rounded-2xl bg-[#24301b]/96 hover:bg-[#2b3a20] transition-colors"
            title="Pilih sesi chat"
          >
            <MessageSquare className="w-4 h-4 text-neutral-400 group-hover:text-smara-300 shrink-0" />
            <span className="font-medium truncate group-hover:text-smara-200">{current.name}</span>
            <span className={`text-[10px] shrink-0 rounded-full border px-1.5 py-0.5 ${statusBadgeClass(current.status)}`}>{current.status || 'idle'}</span>
            <span className="text-[10px] text-neutral-400 shrink-0">{sessions.length} sesi</span>
            <ChevronDown className={`w-4 h-4 text-neutral-400 transition-transform shrink-0 ${showSessions ? 'rotate-180 text-smara-300' : ''}`} />
          </button>
          <span className="hidden md:inline-flex items-center gap-1 text-[10px] text-neutral-400 font-mono truncate max-w-[220px]"><Server className="w-3 h-3" />{sessionId}</span>
        </div>
        <div className="flex items-center gap-3">
          <button
            onClick={newSession}
            className="flex items-center gap-1.5 rounded-2xl bg-smara-300 px-3.5 py-2 text-xs font-semibold text-black shadow-lg shadow-smara-950/20 transition-colors hover:bg-smara-200"
          >
            <Plus className="w-3 h-3" /> Sesi Baru
          </button>
          {!connected && (
            <button onClick={connectWs} className="flex items-center gap-1 text-xs text-smara-400 hover:text-smara-300 transition-colors">
              <RefreshCw className="w-3 h-3" /> Reconnect
            </button>
          )}
          <span className={`inline-flex items-center gap-1.5 rounded-full border px-2.5 py-1 text-xs ${connected ? 'border-emerald-400/20 bg-emerald-400/10 text-emerald-300' : 'border-red-400/20 bg-red-500/10 text-red-300'}`}>
            <span className={`w-1.5 h-1.5 rounded-full ${connected ? 'bg-emerald-400 shadow-[0_0_10px_rgba(52,211,153,.9)]' : 'bg-red-400'}`} />
            {connected ? 'Online' : 'Offline'}
          </span>
        </div>
      </div>

      {/* Session dropdown */}
      {showSessions && (
        <div className="absolute top-[72px] left-3 right-3 z-50 overflow-hidden rounded-3xl bg-[#202b18] shadow-2xl shadow-black/35 ring-1 ring-black/55">
          <div className="mx-auto max-w-5xl p-4">
            <div className="flex items-center justify-between mb-3 px-1">
              <div>
                <div className="text-xs text-gray-200 font-semibold uppercase tracking-wider">Sesi dari Smara Web</div>
                <div className="text-[11px] text-neutral-400">Pilih sesi untuk ditampilkan di chat</div>
              </div>
              <button onClick={() => setShowSessions(false)} className="text-xs text-gray-400 hover:text-gray-100 px-2 py-1 rounded-lg hover:bg-[#26331d]">Tutup</button>
            </div>
            <div className="grid gap-2 max-h-80 overflow-y-auto pr-1">
              {sessions.length === 0 && (
                <div className="rounded-xl bg-[#27331f] p-4 text-sm text-gray-300 ring-1 ring-black/35">Belum ada sesi tersimpan.</div>
              )}
              {sessions.map(s => {
                const last = s.messages[s.messages.length - 1]
                return (
                  <div
                    key={s.id}
                    onClick={() => { setCurrent(s); setShowSessions(false); }}
                    className={`group flex items-center justify-between gap-3 p-3 rounded-xl cursor-pointer text-sm transition-all ring-1 ${
                      s.id === current.id
                        ? 'bg-[#31421f] ring-black/35 shadow-sm shadow-black/20'
                        : 'bg-[#26331d] ring-black/35 hover:bg-[#2b3a20] hover:ring-black/25'
                    }`}
                  >
                    <div className="flex items-start gap-3 min-w-0">
                      <div className={`mt-0.5 rounded-lg p-2 ${s.id === current.id ? 'bg-smara-500/15 text-smara-300' : 'bg-[#26331d] text-neutral-400 group-hover:text-gray-300'}`}>
                        <MessageSquare className="w-4 h-4" />
                      </div>
                      <div className="min-w-0">
                        <div className="flex items-center gap-2 min-w-0">
                          <span className="font-medium text-gray-100 truncate max-w-[360px]">{s.name}</span>
                          {s.id === current.id && <span className="text-[10px] px-1.5 py-0.5 rounded-full bg-smara-500/15 text-smara-200 ring-1 ring-smara-300/12">aktif</span>}
                        </div>
                        <div className="mt-1 text-xs text-gray-400 truncate max-w-[520px]">
                          {last?.content ? last.content : 'Sesi kosong'}
                        </div>
                      </div>
                    </div>
                    <div className="flex items-center gap-3 shrink-0">
                      <div className="hidden sm:flex flex-col items-end text-[10px] text-neutral-400">
                        <span>{s.totalHistory ?? s.messages.length} pesan</span>
                        <span className={`inline-flex rounded-full border px-1.5 py-0.5 ${statusBadgeClass(s.status)}`}>{s.status || 'idle'}</span>
                        <span className="flex items-center gap-1"><Clock className="w-3 h-3" /> {new Date(s.updatedAt).toLocaleString('id-ID', { day: '2-digit', month: 'short', hour: '2-digit', minute: '2-digit' })}</span>
                      </div>
                      <div className="flex items-center gap-1 opacity-80 group-hover:opacity-100">
                        {s.status === 'running' && (
                          <button onClick={(e) => { e.stopPropagation(); cancelSession(s.id); }} className="text-smara-400 hover:text-amber-300 transition-colors p-1.5 rounded hover:bg-amber-400/10" title="Stop session">
                            <StopCircle className="w-4 h-4" />
                          </button>
                        )}
                        <button onClick={(e) => { e.stopPropagation(); renameSession(s.id); }} className="text-neutral-500 hover:text-smara-300 transition-colors p-1.5 rounded hover:bg-smara-950/30" title="Rename sesi">
                          <Pencil className="w-4 h-4" />
                        </button>
                        <button onClick={(e) => { e.stopPropagation(); toggleArchiveSession(s); }} className="text-neutral-600 hover:text-amber-300 transition-colors p-1.5 rounded hover:bg-[#26331d]/80" title={s.archived ? 'Unarchive sesi' : 'Archive sesi'}>
                          {s.archived ? <ArchiveRestore className="w-4 h-4" /> : <Archive className="w-4 h-4" />}
                        </button>
                        {sessions.length > 1 && (
                          <button
                            onClick={(e) => { e.stopPropagation(); deleteSession(s.id); }}
                            className="text-neutral-600 hover:text-red-400 transition-colors p-1.5 rounded hover:bg-[#26331d]/80"
                            title="Hapus sesi"
                          >
                            <Trash2 className="w-4 h-4" />
                          </button>
                        )}
                      </div>
                    </div>
                  </div>
                )
              })}
            </div>
          </div>
        </div>
      )}

      {/* Messages */}
      <div
        ref={messagesScrollRef}
        onScroll={markAutoScrollIfNearBottom}
        className="flex-1 overflow-y-auto px-4 py-5 md:px-8 md:py-7 space-y-5 bg-[linear-gradient(180deg,rgba(163,230,53,0.09),rgba(255,255,255,0.035)_38%,rgba(21,29,16,0.22)_72%)]"
      >
        {(current.totalHistory ?? messages.length) > messages.length && (
          <div className="mx-auto max-w-3xl rounded-2xl border border-[#31421f]/60 bg-[#202b18]/48 px-4 py-3 text-center text-xs text-smara-100 shadow-lg shadow-black/20">
            Menampilkan {messages.length} pesan terakhir dari {current.totalHistory} pesan. Riwayat lama disimpan di backend tapi tidak dirender agar sesi tetap ringan.
          </div>
        )}
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
              <div key={i} className="ml-11 px-3 py-2 bg-[#20291a]/78 border border-[#223018]/75 rounded text-xs text-gray-400 font-mono whitespace-pre-wrap max-h-40 overflow-y-auto">
                {msg.output || msg.content}
              </div>
            )
          }
          if (msg.role === 'log') {
            return (
              <div key={i} className="flex items-center gap-2 text-xs text-neutral-400 px-2 ml-11">
                <span className="text-neutral-500">&#9654;</span>
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
                  : 'bg-gradient-to-br from-[#4f6138] via-[#38482a] to-[#24301b] ring-smara-400/20 shadow-smara-950/40'
              }`}>
                {msg.role === 'user' ? <User className="w-4 h-4" /> : <Bot className="w-4 h-4 text-smara-200" />}
              </div>
              <div className={`max-w-[90%] md:max-w-[76%] rounded-[1.45rem] px-5 py-4 text-sm leading-relaxed relative shadow-xl backdrop-blur-md overflow-hidden transition-all duration-200 ${
                msg.role === 'user'
                  ? 'bg-[#49751a]/96 text-white shadow-smara-950/18'
                  : msg.role === 'error'
                  ? 'bg-gradient-to-br from-red-950/70 to-neutral-950/60 border border-red-600/40 text-red-100 shadow-red-950/20'
                  : 'bg-[#2b3522]/98 text-gray-50 shadow-black/12'
              }`}>
                {msg.role !== 'user' && msg.role !== 'error' && (
                  <div className="mb-2 flex items-center gap-2 text-[10px] font-semibold uppercase tracking-[0.12em] text-smara-200">
                    <span className="h-1.5 w-1.5 rounded-full bg-smara-300/85" />
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
                            <img src={att.preview} alt={att.path} className="max-h-32 rounded-xl border border-[#31421f]/60 shadow-lg" />
                          ) : (
                            <div className="flex items-center gap-2 px-3 py-2 bg-gray-950/60 border border-[#31421f]/60 rounded-xl shadow-inner">
                              <Icon className="w-5 h-5 text-smara-300 shrink-0" />
                              <div className="flex flex-col min-w-0">
                                <span className="text-xs text-gray-200 truncate max-w-[200px]">{att.name || att.path.split('/').pop()}</span>
                                <span className="text-[10px] text-neutral-400 font-mono">{(att.size / 1024).toFixed(0)} KB</span>
                              </div>
                            </div>
                          )}
                          {att.kind === 'image' && (
                            <div className="text-[10px] text-neutral-400 mt-0.5 font-mono truncate max-w-[200px]">{(att.size / 1024).toFixed(0)} KB</div>
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
                  <>
                    <div className="text-gray-100"><SmaraMarkdown content={msg.content} /></div>
                    {msg.requestPrompt && (
                      <details className="mt-3 rounded-lg border border-[#31421f]/60 bg-[#20291a]/78 px-2 py-1 text-[10px] text-gray-400">
                        <summary className="cursor-pointer select-none text-smara-200">Request prompt</summary>
                        <pre className="mt-2 max-h-48 overflow-auto whitespace-pre-wrap break-words font-mono text-[10px] leading-4 text-gray-300">{msg.requestPrompt}</pre>
                      </details>
                    )}
                  </>
                )}
                {(msg.role !== 'user' && msg.role !== 'error' && (msg.inputTokens !== undefined || msg.outputTokens !== undefined || msg.totalTokens !== undefined || msg.duration || msg.estimatedCostUSD !== undefined || msg.model || msg.provider)) ? (
                  <div className="mt-3 flex items-center justify-between gap-3 border-t border-neutral-800/60 pt-2 text-[10px] text-gray-400">
                    <div className="flex min-w-0 flex-wrap items-center gap-1.5">
                      {(msg.provider || msg.model) && <span className="rounded-full bg-[#20291a]/78 px-2 py-0.5">{msg.provider ? `${msg.provider}/` : ''}{msg.model || 'unknown'}</span>}
                      {msg.inputTokens !== undefined && <span className="rounded-full bg-[#20291a]/78 px-2 py-0.5">in {msg.inputTokens}</span>}
                      {msg.outputTokens !== undefined && <span className="rounded-full bg-[#20291a]/78 px-2 py-0.5">out {msg.outputTokens}</span>}
                      {msg.totalTokens !== undefined && <span className="rounded-full bg-[#20291a]/78 px-2 py-0.5">total {msg.totalTokens}</span>}
                      {msg.duration && <span className="rounded-full bg-[#20291a]/78 px-2 py-0.5">{msg.duration}</span>}
                      {msg.estimatedCostUSD !== undefined && <span className="rounded-full bg-[#20291a]/78 px-2 py-0.5 text-smara-200">~{formatCostUSD(msg.estimatedCostUSD)}</span>}
                    </div>
                    <div className="shrink-0 text-right">{new Date(msg.timestamp).toLocaleTimeString()}</div>
                  </div>
                ) : (
                  <div className="mt-3 flex justify-end border-t border-neutral-800/60 pt-2 text-[10px] text-gray-400">
                    {new Date(msg.timestamp).toLocaleTimeString()}
                  </div>
                )}
                <button
                  onClick={() => copyMessage(i, msg.content)}
                  title="Salin pesan"
                  aria-label="Salin pesan"
                  className={`absolute top-2 ${msg.role === 'user' ? 'left-2' : 'right-2'} z-10 inline-flex h-8 w-8 items-center justify-center rounded-xl bg-[#202b18]/90 text-gray-300 shadow-lg shadow-black/30 backdrop-blur-md opacity-80 transition-all hover:bg-[#26331d] hover:text-white hover:opacity-100 group-hover:opacity-100 md:opacity-0 md:group-hover:opacity-100`}
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
                <div className="bg-[#20291a]/82 border border-[#223018]/75 rounded-lg p-3 space-y-1.5">
                  <div className="flex items-center gap-2 text-[10px] text-neutral-400 uppercase tracking-wider font-medium mb-1">
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
                      <span className={ph.status === 'running' ? 'text-gray-200 font-medium' : 'text-neutral-400'}>
                        {ph.description || ph.phase}
                      </span>
                    </div>
                  ))}
                </div>
              )}
              {activePhases.length === 0 && (
                <div className="bg-[#20291a]/78 border border-[#223018]/75 rounded-lg px-4 py-3 flex items-center gap-2">
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
        <div className="px-5 py-1.5 bg-[#1a2314]/96 flex items-center gap-3 text-[10px] text-neutral-500">
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
      <div className="p-4 bg-[#1b2416]/98 backdrop-blur-xl space-y-3 shadow-[0_-18px_40px_rgba(0,0,0,0.22)]">
        <div className="flex flex-wrap gap-1.5 rounded-2xl border border-[#31421f]/60 bg-[#20291a]/78 p-1.5 w-fit">
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
                    ? `${m.bg} text-white shadow-lg shadow-black/20`
                    : 'text-gray-400 hover:bg-[#26331d]/80 hover:text-gray-200'
                }`}
                title={m.label}
              >
                <Icon className="w-3 h-3" />
                <span className="hidden sm:inline">{m.label}</span>
              </button>
            )
          })}
        </div>
        {activePlanQuest && (
          <div className="rounded-2xl border border-[#31421f]/60 bg-[#20291a]/78 p-3 shadow-lg shadow-lime-950/20">
            <div className="mb-2 flex items-center gap-2 text-xs font-semibold uppercase tracking-wide text-lime-200">
              <ClipboardList className="h-3.5 w-3.5" /> Open question
            </div>
            <div className="mb-3 text-sm font-medium text-gray-100">{activePlanQuest.title}</div>
            <div className="flex flex-wrap gap-2">
              {activePlanQuest.options.map(opt => (
                <button
                  key={opt}
                  onClick={() => sendPlanQuestAnswer(opt)}
                  disabled={thinking}
                  className="rounded-xl bg-smara-300/8 px-3 py-2 text-xs text-gray-100 transition-colors hover:bg-smara-300/14 disabled:cursor-not-allowed disabled:opacity-50"
                >
                  {opt}
                </button>
              ))}
              {activePlanQuest.allowCustom && (
                <span className="rounded-xl border border-[#31421f]/60 bg-[#20291a]/78 px-3 py-2 text-xs text-gray-400">Custom: ketik jawaban di input</span>
              )}
            </div>
          </div>
        )}
        {attachments.length > 0 && (
          <div className="flex flex-wrap gap-2">
            {attachments.map((att, i) => {
              const Icon = attachmentIcon(att.name, att.mime)
              return (
                <div key={i} className="relative group inline-flex items-center gap-2 pl-1 pr-2 py-1 bg-[#20291a]/82 border border-[#223018]/75 rounded">
                  {att.kind === 'image' && att.preview ? (
                    <img src={att.preview} alt="" className="h-8 w-8 object-cover rounded" />
                  ) : (
                    <div className="h-8 w-8 flex items-center justify-center bg-[#27331f]/80 rounded">
                      <Icon className="w-4 h-4 text-smara-300" />
                    </div>
                  )}
                  <span className="text-xs text-gray-300 font-mono truncate max-w-[160px]" title={att.path}>
                    {att.name}
                  </span>
                  <span className="text-[10px] text-neutral-400">{(att.size / 1024).toFixed(0)} KB</span>
                  <button
                    onClick={() => removeAttachment(i)}
                    className="ml-1 text-neutral-400 hover:text-red-400 transition-colors"
                    title="Hapus lampiran"
                  >
                    <X className="w-3 h-3" />
                  </button>
                </div>
              )
            })}
          </div>
        )}
        <div className="flex gap-2 rounded-[1.35rem] bg-[#2b3522]/98 p-2 shadow-inner shadow-black/30 focus-within:ring-1 focus-within:ring-smara-300/20 transition-all">
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
            className="px-3 py-2 bg-[#20291a]/78 hover:bg-[#26331d]/80 rounded-2xl transition-colors disabled:opacity-50"
          >
            <Paperclip className="w-4 h-4 text-gray-400" />
          </button>
          <textarea
            value={input}
            onChange={e => setInput(e.target.value)}
            onKeyDown={handleKeyDown}
            onPaste={handlePaste}
            placeholder={uploading ? 'Mengunggah...' : 'Ketik pesan... (Enter kirim · Ctrl+V paste · drop file untuk lampirkan)'}
            className="flex-1 bg-transparent border border-transparent rounded-2xl px-3 py-2 text-sm text-gray-100 resize-none focus:outline-none min-h-[42px] max-h-[140px] placeholder:text-gray-400"
            rows={1}
          />
          <button
            onClick={send}
            disabled={(!input.trim() && attachments.length === 0) || current.status === 'running' || uploading}
            className="px-4 py-2 rounded-2xl bg-smara-300 text-black shadow-lg shadow-smara-950/25 transition-colors hover:bg-smara-200 disabled:opacity-50 disabled:cursor-not-allowed"
          >
            <Send className="w-4 h-4" />
          </button>
        </div>
      </div>

      {dragOver && (
        <div className="absolute inset-0 z-50 flex items-center justify-center bg-[#11170d]/78 backdrop-blur-sm rounded-lg pointer-events-none">
          <div className="flex flex-col items-center gap-2 text-smara-200">
            <Upload className="w-12 h-12" />
            <span className="text-lg font-semibold">Drop file untuk lampirkan</span>
            <span className="text-xs text-smara-300/80">Gambar, PDF, dokumen, kode — max 25 MB</span>
          </div>
        </div>
      )}

      {toast && (
        <div className="fixed bottom-24 left-1/2 -translate-x-1/2 z-50 px-4 py-2 bg-[#202b18] border border-[#223018]/75 rounded-lg shadow-lg text-sm text-gray-200">
          {toast}
        </div>
      )}
    </div>
  )
}

export default forwardRef(Chat)