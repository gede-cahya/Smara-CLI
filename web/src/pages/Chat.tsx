import { useState, useRef, useEffect, useCallback, useMemo, memo, forwardRef, useImperativeHandle } from 'react'
import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import mermaid from 'mermaid'
import { BarChart, Bar, LineChart, Line, XAxis, YAxis, CartesianGrid, Tooltip, ResponsiveContainer, PieChart, Pie, Cell } from 'recharts'
import {
  Send, Bot, User, RefreshCw, Plus, Trash2, MessageSquare, Clock,
  Zap, ClipboardList, FlaskConical, ArrowRightLeft, MessageCircle, ImageIcon,
  CheckCircle2, BrainCircuit, Copy, Check, X,
  Paperclip, FileText, FileCode, FileJson, File as FileIcon, Upload,
  Terminal, ChevronDown, ChevronRight, Loader2, AlertCircle, Wrench,
  Archive, ArchiveRestore, StopCircle, Pencil, Server, Settings, Mic,
} from 'lucide-react'
import { APIError, type ChatMessage, type WebSessionItem, type WebSessionStatus } from '../api'
import type { BackendHealth } from '../App'
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
  fetchRoadmapFile,
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

function chatWebSocketURL() {
  const protocol = location.protocol === 'https:' ? 'wss' : 'ws'
  const host = location.port === '5173' ? `${location.hostname}:8080` : location.host
  return `${protocol}://${host}/ws`
}

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

const SmaraMarkdown = memo(function SmaraMarkdown({ content }: { content: string }) {
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
})

// Tools whose primary purpose is to mutate the filesystem or remote state.
// We use this to pick a color and icon for the card chrome.
const WRITE_TOOLS = new Set(['edit_file', 'write_file', 'create_file', 'rm', 'remove_file'])
const SHELL_TOOLS = new Set(['run_command', 'ssh_exec'])
const FILE_VIEW_TOOLS = new Set(['view_file', 'read_file', 'ssh_view_file'])
const NUMBERED_SOURCE_LINE_RE = /^\s*(\d+)\s*\|\s?(.*)$/

function toolKind(tool?: string): 'shell' | 'write' | 'read' | 'tool' {
  if (!tool) return 'tool'
  if (SHELL_TOOLS.has(tool)) return 'shell'
  if (WRITE_TOOLS.has(tool)) return 'write'
  if (FILE_VIEW_TOOLS.has(tool) || tool.startsWith('read_') || tool === 'list_directory' || tool === 'search_path') return 'read'
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
    case 'view_file':
    case 'ssh_view_file':
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
    case 'skill_run': {
      const name = String(args.skill_name ?? '')
      const automatic = args.automatic === true
      return {
        title: name ? `Skill: ${name}` : 'Skill',
        subtitle: automatic ? 'Dipilih dan dijalankan otomatis' : 'Dijalankan oleh agent',
      }
    }
    default: {
      // Fall back to first short string arg
      const first = Object.values(args).find(v => typeof v === 'string' && v.length < 200)
      return { title: tool, subtitle: typeof first === 'string' ? first : undefined }
    }
  }
}

interface SourceLine {
  number: string
  content: string
}

function sourceLanguageLabel(path?: string): string {
  const filename = (path || '').split('/').pop() || ''
  const extension = filename.includes('.') ? filename.split('.').pop()?.toLowerCase() : ''
  return extension ? extension.slice(0, 8) : 'text'
}

function parseSourceLines(output: string): SourceLine[] {
  // Older sessions may contain the legacy single-line preview. Recover its
  // numbered boundaries so those cards also become readable after this fix.
  const normalized = output.includes('\n')
    ? output
    : output.replace(/\s+(?=\d+\s*\|\s)/g, '\n')

  return normalized.split(/\r?\n/).map((line, index) => {
    const match = line.match(NUMBERED_SOURCE_LINE_RE)
    return match
      ? { number: match[1], content: match[2] }
      : { number: String(index + 1), content: line }
  })
}

const SourceFileViewer = memo(function SourceFileViewer({
  output,
  path,
  copied,
  onCopy,
}: {
  output: string
  path?: string
  copied: boolean
  onCopy: (text: string) => void
}) {
  const lines = useMemo(() => parseSourceLines(output), [output])
  const language = sourceLanguageLabel(path)

  return (
    <div className="min-w-0">
      <div className="flex items-center gap-2 border-b border-[#26351c]/70 bg-[#1b2814]/88 px-4 py-2 text-[10px] text-neutral-400">
        <span className="rounded-md border border-[#42552f]/60 bg-[#26351d]/80 px-2 py-0.5 font-mono font-semibold uppercase tracking-wide text-smara-200">
          {language}
        </span>
        <span>{lines.length} baris</span>
        <button
          onClick={() => onCopy(output)}
          className="ml-auto inline-flex items-center gap-1 rounded-lg border border-[#42552f]/60 bg-[#26351d]/70 px-2 py-1 text-neutral-300 transition-colors hover:bg-[#304326] hover:text-white"
          title="Salin isi file"
        >
          {copied ? <Check className="h-3 w-3 text-green-400" /> : <Copy className="h-3 w-3" />}
          Copy
        </button>
      </div>
      <div className="max-h-[32rem] overflow-auto font-mono text-[12px] leading-6 [tab-size:2]">
        <div className="min-w-max py-2">
          {lines.map((line, index) => (
            <div
              key={`${line.number}-${index}`}
              className="grid grid-cols-[minmax(3.5rem,auto)_minmax(max-content,1fr)] hover:bg-smara-400/[0.04]"
            >
              <span className="sticky left-0 select-none border-r border-[#31421f]/60 bg-[#172211] px-3 text-right text-neutral-500">
                {line.number}
              </span>
              <code className="whitespace-pre px-4 text-gray-200">{line.content || ' '}</code>
            </div>
          ))}
        </div>
      </div>
    </div>
  )
})

const ToolCallCard = memo(function ToolCallCard({
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
  const [showAllLogs, setShowAllLogs] = useState(false)
  const kind = toolKind(msg.tool)
  const { title, subtitle } = describeToolCall(msg)
  const status = msg.status || 'running'
  const logs = msg.logs || []
  const logLines = useMemo(() => logs.flatMap(log => log.split(/\r?\n/)).filter(Boolean), [logs])
  const snapshotCount = useMemo(() => logLines.filter(line => /found snapshot/i.test(line)).length, [logLines])
  const warningCount = useMemo(() => logLines.filter(line => /\b(warn|warning|failed|error)\b/i.test(line)).length, [logLines])
  const compactLogLimit = 18
  const hiddenLogCount = Math.max(0, logLines.length - compactLogLimit)
  const visibleLogLines = showAllLogs || hiddenLogCount === 0 ? logLines : logLines.slice(-compactLogLimit)
  const hasOutput = !!msg.output && msg.output.trim().length > 0
  const isFileView = hasOutput && FILE_VIEW_TOOLS.has(msg.tool || '')
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
    <div className={`ml-11 max-w-4xl rounded-2xl border ${accent} overflow-hidden shadow-lg shadow-black/20 backdrop-blur-sm [content-visibility:auto] [contain-intrinsic-size:180px]`}>
      {/* Header */}
      <div className="flex items-center gap-2.5 px-4 py-3">
        <button
          onClick={onToggle}
          disabled={!hasBody}
          className="inline-flex h-6 w-6 items-center justify-center rounded-lg text-gray-400 transition-colors hover:bg-[#26331d]/80 hover:text-white disabled:opacity-40"
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
          <span className="rounded-md bg-[#1a2314]/70 px-1.5 py-0.5 text-[10px] text-neutral-400 font-mono">[{msg.server}]</span>
        )}
        {subtitle && (
          <span className="text-xs text-gray-300 font-mono truncate flex-1 min-w-0 leading-5" title={subtitle}>
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
        <div className="border-t border-[#223018]/75 bg-[#172211]/72">
          {logs.length > 0 && (
            <>
              <div className="flex flex-wrap items-center gap-2 border-b border-[#26351c]/70 bg-[#1b2814]/88 px-4 py-2 text-[10px] text-neutral-400">
                <span className="inline-flex items-center gap-1.5 font-medium text-smara-200">
                  <Terminal className="h-3 w-3" />
                  Live output
                </span>
                <span className="rounded-full bg-[#2a391f]/80 px-2 py-0.5">{logLines.length} baris</span>
                {snapshotCount > 0 && <span className="rounded-full bg-sky-400/10 px-2 py-0.5 text-sky-200">{snapshotCount} snapshot</span>}
                {warningCount > 0 && <span className="rounded-full bg-amber-400/10 px-2 py-0.5 text-amber-200">{warningCount} perhatian</span>}
                <div className="ml-auto flex items-center gap-2">
                  {hiddenLogCount > 0 && (
                    <button
                      onClick={() => setShowAllLogs(value => !value)}
                      className="rounded-lg border border-[#42552f]/60 bg-[#26351d]/70 px-2 py-1 text-neutral-300 transition-colors hover:bg-[#304326] hover:text-white"
                    >
                      {showAllLogs ? 'Ringkas' : `Tampilkan semua (+${hiddenLogCount})`}
                    </button>
                  )}
                  <button
                    onClick={() => onCopy(logLines.join('\n'))}
                    className="inline-flex items-center gap-1 rounded-lg border border-[#42552f]/60 bg-[#26351d]/70 px-2 py-1 text-neutral-300 transition-colors hover:bg-[#304326] hover:text-white"
                  >
                    {copied ? <Check className="h-3 w-3 text-green-400" /> : <Copy className="h-3 w-3" />}
                    Copy
                  </button>
                </div>
              </div>
              {!showAllLogs && hiddenLogCount > 0 && (
                <div className="border-b border-[#26351c]/60 px-4 py-2 text-[10px] text-neutral-500">
                  Menampilkan {compactLogLimit} baris terbaru. Output sebelumnya disembunyikan agar chat tetap ringkas.
                </div>
              )}
              <div className={`overflow-y-auto px-4 py-3 font-mono text-[11px] leading-5 sm:px-5 ${showAllLogs ? 'max-h-[32rem]' : 'max-h-72'}`}>
                <div className="space-y-0.5">
                  {visibleLogLines.map((line, index) => {
                    const clean = line.replace(/^▶\s?/, '')
                    const isWarning = /\b(warn|warning|failed|error)\b/i.test(clean)
                    const isSuccess = /\b(success|done|completed|up-to-date)\b/i.test(clean)
                    return (
                      <div key={`${index}-${clean.slice(0, 32)}`} className="grid grid-cols-[14px_minmax(0,1fr)] gap-2">
                        <span className={isWarning ? 'text-amber-300' : isSuccess ? 'text-emerald-300' : 'text-smara-400/70'}>
                          {isWarning ? '!' : isSuccess ? '✓' : '›'}
                        </span>
                        <span className={`break-words ${isWarning ? 'text-amber-100/90' : isSuccess ? 'text-emerald-100/90' : 'text-gray-300'}`}>{clean}</span>
                      </div>
                    )
                  })}
                </div>
              </div>
            </>
          )}
          {isFileView && logs.length === 0 && msg.output && (
            <SourceFileViewer output={msg.output} path={subtitle} copied={copied} onCopy={onCopy} />
          )}
          {hasOutput && logs.length === 0 && !isFileView && (
            <pre className="m-0 max-h-80 overflow-y-auto whitespace-pre-wrap break-words px-4 py-4 font-mono text-[12px] leading-6 text-gray-300 [tab-size:2] sm:px-5">
              {(msg.output || '').slice(0, 4000)}
              {(msg.output || '').length > 4000 && (
                <span className="text-neutral-400">{`\n[... ${(msg.output || '').length - 4000} chars truncated ...]`}</span>
              )}
            </pre>
          )}
          {hasOutput && logs.length > 0 && msg.output && msg.output.trim() !== logs.join('\n').trim() && (
            <details className="border-t border-[#26351c]/60 px-4 py-3 text-gray-400">
              <summary className="cursor-pointer text-[10px] text-neutral-400 hover:text-gray-300">
                full result ({msg.output.length} chars)
              </summary>
              <pre className="m-0 mt-2 whitespace-pre-wrap break-words leading-6 [tab-size:2]">{msg.output}</pre>
            </details>
          )}
          {imageUrls.length > 0 && (
            <div className="grid gap-2 border-t border-[#26351c]/60 p-4">
              {imageUrls.map(url => (
                <img key={url} src={url} alt="generated image" className="max-h-72 rounded-lg border border-[#31421f]/60 object-contain shadow-lg shadow-black/30" />
              ))}
            </div>
          )}
        </div>
      )}
    </div>
  )
})

const spinnerFrames = ['\u280B','\u2819','\u2839','\u2838','\u283C','\u2834','\u2826','\u2827','\u2807','\u280F']

const MODES: Array<{ id: string; label: string; emoji: string; icon: typeof MessageCircle; bg: string; border: string; text: string }> = [
  { id: 'ask', label: 'Ask', emoji: '\uD83D\uDCAC', icon: MessageCircle, bg: 'bg-smara-600', border: 'border-smara-500', text: 'text-smara-400' },
  { id: 'rush', label: 'Rush', emoji: '\u26A1', icon: Zap, bg: 'bg-yellow-600', border: 'border-yellow-500', text: 'text-yellow-400' },
  { id: 'plan', label: 'Plan', emoji: '\uD83D\uDCCB', icon: ClipboardList, bg: 'bg-lime-600', border: 'border-lime-500', text: 'text-lime-400' },
  { id: 'test', label: 'Test', emoji: '\uD83E\uDDEA', icon: FlaskConical, bg: 'bg-green-600', border: 'border-green-500', text: 'text-green-400' },
  { id: 'image', label: 'Image', emoji: '\uD83C\uDFA8', icon: ImageIcon, bg: 'bg-purple-600', border: 'border-purple-500', text: 'text-purple-400' },
  { id: 'workflow', label: 'Workflow', emoji: '\uD83D\uDD04', icon: ArrowRightLeft, bg: 'bg-smara-600', border: 'border-smara-500', text: 'text-smara-400' },
  { id: 'parallel', label: 'Parallel', emoji: '\uD83E\uDDE9', icon: ArrowRightLeft, bg: 'bg-indigo-600', border: 'border-indigo-500', text: 'text-indigo-300' },
  { id: 'voice', label: 'Voice', emoji: '\uD83C\uDFA4', icon: Mic, bg: 'bg-cyan-600', border: 'border-cyan-500', text: 'text-cyan-300' },
]
const SESSION_META_KEY = 'smara_chat_sessions'
const CURRENT_SESSION_KEY = 'smara_current_session'
const ROADMAP_CACHE_KEY = 'smara_roadmap_cache'
const MAX_CACHED_ROADMAPS = 20
const MAX_CACHED_ROADMAP_BYTES = 640 * 1024


interface PlanQuest {
  title: string
  options: string[]
  allowCustom: boolean
}

interface PlanInsight {
  title: string
  steps: string[]
  phases?: PlanPhaseDetail[]
}

interface PlanPhaseDetail {
  phase: number
  title: string
  status?: string
  objective?: string
  output?: string
  deliverables: string[]
  acceptance: Array<{ text: string; checked: boolean }>
  implemented: string[]
  validated: string[]
}

interface CachedRoadmap {
  path: string
  relativePath: string
  name: string
  content: string
  size: number
  updatedAt: string
  cachedAt: string
  workspace?: string
}

interface ActivePhase {
  phase: string
  description: string
  status: 'running' | 'done'
  startedAt?: number // Date.now() timestamp
}

interface AnalysisEvent {
  id: string
  kind: 'phase' | 'thinking' | 'tool' | 'log'
  title: string
  detail: string
  status: 'running' | 'done'
  level?: 'info' | 'warning' | 'error'
  event?: string
  tool?: string
  timestamp: Date
}

type AnalysisFilter = 'all' | 'phase' | 'thinking' | 'tool' | 'warning' | 'error'

const ANALYSIS_FILTERS: Array<{ id: AnalysisFilter; label: string }> = [
  { id: 'all', label: 'Semua' },
  { id: 'phase', label: 'Phase' },
  { id: 'thinking', label: 'Think' },
  { id: 'tool', label: 'Tool' },
  { id: 'warning', label: 'Warn' },
  { id: 'error', label: 'Error' },
]

