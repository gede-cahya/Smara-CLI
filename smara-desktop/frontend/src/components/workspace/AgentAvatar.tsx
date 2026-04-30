import React from 'react'
import { motion, AnimatePresence } from 'framer-motion'
import { AgentState, AgentRole } from '../../types/workspace'
import { Code, Database, Search, Sparkles, Cpu } from 'lucide-react'

const ROLE_CONFIG: Record<AgentRole, { icon: React.ElementType; color: string; bg: string }> = {
  orchestrator: { icon: Sparkles, color: '#8b5cf6', bg: 'bg-violet-500/20' },
  frontend: { icon: Code, color: '#3b82f6', bg: 'bg-blue-500/20' },
  backend: { icon: Cpu, color: '#ef4444', bg: 'bg-red-500/20' },
  database: { icon: Database, color: '#10b981', bg: 'bg-emerald-500/20' },
  qa: { icon: Search, color: '#f59e0b', bg: 'bg-amber-500/20' },
}

interface AgentAvatarProps {
  agent: AgentState
}

const AgentAvatar: React.FC<AgentAvatarProps> = ({ agent }) => {
  const config = ROLE_CONFIG[agent.role]
  const Icon = config.icon
  return (
    <motion.div
      className="absolute"
      animate={{
        x: agent.x - 24,
        y: agent.y - 24,
      }}
      transition={{
        type: agent.status === 'walking' ? 'spring' : 'tween',
        stiffness: agent.status === 'walking' ? 60 : 120,
        damping: agent.status === 'walking' ? 15 : 20,
        duration: agent.status === 'walking' ? undefined : 0.5,
      }}
      style={{ willChange: 'transform', zIndex: 10 }}
    >
      {/* Thought bubble */}
      <AnimatePresence>
        {agent.currentTask && (agent.status === 'working' || agent.status === 'thinking') && (
          <motion.div
            initial={{ opacity: 0, y: 10, scale: 0.8 }}
            animate={{ opacity: 1, y: 0, scale: 1 }}
            exit={{ opacity: 0, y: -5, scale: 0.9 }}
            className="absolute -top-14 left-1/2 -translate-x-1/2 whitespace-nowrap"
          >
            <div className="relative bg-card border border-border/50 rounded-xl px-3 py-1.5 shadow-lg text-[10px] font-medium text-foreground max-w-[180px] truncate">
              {agent.currentTask}
              <div className="absolute -bottom-1 left-1/2 -translate-x-1/2 w-2 h-2 bg-card border-r border-b border-border/50 rotate-45" />
            </div>
          </motion.div>
        )}
      </AnimatePresence>

      {/* Status ring when working */}
      {agent.status === 'working' && (
        <svg className="absolute -inset-2 w-14 h-14" viewBox="0 0 52 52">
          <circle
            cx="26" cy="26" r="24"
            fill="none"
            stroke={config.color}
            strokeWidth="2"
            strokeDasharray={`${(agent.progress || 0) * 150} 150`}
            strokeLinecap="round"
            transform="rotate(-90 26 26)"
            opacity={0.4}
          >
            <animateTransform
              attributeName="transform"
              type="rotate"
              from="0 26 26"
              to="360 26 26"
              dur="2s"
              repeatCount="indefinite"
            />
          </circle>
        </svg>
      )}

      {/* Avatar body */}
      <motion.div
        className={`w-12 h-12 rounded-full ${config.bg} border-2 flex items-center justify-center relative`}
        style={{ borderColor: config.color }}
        animate={{
          scale: agent.status === 'celebrating' ? [1, 1.2, 1] : 1,
        }}
        transition={{
          repeat: agent.status === 'celebrating' ? Infinity : 0,
          duration: 0.6,
        }}
      >
        {/* Walking bounce */}
        <motion.div
          animate={{
            y: agent.status === 'walking' ? [0, -3, 0] : 0,
          }}
          transition={{
            repeat: agent.status === 'walking' ? Infinity : 0,
            duration: 0.4,
          }}
        >
          <Icon size={20} style={{ color: config.color }} />
        </motion.div>

        {/* Typing dots when working */}
        {agent.status === 'working' && (
          <div className="absolute -bottom-1 left-1/2 -translate-x-1/2 flex gap-0.5">
            <motion.div
              className="w-1 h-1 rounded-full"
              style={{ backgroundColor: config.color }}
              animate={{ opacity: [0.3, 1, 0.3] }}
              transition={{ repeat: Infinity, duration: 1, delay: 0 }}
            />
            <motion.div
              className="w-1 h-1 rounded-full"
              style={{ backgroundColor: config.color }}
              animate={{ opacity: [0.3, 1, 0.3] }}
              transition={{ repeat: Infinity, duration: 1, delay: 0.2 }}
            />
            <motion.div
              className="w-1 h-1 rounded-full"
              style={{ backgroundColor: config.color }}
              animate={{ opacity: [0.3, 1, 0.3] }}
              transition={{ repeat: Infinity, duration: 1, delay: 0.4 }}
            />
          </div>
        )}

        {/* Online indicator */}
        <div
          className="absolute -top-0.5 -right-0.5 w-3 h-3 rounded-full border-2 border-background"
          style={{
            backgroundColor:
              agent.status === 'working'
                ? '#22c55e'
                : agent.status === 'walking'
                ? '#3b82f6'
                : agent.status === 'celebrating'
                ? '#f59e0b'
                : '#9ca3af',
          }}
        />
      </motion.div>

      {/* Agent label */}
      <div className="absolute -bottom-5 left-1/2 -translate-x-1/2 whitespace-nowrap">
        <span
          className="text-[9px] font-bold uppercase tracking-wider px-1.5 py-0.5 rounded-md bg-background/80 border border-border/30 text-muted-foreground"
        >
          {agent.id}
        </span>
      </div>
    </motion.div>
  )
}

export default React.memo(AgentAvatar)
