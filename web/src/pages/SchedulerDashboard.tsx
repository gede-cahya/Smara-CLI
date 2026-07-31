import { useState, useEffect } from 'react'
import { Clock, Plus, Play, Trash2, RefreshCw, CheckCircle2, AlertTriangle, ArrowRight, Shield } from 'lucide-react'
import { fetchJSON } from '../api'

export interface ScheduledJob {
  id: string
  spec: string
  workflow: string
  enabled: boolean
  max_retries?: number
  retry_count?: number
  retry_interval_sec?: number
  depends_on?: string
  created_at: string
  updated_at: string
  next_run_at: string
  last_run_at?: string
  last_status?: string
}

export default function SchedulerDashboard() {
  const [jobs, setJobs] = useState<ScheduledJob[]>([])
  const [loading, setLoading] = useState(false)
  const [showAddModal, setShowAddModal] = useState(false)
  const [spec, setSpec] = useState('every 15m')
  const [workflow, setWorkflow] = useState('')
  const [retries, setRetries] = useState(3)
  const [dependsOn, setDependsOn] = useState('')

  const loadJobs = async () => {
    setLoading(true)
    try {
      const res = await fetchJSON<{ jobs: ScheduledJob[] }>('/api/scheduler/jobs')
      setJobs(res.jobs || [])
    } catch (e) {
      console.error('Failed to load scheduled jobs:', e)
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    loadJobs()
  }, [])

  const handleAddJob = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!spec || !workflow) return
    try {
      await fetch('/api/scheduler/jobs', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          spec,
          workflow,
          retries: Number(retries),
          retry_interval_sec: 10,
          depends_on: dependsOn,
        }),
      })
      setShowAddModal(false)
      setWorkflow('')
      setDependsOn('')
      loadJobs()
    } catch (e) {
      console.error('Failed to add job:', e)
    }
  }

  const handleRunNow = async (id: string) => {
    try {
      await fetch('/api/scheduler/jobs', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ action: 'run', id }),
      })
      loadJobs()
    } catch (e) {
      console.error('Failed to trigger job:', e)
    }
  }

  const handleDeleteJob = async (id: string) => {
    if (!confirm(`Hapus jadwal ${id}?`)) return
    try {
      await fetch(`/api/scheduler/jobs?id=${encodeURIComponent(id)}`, {
        method: 'DELETE',
      })
      loadJobs()
    } catch (e) {
      console.error('Failed to delete job:', e)
    }
  }

  const formatDate = (dateStr?: string) => {
    if (!dateStr) return '-'
    const d = new Date(dateStr)
    return d.toLocaleString('id-ID', { dateStyle: 'short', timeStyle: 'medium' })
  }

  return (
    <div className="flex flex-col h-full overflow-y-auto p-5 md:p-6 space-y-5">
      {/* Header */}
      <div className="flex items-center justify-between gap-4 rounded-3xl border border-neutral-800/70 bg-white/[0.035] p-4 shadow-lg shadow-black/20">
        <div className="flex items-center gap-3">
          <div className="grid h-11 w-11 place-items-center rounded-2xl border border-smara-300/20 bg-smara-300/10 text-smara-300 shadow-lg shadow-smara-950/20">
            <Clock className="w-5 h-5" />
          </div>
          <div>
            <h2 className="text-xl font-semibold tracking-tight text-white">Scheduler & Cronjob Dashboard</h2>
            <p className="text-xs text-neutral-500">Manajemen jadwal otomatisasi, cron 5-field, auto-retry, dan DAG chaining.</p>
          </div>
        </div>

        <div className="flex items-center gap-2">
          <button
            onClick={loadJobs}
            className="flex items-center gap-2 rounded-2xl border border-neutral-800 bg-neutral-900/80 px-3.5 py-2 text-xs font-medium text-neutral-300 hover:bg-neutral-800 transition-colors"
          >
            <RefreshCw className={`w-3.5 h-3.5 ${loading ? 'animate-spin' : ''}`} />
            Refresh
          </button>
          <button
            onClick={() => setShowAddModal(true)}
            className="flex items-center gap-2 rounded-2xl border border-smara-300/20 bg-smara-300 px-4 py-2 text-xs font-semibold text-black shadow-lg shadow-smara-950/20 transition-colors hover:bg-smara-200"
          >
            <Plus className="w-4 h-4" />
            Tambah Jadwal
          </button>
        </div>
      </div>

      {/* Add Job Modal */}
      {showAddModal && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-sm p-4">
          <div className="w-full max-w-lg rounded-3xl border border-neutral-800 bg-neutral-950 p-6 shadow-2xl space-y-4">
            <h3 className="text-lg font-semibold text-white">Tambah Jadwal Cronjob Baru</h3>
            <form onSubmit={handleAddJob} className="space-y-4">
              <div>
                <label className="block text-xs font-medium text-neutral-400 mb-1">Cron Spec / Interval</label>
                <input
                  type="text"
                  value={spec}
                  onChange={e => setSpec(e.target.value)}
                  placeholder="every 15m, daily 09:00, atau */5 * * * *"
                  className="w-full rounded-xl border border-neutral-800 bg-neutral-900 px-3.5 py-2 text-sm text-white focus:outline-none focus:border-smara-400"
                  required
                />
                <span className="text-[10px] text-neutral-500 mt-1 block">
                  Contoh: `every 15m`, `daily 08:30`, `*/5 * * * *`, atau `@hourly`
                </span>
              </div>

              <div>
                <label className="block text-xs font-medium text-neutral-400 mb-1">Workflow / Command</label>
                <input
                  type="text"
                  value={workflow}
                  onChange={e => setWorkflow(e.target.value)}
                  placeholder="python3 script.py atau nama workflow"
                  className="w-full rounded-xl border border-neutral-800 bg-neutral-900 px-3.5 py-2 text-sm text-white focus:outline-none focus:border-smara-400"
                  required
                />
              </div>

              <div className="grid grid-cols-2 gap-3">
                <div>
                  <label className="block text-xs font-medium text-neutral-400 mb-1">Max Retries</label>
                  <input
                    type="number"
                    value={retries}
                    onChange={e => setRetries(Number(e.target.value))}
                    min={0}
                    max={10}
                    className="w-full rounded-xl border border-neutral-800 bg-neutral-900 px-3.5 py-2 text-sm text-white focus:outline-none focus:border-smara-400"
                  />
                </div>
                <div>
                  <label className="block text-xs font-medium text-neutral-400 mb-1">Depends On (After Job ID)</label>
                  <input
                    type="text"
                    value={dependsOn}
                    onChange={e => setDependsOn(e.target.value)}
                    placeholder="sch-12345 (opsional)"
                    className="w-full rounded-xl border border-neutral-800 bg-neutral-900 px-3.5 py-2 text-sm text-white focus:outline-none focus:border-smara-400"
                  />
                </div>
              </div>

              <div className="flex justify-end gap-2 pt-2">
                <button
                  type="button"
                  onClick={() => setShowAddModal(false)}
                  className="px-4 py-2 rounded-xl text-xs font-medium text-neutral-400 hover:bg-neutral-900"
                >
                  Batal
                </button>
                <button
                  type="submit"
                  className="px-4 py-2 rounded-xl text-xs font-semibold bg-smara-300 text-black hover:bg-smara-200"
                >
                  Simpan Jadwal
                </button>
              </div>
            </form>
          </div>
        </div>
      )}

      {/* Scheduled Jobs Table */}
      <div className="rounded-3xl border border-neutral-800/70 bg-neutral-950/60 p-4 shadow-xl">
        {jobs.length === 0 ? (
          <div className="py-12 text-center text-neutral-500 text-sm">
            Belum ada jadwal cronjob tersimpan. Klik "Tambah Jadwal" untuk membuat.
          </div>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-left text-xs">
              <thead>
                <tr className="border-b border-neutral-800/80 text-neutral-400 uppercase tracking-wider text-[10px]">
                  <th className="pb-3 px-3">ID / Spec</th>
                  <th className="pb-3 px-3">Workflow</th>
                  <th className="pb-3 px-3">Next Run</th>
                  <th className="pb-3 px-3">Last Run / Status</th>
                  <th className="pb-3 px-3">Resilience / DAG</th>
                  <th className="pb-3 px-3 text-right">Aksi</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-neutral-800/40">
                {jobs.map(j => (
                  <tr key={j.id} className="hover:bg-white/[0.02] transition-colors">
                    <td className="py-3 px-3 font-medium text-white">
                      <div className="font-mono text-xs text-smara-300">{j.id}</div>
                      <div className="text-[11px] text-neutral-400 font-mono mt-0.5">{j.spec}</div>
                    </td>
                    <td className="py-3 px-3 text-neutral-200 font-mono text-xs max-w-xs truncate">
                      {j.workflow}
                    </td>
                    <td className="py-3 px-3 text-neutral-300 font-mono">
                      {formatDate(j.next_run_at)}
                    </td>
                    <td className="py-3 px-3">
                      <div className="text-neutral-400">{formatDate(j.last_run_at)}</div>
                      {j.last_status && (
                        <span
                          className={`inline-flex items-center gap-1 mt-1 px-2 py-0.5 rounded-full text-[10px] font-semibold ${
                            j.last_status.includes('success')
                              ? 'bg-emerald-500/10 text-emerald-400 border border-emerald-500/20'
                              : j.last_status.includes('retrying')
                              ? 'bg-amber-500/10 text-amber-300 border border-amber-500/20'
                              : 'bg-red-500/10 text-red-400 border border-red-500/20'
                          }`}
                        >
                          {j.last_status.includes('success') ? (
                            <CheckCircle2 className="w-3 h-3" />
                          ) : (
                            <AlertTriangle className="w-3 h-3" />
                          )}
                          {j.last_status}
                        </span>
                      )}
                    </td>
                    <td className="py-3 px-3 text-neutral-400 text-[11px]">
                      <div className="flex items-center gap-1">
                        <Shield className="w-3 h-3 text-smara-400" />
                        <span>Retries: {j.max_retries || 0}x</span>
                      </div>
                      {j.depends_on && (
                        <div className="flex items-center gap-1 mt-1 text-neutral-500 font-mono">
                          <ArrowRight className="w-3 h-3 text-neutral-500" />
                          <span>After: {j.depends_on}</span>
                        </div>
                      )}
                    </td>
                    <td className="py-3 px-3 text-right">
                      <div className="flex items-center justify-end gap-2">
                        <button
                          onClick={() => handleRunNow(j.id)}
                          title="Run Now"
                          className="p-1.5 rounded-lg border border-neutral-800 bg-neutral-900 text-neutral-300 hover:text-white hover:bg-neutral-800 transition-colors"
                        >
                          <Play className="w-3.5 h-3.5" />
                        </button>
                        <button
                          onClick={() => handleDeleteJob(j.id)}
                          title="Hapus"
                          className="p-1.5 rounded-lg border border-neutral-800 bg-neutral-900 text-red-400 hover:bg-red-500/10 transition-colors"
                        >
                          <Trash2 className="w-3.5 h-3.5" />
                        </button>
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>
    </div>
  )
}
