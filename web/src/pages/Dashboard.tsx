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
    <div className="flex flex-col h-full p-4 overflow-y-auto space-y-4">
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-2">
          <BarChart3 className="w-5 h-5 text-smara-400" />
          <h2 className="text-lg font-medium">Dashboard Analytics</h2>
        </div>
        <button onClick={load} className="px-3 py-1.5 bg-smara-700 hover:bg-smara-600 rounded text-xs text-white">Refresh</button>
      </div>

      {loading && <div className="text-gray-500 text-sm">Loading...</div>}

      {status && (
        <div className="grid grid-cols-2 lg:grid-cols-4 gap-3">
          <StatCard icon={Activity} label="Status" value={status.status} color="text-green-400" />
          <StatCard icon={Cpu} label="Mode" value={`${status.mode_emoji} ${status.mode_label}`} color="text-smara-400" />
          <StatCard icon={Server} label="Provider" value={status.provider} color="text-blue-400" />
          <StatCard icon={Activity} label="Workspace" value={status.workspace || 'default'} color="text-purple-400" />
        </div>
      )}

      {analytics && (
        <>
          <div className="grid grid-cols-2 lg:grid-cols-6 gap-3">
            <StatCard icon={Hash} label="Prompts" value={String(analytics.total_prompts || 0)} color="text-cyan-400" />
            <StatCard icon={Zap} label="Requests" value={String(analytics.total_requests || 0)} color="text-yellow-400" />
            <StatCard icon={BarChart3} label="Total Tokens" value={formatNum(analytics.total_tokens || 0)} color="text-smara-400" />
            <StatCard icon={BarChart3} label="Input Tokens" value={formatNum(analytics.input_tokens || 0)} color="text-blue-400" />
            <StatCard icon={BarChart3} label="Output Tokens" value={formatNum(analytics.output_tokens || 0)} color="text-purple-400" />
            <StatCard icon={DollarSign} label="Cost Est." value={`$${(analytics.estimated_cost_usd || 0).toFixed(6)}`} color="text-green-400" />
          </div>

          <div className="grid grid-cols-1 xl:grid-cols-2 gap-4">
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
            <div key={s.name} className="flex items-center justify-between p-3 bg-gray-950/50 border border-gray-800 rounded-lg">
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
  return <div className="bg-gray-900/50 border border-gray-800 rounded-lg p-3"><div className="flex items-center gap-2 mb-1"><Icon className={`w-4 h-4 ${color}`} /><span className="text-xs text-gray-500">{label}</span></div><div className="text-sm font-medium truncate">{value}</div></div>
}
function Panel({ title, children }: { title: string, children: React.ReactNode }) { return <div className="p-3 bg-gray-900/50 border border-gray-800 rounded-lg"><div className="text-sm font-medium text-gray-300 mb-3">{title}</div>{children}</div> }
function Empty({ text }: { text: string }) { return <div className="text-gray-600 text-sm p-4 bg-gray-950/40 rounded-lg">{text}</div> }
function formatNum(n: number) { return new Intl.NumberFormat().format(n) }
function BarRow({ label, value, max, right }: { label: string, value: number, max: number, right: string }) { const pct = Math.max(2, Math.round((value / max) * 100)); return <div className="space-y-1 mb-3"><div className="flex justify-between text-xs"><span className="text-gray-300 truncate">{label}</span><span className="text-gray-500 ml-2">{right}</span></div><div className="h-2 bg-gray-800 rounded"><div className="h-2 bg-smara-500 rounded" style={{ width: `${pct}%` }} /></div></div> }
function DailyChart({ data }: { data: DailyUsage[] }) { const max = Math.max(...data.map(d => d.total_tokens), 1); if (!data.length) return <Empty text="Belum ada data harian." />; return <div className="h-48 flex items-end gap-2 border-b border-l border-gray-800 px-2 pt-2">{data.slice(-14).map(d => { const h = Math.max(4, Math.round((d.total_tokens / max) * 160)); return <div key={d.date} className="flex-1 flex flex-col items-center gap-1"><div title={`${d.date}: ${d.total_tokens} tokens (in ${d.input_tokens || 0} / out ${d.output_tokens || 0}), $${d.cost_usd.toFixed(5)}`} className="w-full bg-gradient-to-t from-smara-700 to-cyan-400 rounded-t" style={{ height: h }} /><span className="text-[9px] text-gray-500 rotate-45 origin-left w-10">{d.date.slice(5)}</span></div> })}</div> }
