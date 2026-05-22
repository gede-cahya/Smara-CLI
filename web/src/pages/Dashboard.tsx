import { useState, useEffect } from 'react'
import { BarChart3, Cpu, Plug, Activity, Server, DollarSign, Hash, Zap, Trophy } from 'lucide-react'
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
        fetchJSON<UsageAnalytics>('/api/metrics?days=30'),
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

  return (
    <div className="flex flex-col h-full overflow-y-auto p-5 md:p-6 space-y-5">
      <div className="flex items-start justify-between gap-4 rounded-3xl border border-neutral-800/70 bg-white/[0.035] p-4 shadow-lg shadow-black/20">
        <div className="flex items-center gap-3">
          <div className="grid h-11 w-11 place-items-center rounded-2xl border border-smara-300/20 bg-smara-300/10 text-smara-300 shadow-lg shadow-smara-950/20"><BarChart3 className="w-5 h-5" /></div>
          <div><h2 className="text-xl font-semibold tracking-tight text-white">Dashboard Analytics</h2><p className="text-xs text-neutral-500">Ringkasan runtime, model, token, cost, dan MCP server.</p></div>
        </div>
        <button onClick={load} className="rounded-2xl border border-smara-300/20 bg-smara-300 px-4 py-2 text-xs font-semibold text-black shadow-lg shadow-smara-950/20 transition-colors hover:bg-smara-200">Refresh</button>
      </div>

      {loading && <div className="text-gray-500 text-sm">Loading...</div>}

      {status && (
        <div className="grid grid-cols-1 sm:grid-cols-2 xl:grid-cols-4 gap-3">
          <StatCard icon={Activity} label="Status" value={status.status} color="text-green-400" />
          <StatCard icon={Cpu} label="Mode" value={`${status.mode_emoji} ${status.mode_label}`} color="text-smara-400" />
          <StatCard icon={Server} label="Provider" value={status.provider} color="text-smara-400" />
          <StatCard icon={Activity} label="Workspace" value={status.workspace || 'default'} color="text-lime-400" />
        </div>
      )}

      {analytics && (
        <>
          <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 2xl:grid-cols-6 gap-3">
            <StatCard icon={Hash} label="Prompts" value={String(analytics.total_prompts || 0)} color="text-smara-400" />
            <StatCard icon={Zap} label="Requests" value={String(analytics.total_requests || 0)} color="text-yellow-400" />
            <StatCard icon={BarChart3} label="Total Tokens" value={formatNum(analytics.total_tokens || 0)} color="text-smara-400" />
            <StatCard icon={BarChart3} label="Input Tokens" value={formatNum(analytics.input_tokens || 0)} color="text-smara-400" />
            <StatCard icon={BarChart3} label="Output Tokens" value={formatNum(analytics.output_tokens || 0)} color="text-lime-400" />
            <StatCard icon={DollarSign} label="Cost Est." value={`$${(analytics.estimated_cost_usd || 0).toFixed(6)}`} color="text-green-400" />
          </div>

          <div className="grid grid-cols-1 xl:grid-cols-2 gap-5">
            <Panel title="Grafik Token & Cost Harian">
              <DailyChart data={analytics.daily || []} />
            </Panel>
            <Panel title="Model yang Dipakai">
              {(analytics.models || []).length === 0 ? <Empty text="Belum ada data model usage." /> : analytics.models.map(m => (
                <BarRow key={`${m.provider}/${m.model}`} label={`${m.provider}/${m.model}`} value={m.total_tokens} max={Math.max(...analytics.models.map(x => x.total_tokens), 1)} right={`${m.requests} req • in ${formatNum(m.input_tokens || 0)} / out ${formatNum(m.output_tokens || 0)} • $${m.cost_usd.toFixed(5)}`} />
              ))}
            </Panel>
          </div>

          <Panel title="Skill yang Sering Dipakai">
            {(analytics.top_skills || []).length === 0 ? <Empty text="Belum ada data eksekusi skill." /> : analytics.top_skills.map(sk => (
              <div key={sk.name} className="flex items-center justify-between py-1.5 text-xs">
                <div className="flex items-center gap-2 min-w-0"><Trophy className="w-3.5 h-3.5 text-yellow-400" /><span className="truncate text-gray-200">{sk.name}</span></div>
                <div className="text-gray-500">{sk.run_count} run • {Math.round(sk.success_rate)}%</div>
              </div>
            ))}
          </Panel>
        </>
      )}

      <Panel title="MCP Servers">
        <div className="space-y-2">
          {mcp.length === 0 && <Empty text="Tidak ada MCP server terhubung." />}
          {mcp.map(s => (
            <div key={s.name} className="flex items-center justify-between rounded-2xl border border-neutral-800/60 bg-neutral-950/45 p-3.5 transition-colors hover:border-smara-300/18 hover:bg-white/[0.035]">
              <div className="flex items-center gap-3">
                <Plug className={`w-4 h-4 ${s.connected ? 'text-green-400' : 'text-red-400'}`} />
                <div><div className="text-sm font-medium">{s.name}</div>{s.error && <div className="text-xs text-red-400">{s.error}</div>}</div>
              </div>
              <div className="text-xs text-gray-500">{s.connected ? `${s.tools} tools` : 'offline'}</div>
            </div>
          ))}
        </div>
      </Panel>
    </div>
  )
}

