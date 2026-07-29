import { useState, useEffect } from 'react'
import { BarChart3, Cpu, Activity, Server, DollarSign, Hash, Zap, Trophy, Wifi, WifiOff, Globe, Cog, TrendingUp } from 'lucide-react'
import { fetchJSON } from '../api'
import type { Status, MCPInfo } from '../api'

interface ModelUsage { provider: string; model: string; requests: number; prompts: number; input_tokens: number; output_tokens: number; total_tokens: number; cost_usd: number }
interface DailyUsage { date: string; requests: number; prompts: number; input_tokens: number; output_tokens: number; total_tokens: number; cost_usd: number }
interface SkillUsage { name: string; run_count: number; success_rate: number }
interface UsageAnalytics {
  total_prompts: number
  total_requests: number
  input_tokens: number
  output_tokens: number
  total_tokens: number
  estimated_cost_usd: number
  models: ModelUsage[]
  daily: DailyUsage[]
  top_skills: SkillUsage[]
}

export default function Dashboard() {
  const [status, setStatus] = useState<Status | null>(null)
  const [mcp, setMcp] = useState<MCPInfo[]>([])
  const [analytics, setAnalytics] = useState<UsageAnalytics | null>(null)
  const [loading, setLoading] = useState(false)

  const load = async () => {
    setLoading(true)
    try {
      const [s, m, a] = await Promise.all([
        fetchJSON<Status>('/api/status'),
        fetchJSON<{ servers: MCPInfo[] }>('/api/mcp'),
        fetchJSON<UsageAnalytics>('/api/metrics?days=90'),
      ])
      setStatus(s)
      setMcp(m.servers || [])
      setAnalytics(a)
    } catch (e) {
      console.error(e)
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => { load() }, [])

  const cleanModelName = (provider: string, model: string) => {
    if (provider === 'custom') {
      // Strip common prefixes: cx/, sr/, bai/, tr/, tk/, ae/, etc.
      return model.replace(/^(cx|sr|bai|tr|tk|ae|kr|nara|glm|mimo|cmc|n|cl|gemini|openrouter|bm)\//, '')
    }
    return model
  }

  const formatCost = (cost: number) => {
    if (cost === 0) return '$0'
    if (cost < 0.001) return `$${cost.toFixed(6)}`
    if (cost < 0.01) return `$${cost.toFixed(4)}`
    return `$${cost.toFixed(2)}`
  }

  const formatTokens = (n: number) => {
    if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`
    if (n >= 1_000) return `${(n / 1_000).toFixed(1)}K`
    return String(n)
  }

  return (
    <div className="flex flex-col h-full overflow-y-auto p-5 md:p-6 space-y-5">
      {/* Header */}
      <div className="flex items-start justify-between gap-4 rounded-3xl border border-neutral-800/70 bg-white/[0.035] p-4 shadow-lg shadow-black/20">
        <div className="flex items-center gap-3">
          <div className="grid h-11 w-11 place-items-center rounded-2xl border border-smara-300/20 bg-smara-300/10 text-smara-300 shadow-lg shadow-smara-950/20"><BarChart3 className="w-5 h-5" /></div>
          <div><h2 className="text-xl font-semibold tracking-tight text-white">Dashboard Analytics</h2><p className="text-xs text-neutral-500">Ringkasan runtime, model, token, cost, dan MCP server.</p></div>
        </div>
        <button onClick={load} className="rounded-2xl border border-smara-300/20 bg-smara-300 px-4 py-2 text-xs font-semibold text-black shadow-lg shadow-smara-950/20 transition-colors hover:bg-smara-200">{loading ? 'Loading...' : 'Refresh'}</button>
      </div>

      {/* Status Cards */}
      {status && (
        <>
          <div className="grid grid-cols-2 xl:grid-cols-4 gap-3">
            <StatCard icon={Activity} label="Status" value={status.status} color="text-green-400" />
            <StatCard icon={Cpu} label="Mode" value={`${status.mode_emoji} ${status.mode_label}`} color="text-smara-400" />
            <StatCard icon={Server} label="Provider" value={status.provider === 'custom' ? (status.router9?.provider_name || '9Router') : status.provider} color="text-smara-400" />
            <StatCard icon={Globe} label="Model" value={cleanModelName(status.provider, status.model || status.router9?.model || '-')} color="text-lime-400" />
          </div>

          {/* 9Router Connection */}
          {status.router9 && (
            <Panel title="9Router Connection">
              <div className="grid grid-cols-2 lg:grid-cols-4 gap-4">
                <InfoChip icon={status.provider_online ? Wifi : WifiOff} label="Status" value={status.provider_online ? 'Online' : 'Offline'} color={status.provider_online ? 'text-green-400' : 'text-red-400'} />
                <InfoChip icon={Globe} label="Endpoint" value={status.router9.base_url.replace(/^https?:\/\//, '')} color="text-gray-200" />
                <InfoChip icon={Cog} label="Tool Call" value={status.router9.native_tool ? 'Native' : 'DSML'} color={status.router9.native_tool ? 'text-green-400' : 'text-yellow-400'} />
                <InfoChip icon={Activity} label="Streaming" value={status.router9.stream_disabled ? 'Off' : 'On'} color={status.router9.stream_disabled ? 'text-yellow-400' : 'text-green-400'} />
              </div>
              {status.provider_error && <div className="mt-3 text-xs text-red-400 bg-red-500/10 rounded-lg p-2">{status.provider_error}</div>}
            </Panel>
          )}
        </>
      )}

      {/* Analytics */}
      {analytics && (
        <>
          {/* Summary Stats */}
          <div className="grid grid-cols-2 lg:grid-cols-3 2xl:grid-cols-6 gap-3">
            <StatCard icon={Hash} label="Prompts" value={formatNum(analytics.total_prompts || 0)} color="text-smara-400" />
            <StatCard icon={Zap} label="Requests" value={formatNum(analytics.total_requests || 0)} color="text-yellow-400" />
            <StatCard icon={BarChart3} label="Total Tokens" value={formatTokens(analytics.total_tokens || 0)} color="text-smara-400" />
            <StatCard icon={TrendingUp} label="Input" value={formatTokens(analytics.input_tokens || 0)} color="text-blue-400" />
            <StatCard icon={TrendingUp} label="Output" value={formatTokens(analytics.output_tokens || 0)} color="text-lime-400" />
            <StatCard icon={DollarSign} label="Cost" value={formatCost(analytics.estimated_cost_usd || 0)} color="text-green-400" />
          </div>

          <div className="grid grid-cols-1 xl:grid-cols-2 gap-5">
            {/* Daily Chart */}
            <Panel title="Token Harian">
              <DailyChart data={analytics.daily || []} />
            </Panel>

            {/* Model Usage */}
            <Panel title="Model Usage">
              {(analytics.models || []).length === 0 ? <Empty text="Belum ada data model usage." /> : (
                <div className="space-y-3">
                  {analytics.models.map(m => {
                    const maxTokens = Math.max(...analytics.models.map(x => x.total_tokens), 1)
                    const pct = Math.max(3, Math.round((m.total_tokens / maxTokens) * 100))
                    const displayName = cleanModelName(m.provider, m.model)
                    return (
                      <div key={`${m.provider}/${m.model}`} className="space-y-1.5">
                        <div className="flex items-center justify-between">
                          <span className="text-sm text-gray-200 font-medium truncate">{displayName}</span>
                          <div className="flex items-center gap-3 text-xs text-gray-500">
                            <span>{formatTokens(m.total_tokens)} tok</span>
                            <span>{m.requests} req</span>
                            <span className="text-green-400">{formatCost(m.cost_usd)}</span>
                          </div>
                        </div>
                        <div className="flex gap-1 h-2.5">
                          <div className="rounded-full bg-blue-500/70" style={{ width: `${pct}%` }} title={`Input: ${formatTokens(m.input_tokens)}`} />
                          <div className="rounded-full bg-lime-400/70" style={{ width: `${Math.max(2, Math.round((m.output_tokens / m.total_tokens) * pct))}%` }} title={`Output: ${formatTokens(m.output_tokens)}`} />
                        </div>
                      </div>
                    )
                  })}
                  <div className="flex items-center gap-4 mt-2 text-[10px] text-gray-600">
                    <span className="flex items-center gap-1"><span className="w-2.5 h-2.5 rounded-full bg-blue-500/70" />Input</span>
                    <span className="flex items-center gap-1"><span className="w-2.5 h-2.5 rounded-full bg-lime-400/70" />Output</span>
                  </div>
                </div>
              )}
            </Panel>
          </div>

          {/* Skills */}
          <Panel title="Top Skills">
            {(analytics.top_skills || []).length === 0 ? <Empty text="Belum ada data eksekusi skill." /> : (
              <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-2">
                {analytics.top_skills.map(sk => (
                  <div key={sk.name} className="flex items-center justify-between rounded-xl border border-neutral-800/60 bg-neutral-950/40 px-3 py-2">
                    <div className="flex items-center gap-2 min-w-0">
                      <Trophy className="w-3.5 h-3.5 text-yellow-400 flex-shrink-0" />
                      <span className="truncate text-xs text-gray-200">{sk.name}</span>
                    </div>
                    <div className="flex items-center gap-2 text-xs text-gray-500 flex-shrink-0 ml-2">
                      <span>{sk.run_count}x</span>
                      <span className={sk.success_rate >= 80 ? 'text-green-400' : sk.success_rate >= 50 ? 'text-yellow-400' : 'text-red-400'}>{Math.round(sk.success_rate)}%</span>
                    </div>
                  </div>
                ))}
              </div>
            )}
          </Panel>
        </>
      )}

      {/* MCP Servers */}
      <Panel title="MCP Servers">
        {mcp.length === 0 ? <Empty text="Tidak ada MCP server terhubung." /> : (
          <div className="grid grid-cols-1 sm:grid-cols-2 gap-2">
            {mcp.map(s => (
              <div key={s.name} className="flex items-center justify-between rounded-xl border border-neutral-800/60 bg-neutral-950/40 px-3.5 py-2.5 transition-colors hover:border-smara-300/18 hover:bg-white/[0.035]">
                <div className="flex items-center gap-2.5 min-w-0">
                  <div className={`w-2 h-2 rounded-full flex-shrink-0 ${s.connected ? 'bg-green-400 shadow-[0_0_6px_rgba(74,222,128,.6)]' : 'bg-red-400'}`} />
                  <div className="min-w-0">
                    <div className="text-sm font-medium text-gray-200 truncate">{s.name}</div>
                    {s.error && <div className="text-[10px] text-red-400 truncate">{s.error}</div>}
                  </div>
                </div>
                <div className="text-xs text-gray-500 flex-shrink-0 ml-2">{s.connected ? `${s.tools} tools` : 'offline'}</div>
              </div>
            ))}
          </div>
        )}
      </Panel>
    </div>
  )
}

/* ── Sub-components ── */

function StatCard({ icon: Icon, label, value, color }: { icon: typeof Activity, label: string, value: string, color: string }) {
  return (
    <div className="group rounded-2xl border border-neutral-800/70 bg-white/[0.035] p-3.5 shadow-lg shadow-black/15 transition-all hover:-translate-y-0.5 hover:border-smara-300/20 hover:bg-white/[0.055]">
      <div className="flex items-center gap-2 mb-2">
        <span className="grid h-8 w-8 place-items-center rounded-xl border border-neutral-800/60 bg-neutral-950/50">
          <Icon className={`w-3.5 h-3.5 ${color}`} />
        </span>
        <span className="text-[11px] text-neutral-500">{label}</span>
      </div>
      <div className="text-base font-semibold tracking-tight text-white truncate">{value}</div>
    </div>
  )
}

function InfoChip({ icon: Icon, label, value, color }: { icon: typeof Wifi, label: string, value: string, color: string }) {
  return (
    <div className="flex items-center gap-2 rounded-xl bg-neutral-950/30 border border-neutral-800/40 px-3 py-2">
      <Icon className={`w-3.5 h-3.5 ${color} flex-shrink-0`} />
      <div className="min-w-0">
        <div className="text-[10px] text-gray-500">{label}</div>
        <div className={`text-xs font-medium truncate ${color}`}>{value}</div>
      </div>
    </div>
  )
}

function Panel({ title, children }: { title: string, children: React.ReactNode }) {
  return (
    <div className="rounded-2xl border border-neutral-800/70 bg-white/[0.035] p-4 shadow-lg shadow-black/15">
      <div className="mb-3 flex items-center justify-between border-b border-neutral-800/60 pb-2.5">
        <div className="text-sm font-semibold text-neutral-200">{title}</div>
        <div className="h-1.5 w-1.5 rounded-full bg-smara-300 shadow-[0_0_12px_rgba(190,242,100,.8)]" />
      </div>
      {children}
    </div>
  )
}

function Empty({ text }: { text: string }) {
  return <div className="rounded-xl border border-neutral-800/60 bg-neutral-950/35 p-4 text-xs text-neutral-600 text-center">{text}</div>
}

function formatNum(n: number) { return new Intl.NumberFormat().format(n) }

function DailyChart({ data }: { data: DailyUsage[] }) {
  const recent = data.slice(-14)
  const max = Math.max(...recent.map(d => d.total_tokens), 1)

  if (!recent.length) return <Empty text="Belum ada data harian." />

  return (
    <div className="space-y-2">
      <div className="h-44 flex items-end gap-1 rounded-xl bg-neutral-950/25 px-2 pb-2 pt-1">
        {recent.map(d => {
          const h = Math.max(4, Math.round((d.total_tokens / max) * 140))
          const inputH = d.total_tokens > 0 ? Math.round((d.input_tokens / d.total_tokens) * h) : h
          const outputH = h - inputH
          return (
            <div key={d.date} className="flex-1 flex flex-col items-center gap-0.5 group relative">
              <div className="w-full flex flex-col justify-end" style={{ height: 140 }}>
                <div className="w-full flex flex-col rounded-t overflow-hidden">
                  <div className="bg-blue-500/60 w-full" style={{ height: Math.max(2, inputH) }} />
                  <div className="bg-lime-400/60 w-full" style={{ height: Math.max(1, outputH) }} />
                </div>
              </div>
              {/* Tooltip */}
              <div className="absolute -top-2 left-1/2 -translate-x-1/2 hidden group-hover:block z-10 bg-neutral-900 border border-neutral-700 rounded-lg px-2.5 py-1.5 text-[10px] text-gray-300 whitespace-nowrap shadow-xl">
                <div className="font-medium text-gray-100">{d.date}</div>
                <div>In: {formatNum(d.input_tokens || 0)} · Out: {formatNum(d.output_tokens || 0)}</div>
                <div className="text-green-400">{formatNum(d.total_tokens)} tok · ${(d.cost_usd || 0).toFixed(4)}</div>
              </div>
            </div>
          )
        })}
      </div>
      <div className="flex items-center justify-between px-2">
        <div className="flex items-center gap-3 text-[10px] text-gray-600">
          <span className="flex items-center gap-1"><span className="w-2 h-2 rounded-sm bg-blue-500/60" />Input</span>
          <span className="flex items-center gap-1"><span className="w-2 h-2 rounded-sm bg-lime-400/60" />Output</span>
        </div>
        <div className="text-[10px] text-gray-600">
          {recent.length > 0 && `${recent[0].date.slice(5)} — ${recent[recent.length - 1].date.slice(5)}`}
        </div>
      </div>
    </div>
  )
}
