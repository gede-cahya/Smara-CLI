import { useState, useEffect } from 'react'
import { BarChart3, Cpu, Plug, Activity, Server } from 'lucide-react'
import { fetchJSON } from '../api'
import type { Status, MCPInfo } from '../api'

export default function Dashboard() {
  const [status, setStatus] = useState<Status | null>(null)
  const [mcp, setMcp] = useState<MCPInfo[]>([])
  const [loading, setLoading] = useState(false)

  const load = async () => {
    setLoading(true)
    try {
      const s = await fetchJSON<Status>('/api/status')
      setStatus(s)
      const m = await fetchJSON<{ servers: MCPInfo[] }>('/api/mcp')
      setMcp(m.servers || [])
    } catch (e) {
      console.error(e)
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => { load() }, [])

  return (
    <div className="flex flex-col h-full p-4 overflow-y-auto">
      <div className="flex items-center gap-2 mb-4">
        <BarChart3 className="w-5 h-5 text-smara-400" />
        <h2 className="text-lg font-medium">Dashboard</h2>
      </div>

      {loading && <div className="text-gray-500 text-sm">Loading...</div>}

      {status && (
        <div className="grid grid-cols-2 lg:grid-cols-4 gap-3 mb-4">
          <StatCard icon={Activity} label="Status" value={status.status} color="text-green-400" />
          <StatCard icon={Cpu} label="Mode" value={`${status.mode_emoji} ${status.mode_label}`} color="text-smara-400" />
          <StatCard icon={Server} label="Provider" value={status.provider} color="text-blue-400" />
          <StatCard icon={Activity} label="Workspace" value={status.workspace || 'default'} color="text-purple-400" />
        </div>
      )}

      <div className="mb-2 text-sm font-medium text-gray-300">MCP Servers</div>
      <div className="space-y-2">
        {mcp.length === 0 && (
          <div className="text-gray-600 text-sm p-4 bg-gray-900/50 rounded-lg">
            Tidak ada MCP server terhubung.
          </div>
        )}
        {mcp.map(s => (
          <div key={s.name} className="flex items-center justify-between p-3 bg-gray-900/50 border border-gray-800 rounded-lg">
            <div className="flex items-center gap-3">
              <Plug className={`w-4 h-4 ${s.connected ? 'text-green-400' : 'text-red-400'}`} />
              <div>
                <div className="text-sm font-medium">{s.name}</div>
                {s.error && <div className="text-xs text-red-400">{s.error}</div>}
              </div>
            </div>
            <div className="text-xs text-gray-500">
              {s.connected ? `${s.tools} tools` : 'offline'}
            </div>
          </div>
        ))}
      </div>
    </div>
  )
}

function StatCard({ icon: Icon, label, value, color }: { icon: typeof Activity, label: string, value: string, color: string }) {
  return (
    <div className="bg-gray-900/50 border border-gray-800 rounded-lg p-3">
      <div className="flex items-center gap-2 mb-1">
        <Icon className={`w-4 h-4 ${color}`} />
        <span className="text-xs text-gray-500">{label}</span>
      </div>
      <div className="text-sm font-medium">{value}</div>
    </div>
  )
}