function analysisEventMatchesFilter(event: AnalysisEvent, filter: AnalysisFilter): boolean {
  switch (filter) {
    case 'all':
      return true
    case 'phase':
    case 'thinking':
    case 'tool':
      return event.kind === filter
    case 'warning':
      return event.level === 'warning'
    case 'error':
      return event.level === 'error'
    default:
      return true
  }
}

interface RunStatus {
  state: 'starting' | 'thinking' | 'running' | 'waiting' | 'completed' | 'cancelled' | 'error'
  startedAt: number
  updatedAt: number
  prompt: string
  mode: string
  lastEvent: string
  lastMessage: string
  runID?: string
  currentPhase?: string
  currentTool?: string
  logPath?: string
  provider?: string
  model?: string
  reasoningEffort?: string
  customDisableStream?: boolean
  providerIdleMs?: number
  heartbeatLastEvent?: string
  toolCount: number
  heartbeatCount: number
}

interface PendingChatSend {
  payload: string
  mode: string
  sessionId: string
  runID: string
}

function createClientRunID(): string {
  return `web-client-${Date.now()}-${Math.random().toString(36).slice(2, 9)}`
}

function buildPromptWithAttachments(messageText: string, files: Attachment[]): string {
  if (!files.length) return messageText
  const hasImage = files.some(file => file.kind === 'image')
  const hasFile = files.some(file => file.kind === 'file')
  const lines = files.map((a, i) => {
    const token = a.kind === 'image' ? `[image:${a.path}]` : `[file:${a.path}]`
    const label = a.name ? ` (${a.name})` : ''
    return `${i + 1}. ${token}${label}`
  })
  const guidance = [
    hasImage
      ? 'Gambar terlampir adalah bagian utama dari pesan ini. Baca dan analisis gambar secara otomatis sebelum menjawab, termasuk saat teks user singkat atau kosong.'
      : '',
    hasFile
      ? 'Dokumen terlampir tersedia untuk dibaca ketika diperlukan untuk menjawab pesan user.'
      : '',
  ].filter(Boolean).join(' ')
  return [messageText, guidance, 'Lampiran:', ...lines].filter(Boolean).join('\n\n')
}

