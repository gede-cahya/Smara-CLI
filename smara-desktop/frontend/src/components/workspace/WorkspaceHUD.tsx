import React from 'react'
import { motion, AnimatePresence } from 'framer-motion'
import { VirtualWorkspaceState } from '../../types/workspace'
import { Waves, Users, Activity, Terminal } from 'lucide-react'

interface WorkspaceHUDProps {
  state: VirtualWorkspaceState
}

const ROLE_LABELS: Record<string, string> = {
  orchestrator: 'Orchestrator',
  frontend: 'Frontend',
  backend: 'Backend',
  database: 'Database',
  qa: 'QA',
}

const STATUS_COLORS: Record<string, string> = {
  idle: 'bg-gray-500',
  walking: 'bg-blue-500',
  working: 'bg-green-500',
  thinking: 'bg-yellow-500',
  celebrating: 'bg-purple-500',
}

const WorkspaceHUD: React.FC<WorkspaceHUDProps> = ({ state }) => {
  const agents = Object.values(state.agents)
  const progress = state.totalWaves > 0 ? ((state.wave) / state.totalWaves) * 100 : 0

  return (
    <div className="absolute inset-x-0 top-0 z-20 pointer-events-none">
      {/* Top Bar */}
      <div className="flex items-center justify-between px-6 py-3">
        <div className="flex items-center gap-3">
          <div className="glass-card px-4 py-2 rounded-xl flex items-center gap-2 border border-border/30">
            <Waves size={14} className="text-primary" />
            <span className="text-xs font-bold uppercase tracking-wider">
              Wave {state.wave}/{state.totalWaves || '?'}
            </span>
            <div className="w-24 h-1.5 bg-muted rounded-full overflow-hidden ml-1">
              <motion.div
                className="h-full bg-primary rounded-full"
                initial={{ width: 0 }}
                animate={{ width: `${progress}%` }}
                transition={{ duration: 0.5 }}
              />
            </div>
          </div>

          <div className="glass-card px-3 py-2 rounded-xl flex items-center gap-2 border border-border/30">
            <Users size={14} className="text-primary" />
            <span className="text-xs font-bold">{agents.length} Agents</span>
          </div>
        </div>

        <div className="flex items-center gap-2">
          <div className={`glass-card px-3 py-2 rounded-xl flex items-center gap-2 border border-border/30 ${state.isActive ? 'border-green-500/30' : ''}`}>
            <Activity size={14} className={state.isActive ? 'text-green-500' : 'text-muted-foreground'} />
            <span className={`text-xs font-bold uppercase tracking-wider ${state.isActive ? 'text-green-500' : 'text-muted-foreground'}`}>
              {state.isActive ? 'Running' : 'Idle'}
            </span>
          </div>
        </div>
      </div>

      {/* Agent Roster (bottom-left) */}
      <div className="absolute bottom-4 left-4 pointer-events-auto">
        <div className="glass-card rounded-xl border border-border/30 overflow-hidden max-w-[240px]">
          <div className="px-3 py-2 border-b border-border/20 flex items-center gap-2">
            <Users size={12} className="text-muted-foreground" />
            <span className="text-[10px] font-bold uppercase tracking-wider text-muted-foreground">Agent Roster</span>
          </div>
          <div className="p-2 space-y-1 max-h-[200px] overflow-y-auto">
            <AnimatePresence>
              {agents.map((agent) => (
                <motion.div
                  key={agent.id}
                  initial={{ opacity: 0, x: -10 }}
                  animate={{ opacity: 1, x: 0 }}
                  exit={{ opacity: 0, x: -10 }}
                  className="flex items-center gap-2 px-2 py-1.5 rounded-lg hover:bg-muted/30 transition-colors"
                >
                  <div className={`w-2 h-2 rounded-full ${STATUS_COLORS[agent.status] || 'bg-gray-500'}`} />
                  <span className="text-[10px] font-medium flex-1 truncate">{agent.id}</span>
                  <span className="text-[9px] text-muted-foreground uppercase">{ROLE_LABELS[agent.role]}</span>
                </motion.div>
              ))}
            </AnimatePresence>
            {agents.length === 0 && (
              <div className="px-2 py-3 text-center text-[10px] text-muted-foreground">
                No agents active
              </div>
            )}
          </div>
        </div>
      </div>

      {/* Event Log Mini (bottom-right) */}
      <div className="absolute bottom-4 right-4 pointer-events-auto">
        <div className="glass-card rounded-xl border border-border/30 overflow-hidden max-w-[260px]">
          <div className="px-3 py-2 border-b border-border/20 flex items-center gap-2">
            <Terminal size={12} className="text-muted-foreground" />
            <span className="text-[10px] font-bold uppercase tracking-wider text-muted-foreground">Event Log</span>
          </div>
          <div className="p-2 space-y-1 max-h-[160px] overflow-y-auto">
            {state.events.slice(-5).reverse().map((ev, i) => (
              <motion.div
                key={`${ev.type}-${i}`}
                initial={{ opacity: 0, y: -5 }}
                animate={{ opacity: 1, y: 0 }}
                className="text-[10px] text-muted-foreground px-1 py-0.5"
              >
                <span className="text-primary font-mono">[{ev.type}]</span>
                {ev.agent_id && <span className="ml-1">{ev.agent_id}</span>}
                {ev.message && <span className="ml-1 truncate">→ {ev.message}</span>}
              </motion.div>
            ))}
            {state.events.length === 0 && (
              <div className="px-1 py-2 text-center text-[10px] text-muted-foreground/60">
                Waiting for workflow events...
              </div>
            )}
          </div>
        </div>
      </div>
    </div>
  )
}

export default React.memo(WorkspaceHUD)