function StatCard({ icon: Icon, label, value, color }: { icon: typeof Activity, label: string, value: string, color: string }) {
  return <div className="group rounded-3xl border border-neutral-800/70 bg-white/[0.035] p-4 shadow-lg shadow-black/15 transition-all hover:-translate-y-0.5 hover:border-smara-300/20 hover:bg-white/[0.055]"><div className="flex items-center gap-2 mb-3"><span className="grid h-9 w-9 place-items-center rounded-2xl border border-neutral-800/60 bg-neutral-950/50"><Icon className={`w-4 h-4 ${color}`} /></span><span className="text-xs text-neutral-500">{label}</span></div><div className="text-lg font-semibold tracking-tight text-white truncate">{value}</div></div>
}
function Panel({ title, children }: { title: string, children: React.ReactNode }) { return <div className="rounded-3xl border border-neutral-800/70 bg-white/[0.035] p-4 shadow-lg shadow-black/15"><div className="mb-4 flex items-center justify-between border-b border-neutral-800/60 pb-3"><div className="text-sm font-semibold text-neutral-200">{title}</div><div className="h-1.5 w-1.5 rounded-full bg-smara-300 shadow-[0_0_12px_rgba(190,242,100,.8)]" /></div>{children}</div> }
function Empty({ text }: { text: string }) { return <div className="rounded-2xl border border-neutral-800/60 bg-neutral-950/35 p-4 text-sm text-neutral-600">{text}</div> }
function formatNum(n: number) { return new Intl.NumberFormat().format(n) }
function BarRow({ label, value, max, right }: { label: string, value: number, max: number, right: string }) { const pct = Math.max(2, Math.round((value / max) * 100)); return <div className="space-y-1 mb-3"><div className="flex justify-between text-xs"><span className="text-gray-300 truncate">{label}</span><span className="text-gray-500 ml-2">{right}</span></div><div className="h-2 rounded-full bg-neutral-900"><div className="h-2 rounded-full bg-gradient-to-r from-smara-600 to-smara-300" style={{ width: `${pct}%` }} /></div></div> }
function DailyChart({ data }: { data: DailyUsage[] }) { const max = Math.max(...data.map(d => d.total_tokens), 1); if (!data.length) return <Empty text="Belum ada data harian." />; return <div className="h-48 flex items-end gap-2 rounded-2xl border border-neutral-800/60 bg-neutral-950/25 px-3 pb-4 pt-2">{data.slice(-14).map(d => { const h = Math.max(4, Math.round((d.total_tokens / max) * 160)); return <div key={d.date} className="flex-1 flex flex-col items-center gap-1"><div title={`${d.date}: ${d.total_tokens} tokens (in ${d.input_tokens || 0} / out ${d.output_tokens || 0}), $${d.cost_usd.toFixed(5)}`} className="w-full bg-gradient-to-t from-smara-600 to-smara-300 rounded-t" style={{ height: h }} /><span className="text-[9px] text-gray-500 rotate-45 origin-left w-10">{d.date.slice(5)}</span></div> })}</div> }