function parsePlanQuest(content: string): { cleanContent: string; quest: PlanQuest | null } {
  const startToken = '[[SMARA_PLAN_QUEST]]'
  const endToken = '[[/SMARA_PLAN_QUEST]]'
  const start = content.indexOf(startToken)
  if (start < 0) return { cleanContent: content, quest: null }
  const afterStart = start + startToken.length
  const end = content.indexOf(endToken, afterStart)
  const block = (end >= 0 ? content.slice(afterStart, end) : content.slice(afterStart)).trim()
  const blockWithNewlines = block
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

function isPlanApprovalQuest(quest: PlanQuest): boolean {
  return /lanjut|eksekusi|approval|setuju/i.test(`${quest.title} ${quest.options.join(' ')}`)
}

function cleanPlanStep(line: string): string {
  return line
    .replace(/^\s*(?:[-*+]|\d+[.)])\s+/, '')
    .replace(/^\[[ xX-]\]\s+/, '')
    .replace(/\*\*/g, '')
    .replace(/`/g, '')
    .replace(/\s+/g, ' ')
    .trim()
}

function parsePlanInsight(content: string): PlanInsight | null {
  const text = content.replace(/```[\s\S]*?```/g, '')
  const lines = text.split(/\r?\n/)
  const sectionStart = /^(?:#{1,4}\s*)?(?:roadmap|roadmap table|steps|langkah|rencana implementasi|implementation plan|implementation steps)\s*:?\s*$/i
  const nextSection = /^(?:#{1,4}\s*)?(?:context|assumptions|open questions|recommended approach|files\/tools|files|tools|verification|risks|rollback|flow diagram)\s*:?\s*$/i
  const steps: string[] = []
  let collecting = false

  for (const raw of lines) {
    const line = raw.trim()
    if (!line) {
      if (collecting && steps.length > 0) break
      continue
    }
    if (sectionStart.test(line)) {
      collecting = true
      continue
    }
    if (collecting && nextSection.test(line)) break
    if (!collecting) continue
    if (/^\|/.test(line)) {
      const cells = line.split('|').map(x => x.trim()).filter(Boolean)
      const candidate = cells.find((cell, idx) => idx > 0 && !/^no\.?$/i.test(cell) && !/^langkah$/i.test(cell) && !/^output$/i.test(cell) && !/^status$/i.test(cell))
      if (candidate && !/^[-:]+$/.test(candidate)) steps.push(cleanPlanStep(candidate))
      continue
    }
    if (/^\s*(?:[-*+]|\d+[.)])\s+/.test(raw)) steps.push(cleanPlanStep(raw))
  }

  if (steps.length < 2) {
    for (const raw of lines) {
      if (/^\s*\d+[.)]\s+/.test(raw)) steps.push(cleanPlanStep(raw))
      if (steps.length >= 8) break
    }
  }

  const unique = Array.from(new Set(steps.filter(step => step.length >= 8))).slice(0, 10)
  const phases = parseRoadmapPhases(content)
  const phaseSteps = phases.map(phase => phase.title).filter(Boolean)
  const hasPhaseStatus = phases.some(phase => !!phase.status)
  const finalSteps = phases.length >= 2 && hasPhaseStatus ? phaseSteps : unique.length >= 2 ? unique : phaseSteps
  if (finalSteps.length < 2) return null
  const planish = /(context|assumptions|recommended approach|verification|risks|rollback|roadmap|status sekarang|progress|progres|mermaid|lanjutkan|eksekusi)/i.test(content)
  if (!planish) return null
  return { title: 'Roadmap Plan', steps: finalSteps.slice(0, 10), phases }
}

function parseRoadmapPhases(content: string): PlanPhaseDetail[] {
  const tableMeta = parseRoadmapTableMeta(content)
  const headingRe = /^##\s+Phase\s+(\d+)\s*(?:[-–—]\s*)?(.+?)\s*$/gim
  const matches = Array.from(content.matchAll(headingRe))
  const phases: PlanPhaseDetail[] = []
  const byPhase = new Map<number, PlanPhaseDetail>()

  for (let idx = 0; idx < matches.length; idx++) {
    const match = matches[idx]
    const phase = Number(match[1])
    const title = cleanPlanStep(match[2] || `Phase ${phase}`)
    const start = (match.index || 0) + match[0].length
    const end = idx + 1 < matches.length ? matches[idx + 1].index || content.length : content.length
    const block = content.slice(start, end)
    const meta = tableMeta.get(phase)
    const detail = {
      phase,
      title,
      status: firstNonEmpty(extractStatus(block), meta?.status),
      objective: extractSectionText(block, 'Objective'),
      output: meta?.output,
      deliverables: extractSectionList(block, 'Deliverables'),
      acceptance: extractAcceptance(block),
      implemented: extractSectionList(block, 'Implemented in').concat(extractLabelList(block, 'Implemented in')),
      validated: extractValidated(block),
    }
    phases.push(detail)
    byPhase.set(phase, detail)
  }

  if (tableMeta.size > 0) {
    return Array.from(tableMeta.entries()).map(([phase, meta]) => {
      const detail = byPhase.get(phase)
      if (!detail) {
        return {
          phase,
          title: meta.focus || `Phase ${phase}`,
          status: meta.status,
          output: meta.output,
          deliverables: [],
          acceptance: [],
          implemented: [],
          validated: [],
        }
      }
      return {
        ...detail,
        title: detail.title || meta.focus || `Phase ${phase}`,
        status: firstNonEmpty(detail.status, meta.status),
        output: firstNonEmpty(detail.output, meta.output),
      }
    })
  }

  if (phases.length > 0) return phases.sort((a, b) => a.phase - b.phase)
  return Array.from(tableMeta.entries()).map(([phase, meta]) => ({
    phase,
    title: meta.focus || `Phase ${phase}`,
    status: meta.status,
    output: meta.output,
    deliverables: [],
    acceptance: [],
    implemented: [],
    validated: [],
  }))
}

function parseRoadmapTableMeta(content: string): Map<number, { focus?: string; output?: string; status?: string }> {
  const out = new Map<number, { focus?: string; output?: string; status?: string }>()
  const lines = content.split(/\r?\n/)
  for (const line of lines) {
    if (!/^\|/.test(line)) continue
    const cells = line.split('|').map(x => x.trim()).filter(Boolean)
    if (cells.length < 2 || cells.every(isMarkdownTableSeparator)) continue
    if (/^(?:phase|fase|no\.?|langkah|fokus|status)$/i.test(cells[0])) continue

    const numericPhase = Number(cells[0])
    if (Number.isFinite(numericPhase)) {
      out.set(numericPhase, { focus: cells[1], output: cells[2], status: cells[3] })
      continue
    }

    const phaseMatch = cells[0].match(/^(?:phase|fase)\s*(\d+)\s*(?:[-–—:]\s*)?(.+)?$/i)
    if (!phaseMatch) continue
    const phase = Number(phaseMatch[1])
    if (!Number.isFinite(phase)) continue
    const focus = cleanPlanStep(phaseMatch[2] || cells[0].replace(/^(?:phase|fase)\s*\d+\s*(?:[-–—:]\s*)?/i, ''))
    const status = cells[cells.length - 1]
    out.set(phase, {
      focus: focus || `Phase ${phase}`,
      output: cells.length > 2 ? cells.slice(1, -1).join(' | ') : undefined,
      status,
    })
  }
  return out
}

function isMarkdownTableSeparator(value: string): boolean {
  const trimmed = value.trim()
  if (!trimmed || !trimmed.includes('-')) return false
  for (const char of trimmed) {
    if (char !== '-' && char !== ':' && char !== ' ') return false
  }
  return true
}

function firstNonEmpty(...values: Array<string | undefined>): string | undefined {
  return values.find(value => !!value && value.trim().length > 0)
}

function extractStatus(block: string): string | undefined {
  const match = block.match(/Status\s*:\s*\*\*?([^*\n]+)\*\*?/i)
  return match?.[1]?.trim()
}

function extractSectionText(block: string, heading: string): string | undefined {
  const section = extractSectionBlock(block, heading)
  if (!section) return undefined
  const text = section
    .split(/\r?\n/)
    .map(line => line.trim())
    .filter(line => line && !line.startsWith('```') && !line.startsWith('|'))
    .join(' ')
    .trim()
  return text || undefined
}

function extractSectionList(block: string, heading: string): string[] {
  const section = extractSectionBlock(block, heading)
  if (!section) return []
  return section
    .split(/\r?\n/)
    .filter(line => /^\s*-\s+/.test(line))
    .map(cleanPlanStep)
    .filter(Boolean)
}

function extractAcceptance(block: string): Array<{ text: string; checked: boolean }> {
  const section = extractSectionBlock(block, 'Acceptance Criteria')
  if (!section) return []
  return section
    .split(/\r?\n/)
    .filter(line => /^\s*-\s+(?:\[[ xX]\]\s*)?/.test(line))
    .map(line => {
      const checked = /^\s*-\s+\[[xX]\]/.test(line)
      return { text: cleanPlanStep(line), checked }
    })
    .filter(item => item.text.length > 0)
}

function extractValidated(block: string): string[] {
  const section = extractSectionBlock(block, 'Validated with') || extractSectionBlock(block, 'Commands')
  if (!section) return extractLabelCodeOrList(block, 'Validated with')
  const codeMatches = Array.from(section.matchAll(/```(?:\w+)?\n([\s\S]*?)```/g))
  if (codeMatches.length > 0) {
    return codeMatches.flatMap(match => match[1].split(/\r?\n/).map(line => line.trim()).filter(Boolean))
  }
  return extractSectionList(block, 'Validated with')
}

function extractLabelList(block: string, label: string): string[] {
  const section = extractLabelBlock(block, label)
  if (!section) return []
  return section
    .split(/\r?\n/)
    .filter(line => /^\s*-\s+/.test(line))
    .map(cleanPlanStep)
    .filter(Boolean)
}

function extractLabelCodeOrList(block: string, label: string): string[] {
  const section = extractLabelBlock(block, label)
  if (!section) return []
  const codeMatches = Array.from(section.matchAll(/```(?:\w+)?\n([\s\S]*?)```/g))
  if (codeMatches.length > 0) {
    return codeMatches.flatMap(match => match[1].split(/\r?\n/).map(line => line.trim()).filter(Boolean))
  }
  return section
    .split(/\r?\n/)
    .filter(line => /^\s*-\s+/.test(line) || /^[\w./-]+\s/.test(line.trim()))
    .map(cleanPlanStep)
    .filter(Boolean)
}

function extractLabelBlock(block: string, label: string): string {
  const escaped = label.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
  const re = new RegExp(`^\\s*${escaped}\\s*:\\s*$`, 'im')
  const match = block.match(re)
  if (!match || match.index === undefined) return ''
  const start = match.index + match[0].length
  const rest = block.slice(start)
  const next = rest.search(/^(?:###\s+|\s*[A-Z][A-Za-z ]+:\s*$)/m)
  return (next >= 0 ? rest.slice(0, next) : rest).trim()
}

function extractSectionBlock(block: string, heading: string): string {
  const escaped = heading.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
  const re = new RegExp(`^###\\s+${escaped}\\s*$`, 'im')
  const match = block.match(re)
  if (!match || match.index === undefined) return ''
  const start = match.index + match[0].length
  const rest = block.slice(start)
  const next = rest.search(/^###\s+/m)
  return (next >= 0 ? rest.slice(0, next) : rest).trim()
}

function planFlowMermaid(steps: string[]): string {
  const safe = (value: string) => value.replace(/[|[\]{}"]/g, '').slice(0, 48)
  const body = steps.map((step, idx) => `  S${idx + 1}["${idx + 1}. ${safe(step)}"]`).join('\n')
  const edges = steps.slice(1).map((_, idx) => `  S${idx + 1} --> S${idx + 2}`).join('\n')
  return `flowchart TD\n${body}\n${edges}`
}

function planStepState(step: string, idx: number, activePhases: ActivePhase[], runStatus: RunStatus | null): 'done' | 'running' | 'planned' {
  if (!runStatus || runStatus.state === 'completed' || runStatus.state === 'cancelled' || runStatus.state === 'error') return 'planned'
  const haystack = activePhases.map(p => `${p.phase} ${p.description}`).join(' ').toLowerCase()
  const keywords = cleanPlanStep(step).toLowerCase().split(/\s+/).filter(w => w.length > 5).slice(0, 4)
  if (keywords.some(word => haystack.includes(word))) return 'running'
  if (activePhases.length > idx && activePhases[idx]?.status === 'done') return 'done'
  if (activePhases.length === idx + 1 && activePhases[idx]?.status === 'running') return 'running'
  return 'planned'
}

function isRoadmapProgressPrompt(text: string): boolean {
  return /roadmap/i.test(text) && /(progress|progres|phase|fase|step|langkah)/i.test(text)
}

function requestedRoadmapStepIndex(messages: ChatMessage[]): number | null {
  for (let i = messages.length - 1; i >= 0; i--) {
    const msg = messages[i]
    if (msg.role !== 'user') continue
    const text = String(msg.content || '')
    if (!isRoadmapProgressPrompt(text)) return null
    const match = text.match(/(?:phase|fase|step|langkah)\s*#?\s*(\d+)/i)
    if (!match) return null
    const index = Number(match[1]) - 1
    return Number.isFinite(index) && index >= 0 ? index : null
  }
  return null
}

function latestPlanInsightFromMessages(messages: ChatMessage[]): PlanInsight | null {
  for (let i = messages.length - 1; i >= 0; i--) {
    if (messages[i].role !== 'assistant') continue
    const insight = parsePlanInsight(messages[i].content || '')
    if (insight) return insight
  }
  return null
}

function roadmapProgressSummary(insight: PlanInsight, activePhases: ActivePhase[], runStatus: RunStatus | null) {
  const states = insight.steps.map((step, idx) => {
    const declared = insight.phases?.[idx]?.status?.toLowerCase() || ''
    if (/done|complete|selesai|success/.test(declared)) return 'done' as const
    if (/running|in progress|progress|berjalan/.test(declared)) return 'running' as const
    return planStepState(step, idx, activePhases, runStatus)
  })
  const done = states.filter(s => s === 'done').length
  const running = states.findIndex(s => s === 'running')
  return { states, done, running, total: states.length }
}

function RoadmapProgressPanel({
  insight,
  activePhases,
  runStatus,
  focusIndex,
}: {
  insight: PlanInsight
  activePhases: ActivePhase[]
  runStatus: RunStatus | null
  focusIndex: number | null
}) {
  const initialOpen = focusIndex !== null && focusIndex < insight.steps.length ? focusIndex : null
  const [openPhase, setOpenPhase] = useState<number | null>(initialOpen)
  useEffect(() => {
    if (focusIndex !== null && focusIndex < insight.steps.length) setOpenPhase(focusIndex)
  }, [focusIndex, insight.steps.length])
  const summary = roadmapProgressSummary(insight, activePhases, runStatus)
  const activeIndex = focusIndex !== null && focusIndex < insight.steps.length ? focusIndex : summary.running

  return (
    <div className="rounded-xl border border-[#31421f]/50 bg-[#1a2314]/62">
      <div className="flex flex-wrap items-center justify-between gap-2 border-b border-[#31421f]/45 px-3 py-2">
        <div className="flex items-center gap-2 text-xs font-semibold text-lime-100">
          <CheckCircle2 className="h-3.5 w-3.5 text-smara-300" />
          Progress roadmap
        </div>
        <span className="rounded-full bg-[#26331d]/80 px-2 py-0.5 text-[10px] text-neutral-300">
          {summary.done}/{summary.total} done
        </span>
      </div>
      <div className="max-h-64 overflow-y-auto p-2">
        {insight.steps.map((step, idx) => {
          const state = summary.states[idx]
          const focused = idx === activeIndex
          const phase = insight.phases?.[idx]
          const expanded = openPhase === idx
          return (
            <div
              key={`${idx}-${step}`}
              className={`rounded-lg text-xs ${
                focused ? 'bg-smara-300/10 ring-1 ring-smara-300/20' : 'hover:bg-[#20291a]/72'
              }`}
            >
              <button
                onClick={() => setOpenPhase(expanded ? null : idx)}
                className="grid w-full grid-cols-[26px_1fr_auto_18px] items-start gap-2 px-2 py-2 text-left"
              >
                <span className="font-mono text-[10px] text-neutral-500">{phase?.phase || idx + 1}</span>
                <div className="min-w-0">
                  <div className={focused ? 'text-gray-100' : 'text-gray-300'}>{phase?.title || step}</div>
                  {phase?.output && <div className="mt-0.5 truncate text-[10px] text-neutral-500">{phase.output}</div>}
                  {focused && (
                    <div className="mt-1 text-[10px] text-smara-200">
                      {state === 'running' ? 'Sedang diproses sekarang.' : focusIndex === idx ? 'Phase ini sedang diminta untuk dicek.' : 'Phase aktif roadmap.'}
                    </div>
                  )}
                </div>
                <span className={`inline-flex items-center gap-1 rounded-full px-1.5 py-0.5 text-[10px] ${
                  state === 'done' ? 'bg-emerald-400/10 text-emerald-300'
                  : state === 'running' ? 'bg-smara-500/10 text-smara-200'
                  : focused ? 'bg-yellow-500/10 text-yellow-200'
                  : 'bg-[#26331d]/80 text-neutral-400'
                }`}>
                  {state === 'done' ? <CheckCircle2 className="h-3 w-3" /> : state === 'running' ? <Loader2 className="h-3 w-3 animate-spin" /> : focused ? <AlertCircle className="h-3 w-3" /> : <Clock className="h-3 w-3" />}
                  {state === 'done' ? 'done' : state === 'running' ? 'running' : focused ? 'focus' : 'planned'}
                </span>
                {expanded ? <ChevronDown className="mt-0.5 h-3.5 w-3.5 text-neutral-500" /> : <ChevronRight className="mt-0.5 h-3.5 w-3.5 text-neutral-500" />}
              </button>
              {expanded && phase && (
                <div className="border-t border-[#31421f]/35 px-2 pb-2 pt-1">
                  {phase.objective && (
                    <div className="mb-2 rounded-md bg-[#20291a]/70 px-2 py-1.5 text-[11px] leading-5 text-gray-300">
                      {phase.objective}
                    </div>
                  )}
                  {phase.deliverables.length > 0 && (
                    <div className="mb-2">
                      <div className="mb-1 text-[10px] font-semibold uppercase tracking-wide text-neutral-500">Deliverables</div>
                      <div className="space-y-1">
                        {phase.deliverables.map(item => (
                          <div key={item} className="flex gap-1.5 text-[11px] text-gray-300">
                            <span className="mt-1 h-1 w-1 rounded-full bg-smara-300/70" />
                            <span>{item}</span>
                          </div>
                        ))}
                      </div>
                    </div>
                  )}
                  {phase.acceptance.length > 0 && (
                    <div className="mb-2">
                      <div className="mb-1 text-[10px] font-semibold uppercase tracking-wide text-neutral-500">Checklist</div>
                      <div className="space-y-1">
                        {phase.acceptance.map(item => (
                          <div key={item.text} className="flex gap-1.5 text-[11px] text-gray-300">
                            {item.checked || state === 'done' ? <CheckCircle2 className="mt-0.5 h-3 w-3 shrink-0 text-emerald-300" /> : <Clock className="mt-0.5 h-3 w-3 shrink-0 text-neutral-500" />}
                            <span>{item.text}</span>
                          </div>
                        ))}
                      </div>
                    </div>
                  )}
                  {(phase.implemented.length > 0 || phase.validated.length > 0) && (
                    <div className="grid gap-2 sm:grid-cols-2">
                      {phase.implemented.length > 0 && (
                        <div>
                          <div className="mb-1 text-[10px] font-semibold uppercase tracking-wide text-neutral-500">Implemented</div>
                          <div className="space-y-1">
                            {phase.implemented.map(item => <div key={item} className="truncate font-mono text-[10px] text-smara-200">{item}</div>)}
                          </div>
                        </div>
                      )}
                      {phase.validated.length > 0 && (
                        <div>
                          <div className="mb-1 text-[10px] font-semibold uppercase tracking-wide text-neutral-500">Validated</div>
                          <div className="space-y-1">
                            {phase.validated.map(item => <div key={item} className="truncate font-mono text-[10px] text-gray-400">{item}</div>)}
                          </div>
                        </div>
                      )}
                    </div>
                  )}
                  {!phase.objective && phase.deliverables.length === 0 && phase.acceptance.length === 0 && (
                    <div className="text-[11px] text-neutral-500">Belum ada detail phase di roadmap.</div>
                  )}
                </div>
              )}
              {expanded && !phase && (
                <div className="border-t border-[#31421f]/35 px-2 pb-2 pt-1 text-[11px] text-neutral-500">
                  Detail phase belum tersedia dari response roadmap.
                </div>
              )}
            </div>
          )
        })}
      </div>
    </div>
  )
}

function PlanInsightCard({
  insight,
  activePhases,
  runStatus,
  showApproval,
  onApprove,
  onReject,
}: {
  insight: PlanInsight
  activePhases: ActivePhase[]
  runStatus: RunStatus | null
  showApproval: boolean
  onApprove: () => void
  onReject: () => void
}) {
  const summary = roadmapProgressSummary(insight, activePhases, runStatus)
  return (
    <div className="mt-3 overflow-hidden rounded-xl border border-[#31421f]/60 bg-[#20291a]/78">
      <div className="flex flex-wrap items-center justify-between gap-2 border-b border-[#31421f]/60 px-3 py-2">
        <div className="flex items-center gap-2 text-xs font-semibold text-lime-100">
          <ClipboardList className="h-3.5 w-3.5 text-smara-300" />
          {insight.title}
          <span className="rounded-full bg-[#26331d]/80 px-2 py-0.5 text-[10px] font-normal text-neutral-300">{summary.done}/{summary.total} done</span>
        </div>
        {showApproval && (
          <div className="flex gap-1.5">
            <button onClick={onApprove} className="inline-flex items-center gap-1 rounded-lg bg-smara-300 px-2.5 py-1.5 text-[11px] font-semibold text-black hover:bg-smara-200">
              <Check className="h-3 w-3" /> Lanjutkan
            </button>
            <button onClick={onReject} className="inline-flex items-center gap-1 rounded-lg border border-[#5f7446]/35 bg-[#26331d]/72 px-2.5 py-1.5 text-[11px] font-medium text-gray-300 hover:bg-[#2f3f23]">
              <X className="h-3 w-3" /> Tidak
            </button>
          </div>
        )}
      </div>
      <div className="grid gap-3 p-3 lg:grid-cols-[1.1fr_0.9fr]">
        <div className="overflow-hidden rounded-lg border border-[#31421f]/45">
          <table className="min-w-full text-left text-xs">
            <thead className="bg-[#26331d]/80 text-neutral-400">
              <tr>
                <th className="w-12 px-2 py-2 font-medium">No</th>
                <th className="px-2 py-2 font-medium">Langkah</th>
                <th className="w-24 px-2 py-2 font-medium">Status</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-[#31421f]/45">
              {insight.steps.map((step, idx) => {
                const state = summary.states[idx]
                const phase = insight.phases?.[idx]
                return (
                  <tr key={`${idx}-${step}`} className="bg-[#1a2314]/54">
                    <td className="px-2 py-2 font-mono text-neutral-500">{phase?.phase || idx + 1}</td>
                    <td className="px-2 py-2 text-gray-200">{phase?.title || step}</td>
                    <td className="px-2 py-2">
                      <span className={`inline-flex items-center gap-1 rounded-full px-2 py-0.5 text-[10px] ${
                        state === 'done' ? 'bg-emerald-400/10 text-emerald-300'
                        : state === 'running' ? 'bg-smara-500/10 text-smara-200'
                        : 'bg-[#26331d]/80 text-neutral-400'
                      }`}>
                        {state === 'done' ? <CheckCircle2 className="h-3 w-3" /> : state === 'running' ? <Loader2 className="h-3 w-3 animate-spin" /> : <Clock className="h-3 w-3" />}
                        {state === 'done' ? 'done' : state === 'running' ? 'running' : 'planned'}
                      </span>
                    </td>
                  </tr>
                )
              })}
            </tbody>
          </table>
        </div>
        <div className="min-w-0">
          <MermaidBlock code={planFlowMermaid(insight.steps)} />
        </div>
      </div>
    </div>
  )
}

const AssistantMessageContent = memo(function AssistantMessageContent({
  msg,
  activePhases,
  runStatus,
  showApproval,
  onApprove,
  onReject,
}: {
  msg: ChatMessage
  activePhases: ActivePhase[]
  runStatus: RunStatus | null
  showApproval: boolean
  onApprove: () => void
  onReject: () => void
}) {
  const insight = useMemo(() => parsePlanInsight(msg.content), [msg.content])

  return (
    <>
      <div className="text-gray-100"><SmaraMarkdown content={msg.content} /></div>
      {insight && (
        <PlanInsightCard
          insight={insight}
          activePhases={activePhases}
          runStatus={runStatus}
          showApproval={showApproval}
          onApprove={onApprove}
          onReject={onReject}
        />
      )}
      {msg.requestPrompt && (
        <details className="mt-3 rounded-lg border border-[#31421f]/60 bg-[#20291a]/78 px-2 py-1 text-[10px] text-gray-400">
          <summary className="cursor-pointer select-none text-smara-200">Request prompt</summary>
          <pre className="mt-2 max-h-48 overflow-auto whitespace-pre-wrap break-words font-mono text-[10px] leading-4 text-gray-300">{msg.requestPrompt}</pre>
        </details>
      )}
    </>
  )
})

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

function normalizeCachedRoadmap(value: unknown): CachedRoadmap | null {
  if (!value || typeof value !== 'object') return null
  const item = value as Partial<CachedRoadmap>
  const path = String(item.path || item.relativePath || '').trim()
  const relativePath = String(item.relativePath || item.path || '').trim()
  const content = String(item.content || '')
  if (!path || !relativePath || !content) return null
  return {
    path,
    relativePath,
    name: String(item.name || relativePath.split('/').pop() || 'roadmap.md'),
    content,
    size: Number.isFinite(item.size) ? Number(item.size) : content.length,
    updatedAt: String(item.updatedAt || ''),
    cachedAt: String(item.cachedAt || ''),
    workspace: item.workspace ? String(item.workspace) : undefined,
  }
}

function getCachedRoadmaps(): CachedRoadmap[] {
  try {
    const raw = localStorage.getItem(ROADMAP_CACHE_KEY)
    if (!raw) return []
    const parsed = JSON.parse(raw)
    if (!Array.isArray(parsed)) return []
    return parsed.map(normalizeCachedRoadmap).filter(Boolean).slice(0, MAX_CACHED_ROADMAPS) as CachedRoadmap[]
  } catch {
    return []
  }
}

function saveCachedRoadmaps(items: CachedRoadmap[]): CachedRoadmap[] {
  let next = items.slice(0, MAX_CACHED_ROADMAPS)
  while (next.length > 0) {
    if (rawSetItem(ROADMAP_CACHE_KEY, JSON.stringify(next))) return next
    next = next.slice(0, -1)
  }
  try { localStorage.removeItem(ROADMAP_CACHE_KEY) } catch { /* ignore */ }
  return []
}

function upsertCachedRoadmap(items: CachedRoadmap[], item: CachedRoadmap): CachedRoadmap[] {
  const key = item.relativePath || item.path
  return [item, ...items.filter(existing => (existing.relativePath || existing.path) !== key)].slice(0, MAX_CACHED_ROADMAPS)
}

function cachedRoadmapLabel(item: CachedRoadmap): string {
  const date = item.cachedAt ? new Date(item.cachedAt) : null
  const when = date && Number.isFinite(date.getTime())
    ? date.toLocaleString('id-ID', { day: '2-digit', month: 'short', hour: '2-digit', minute: '2-digit' })
    : 'cache lama'
  return `${item.relativePath || item.path} (${when})`
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
const MAX_RUNTIME_PREVIEW_MESSAGES = 12
const MAX_RUNTIME_LOG_LINES = 120
const MAX_RUNTIME_LOG_CHARS = 40_000
const MAX_ANALYSIS_EVENTS = 18
const MAX_ANALYSIS_DETAIL_CHARS = 1800

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
  if (out.logs) {
    out.logs = out.logs.map(capRuntimeLogText)
  }
  return out
}

function capRuntimeMessages(messages: ChatMessage[]): ChatMessage[] {
  const capped = messages.length <= MAX_RUNTIME_MESSAGES ? messages : messages.slice(-MAX_RUNTIME_MESSAGES)
  const previewCutoff = Math.max(0, capped.length - MAX_RUNTIME_PREVIEW_MESSAGES)
  let changed = capped !== messages
  const next = capped.map((message, index) => {
    let out = message
    if (message.output && message.output.length > MAX_RUNTIME_OUTPUT_CHARS) {
      const suffix = `\n[... ${message.output.length - MAX_RUNTIME_OUTPUT_CHARS} chars truncated from live view ...]`
      out = {
        ...out,
        output: `${message.output.slice(0, MAX_RUNTIME_OUTPUT_CHARS - suffix.length)}${suffix}`,
      }
    }
    if (index < previewCutoff && message.attachments?.some(attachment => attachment.preview)) {
      out = {
        ...out,
        attachments: message.attachments.map(attachment => ({ ...attachment, preview: undefined })),
      }
    }
    if (out !== message) changed = true
    return out
  })
  return changed ? next : messages
}

function capRuntimeLogs(logs: string[]): string[] {
  if (logs.length <= MAX_RUNTIME_LOG_LINES) return logs
  const dropped = logs.length - MAX_RUNTIME_LOG_LINES
  return [`[... ${dropped} earlier live lines truncated ...]`, ...logs.slice(-MAX_RUNTIME_LOG_LINES)]
}

function capRuntimeLogText(text: string): string {
  if (text.length <= MAX_RUNTIME_LOG_CHARS) return text
  return `[... ${text.length - MAX_RUNTIME_LOG_CHARS} earlier chars truncated ...]\n${text.slice(-MAX_RUNTIME_LOG_CHARS)}`
}

function appendRuntimeLog(logs: string[], chunk: string, appendToLast: boolean): string[] {
  if (!appendToLast || logs.length === 0) return capRuntimeLogs([...logs, chunk])
  const next = [...logs]
  next[next.length - 1] = capRuntimeLogText(`${next[next.length - 1]}${chunk}`)
  return capRuntimeLogs(next)
}

function capAnalysisDetail(detail: string): string {
  if (detail.length <= MAX_ANALYSIS_DETAIL_CHARS) return detail
  return detail.slice(-MAX_ANALYSIS_DETAIL_CHARS)
}

function analysisID(kind: AnalysisEvent['kind']): string {
  return `${kind}-${Date.now()}-${Math.random().toString(36).slice(2)}`
}

function isTerminalRunState(state?: RunStatus['state']): boolean {
  return state === 'completed' || state === 'cancelled' || state === 'error'
}

function runStateLabel(state: RunStatus['state']): string {
  switch (state) {
    case 'starting': return 'Mulai'
    case 'thinking': return 'Menganalisis'
    case 'running': return 'Menjalankan'
    case 'waiting': return 'Menunggu'
    case 'completed': return 'Selesai'
    case 'cancelled': return 'Dibatalkan'
    case 'error': return 'Error'
    default: return state
  }
}

function runStateClass(state: RunStatus['state']): string {
  switch (state) {
    case 'completed': return 'border-emerald-400/25 bg-emerald-400/10 text-emerald-300'
    case 'cancelled': return 'border-amber-400/25 bg-amber-500/10 text-amber-300'
    case 'error': return 'border-red-400/30 bg-red-500/10 text-red-300'
    case 'running': return 'border-smara-400/30 bg-smara-500/10 text-smara-200'
    case 'waiting': return 'border-yellow-400/25 bg-yellow-500/10 text-yellow-200'
    default: return 'border-[#5f7446]/30 bg-[#31421f]/30 text-gray-300'
  }
}

function formatElapsed(ms: number): string {
  if (ms < 1000) return '<1s'
  const total = Math.floor(ms / 1000)
  const min = Math.floor(total / 60)
  const sec = total % 60
  if (min <= 0) return `${sec}s`
  return `${min}m ${sec.toString().padStart(2, '0')}s`
}

function stringFromDetail(value: unknown): string | undefined {
  return typeof value === 'string' && value.trim() ? value : undefined
}

function numberFromDetail(value: unknown): number | undefined {
  return typeof value === 'number' && Number.isFinite(value) ? value : undefined
}

function boolFromDetail(value: unknown): boolean | undefined {
  return typeof value === 'boolean' ? value : undefined
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

function Chat({ health }: { health: BackendHealth }, ref: React.Ref<ChatHandle>) {
  const [sessions, setSessions] = useState<ChatSession[]>(getAllSessions)
  const [current, setCurrentRaw] = useState<ChatSession>(loadCurrentSession)
  const [messages, setMessages] = useState<ChatMessage[]>(current.messages)
  const [sessionId, setSessionId] = useState(current.id)
  const [input, setInput] = useState('')
  const [thinking, setThinking] = useState(false)
  const [connected, setConnected] = useState(false)
  const [showSessions, setShowSessions] = useState(false)
  const [showRoadmapPopup, setShowRoadmapPopup] = useState(false)
  const [roadmapGoal, setRoadmapGoal] = useState('')
  const [roadmapContext, setRoadmapContext] = useState('')
  const [roadmapPath, setRoadmapPath] = useState('roadmap/parallel-task-orchestration.md')
  const [cachedRoadmaps, setCachedRoadmaps] = useState<CachedRoadmap[]>(getCachedRoadmaps)
  const [loadedRoadmap, setLoadedRoadmap] = useState<PlanInsight | null>(() => {
    const cached = getCachedRoadmaps()[0]
    return cached ? parsePlanInsight(cached.content) : null
  })
  const [loadedRoadmapPath, setLoadedRoadmapPath] = useState(() => {
    const cached = getCachedRoadmaps()[0]
    return cached?.relativePath || cached?.path || ''
  })
  const [activeRoadmapPath, setActiveRoadmapPath] = useState(() => {
    const cached = getCachedRoadmaps()[0]
    return cached?.relativePath || cached?.path || ''
  })
  const [loadingRoadmap, setLoadingRoadmap] = useState(false)
  const [mode, setMode] = useState('ask')
  const [spinnerIdx, setSpinnerIdx] = useState(0)
  const [elapsedTick, setElapsedTick] = useState(0) // bumped every ~1s for elapsed display
  const [statusStats, setStatusStats] = useState<{ prompts: number; inputTokens: number; outputTokens: number; tokens: number; duration: string; cost: number } | null>(null)
  const [activePhases, setActivePhases] = useState<ActivePhase[]>([])
  const [analysisEvents, setAnalysisEvents] = useState<AnalysisEvent[]>([])
  const [analysisFilter, setAnalysisFilter] = useState<AnalysisFilter>('all')
  const [runStatus, setRunStatus] = useState<RunStatus | null>(null)
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
  useEffect(() => {
    sessionIdRef.current = sessionId
  }, [sessionId])
  const thinkingRef = useRef(thinking)
  useEffect(() => {
    thinkingRef.current = thinking
  }, [thinking])
  const runStatusRef = useRef(runStatus)
  useEffect(() => {
    runStatusRef.current = runStatus
  }, [runStatus])
  const pendingChatRef = useRef<PendingChatSend | null>(null)
  const activeRunIDRef = useRef<string | null>(null)
  useEffect(() => {
    activeRunIDRef.current = null
  }, [sessionId])
  const streamingAssistantRef = useRef(false)
  const streamBufferRef = useRef('')
  const generatingPhaseActiveRef = useRef(false)
  const voiceAudioRef = useRef<HTMLAudioElement | null>(null)
  const lastVoiceSpokenRef = useRef<string>('')
  const [voiceSpeaking, setVoiceSpeaking] = useState(false)
  const [voiceSettings, setVoiceSettings] = useState<any>(null)
  const closingWsRef = useRef(false)

  useImperativeHandle(ref, () => ({
    openSessions: () => setShowSessions(true),
  }), [])

  const stopProcessSpinner = useCallback(() => {
    if (spinnerTimer.current) clearInterval(spinnerTimer.current)
    spinnerTimer.current = null
  }, [])

  const interruptLocalRun = useCallback((state: RunStatus['state'], lastEvent: string, message: string, status: WebSessionStatus = 'cancelled') => {
    const currentRun = runStatusRef.current
    if (!thinkingRef.current && (!currentRun || isTerminalRunState(currentRun.state))) return

    stopProcessSpinner()
    setThinking(false)
    setActivePhases([])
    setAnalysisEvents(prev => prev.map(event => (
      event.status === 'running'
        ? {
            ...event,
            status: 'done' as const,
            level: state === 'error' ? 'error' as const : event.level,
            detail: event.detail || message,
            timestamp: new Date(),
          }
        : event
    )))
    streamingAssistantRef.current = false
    streamBufferRef.current = ''
    generatingPhaseActiveRef.current = false

    setRunStatus(prev => prev ? {
      ...prev,
      state,
      updatedAt: Date.now(),
      lastEvent,
      lastMessage: message,
      currentTool: undefined,
    } : prev)

    setCurrentRaw(prev => ({ ...prev, status, error: state === 'error' ? message : prev.error }))
    setMessages(prev => capRuntimeMessages(prev.map(msg => {
      if (msg.role !== 'tool_call' || msg.status !== 'running') return msg
      return {
        ...msg,
        status: 'error',
        output: msg.output || message,
        logs: msg.logs && msg.logs.length > 0 ? [...msg.logs, message] : [message],
        timestamp: new Date(),
      }
    })))
  }, [stopProcessSpinner])

  const settleFromSessionStatus = useCallback(async (status: WebSessionStatus, reason?: string) => {
    if (status === 'running') return
    const currentRun = runStatusRef.current
    if (pendingChatRef.current || (!thinkingRef.current && !currentRun)) return
    const alreadySettled = currentRun && isTerminalRunState(currentRun.state)
    if (!thinkingRef.current && alreadySettled) return

    let state: RunStatus['state'] = 'completed'
    let lastMessage = 'Status sesi sudah sinkron dengan backend.'
    if (status === 'cancelled') {
      state = 'cancelled'
      lastMessage = 'Proses dibatalkan.'
    } else if (status === 'error') {
      state = 'error'
      lastMessage = reason || 'Proses gagal.'
    } else if (status === 'idle') {
      state = 'cancelled'
      lastMessage = reason || 'Backend restart/offline saat proses berjalan; proses lama dihentikan.'
    }

    interruptLocalRun(state, status, lastMessage, status)

    if (status === 'completed' || status === 'cancelled' || status === 'error') {
      try {
        const fresh = webToChatSession(await getWebSession(sessionIdRef.current, SESSION_VIEW_HISTORY_LIMIT))
        setCurrentRaw(fresh)
        setMessages(capRuntimeMessages(fresh.messages.map(m => ({ ...m, timestamp: new Date(m.timestamp) }))))
      } catch {
        // Kalau backend baru hidup dan history belum siap, status UI tetap harus berhenti.
      }
    }
  }, [interruptLocalRun])

  const queuePendingChat = useCallback((payload: string, activeMode: string, runID: string) => {
    pendingChatRef.current = { payload, mode: activeMode, sessionId: sessionIdRef.current, runID }
    setThinking(true)
    setRunStatus(prev => prev ? {
      ...prev,
      state: 'waiting',
      updatedAt: Date.now(),
      lastEvent: 'queued',
      lastMessage: 'Menunggu koneksi WebSocket ke backend kembali.',
    } : prev)
  }, [])

  const flushPendingChat = useCallback((ws: WebSocket) => {
    const pending = pendingChatRef.current
    if (!pending) return false
    pendingChatRef.current = null
    if (pending.sessionId !== sessionIdRef.current) {
      setThinking(false)
      setRunStatus(prev => prev ? {
        ...prev,
        state: 'cancelled',
        updatedAt: Date.now(),
        lastEvent: 'session_changed',
        lastMessage: 'Pengiriman dibatalkan karena sesi aktif berubah saat reconnect.',
      } : prev)
      return false
    }
    activeRunIDRef.current = pending.runID
    ws.send(JSON.stringify({ type: 'chat', payload: pending.payload, mode: pending.mode, session_id: pending.sessionId, run_id: pending.runID }))
    setThinking(true)
    setRunStatus(prev => prev ? {
      ...prev,
      state: 'starting',
      updatedAt: Date.now(),
      mode: pending.mode,
      lastEvent: 'request',
      lastMessage: 'Request dikirim setelah koneksi WebSocket siap.',
    } : prev)
    return true
  }, [])

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
        void settleFromSessionStatus(currentBackend.status || 'idle', currentBackend.error)
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
      const backendUnavailable = err instanceof APIError && err.status === 503 && err.message === 'backend unavailable'
      if (!backendUnavailable) console.warn('[smara] backend sessions unavailable, fallback localStorage:', err)
    }
  }, [mode, settleFromSessionStatus])

  const setCurrent = async (s: ChatSession) => {
    setCurrentRaw(s)
    setSessionId(s.id)
    setItemSafe(CURRENT_SESSION_KEY, s.id)
    setActivePhases([])
    setRunStatus(null)
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
    return el.scrollHeight - el.scrollTop - el.clientHeight < 160
  }, [])

  const markAutoScrollIfNearBottom = useCallback(() => {
    shouldAutoScrollRef.current = isNearBottom()
  }, [isNearBottom])

  const scrollToBottom = useCallback((force = false) => {
    if (!force && !shouldAutoScrollRef.current) return
    const el = messagesScrollRef.current
    if (!el) return
    window.requestAnimationFrame(() => {
      const latest = messagesScrollRef.current
      if (!latest) return
      // Pakai direct scrollTop, bukan smooth scrollIntoView.
      // Smooth animation yang dipanggil berulang saat streaming/tool progress
      // bisa saling menimpa dan membuat scroll chat glitch/jitter.
      latest.scrollTop = latest.scrollHeight
    })
  }, [])

  useEffect(() => {
    const refreshWhenVisible = () => {
      if (!document.hidden) void refreshBackendSessions()
    }
    refreshWhenVisible()
    const timer = window.setInterval(refreshWhenVisible, 5000)
    document.addEventListener('visibilitychange', refreshWhenVisible)
    return () => {
      window.clearInterval(timer)
      document.removeEventListener('visibilitychange', refreshWhenVisible)
    }
  }, [refreshBackendSessions])

  useEffect(() => {
    scrollToBottom()
  }, [messages, thinking, activePhases, analysisEvents, activePlanQuest, scrollToBottom])
  const pushAnalysisEvent = useCallback((event: Omit<AnalysisEvent, 'id' | 'timestamp'>) => {
    setAnalysisEvents(prev => {
      const next = [
        ...prev.map(item => item.status === 'running' && item.kind === event.kind ? { ...item, status: 'done' as const } : item),
        { ...event, id: analysisID(event.kind), timestamp: new Date() },
      ]
      return next.slice(-MAX_ANALYSIS_EVENTS)
    })
  }, [])

  const completeAnalysisEvent = useCallback((kind: AnalysisEvent['kind'], detail?: string) => {
    setAnalysisEvents(prev => {
      const next = [...prev]
      for (let i = next.length - 1; i >= 0; i--) {
        if (next[i].kind === kind && next[i].status === 'running') {
          next[i] = {
            ...next[i],
            detail: detail ? capAnalysisDetail(detail) : next[i].detail,
            status: 'done',
            timestamp: new Date(),
          }
          return next.slice(-MAX_ANALYSIS_EVENTS)
        }
      }
      return prev
    })
  }, [])

  const appendThinkingAnalysis = useCallback((chunk: string) => {
    setAnalysisEvents(prev => {
      const next = [...prev]
      for (let i = next.length - 1; i >= 0; i--) {
        if (next[i].kind === 'thinking' && next[i].status === 'running') {
          next[i] = { ...next[i], detail: capAnalysisDetail(`${next[i].detail}${chunk}`), timestamp: new Date() }
          return next.slice(-MAX_ANALYSIS_EVENTS)
        }
      }
      next.push({
        id: analysisID('thinking'),
        kind: 'thinking',
        title: 'Analisis model',
        detail: capAnalysisDetail(chunk),
        status: 'running',
        level: 'info',
        event: 'thinking',
        timestamp: new Date(),
      })
      return next.slice(-MAX_ANALYSIS_EVENTS)
    })
  }, [])

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
      if (id === sessionIdRef.current) {
        interruptLocalRun('cancelled', 'cancelled', 'Proses dibatalkan.')
      }
      await refreshBackendSessions()
    } catch (err) {
      showToast(`Gagal stop: ${err instanceof Error ? err.message : 'unknown'}`)
    }
  }

  const openConfigTab = () => {
    try {
      localStorage.setItem('smara_active_tab', 'config')
      window.dispatchEvent(new CustomEvent('smara:set-active-tab', { detail: 'config' }))
      window.dispatchEvent(new StorageEvent('storage', { key: 'smara_active_tab', newValue: 'config' }))
    } catch {
      showToast('Buka tab Config dari sidebar untuk mengubah provider.')
    }
  }

  const openParallelAgentPanel = useCallback(() => {
    try {
      localStorage.setItem('smara_active_tab', 'parallel-tasks')
      window.dispatchEvent(new CustomEvent('smara:set-active-tab', { detail: 'parallel-tasks' }))
      window.dispatchEvent(new StorageEvent('storage', { key: 'smara_active_tab', newValue: 'parallel-tasks' }))
    } catch {
      // User can still open the Parallel Agent tab manually from sidebar.
    }
  }, [])

  const setModeAndNotify = useCallback((nextMode: string) => {
    setMode(nextMode)
    if (nextMode === 'parallel') openParallelAgentPanel()
    if (wsRef.current?.readyState === WebSocket.OPEN) {
      wsRef.current.send(JSON.stringify({ type: 'mode_change', mode: nextMode }))
    }
  }, [openParallelAgentPanel])

  const connectWs = useCallback(() => {
    if (
      wsRef.current?.readyState === WebSocket.OPEN ||
      wsRef.current?.readyState === WebSocket.CONNECTING
    ) return
    if (reconnectTimer.current) {
      clearTimeout(reconnectTimer.current)
      reconnectTimer.current = null
    }
    closingWsRef.current = false
    const ws = new WebSocket(chatWebSocketURL())
    wsRef.current = ws

    ws.onopen = () => {
      if (wsRef.current !== ws) return
      setConnected(true)
      ws.send(JSON.stringify({ type: 'session', payload: sessionIdRef.current, session_id: sessionIdRef.current }))
      if (!flushPendingChat(ws)) void refreshBackendSessions()
    }
    ws.onclose = () => {
      if (wsRef.current !== ws) return
      setConnected(false)
      wsRef.current = null
      const activeRun = runStatusRef.current
      if (!pendingChatRef.current && (thinkingRef.current || (activeRun && !isTerminalRunState(activeRun.state)))) {
        const prompt = (activeRun?.prompt || '').trim()
        if (prompt) {
          pendingChatRef.current = {
            payload: prompt,
            mode: activeRun?.mode || mode,
            sessionId: sessionIdRef.current,
            runID: activeRun?.runID || createClientRunID(),
          }
        }
        setThinking(true)
        setRunStatus(prev => prev ? {
          ...prev,
          state: 'waiting',
          updatedAt: Date.now(),
          lastEvent: 'backend_rebuilding',
          lastMessage: prompt
            ? 'Backend sedang rebuilding/restart. Smara Web menunggu backend online lalu menjalankan ulang request otomatis.'
            : 'Backend sedang rebuilding/restart. Smara Web menunggu backend online kembali.',
          currentTool: undefined,
        } : {
          state: 'waiting',
          startedAt: Date.now(),
          updatedAt: Date.now(),
          prompt,
          mode,
          lastEvent: 'backend_rebuilding',
          lastMessage: 'Backend sedang rebuilding/restart. Smara Web menunggu backend online kembali.',
          toolCount: 0,
          heartbeatCount: 0,
        })
        pushAnalysisEvent({
          kind: 'log',
          title: 'Backend rebuilding',
          detail: prompt
            ? 'Koneksi terputus saat backend rebuild/restart. Request akan dijalankan ulang otomatis setelah backend online.'
            : 'Koneksi terputus saat backend rebuild/restart. Menunggu backend online.',
          status: 'running',
          level: 'warning',
          event: 'backend_rebuilding',
        })
      }
      if (!closingWsRef.current) reconnectTimer.current = setTimeout(connectWs, 3000)
    }
    ws.onerror = () => {
      if (wsRef.current !== ws) return
      setConnected(false)
    }

    ws.onmessage = (event) => {
      const msg = JSON.parse(event.data)
      if (msg.session_id && msg.session_id !== sessionIdRef.current && msg.type !== 'session_status') return
      const messageRunID = String(msg.run_id || msg.args?.run_id || '')
      if (messageRunID && activeRunIDRef.current && messageRunID !== activeRunIDRef.current) return
      switch (msg.type) {
        case 'connected':
          break
        case 'thinking':
          setThinking(msg.payload === 'true')
          if (msg.payload === 'true') {
            streamBufferRef.current = ''
            generatingPhaseActiveRef.current = false
            setAnalysisFilter('all')
            setRunStatus(prev => ({
              state: 'starting',
              startedAt: prev?.startedAt || Date.now(),
              updatedAt: Date.now(),
              prompt: prev?.prompt || '',
              mode: prev?.mode || mode,
              lastEvent: 'request',
              lastMessage: 'Request diterima.',
              logPath: prev?.logPath,
              toolCount: prev?.toolCount || 0,
              heartbeatCount: prev?.heartbeatCount || 0,
            }))
            setAnalysisEvents([{
              id: analysisID('phase'),
              kind: 'phase',
              title: 'Request diterima',
              detail: 'Menyiapkan konteks, mode, dan koneksi sesi.',
              status: 'running',
              level: 'info',
              event: 'request',
              timestamp: new Date(),
            }])
            if (!spinnerTimer.current) {
              let tickCounter = 0
              spinnerTimer.current = setInterval(() => {
                setSpinnerIdx(i => (i + 1) % spinnerFrames.length)
                tickCounter++
                if (tickCounter % 12 === 0) setElapsedTick(t => t + 1)
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
          setRunStatus(prev => prev ? {
            ...prev,
            state: 'completed',
            updatedAt: Date.now(),
            lastEvent: 'run_complete',
            lastMessage: 'Response final diterima.',
            currentTool: undefined,
          } : prev)
          const payloadText = String(msg.payload || '')
          const parsed = parsePlanQuest(payloadText)
          setActivePlanQuest(parsed.quest)
          const contentSource = parsed.cleanContent.trim()
            ? parsed.cleanContent
            : (!parsed.quest && payloadText.trim() ? payloadText : streamBufferRef.current)
          const content = contentSource.trim()
          if (content.trim()) {
            markAutoScrollIfNearBottom()
            const finalMessage: ChatMessage = {
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
            }
            setMessages(prev => {
              if (streamingAssistantRef.current) {
                const idx = prev.map(m => m.role).lastIndexOf('assistant')
                if (idx >= 0) {
                  const next = [...prev]
                  next[idx] = finalMessage
                  return capRuntimeMessages(next)
                }
              }
              const normalizedFinal = content.trim().replace(/\s+/g, ' ')
              for (let i = prev.length - 1; i >= 0; i--) {
                const candidate = prev[i]
                if (candidate.role !== 'assistant') continue
                const normalizedCandidate = candidate.content.trim().replace(/\s+/g, ' ')
                if (
                  normalizedCandidate === normalizedFinal ||
                  normalizedFinal.startsWith(normalizedCandidate) ||
                  normalizedCandidate.startsWith(normalizedFinal)
                ) {
                  const next = [...prev]
                  next[i] = finalMessage
                  return capRuntimeMessages(next)
                }
              }
              return capRuntimeMessages([...prev, finalMessage])
            })
          }
          streamingAssistantRef.current = false
          streamBufferRef.current = ''
          generatingPhaseActiveRef.current = false
          break
        }
        case 'error':
          setThinking(false)
          if (spinnerTimer.current) { clearInterval(spinnerTimer.current); spinnerTimer.current = null }
          setActivePhases([])
          setRunStatus(prev => prev ? {
            ...prev,
            state: 'error',
            updatedAt: Date.now(),
            lastEvent: 'error',
            lastMessage: String(msg.payload || 'Proses gagal.'),
            currentTool: undefined,
          } : prev)
          streamingAssistantRef.current = false
          generatingPhaseActiveRef.current = false
          markAutoScrollIfNearBottom()
          setMessages(prev => capRuntimeMessages([...prev, { role: 'error', content: msg.payload, timestamp: new Date() }]))
          break
        case 'stream': {
          const isThinking = Boolean(msg.args?.is_thinking)
          const chunk = String(msg.payload || '')
          if (!chunk) break
          if (isThinking) {
            markAutoScrollIfNearBottom()
            appendThinkingAnalysis(chunk)
            break
          }
          const wasStreaming = streamingAssistantRef.current
          streamBufferRef.current += chunk
          streamingAssistantRef.current = true
          markAutoScrollIfNearBottom()
          setMessages(prev => {
            const next = [...prev]
            if (wasStreaming) {
              for (let i = next.length - 1; i >= 0; i--) {
                if (next[i].role === 'assistant') {
                  next[i] = { ...next[i], content: streamBufferRef.current, timestamp: new Date() }
                  return capRuntimeMessages(next)
                }
              }
            }
            next.push({ role: 'assistant', content: streamBufferRef.current, timestamp: new Date() })
            return capRuntimeMessages(next)
          })
          if (!generatingPhaseActiveRef.current) {
            generatingPhaseActiveRef.current = true
            // Saat token jawaban mulai mengalir, pastikan panel proses menampilkan
            // tahap Generating. Jangan update state untuk setiap token karena itu
            // membuat React render berlebihan pada respons panjang.
            pushAnalysisEvent({
              kind: 'phase',
              title: 'Generating',
              detail: 'Composing final response...',
              status: 'running',
              level: 'info',
              event: 'stream',
            })
            setActivePhases(prev => { const _now = Date.now()
              const next: ActivePhase[] = prev.map(p => ({ ...p, status: 'done' }))
              const idx = next.findIndex(p => p.phase === 'Generating')
              if (idx >= 0) {
                next[idx] = { phase: 'Generating', description: 'Composing final response...', status: 'running', startedAt: _now }
              } else {
                next.push({ phase: 'Generating', description: 'Composing final response...', status: 'running', startedAt: _now })
              }
              return next.slice(-12)
            })
          }
          break
        }
        case 'tool_call':
          if (msg.tool === 'parallel_orchestration' || msg.tool === 'agent_swarm_workflow') {
            openParallelAgentPanel()
          }
          markAutoScrollIfNearBottom()
          setRunStatus(prev => prev ? {
            ...prev,
            state: 'running',
            updatedAt: Date.now(),
            lastEvent: 'tool_start',
            lastMessage: `Menjalankan ${msg.tool || 'tool'}.`,
            currentTool: msg.tool || 'tool',
            toolCount: prev.toolCount + 1,
          } : prev)
          pushAnalysisEvent({
            kind: 'tool',
            title: `Tool: ${msg.tool || 'tool'}`,
            detail: msg.server ? `Server ${msg.server}` : 'Menjalankan tool.',
            status: 'running',
            level: 'info',
            event: 'tool_start',
            tool: msg.tool || 'tool',
          })
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
          setRunStatus(prev => prev ? {
            ...prev,
            state: 'waiting',
            updatedAt: Date.now(),
            lastEvent: 'tool_done',
            lastMessage: 'Tool selesai, menunggu langkah berikutnya.',
            currentTool: undefined,
          } : prev)
          completeAnalysisEvent('tool', String(msg.output || msg.content || 'Tool selesai.'))
          setMessages(prev => {
            const next = [...prev]
            for (let i = next.length - 1; i >= 0; i--) {
              if (next[i].role === 'tool_call' && next[i].status === 'running') {
                next[i] = {
                  ...next[i],
                  status: 'done',
                  output: msg.output || msg.content,
                  content: next[i].content || msg.tool || 'tool',
                }
                return capRuntimeMessages(next)
              }
            }
            return capRuntimeMessages([...next, {
              role: 'tool_result',
              content: msg.output || msg.content || '',
              output: msg.output || msg.content,
              timestamp: new Date(),
            }])
          })
          // Setelah tool selesai, agent biasanya masuk tahap menyusun jawaban akhir.
          // Tampilkan phase ini langsung agar user melihat proses "Generating" meskipun
          // event backend datang sangat cepat/berdekatan dengan final response.
          pushAnalysisEvent({
            kind: 'phase',
            title: 'Generating',
            detail: 'Reviewing tool results and composing final response...',
            status: 'running',
            level: 'info',
            event: 'tool_done',
          })
          setActivePhases(prev => { const _now = Date.now()
            const next: ActivePhase[] = prev.map(p => ({ ...p, status: 'done' as const }))
            const idx = next.findIndex(p => p.phase === 'Generating')
            if (idx >= 0) {
              next[idx] = { phase: 'Generating', description: 'Reviewing tool results and composing final response...', status: 'running', startedAt: _now }
            } else {
              next.push({ phase: 'Generating', description: 'Reviewing tool results and composing final response...', status: 'running', startedAt: _now })
            }
            return next.slice(-12)
          })
          break
        case 'process_log': {
          if (msg.payload !== undefined) {
            const role = String(msg.role || 'process').toLowerCase()
            const eventName = String(msg.args?.event || 'process_log')
            const level = String(msg.args?.level || role)
            const tool = String(msg.args?.tool || '')
            const logPath = String(msg.args?.log_path || '')
            const phase = String(msg.args?.phase || '')
            const details = (msg.args?.details && typeof msg.args.details === 'object') ? msg.args.details as Record<string, unknown> : {}
            const runID = stringFromDetail(msg.args?.run_id)
            const provider = stringFromDetail(details.provider)
            const modelName = stringFromDetail(details.model)
            const reasoningEffort = stringFromDetail(details.reasoning_effort)
            const customDisableStream = boolFromDetail(details.custom_disable_stream)
            const providerIdleMs = eventName === 'heartbeat' ? numberFromDetail(details.silence_ms) : undefined
            const heartbeatLastEvent = eventName === 'heartbeat' ? stringFromDetail(details.last_event) : undefined
            const logLevel: AnalysisEvent['level'] =
              level === 'error' || role === 'error' ? 'error'
              : level === 'warning' || role === 'warning' || eventName === 'heartbeat' || eventName === 'tool_timeout' ? 'warning'
              : 'info'
            setRunStatus(prev => {
              const base: RunStatus = prev || {
                state: 'starting',
                startedAt: Date.now(),
                updatedAt: Date.now(),
                prompt: '',
                mode,
                lastEvent: eventName,
                lastMessage: String(msg.payload),
                runID,
                toolCount: 0,
                heartbeatCount: 0,
              }
              let state = base.state
              if (level === 'error') state = 'error'
              else if (eventName === 'run_complete') state = 'completed'
              else if (eventName === 'run_cancelled') state = 'cancelled'
              else if (eventName === 'tool_start') state = 'running'
              else if (eventName === 'heartbeat') state = 'waiting'
              else if (eventName === 'phase' && phase === 'Waiting') state = 'waiting'
              else if (eventName === 'phase' || eventName === 'iteration') state = base.toolCount > 0 ? 'waiting' : 'thinking'
              return {
                ...base,
                state,
                updatedAt: Date.now(),
                lastEvent: eventName === 'heartbeat' ? base.lastEvent : eventName,
                lastMessage: String(msg.payload),
                runID: runID || base.runID,
                currentPhase: phase || base.currentPhase,
                currentTool: tool || base.currentTool,
                logPath: logPath || base.logPath,
                provider: provider || base.provider,
                model: modelName || base.model,
                reasoningEffort: reasoningEffort || base.reasoningEffort,
                customDisableStream: customDisableStream ?? base.customDisableStream,
                providerIdleMs: providerIdleMs ?? base.providerIdleMs,
                heartbeatLastEvent: heartbeatLastEvent || base.heartbeatLastEvent,
                heartbeatCount: eventName === 'heartbeat' ? base.heartbeatCount + 1 : base.heartbeatCount,
              }
            })
            pushAnalysisEvent({
              kind: eventName.startsWith('tool_') ? 'tool' : 'log',
              title: role === 'error' ? 'Error proses' : role === 'warning' ? 'Peringatan proses' : eventName.startsWith('tool_') ? 'Progress tool' : 'Log proses',
              detail: String(msg.payload),
              status: role === 'warning' ? 'running' : 'done',
              level: logLevel,
              event: eventName,
              tool: tool || undefined,
            })
          }
          break
        }
        case 'log':
          markAutoScrollIfNearBottom()
          if (msg.role !== 'Terminal' && msg.role !== 'terminal') {
            if (msg.payload !== undefined) {
              const role = String(msg.role || 'log').toLowerCase()
              pushAnalysisEvent({
                kind: 'log',
                title: role === 'error' ? 'Error proses' : role === 'warning' ? 'Peringatan proses' : role === 'process' ? 'Log proses' : msg.role || 'Log',
                detail: String(msg.payload),
                status: role === 'warning' ? 'running' : 'done',
                level: role === 'error' ? 'error' : role === 'warning' ? 'warning' : 'info',
                event: 'log',
              })
            }
            break
          }
          setMessages(prev => {
            const next = [...prev]
            // Terminal logs from run_command stream into the most recent
            // running tool_call card. Other logs (system / explore) stay
            // as standalone rows.
            if ((msg.role === 'Terminal' || msg.role === 'terminal') && msg.payload !== undefined) {
              for (let i = next.length - 1; i >= 0; i--) {
                if (next[i].role === 'tool_call' && next[i].status === 'running') {
                  const appendToLast = Boolean(msg.args?.stream_append) || msg.args?.event === 'task_stream'
                  const logs = appendRuntimeLog(next[i].logs || [], String(msg.payload), appendToLast)
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
          if (msg.session_id === sessionIdRef.current) {
            if (msg.payload === 'running' && messageRunID) {
              activeRunIDRef.current = messageRunID
              setRunStatus(prev => prev ? {
                ...prev,
                state: 'starting',
                updatedAt: Date.now(),
                runID: messageRunID,
                lastEvent: 'running',
                lastMessage: 'Proses baru berjalan.',
              } : prev)
            }
            setCurrentRaw(c => ({ ...c, status: msg.payload as WebSessionStatus }))
            if (msg.payload === 'completed' || msg.payload === 'cancelled' || msg.payload === 'error') {
              if (streamingAssistantRef.current && streamBufferRef.current.trim()) {
                const content = streamBufferRef.current.trim()
                setMessages(prev => {
                  const next = [...prev]
                  for (let i = next.length - 1; i >= 0; i--) {
                    if (next[i].role === 'assistant') {
                      next[i] = { ...next[i], content, timestamp: new Date() }
                      return capRuntimeMessages(next)
                    }
                  }
                  return capRuntimeMessages([...next, { role: 'assistant', content, timestamp: new Date() }])
                })
              }
              setThinking(false)
              setActivePhases([])
              setRunStatus(prev => prev ? {
                ...prev,
                state: msg.payload === 'completed' ? 'completed' : msg.payload === 'cancelled' ? 'cancelled' : 'error',
                updatedAt: Date.now(),
                lastEvent: String(msg.payload),
                currentTool: undefined,
              } : prev)
              streamingAssistantRef.current = false
              generatingPhaseActiveRef.current = false
              if (spinnerTimer.current) { clearInterval(spinnerTimer.current); spinnerTimer.current = null }
            }
          }
          break
        case 'mode': {
          const nextMode = msg.mode || 'ask'
          setMode(nextMode)
          if (nextMode === 'parallel') openParallelAgentPanel()
          break
        }
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
  }, [appendThinkingAnalysis, completeAnalysisEvent, flushPendingChat, interruptLocalRun, markAutoScrollIfNearBottom, mode, openParallelAgentPanel, pushAnalysisEvent, refreshBackendSessions])

  useEffect(() => {
    connectWs()
    return () => {
      closingWsRef.current = true
      if (reconnectTimer.current) clearTimeout(reconnectTimer.current)
      reconnectTimer.current = null
      if (spinnerTimer.current) clearInterval(spinnerTimer.current)
      spinnerTimer.current = null
      wsRef.current?.close()
    }
  }, [connectWs])

  useEffect(() => {
    fetch('/api/voice/settings')
      .then(r => r.ok ? r.json() : null)
      .then(setVoiceSettings)
      .catch(() => {})
  }, [])

  const showToast = useCallback((msg: string) => {
    setToast(msg)
    window.setTimeout(() => setToast(null), 2500)
  }, [])

  const speakVoice = useCallback(async (text: string) => {
    const clean = text.trim()
    if (!clean) return
    const speakBrowserFallback = (reason?: string) => {
      if (!('speechSynthesis' in window)) {
        showToast(reason || 'Gagal membuat voice')
        setVoiceSpeaking(false)
        return
      }
      window.speechSynthesis.cancel()
      const utter = new SpeechSynthesisUtterance(clean)
      utter.lang = voiceSettings?.language || 'id-ID'
      utter.rate = voiceSettings?.speed || 1
      utter.volume = voiceSettings?.volume || 1
      utter.onend = () => setVoiceSpeaking(false)
      setVoiceSpeaking(true)
      window.speechSynthesis.speak(utter)
      if (reason) showToast(`${reason} — fallback ke browser voice`)
    }
    if ((voiceSettings?.provider || 'browser') !== 'elevenlabs') {
      speakBrowserFallback()
      return
    }
    try {
      voiceAudioRef.current?.pause()
      voiceAudioRef.current = null
      setVoiceSpeaking(true)
      const res = await fetch('/api/voice/speak', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          text: clean,
          settings: {
            ...(voiceSettings || {}),
            provider: voiceSettings?.provider || 'elevenlabs',
            language: voiceSettings?.language || 'id-ID',
            voice_character: voiceSettings?.voice_character,
            model_id: voiceSettings?.model_id,
            speed: voiceSettings?.speed || 1,
            volume: voiceSettings?.volume || 1,
          },
        }),
      })
      if (!res.ok) {
        setVoiceSpeaking(false)
        speakBrowserFallback(await res.text())
        return
      }
      const contentType = res.headers.get('Content-Type') || ''
      if (!contentType.toLowerCase().startsWith('audio/')) {
        setVoiceSpeaking(false)
        speakBrowserFallback(await res.text().catch(() => `Response bukan audio (${contentType || 'unknown content-type'})`))
        return
      }
      const blob = await res.blob()
      const url = URL.createObjectURL(blob)
      const audio = new Audio(url)
      voiceAudioRef.current = audio
      audio.onended = () => { setVoiceSpeaking(false); URL.revokeObjectURL(url); voiceAudioRef.current = null }
      audio.onerror = () => { setVoiceSpeaking(false); URL.revokeObjectURL(url); voiceAudioRef.current = null; speakBrowserFallback('Gagal memutar voice ElevenLabs') }
      await audio.play()
    } catch (e) {
      setVoiceSpeaking(false)
      speakBrowserFallback(e instanceof Error ? e.message : 'Gagal membuat voice ElevenLabs')
    }
  }, [showToast, voiceSettings])

  useEffect(() => {
    if (mode !== 'voice' || thinking || messages.length === 0) return
    const latest = messages[messages.length - 1]
    if (latest.role !== 'assistant') return
    const text = latest.content.trim()
    if (!text || lastVoiceSpokenRef.current === text) return
    lastVoiceSpokenRef.current = text
    void speakVoice(text)
  }, [messages, mode, speakVoice, thinking])
  const send = useCallback(() => {
    const messageText = input.trim()
    if (!messageText && attachments.length === 0) return
    const activeRun = runStatusRef.current
    if (thinkingRef.current || (activeRun && !isTerminalRunState(activeRun.state))) return
    const agentPayload = buildPromptWithAttachments(messageText, attachments)
    const wantsRoadmapProgress = isRoadmapProgressPrompt(messageText)
    const userMessage: ChatMessage = {
      role: 'user',
      content: messageText,
      timestamp: new Date(),
      attachments: attachments.map(a => ({ path: a.path, size: a.size, kind: a.kind, name: a.name, preview: a.preview })),
    }

    if (mode === 'parallel') openParallelAgentPanel()
    shouldAutoScrollRef.current = true
    setMessages(prev => capRuntimeMessages([...prev, userMessage]))
    setInput('')
    setAttachments([])
    setActivePlanQuest(null)
    if (wantsRoadmapProgress) {
      setShowRoadmapPopup(true)
      setShowSessions(false)
    }
    setAnalysisEvents([])
    setAnalysisFilter('all')
    streamingAssistantRef.current = false
    streamBufferRef.current = ''
    const runID = createClientRunID()
    activeRunIDRef.current = runID
    setRunStatus({
      state: 'starting',
      startedAt: Date.now(),
      updatedAt: Date.now(),
      prompt: messageText,
      mode,
      lastEvent: 'queued',
      lastMessage: 'Menunggu koneksi proses.',
      runID,
      toolCount: 0,
      heartbeatCount: 0,
    })

    if (!wsRef.current || wsRef.current.readyState !== WebSocket.OPEN) {
      queuePendingChat(agentPayload, mode, runID)
      connectWs()
      return
    }

    wsRef.current.send(JSON.stringify({ type: 'chat', payload: agentPayload, mode, session_id: sessionIdRef.current, run_id: runID }))
    setThinking(true)
  }, [input, attachments, connectWs, mode, openParallelAgentPanel, queuePendingChat])

  const sendPlanQuestAnswer = useCallback((answer: string) => {
    const text = `Saya pilih: ${answer}`
    setInput('')
    setActivePlanQuest(null)
    setAnalysisEvents([])
    setAnalysisFilter('all')
    streamingAssistantRef.current = false
    streamBufferRef.current = ''
    const runID = createClientRunID()
    activeRunIDRef.current = runID
    setRunStatus({
      state: 'starting',
      startedAt: Date.now(),
      updatedAt: Date.now(),
      prompt: text,
      mode,
      lastEvent: 'queued',
      lastMessage: 'Menunggu koneksi proses.',
      runID,
      toolCount: 0,
      heartbeatCount: 0,
    })
    shouldAutoScrollRef.current = true
    setMessages(prev => capRuntimeMessages([...prev, { role: 'user', content: text, timestamp: new Date() }]))
    if (!wsRef.current || wsRef.current.readyState !== WebSocket.OPEN) {
      queuePendingChat(text, mode, runID)
      connectWs()
      return
    }
    wsRef.current.send(JSON.stringify({ type: 'chat', payload: text, mode, session_id: sessionIdRef.current, run_id: runID }))
    setThinking(true)
  }, [connectWs, mode, queuePendingChat])

  const sendPlanApproval = useCallback((approved: boolean) => {
    const text = approved ? 'Ya, lanjutkan eksekusi rencana.' : 'Tidak, jangan lanjutkan dulu. Revisi rencana sebelum eksekusi.'
    setInput('')
    setActivePlanQuest(null)
    setAnalysisEvents([])
    setAnalysisFilter('all')
    streamingAssistantRef.current = false
    streamBufferRef.current = ''
    const runID = createClientRunID()
    activeRunIDRef.current = runID
    setRunStatus({
      state: 'starting',
      startedAt: Date.now(),
      updatedAt: Date.now(),
      prompt: text,
      mode,
      lastEvent: 'queued',
      lastMessage: 'Menunggu koneksi proses.',
      runID,
      toolCount: 0,
      heartbeatCount: 0,
    })
    shouldAutoScrollRef.current = true
    setMessages(prev => capRuntimeMessages([...prev, { role: 'user', content: text, timestamp: new Date() }]))
    if (!wsRef.current || wsRef.current.readyState !== WebSocket.OPEN) {
      queuePendingChat(text, mode, runID)
      connectWs()
      return
    }
    wsRef.current.send(JSON.stringify({ type: 'chat', payload: text, mode, session_id: sessionIdRef.current, run_id: runID }))
    setThinking(true)
  }, [connectWs, mode, queuePendingChat])

  const approvePlan = useCallback(() => sendPlanApproval(true), [sendPlanApproval])
  const rejectPlan = useCallback(() => sendPlanApproval(false), [sendPlanApproval])

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault()
      send()
    }
  }

  const activateCachedRoadmap = useCallback((item: CachedRoadmap) => {
    const insight = parsePlanInsight(item.content)
    if (!insight) {
      showToast('Cache roadmap ada, tapi format phase belum dikenali')
      return
    }
    const path = item.relativePath || item.path
    setLoadedRoadmap(insight)
    setLoadedRoadmapPath(path)
    setRoadmapPath(path)
    setActiveRoadmapPath(path)
    if (!roadmapGoal.trim()) setRoadmapGoal(insight.title || item.name)
    setCachedRoadmaps(prev => {
      const next = saveCachedRoadmaps(upsertCachedRoadmap(prev, { ...item, cachedAt: new Date().toISOString() }))
      return next
    })
    showToast(`Roadmap aktif: ${path}`)
  }, [roadmapGoal])

  const clearRoadmapCache = useCallback(() => {
    try { localStorage.removeItem(ROADMAP_CACHE_KEY) } catch { /* ignore */ }
    setCachedRoadmaps([])
    setActiveRoadmapPath('')
    showToast('Cache roadmap dibersihkan')
  }, [])

  const createRoadmapDraft = useCallback(() => {
    const goal = roadmapGoal.trim()
    if (!goal) return
    const context = roadmapContext.trim()
    const draft = [
      `Buat Roadmap Plan untuk: ${goal}`,
      '',
      loadedRoadmapPath ? `Roadmap aktif yang sudah dicek: ${loadedRoadmapPath}` : '',
      loadedRoadmapPath ? '' : '',
      'Konteks dan batasan:',
      context || '- Tidak ada konteks tambahan.',
      '',
      'Susun response dalam format Plan Mode berikut:',
      '- Context: problem, alasan perubahan, dan outcome yang dituju.',
      '- Assumptions / open questions: asumsi dan hal yang perlu diputuskan.',
      '- Recommended approach: satu pendekatan yang direkomendasikan.',
      '- Roadmap table: tabel markdown dengan kolom No, Langkah, Output, Status.',
      '- Flow diagram: blok mermaid flowchart yang menggambarkan alur roadmap.',
      '- Steps: langkah implementasi berurutan.',
      '- Files/tools likely needed: file, command, atau tool yang kemungkinan dipakai.',
      '- Verification: cara menguji end-to-end.',
      '- Risks / rollback: risiko utama dan cara mitigasi.',
      '',
      'Akhiri dengan approval quest SMARA_PLAN_QUEST untuk tombol Lanjutkan/Tidak.',
    ].join('\n')
    setModeAndNotify('plan')
    setInput(draft)
    setActivePlanQuest(null)
    setShowRoadmapPopup(false)
    showToast('Draft Roadmap Plan siap diedit')
  }, [loadedRoadmapPath, roadmapContext, roadmapGoal, setModeAndNotify])

  const loadRoadmapFile = useCallback(async () => {
    const path = roadmapPath.trim()
    if (!path) return
    setLoadingRoadmap(true)
    try {
      const res = await fetchRoadmapFile(path)
      const insight = parsePlanInsight(res.content)
      if (!insight) {
        showToast('Roadmap terbaca, tapi format phase belum dikenali')
        setLoadedRoadmap(null)
        setLoadedRoadmapPath(res.relative_path || res.path)
        return
      }
      setLoadedRoadmap(insight)
      const cached: CachedRoadmap = {
        path: res.path,
        relativePath: res.relative_path || res.path,
        name: res.name,
        content: res.content,
        size: res.size,
        updatedAt: res.updated_at,
        cachedAt: new Date().toISOString(),
        workspace: res.workspace,
      }
      setLoadedRoadmapPath(cached.relativePath)
      setActiveRoadmapPath(cached.relativePath)
      if (res.content.length <= MAX_CACHED_ROADMAP_BYTES) {
        setCachedRoadmaps(prev => saveCachedRoadmaps(upsertCachedRoadmap(prev, cached)))
      } else {
        showToast('Roadmap loaded, tapi terlalu besar untuk cache lokal')
      }
      if (!roadmapGoal.trim()) setRoadmapGoal(insight.title || res.name)
      showToast(`Roadmap loaded: ${res.relative_path || res.name}`)
    } catch (err) {
      showToast(`Load roadmap gagal: ${err instanceof Error ? err.message : 'unknown'}`)
    } finally {
      setLoadingRoadmap(false)
    }
  }, [roadmapGoal, roadmapPath])

  const imageFileToFastDataUrl = useCallback(async (file: File): Promise<string> => {
    const raw = await new Promise<string>((resolve, reject) => {
      const reader = new FileReader()
      reader.onload = () => resolve(String(reader.result))
      reader.onerror = () => reject(reader.error)
      reader.readAsDataURL(file)
    })
    // Small images are cheap enough; keep original for fidelity.
    if (file.size <= 900 * 1024) return raw

    // Large screenshots/photos make chat feel slow because base64 upload +
    // preview state becomes huge. Downscale client-side before upload.
    try {
      const img = await new Promise<HTMLImageElement>((resolve, reject) => {
        const el = new Image()
        el.onload = () => resolve(el)
        el.onerror = reject
        el.src = raw
      })
      const maxSide = 1600
      const scale = Math.min(1, maxSide / Math.max(img.width, img.height))
      if (scale >= 1) return raw
      const canvas = document.createElement('canvas')
      canvas.width = Math.max(1, Math.round(img.width * scale))
      canvas.height = Math.max(1, Math.round(img.height * scale))
      const ctx = canvas.getContext('2d')
      if (!ctx) return raw
      ctx.drawImage(img, 0, 0, canvas.width, canvas.height)
      return canvas.toDataURL('image/jpeg', 0.82)
    } catch {
      return raw
    }
  }, [])

  const uploadImageDataUrl = useCallback(async (dataUrl: string, name = 'pasted-image.png') => {
    setUploading(true)
    try {
      const res = await uploadClipboardImage(dataUrl)
      setAttachments(prev => [...prev, {
        path: res.path,
        size: res.size,
        kind: 'image',
        name,
        preview: dataUrl,
        mime: res.mime || 'image/png',
      }])
      showToast(`📎 ${(res.size / 1024).toFixed(0)} KB → ${res.path.split('/').pop()}`)
    } catch (err) {
      showToast(`Upload gagal: ${err instanceof Error ? err.message : 'unknown'}`)
    } finally {
      setUploading(false)
    }
  }, [])

  const uploadFile = useCallback(async (file: File) => {
    setUploading(true)
    try {
      if (file.type.startsWith('image/')) {
        const dataUrl = await imageFileToFastDataUrl(file)
        const res = await uploadClipboardImage(dataUrl)
        setAttachments(prev => [...prev, {
          path: res.path,
          size: res.size,
          kind: 'image',
          name: file.name || 'pasted-image.png',
          preview: dataUrl,
          mime: res.mime || file.type,
        }])
        showToast(`📎 ${(res.size / 1024).toFixed(0)} KB → ${res.path.split('/').pop()}`)
        return
      }
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
  }, [imageFileToFastDataUrl])

  const uploadFiles = useCallback(async (files: FileList | File[]) => {
    for (const file of Array.from(files)) await uploadFile(file)
  }, [uploadFile])

  const handlePaste = useCallback(async (e: React.ClipboardEvent<HTMLTextAreaElement>) => {
    const items = Array.from(e.clipboardData?.items || [])
    const imageItem = items.find(item => item.kind === 'file' && item.type.startsWith('image/'))
    if (imageItem) {
      const file = imageItem.getAsFile()
      if (file) {
        e.preventDefault()
        await uploadFile(file)
        return
      }
    }
    const html = e.clipboardData?.getData('text/html') || ''
    const match = html.match(/<img[^>]+src=["'](data:image\/[^"']+;base64,[^"']+)["']/i)
    if (match?.[1]) {
      e.preventDefault()
      await uploadImageDataUrl(match[1].replace(/&amp;/g, '&'))
    }
  }, [uploadFile, uploadImageDataUrl])

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

  const copyMessage = useCallback(async (idx: number, text: string) => {
    try {
      await navigator.clipboard.writeText(text)
      setCopiedIdx(idx)
      showToast('Pesan disalin')
      window.setTimeout(() => setCopiedIdx(c => (c === idx ? null : c)), 1500)
    } catch {
      showToast('Browser menolak akses clipboard')
    }
  }, [showToast])

  const toggleCard = useCallback((idx: number) => {
    setMessages(prev => {
      const next = [...prev]
      if (next[idx]) {
        next[idx] = { ...next[idx], collapsed: !next[idx].collapsed }
      }
      return next
    })
  }, [])

  const providerWaitActive = !!runStatus && runStatus.toolCount === 0 && runStatus.state === 'waiting'
  const providerIdleMs = runStatus?.providerIdleMs || 0
  const providerIdleText = providerIdleMs > 0 ? formatElapsed(providerIdleMs) : ''
  const providerLabel = [runStatus?.provider, runStatus?.model].filter(Boolean).join(' / ')
  const streamModeLabel = runStatus?.customDisableStream === true
    ? 'stream off'
    : runStatus?.customDisableStream === false
    ? 'stream on'
    : undefined
  const visibleAnalysisEvents = analysisEvents.filter(event => analysisEventMatchesFilter(event, analysisFilter))
  const latestToolEvent = [...analysisEvents].reverse().find(event => event.kind === 'tool' || event.tool)
  const idleRisk = providerIdleMs >= 180000 || (runStatus?.heartbeatCount || 0) >= 2
  const latestAssistantIndex = (() => {
    for (let idx = messages.length - 1; idx >= 0; idx--) {
      if (messages[idx].role === 'assistant') return idx
    }
    return -1
  })()
  const roadmapFocusIndex = requestedRoadmapStepIndex(messages)
  const latestMessageRoadmapInsight = latestPlanInsightFromMessages(messages)
  const latestRoadmapInsight = roadmapFocusIndex !== null
    ? latestMessageRoadmapInsight || loadedRoadmap
    : loadedRoadmap || latestMessageRoadmapInsight

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
        <div className="flex min-w-0 items-center justify-end gap-2">
          <button
            onClick={() => {
              setShowRoadmapPopup(v => !v)
              setShowSessions(false)
            }}
            className={`inline-flex items-center gap-1.5 rounded-2xl border px-3 py-2 text-xs font-semibold shadow-lg shadow-smara-950/10 transition-colors md:px-3.5 ${
              showRoadmapPopup
                ? 'border-smara-300/45 bg-smara-300/14 text-smara-100'
                : 'border-[#5f7446]/35 bg-[#26331d]/72 text-smara-200 hover:bg-[#2f3f23]'
            }`}
            title="Buat draft Roadmap Plan"
          >
            <ClipboardList className="w-3 h-3" /> <span className="hidden md:inline">Roadmap Plan</span>
          </button>
          <button
            onClick={newSession}
            className="flex items-center gap-1.5 rounded-2xl bg-smara-300 px-3.5 py-2 text-xs font-semibold text-black shadow-lg shadow-smara-950/20 transition-colors hover:bg-smara-200"
          >
            <Plus className="w-3 h-3" /> Sesi Baru
          </button>
          <button
            onClick={() => { if (!connected) connectWs() }}
            disabled={connected}
            className={`inline-flex shrink-0 items-center gap-1.5 rounded-full border px-2.5 py-1.5 text-xs transition-colors ${
              connected && health.providerOnline
                ? 'cursor-default border-emerald-400/20 bg-emerald-400/10 text-emerald-300'
                : connected
                  ? 'cursor-default border-amber-400/25 bg-amber-500/10 text-amber-200'
                  : 'border-red-400/25 bg-red-500/10 text-red-200 hover:bg-red-500/16'
            }`}
            title={!connected ? 'Hubungkan ulang ke backend' : health.providerOnline ? 'Backend dan LLM terhubung' : `Backend terhubung, LLM ${health.provider || 'provider'} offline`}
          >
            <span className={`w-1.5 h-1.5 rounded-full ${connected && health.providerOnline ? 'bg-emerald-400 shadow-[0_0_10px_rgba(52,211,153,.9)]' : connected ? 'bg-amber-400' : 'bg-red-400'}`} />
            {!connected ? <><RefreshCw className="h-3 w-3" /> Backend offline</> : health.providerOnline === null ? 'Checking LLM' : health.providerOnline ? 'Ready' : 'LLM offline'}
          </button>
        </div>
      </div>

      {/* Roadmap plan popup */}
      {showRoadmapPopup && (
        <div className="absolute right-3 top-[72px] z-50 max-h-[calc(100vh-96px)] w-[min(420px,calc(100vw-1.5rem))] overflow-hidden rounded-2xl border border-[#31421f]/70 bg-[#202b18] shadow-2xl shadow-black/35 ring-1 ring-black/45">
          <div className="flex items-center justify-between border-b border-[#31421f]/60 px-4 py-3">
            <div className="flex items-center gap-2 text-sm font-semibold text-gray-100">
              <ClipboardList className="h-4 w-4 text-smara-300" />
              Roadmap Plan
            </div>
            <button
              onClick={() => setShowRoadmapPopup(false)}
              className="inline-flex h-7 w-7 items-center justify-center rounded-lg text-neutral-400 transition-colors hover:bg-[#26331d] hover:text-gray-100"
              title="Tutup"
              aria-label="Tutup Roadmap Plan"
            >
              <X className="h-3.5 w-3.5" />
            </button>
          </div>
          <div className="max-h-[calc(100vh-154px)] space-y-3 overflow-y-auto p-4">
            <div className="rounded-xl border border-[#31421f]/45 bg-[#1a2314]/58 p-2">
              <div className="mb-3 rounded-lg border border-[#31421f]/40 bg-[#20291a]/54 p-2">
                <div className="mb-1.5 flex items-center justify-between gap-2">
                  <span className="text-[11px] font-medium uppercase tracking-wide text-neutral-500">Cached roadmap</span>
                  {cachedRoadmaps.length > 0 && (
                    <button
                      onClick={clearRoadmapCache}
                      className="inline-flex items-center gap-1 rounded-md border border-[#5f7446]/30 px-2 py-1 text-[10px] text-neutral-400 transition-colors hover:bg-[#26331d] hover:text-gray-200"
                    >
                      <Trash2 className="h-3 w-3" />
                      Clear
                    </button>
                  )}
                </div>
                {cachedRoadmaps.length > 0 ? (
                  <select
                    value={activeRoadmapPath || loadedRoadmapPath || ''}
                    onChange={e => {
                      const selected = cachedRoadmaps.find(item => (item.relativePath || item.path) === e.target.value)
                      if (selected) activateCachedRoadmap(selected)
                    }}
                    className="w-full rounded-lg border border-[#31421f]/60 bg-[#1a2314]/82 px-2.5 py-2 font-mono text-[11px] text-gray-100 outline-none transition-colors focus:border-smara-300/45"
                  >
                    {cachedRoadmaps.map(item => {
                      const value = item.relativePath || item.path
                      return <option key={value} value={value}>{cachedRoadmapLabel(item)}</option>
                    })}
                  </select>
                ) : (
                  <div className="rounded-lg border border-dashed border-[#31421f]/45 px-2.5 py-2 text-[11px] leading-5 text-neutral-500">
                    Belum ada cache. Load file roadmap untuk menyimpan dan memilih ulang nanti.
                  </div>
                )}
              </div>
              <div className="mb-1.5 text-[11px] font-medium uppercase tracking-wide text-neutral-500">Roadmap file</div>
              <div className="flex gap-2">
                <input
                  value={roadmapPath}
                  onChange={e => setRoadmapPath(e.target.value)}
                  onKeyDown={e => {
                    if (e.key === 'Enter' && roadmapPath.trim()) {
                      e.preventDefault()
                      loadRoadmapFile()
                    }
                  }}
                  placeholder="roadmap/parallel-task-orchestration.md"
                  className="min-w-0 flex-1 rounded-lg border border-[#31421f]/60 bg-[#20291a]/82 px-2.5 py-2 font-mono text-[11px] text-gray-100 outline-none transition-colors placeholder:text-neutral-500 focus:border-smara-300/45"
                />
                <button
                  onClick={loadRoadmapFile}
                  disabled={!roadmapPath.trim() || loadingRoadmap}
                  className="inline-flex items-center gap-1.5 rounded-lg bg-smara-300 px-3 py-2 text-xs font-semibold text-black transition-colors hover:bg-smara-200 disabled:cursor-not-allowed disabled:opacity-45"
                >
                  {loadingRoadmap ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <Upload className="h-3.5 w-3.5" />}
                  Load
                </button>
              </div>
              {loadedRoadmapPath && (
                <div className="mt-2 flex flex-wrap items-center gap-2">
                  <span className="truncate font-mono text-[10px] text-smara-200">{loadedRoadmapPath}</span>
                  {loadedRoadmap && (
                    <span className="inline-flex items-center gap-1 rounded-full bg-emerald-400/10 px-2 py-0.5 text-[10px] text-emerald-300">
                      <CheckCircle2 className="h-3 w-3" />
                      checked, {loadedRoadmap.steps.length} phase
                    </span>
                  )}
                </div>
              )}
            </div>
            {latestRoadmapInsight ? (
              <RoadmapProgressPanel
                insight={latestRoadmapInsight}
                activePhases={activePhases}
                runStatus={runStatus}
                focusIndex={roadmapFocusIndex}
              />
            ) : (
              <div className="rounded-xl border border-[#31421f]/45 bg-[#1a2314]/58 px-3 py-2 text-[11px] leading-5 text-neutral-400">
                Belum ada roadmap aktif di sesi ini. Buat draft roadmap baru dari form di bawah.
              </div>
            )}
            <label className="block space-y-1.5">
              <span className="text-[11px] font-medium uppercase tracking-wide text-neutral-500">Goal</span>
              <input
                value={roadmapGoal}
                onChange={e => setRoadmapGoal(e.target.value)}
                onKeyDown={e => {
                  if (e.key === 'Enter' && roadmapGoal.trim()) {
                    e.preventDefault()
                    createRoadmapDraft()
                  }
                }}
                placeholder="Contoh: bangun planner task parallel"
                className="w-full rounded-xl border border-[#31421f]/60 bg-[#1a2314]/82 px-3 py-2 text-sm text-gray-100 outline-none transition-colors placeholder:text-neutral-500 focus:border-smara-300/45"
              />
            </label>
            <label className="block space-y-1.5">
              <span className="text-[11px] font-medium uppercase tracking-wide text-neutral-500">Context</span>
              <textarea
                value={roadmapContext}
                onChange={e => setRoadmapContext(e.target.value)}
                placeholder="Batasan, target file, risiko, atau hal yang perlu diprioritaskan."
                className="min-h-[92px] w-full resize-none rounded-xl border border-[#31421f]/60 bg-[#1a2314]/82 px-3 py-2 text-sm leading-5 text-gray-100 outline-none transition-colors placeholder:text-neutral-500 focus:border-smara-300/45"
              />
            </label>
            <div className="rounded-xl border border-[#31421f]/45 bg-[#1a2314]/58 px-3 py-2 text-[11px] leading-5 text-neutral-400">
              Draft akan masuk ke composer, mode berubah ke Plan, dan belum dikirim sampai kamu tekan kirim.
            </div>
            <div className="flex items-center justify-end gap-2">
              <button
                onClick={() => setShowRoadmapPopup(false)}
                className="rounded-xl border border-[#5f7446]/35 bg-[#26331d]/72 px-3 py-2 text-xs text-gray-300 transition-colors hover:bg-[#2f3f23]"
              >
                Batal
              </button>
              <button
                onClick={createRoadmapDraft}
                disabled={!roadmapGoal.trim()}
                className="inline-flex items-center gap-1.5 rounded-xl bg-smara-300 px-3 py-2 text-xs font-semibold text-black transition-colors hover:bg-smara-200 disabled:cursor-not-allowed disabled:opacity-45"
              >
                <ClipboardList className="h-3.5 w-3.5" />
                Buat Draft
              </button>
            </div>
          </div>
        </div>
      )}

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
              <div key={i} className="ml-11 px-3 py-2 bg-[#20291a]/78 border border-[#223018]/75 rounded text-xs text-gray-400 font-mono whitespace-pre-wrap max-h-40 overflow-y-auto [content-visibility:auto] [contain-intrinsic-size:100px]">
                {msg.output || msg.content}
              </div>
            )
          }
          if (msg.role === 'log') {
            return (
              <div key={i} className="flex items-center gap-2 text-xs text-neutral-400 px-2 ml-11 [content-visibility:auto] [contain-intrinsic-size:28px]">
                <span className="text-neutral-500">&#9654;</span>
                <span>{msg.content}</span>
              </div>
            )
          }
          return (
            <div key={i} className={`flex gap-3 group [content-visibility:auto] [contain-intrinsic-size:180px] ${msg.role === 'user' ? 'flex-row-reverse' : ''}`}>
              <div className={`w-10 h-10 rounded-2xl flex items-center justify-center shrink-0 shadow-lg ring-1 ${
                msg.role === 'user'
                  ? 'bg-gradient-to-br from-smara-600 to-smara-800 ring-smara-400/30 shadow-smara-950/40'
                  : msg.role === 'error'
                  ? 'bg-gradient-to-br from-red-700 to-red-950 ring-red-400/30 shadow-red-950/40'
                  : 'bg-gradient-to-br from-[#4f6138] via-[#38482a] to-[#24301b] ring-smara-400/20 shadow-smara-950/40'
              }`}>
                {msg.role === 'user' ? <User className="w-4 h-4" /> : <Bot className="w-4 h-4 text-smara-200" />}
              </div>
              <div className={`relative overflow-hidden text-sm shadow-xl backdrop-blur-md transition-all duration-200 ${
                msg.role === 'user'
                  ? 'max-w-[90%] rounded-[1.45rem] bg-[#49751a]/96 px-5 py-4 leading-relaxed text-white shadow-smara-950/18 md:max-w-[76%]'
                  : msg.role === 'error'
                  ? 'max-w-[90%] rounded-[1.45rem] border border-red-600/40 bg-gradient-to-br from-red-950/70 to-neutral-950/60 px-5 py-4 leading-relaxed text-red-100 shadow-red-950/20 md:max-w-[76%]'
                  : 'max-w-[min(78ch,calc(100%-3.25rem))] rounded-xl bg-[#2b3522]/98 px-4 py-3 leading-6 text-gray-50 shadow-black/12'
              }`}>
                {msg.role !== 'user' && msg.role !== 'error' && (
                  <div className="mb-1.5 flex items-center gap-1.5 text-[9px] font-semibold uppercase tracking-[0.1em] text-smara-200/90">
                    <span className="h-1 w-1 rounded-full bg-smara-300/80" />
                    Smara
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
                  <AssistantMessageContent
                    msg={msg}
                    activePhases={activePhases}
                    runStatus={runStatus}
                    showApproval={mode === 'plan' && i === latestAssistantIndex && !thinking && !activePlanQuest}
                    onApprove={approvePlan}
                    onReject={rejectPlan}
                  />
                )}
                {(msg.role !== 'user' && msg.role !== 'error' && (msg.inputTokens !== undefined || msg.outputTokens !== undefined || msg.totalTokens !== undefined || msg.duration || msg.estimatedCostUSD !== undefined || msg.model || msg.provider)) ? (
                  <div className="mt-2.5 flex items-center justify-between gap-3 border-t border-neutral-800/60 pt-1.5 text-[10px] text-gray-400">
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
                  <div className="mt-2.5 flex justify-end border-t border-neutral-800/60 pt-1.5 text-[10px] text-gray-400">
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

        {runStatus && (thinking || !['completed', 'cancelled', 'error'].includes(runStatus.state)) && (
          <div className="flex gap-3">
            <div className="w-8 h-8 rounded-lg bg-gray-700 flex items-center justify-center shrink-0">
              <Bot className="w-4 h-4 text-smara-300" />
            </div>
            <div className="flex-1 max-w-3xl space-y-2">
              {/* Active phase stepper */}
              <div className="bg-[#20291a]/82 border border-[#223018]/75 rounded-lg p-3 shadow-lg shadow-black/15">
                <div className="mb-2 flex items-center justify-between gap-3">
                  <div className="flex items-center gap-2 text-[10px] text-neutral-400 uppercase tracking-wider font-medium">
                    <BrainCircuit className="w-3 h-3" />
                    Proses Berjalan
                  </div>
                  <div className="flex items-center gap-2">
                    <span className={`rounded-full border px-2 py-0.5 text-[10px] font-medium ${runStateClass(runStatus.state)}`}>
                      {runStateLabel(runStatus.state)}
                    </span>
                    <span className="text-[10px] text-smara-300 font-mono">{spinnerFrames[spinnerIdx]}</span>
                  </div>
                </div>

                <div className="mb-3 grid gap-2 rounded-lg border border-[#31421f]/50 bg-[#1a2314]/62 p-2.5 text-xs text-gray-300 md:grid-cols-[1fr_auto]">
                  <div className="min-w-0">
                    <div className="mb-1 flex flex-wrap items-center gap-1.5">
                      <span className="rounded bg-[#26331d]/85 px-1.5 py-0.5 text-[10px] uppercase tracking-wide text-smara-300">{runStatus.mode}</span>
                      <span className="rounded bg-[#26331d]/85 px-1.5 py-0.5 text-[10px] text-gray-300">
                        {formatElapsed(Date.now() - runStatus.startedAt)}
                      </span>
                      <span className="rounded bg-[#26331d]/85 px-1.5 py-0.5 text-[10px] text-gray-300">
                        tool {runStatus.toolCount}
                      </span>
                      {providerLabel && (
                        <span className="max-w-[220px] truncate rounded bg-[#26331d]/85 px-1.5 py-0.5 text-[10px] text-gray-300" title={providerLabel}>
                          {providerLabel}
                        </span>
                      )}
                      {runStatus.reasoningEffort && (
                        <span className="rounded bg-[#26331d]/85 px-1.5 py-0.5 text-[10px] text-gray-300">
                          reasoning {runStatus.reasoningEffort}
                        </span>
                      )}
                      {streamModeLabel && (
                        <span className="rounded bg-[#26331d]/85 px-1.5 py-0.5 text-[10px] text-gray-300">
                          {streamModeLabel}
                        </span>
                      )}
                      {runStatus.heartbeatCount > 0 && (
                        <span className="rounded border border-yellow-400/25 bg-yellow-500/10 px-1.5 py-0.5 text-[10px] text-yellow-200">
                          idle {runStatus.heartbeatCount}
                        </span>
                      )}
                    </div>
                    <div className="truncate text-gray-100" title={runStatus.prompt || undefined}>
                      {runStatus.currentTool
                        ? `Menjalankan ${runStatus.currentTool}`
                        : runStatus.toolCount === 0 && runStatus.state === 'waiting'
                        ? 'Menunggu respons provider/model'
                        : runStatus.toolCount === 0 && (runStatus.state === 'thinking' || runStatus.state === 'waiting')
                        ? 'Belum ada tool call dari model'
                        : runStatus.lastEvent === 'queued'
                        ? 'Menunggu proses dimulai'
                        : runStatus.lastEvent}
                    </div>
                    <div className="mt-1 max-h-10 overflow-hidden break-words text-[11px] leading-5 text-gray-400">
                      {runStatus.lastMessage}
                    </div>
                    <div className="mt-2 grid gap-1.5 text-[10px] text-neutral-400 sm:grid-cols-3">
                      <div className="rounded-md border border-[#31421f]/45 bg-[#20291a]/70 px-2 py-1">
                        <span className="text-neutral-500">Provider idle</span>
                        <div className={idleRisk ? 'font-mono text-yellow-200' : 'font-mono text-gray-300'}>
                          {providerIdleText || '0s'}
                        </div>
                      </div>
                      <div className="rounded-md border border-[#31421f]/45 bg-[#20291a]/70 px-2 py-1">
                        <span className="text-neutral-500">Current tool</span>
                        <div className="truncate text-gray-300" title={runStatus.currentTool || latestToolEvent?.tool || undefined}>
                          {runStatus.currentTool || latestToolEvent?.tool || '-'}
                        </div>
                      </div>
                      <div className="rounded-md border border-[#31421f]/45 bg-[#20291a]/70 px-2 py-1">
                        <span className="text-neutral-500">Last event</span>
                        <div className="truncate text-gray-300" title={runStatus.lastEvent}>
                          {runStatus.lastEvent}
                        </div>
                      </div>
                    </div>
                    {runStatus.logPath && (
                      <button
                        onClick={() => copyMessage(-1, runStatus.logPath || '')}
                        className="mt-1 max-w-full truncate font-mono text-[10px] text-smara-300 hover:text-smara-200"
                        title="Salin path log"
                      >
                        {runStatus.logPath}
                      </button>
                    )}
                  </div>
                  <div className="flex items-start justify-end gap-2">
                  {providerWaitActive && (
                    <button
                      onClick={openConfigTab}
                      className="inline-flex h-8 items-center justify-center gap-1.5 rounded-lg border border-[#5f7446]/35 bg-[#26331d]/72 px-2.5 text-[11px] font-medium text-smara-200 transition-colors hover:bg-[#2f3f23]"
                      title="Buka Config"
                    >
                      <Settings className="h-3.5 w-3.5" />
                      Config
                    </button>
                  )}
                  {thinking && (
                    <button
                      onClick={() => cancelSession(sessionId)}
                      className="inline-flex h-8 items-center justify-center gap-1.5 rounded-lg border border-amber-400/25 bg-amber-500/10 px-2.5 text-[11px] font-medium text-amber-200 transition-colors hover:bg-amber-500/16"
                    >
                      <StopCircle className="h-3.5 w-3.5" />
                      Stop
                    </button>
                  )}
                  </div>
                </div>

                {providerWaitActive && (
                  <div className="mb-3 rounded-lg border border-yellow-400/25 bg-yellow-500/10 p-3 text-xs text-yellow-50">
                    <div className="flex flex-wrap items-center gap-2">
                      <AlertCircle className="h-4 w-4 shrink-0 text-yellow-300" />
                      <span className="font-medium">
                        Smara sedang menunggu token pertama dari provider/model.
                      </span>
                      {providerIdleText && (
                        <span className="rounded bg-yellow-200/10 px-1.5 py-0.5 font-mono text-[10px] text-yellow-100">
                          tanpa event {providerIdleText}
                        </span>
                      )}
                    </div>
                    <div className="mt-2 grid gap-2 text-[11px] leading-5 text-yellow-100/82 md:grid-cols-3">
                      <div>
                        <span className="font-medium text-yellow-50">Status:</span> belum ada stream, tool call, atau final response.
                      </div>
                      <div>
                        <span className="font-medium text-yellow-50">Kemungkinan:</span> model lama berpikir, router menahan SSE, atau tunnel idle.
                      </div>
                      <div>
                        <span className="font-medium text-yellow-50">Aksi cepat:</span> stop, turunkan reasoning, atau aktifkan disable streaming.
                      </div>
                    </div>
                    {runStatus.runID && (
                      <button
                        onClick={() => copyMessage(-1, runStatus.runID || '')}
                        className="mt-2 max-w-full truncate font-mono text-[10px] text-yellow-100/75 hover:text-yellow-50"
                        title="Salin run id"
                      >
                        run={runStatus.runID}
                      </button>
                    )}
                  </div>
                )}

                {activePhases.length > 0 ? (
                  <div className="space-y-1.5">
                  {activePhases.map((ph, idx) => {
                    // Compute elapsed seconds for running phases
                    const elapsedMs = (ph.status === 'running' && ph.startedAt) ? (Date.now() - ph.startedAt) : 0
                    void elapsedTick // force re-render on tick
                    const elapsedStr = elapsedMs >= 1000 ? `${Math.floor(elapsedMs / 1000)}s` : ''
                    return (
                    <div key={ph.phase + idx} className="flex items-center gap-2 text-xs">
                      {ph.status === 'running' ? (
                        <span className="text-smara-400 font-mono w-4 text-center">{spinnerFrames[spinnerIdx]}</span>
                      ) : (
                        <CheckCircle2 className="w-3.5 h-3.5 text-green-400 shrink-0" />
                      )}
                      <span className={ph.status === 'running' ? 'text-gray-200 font-medium' : 'text-neutral-400'}>
                        {ph.description || ph.phase}
                      </span>
                      {ph.status === 'running' && elapsedStr && (
                        <span className="text-[10px] text-smara-300/70 font-mono tabular-nums">{elapsedStr}</span>
                      )}
                    </div>
                    )
                  })}
                  </div>
                ) : (
                  <div className="flex items-center gap-2 text-xs text-gray-400">
                    <span className="text-smara-400 font-mono">{spinnerFrames[spinnerIdx]}</span>
                    Menunggu event analisis pertama...
                  </div>
                )}

                {analysisEvents.length > 0 && (
                  <div className="mt-3 border-t border-[#31421f]/60 pt-2">
                    <div className="mb-2 flex flex-wrap items-center justify-between gap-2">
                      <div>
                        <div className="text-[10px] font-semibold uppercase tracking-wider text-neutral-500">Live timeline</div>
                        <div className="mt-0.5 text-[10px] text-neutral-500">
                          {analysisEvents.length} event, {visibleAnalysisEvents.length} tampil
                        </div>
                      </div>
                      <div className="flex flex-wrap gap-1 rounded-lg border border-[#31421f]/50 bg-[#1a2314]/70 p-1">
                        {ANALYSIS_FILTERS.map(filter => (
                          <button
                            key={filter.id}
                            onClick={() => setAnalysisFilter(filter.id)}
                            className={`rounded-md px-2 py-1 text-[10px] transition-colors ${
                              analysisFilter === filter.id
                                ? 'bg-smara-300/16 text-smara-100'
                                : 'text-neutral-500 hover:bg-[#26331d]/80 hover:text-gray-300'
                            }`}
                          >
                            {filter.label}
                          </button>
                        ))}
                      </div>
                    </div>
                    <div className="max-h-72 space-y-1.5 overflow-y-auto pr-1">
                      {visibleAnalysisEvents.length === 0 ? (
                        <div className="rounded-lg border border-[#31421f]/45 bg-[#1a2314]/55 px-3 py-2 text-xs text-neutral-500">
                          Tidak ada event untuk filter ini.
                        </div>
                      ) : visibleAnalysisEvents.map(event => {
                        const levelClass = event.level === 'error'
                          ? 'border-red-400/22 bg-red-500/8'
                          : event.level === 'warning'
                          ? 'border-yellow-400/22 bg-yellow-500/8'
                          : 'border-[#31421f]/45 bg-[#1a2314]/70'
                        const markerClass = event.level === 'error'
                          ? 'text-red-300'
                          : event.level === 'warning'
                          ? 'text-yellow-300'
                          : event.kind === 'tool'
                          ? 'text-smara-300'
                          : 'text-green-400'
                        return (
                          <div key={event.id} className={`grid grid-cols-[74px_1fr] gap-3 rounded-lg border px-2.5 py-2 text-xs ${levelClass}`}>
                            <div className="flex items-start gap-1.5 text-[10px] text-neutral-500">
                              {event.status === 'running' ? (
                                <span className={`mt-0.5 font-mono ${markerClass}`}>{spinnerFrames[spinnerIdx]}</span>
                              ) : event.level === 'warning' ? (
                                <AlertCircle className={`mt-0.5 h-3 w-3 shrink-0 ${markerClass}`} />
                              ) : (
                                <CheckCircle2 className={`mt-0.5 h-3 w-3 shrink-0 ${markerClass}`} />
                              )}
                              <span>{event.timestamp.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' })}</span>
                            </div>
                            <div className="min-w-0">
                              <div className="mb-1 flex flex-wrap items-center gap-1.5">
                                <span className={event.status === 'running' ? 'font-medium text-gray-100' : 'font-medium text-gray-400'}>{event.title}</span>
                                <span className="rounded bg-[#26331d]/75 px-1.5 py-0.5 text-[9px] uppercase tracking-wide text-smara-300">{event.kind}</span>
                                {event.event && <span className="rounded bg-[#26331d]/75 px-1.5 py-0.5 text-[9px] text-neutral-400">{event.event}</span>}
                                {event.tool && <span className="max-w-[160px] truncate rounded bg-[#26331d]/75 px-1.5 py-0.5 text-[9px] text-neutral-300">{event.tool}</span>}
                              </div>
                              <pre className="max-h-28 whitespace-pre-wrap break-words overflow-y-auto font-sans text-[11px] leading-5 text-gray-300">{event.detail || '...'}</pre>
                            </div>
                          </div>
                        )
                      })}
                    </div>
                  </div>
                )}
              </div>
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
                onClick={() => setModeAndNotify(m.id)}
                className={`flex items-center gap-1.5 px-3 py-1.5 text-xs rounded-xl transition-all ${
                  active
                    ? `${m.bg} text-white shadow-lg shadow-black/20`
                    : 'text-gray-400 hover:bg-[#26331d]/80 hover:text-gray-200'
                }`}
                title={m.label}
              >
                <Icon className="w-3 h-3" />
                <span className="hidden sm:inline">{m.label}</span>
                {m.id === 'voice' && voiceSpeaking && <span className="h-1.5 w-1.5 rounded-full bg-cyan-200 animate-pulse" />}
              </button>
            )
          })}
        </div>
        {activePlanQuest && (
          <div className="rounded-2xl border border-[#31421f]/60 bg-[#20291a]/78 p-3 shadow-lg shadow-lime-950/20">
            <div className="mb-2 flex items-center gap-2 text-xs font-semibold uppercase tracking-wide text-lime-200">
              <ClipboardList className="h-3.5 w-3.5" /> {isPlanApprovalQuest(activePlanQuest) ? 'Plan approval' : 'Open question'}
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
            disabled={(!input.trim() && attachments.length === 0) || thinking || uploading}
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
