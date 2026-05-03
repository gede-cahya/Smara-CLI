import { useState, useEffect } from 'react'
import { fetchJSON, type SkillItem } from '../api'
import { Trophy, AlertTriangle, Activity, TrendingUp, Clock } from 'lucide-react'

interface SkillStat {
  name: string
  run_count: number
  success_rate: number
}

interface Analytics {
  total_runs: number
  successful_runs: number
  top_skills: SkillStat[]
  struggling: SkillStat[]
}

interface TimelineItem {
  id: number
  skill_name: string
  run_id: string
  started_at: string
  duration_ms: number
  success: boolean
  error_message?: string
  triggered_by: string
}

export default function SkillDashboard() {
  const [analytics, setAnalytics] = useState<Analytics | null>(null)
  const [recentRuns, setRecentRuns] = useState<TimelineItem[]>([])
  const [loading, setLoading] = useState(false)

  const load = async () => {
    setLoading(true)
    try {
      const a = await fetchJSON<Analytics>('/api/skills/analytics')
      setAnalytics(a)
    } catch (e) {
      console.error(e)
    }
    setLoading(false)
  }

  const loadRecent = async () => {
    try {
      const skills = await fetchJSON<{ skills: SkillItem[] }>('/api/skills')
      const allRuns: TimelineItem[] = []
      for (const sk of skills.skills.slice(0, 10)) {
        try {
          const t = await fetchJSON<{ timeline: TimelineItem[] }>(`/api/skills/timeline?name=${encodeURIComponent(sk.name)}&limit=3`)
          allRuns.push(...(t.timeline || []))
        } catch { /* ignore */ }
      }
      allRuns.sort((a, b) => new Date(b.started_at).getTime() - new Date(a.started_at).getTime())
      setRecentRuns(allRuns.slice(0, 15))
    } catch (e) {
      console.error(e)
    }
  }

  useEffect(() => {
    load()
    loadRecent()
  }, [])

  return (
    <div className="flex flex-col h-full p-4 overflow-y-auto space-y-4">
      <div className="flex items-center justify-between">
        <h3 className="text-sm font-medium text-gray-300">Skill Analytics Dashboard</h3>
        <button
          onClick={() => { load(); loadRecent() }}
          className="px-2 py-1 bg-smara-700 hover:bg-smara-600 rounded text-[10px] text-white transition-colors"
        >
          Refresh
        </button>
      </div>

      {loading && <div className="text-gray-500 text-xs">Loading analytics...</div>}

      {/* Summary cards */}
      {analytics && (
        <div className="grid grid-cols-3 gap-3">
          <div className="p-3 bg-gray-900/50 border border-gray-800 rounded-lg">
            <div className="flex items-center gap-2 text-xs text-gray-400 mb-1">
              <Activity className="w-3.5 h-3.5 text-smara-400" /> Total Runs
            </div>
            <div className="text-lg font-semibold text-gray-200">{analytics.total_runs}</div>
          </div>
          <div className="p-3 bg-gray-900/50 border border-gray-800 rounded-lg">
            <div className="flex items-center gap-2 text-xs text-gray-400 mb-1">
              <TrendingUp className="w-3.5 h-3.5 text-green-400" /> Success Rate
            </div>
            <div className="text-lg font-semibold text-gray-200">
              {analytics.total_runs > 0
                ? `${Math.round((analytics.successful_runs / analytics.total_runs) * 100)}%`
                : 'N/A'}
            </div>
          </div>
          <div className="p-3 bg-gray-900/50 border border-gray-800 rounded-lg">
            <div className="flex items-center gap-2 text-xs text-gray-400 mb-1">
              <Trophy className="w-3.5 h-3.5 text-yellow-400" /> Top Skill
            </div>
            <div className="text-sm font-semibold text-gray-200 truncate">
              {analytics.top_skills?.[0]?.name || 'N/A'}
            </div>
          </div>
        </div>
      )}

      {/* Top skills */}
      {analytics?.top_skills && analytics.top_skills.length > 0 && (
        <div className="p-3 bg-gray-900/50 border border-gray-800 rounded-lg">
          <div className="flex items-center gap-2 text-xs text-gray-400 mb-2">
            <Trophy className="w-3.5 h-3.5 text-yellow-400" /> Top Skills (Most Used)
          </div>
          <div className="space-y-1.5">
            {analytics.top_skills.map(sk => (
              <div key={sk.name} className="flex items-center justify-between text-xs">
                <span className="text-gray-300">{sk.name}</span>
                <div className="flex items-center gap-2">
                  <span className="text-gray-500">{sk.run_count} runs</span>
                  <span className={`${sk.success_rate >= 70 ? 'text-green-400' : 'text-red-400'}`}>
                    {Math.round(sk.success_rate)}%
                  </span>
                </div>
              </div>
            ))}
          </div>
        </div>
      )}

      {/* Skills needing attention */}
      {analytics?.struggling && analytics.struggling.length > 0 && (
        <div className="p-3 bg-gray-900/50 border border-red-900/30 rounded-lg">
          <div className="flex items-center gap-2 text-xs text-red-400 mb-2">
            <AlertTriangle className="w-3.5 h-3.5" /> Skills Needing Attention
          </div>
          <div className="space-y-1.5">
            {analytics.struggling.map(sk => (
              <div key={sk.name} className="flex items-center justify-between text-xs">
                <span className="text-gray-300">{sk.name}</span>
                <div className="flex items-center gap-2">
                  <span className="text-gray-500">{sk.run_count} runs</span>
                  <span className="text-red-400 font-medium">{Math.round(sk.success_rate)}%</span>
                </div>
              </div>
            ))}
          </div>
        </div>
      )}

      {/* Recent activity timeline */}
      {recentRuns.length > 0 && (
        <div className="p-3 bg-gray-900/50 border border-gray-800 rounded-lg">
          <div className="flex items-center gap-2 text-xs text-gray-400 mb-2">
            <Clock className="w-3.5 h-3.5 text-smara-400" /> Recent Activity
          </div>
          <div className="space-y-1.5 max-h-64 overflow-y-auto">
            {recentRuns.map(run => (
              <div key={run.id} className="flex items-center gap-2 text-xs">
                <span className={`w-1.5 h-1.5 rounded-full ${run.success ? 'bg-green-400' : 'bg-red-400'}`} />
                <span className="text-gray-300 font-medium">{run.skill_name}</span>
                <span className="text-gray-500 ml-auto">{new Date(run.started_at).toLocaleTimeString()}</span>
                <span className="text-gray-500">{run.duration_ms}ms</span>
              </div>
            ))}
          </div>
        </div>
      )}
    </div>
  )
}
