import { useState, useEffect, useCallback, useRef } from 'react'
import {
  VirtualWorkspaceState,
  WorkspaceEvent,
  AgentRole,
} from '../../types/workspace'

const initialState: VirtualWorkspaceState = {
  isActive: false,
  wave: 0,
  totalWaves: 0,
  agents: {},
  events: [],
}

function getSpawnPosition(role: AgentRole): { x: number; y: number } {
  const positions: Record<AgentRole, { x: number; y: number }> = {
    orchestrator: { x: 500, y: 400 },
    frontend: { x: -60, y: 200 },
    backend: { x: 1060, y: 200 },
    database: { x: 500, y: 860 },
    qa: { x: -60, y: 650 },
  }
  return positions[role] || { x: 500, y: 400 }
}

export function useWorkspace() {
  const [state, setState] = useState<VirtualWorkspaceState>(initialState)
  const eventQueue = useRef<WorkspaceEvent[]>([])
  const processing = useRef(false)

  const processNextEvent = useCallback(() => {
    if (eventQueue.current.length === 0) {
      processing.current = false
      return
    }
    processing.current = true
    const ev = eventQueue.current.shift()!

    setState(prev => {
      const newState = { ...prev, events: [...prev.events, ev].slice(-50) }

      switch (ev.type) {
        case 'workflow_start':
          newState.isActive = true
          newState.wave = 0
          newState.totalWaves = ev.total_waves || 1
          newState.agents = {}
          break

        case 'wave_start':
          newState.wave = ev.wave || prev.wave + 1
          break

        case 'agent_spawn': {
          const spawnPos = getSpawnPosition((ev.agent_role as AgentRole) || 'orchestrator')
          const id = ev.agent_id || `agent-${Date.now()}`
          newState.agents = {
            ...prev.agents,
            [id]: {
              id,
              role: (ev.agent_role as AgentRole) || 'orchestrator',
              x: ev.to_x ?? spawnPos.x,
              y: ev.to_y ?? spawnPos.y,
              status: 'idle',
            },
          }
          break
        }

        case 'agent_move': {
          const id = ev.agent_id
          if (!id || !prev.agents[id]) break
          const targetX = ev.to_x ?? prev.agents[id].x
          const targetY = ev.to_y ?? prev.agents[id].y
          newState.agents = {
            ...prev.agents,
            [id]: {
              ...prev.agents[id],
              x: targetX,
              y: targetY,
              status: 'walking',
              targetX,
              targetY,
            },
          }
          break
        }

        case 'agent_work': {
          const wid = ev.agent_id
          if (!wid || !prev.agents[wid]) break
          newState.agents = {
            ...prev.agents,
            [wid]: {
              ...prev.agents[wid],
              status: 'working',
              currentTask: ev.message || 'Working...',
              progress: 0,
            },
          }
          break
        }

        case 'agent_think': {
          const tid = ev.agent_id
          if (!tid || !prev.agents[tid]) break
          newState.agents = {
            ...prev.agents,
            [tid]: {
              ...prev.agents[tid],
              status: 'thinking',
              currentTask: ev.message || 'Thinking...',
            },
          }
          break
        }

        case 'agent_handoff': {
          const hid = ev.agent_id
          if (hid && prev.agents[hid]) {
            newState.agents = {
              ...prev.agents,
              [hid]: {
                ...prev.agents[hid],
                status: 'walking',
                x: 500,
                y: 350,
              },
            }
          }
          break
        }

        case 'agent_review': {
          const rid = ev.agent_id
          if (rid && prev.agents[rid]) {
            newState.agents = {
              ...prev.agents,
              [rid]: {
                ...prev.agents[rid],
                status: 'walking',
              },
            }
          }
          break
        }

        case 'agent_idle': {
          const iid = ev.agent_id
          if (!iid || !prev.agents[iid]) break
          newState.agents = {
            ...prev.agents,
            [iid]: {
              ...prev.agents[iid],
              status: 'idle',
              currentTask: undefined,
            },
          }
          break
        }

        case 'wave_complete':
          newState.wave = ev.wave || prev.wave
          break

        case 'workflow_complete':
          newState.isActive = false
          Object.keys(newState.agents).forEach(k => {
            newState.agents[k] = { ...newState.agents[k], status: 'celebrating' }
          })
          break
      }
      return newState
    })

    if (ev.type === 'agent_move') {
      setTimeout(() => {
        setState(prev => {
          const id = ev.agent_id
          if (!id || !prev.agents[id]) return prev
          return {
            ...prev,
            agents: {
              ...prev.agents,
              [id]: { ...prev.agents[id], status: 'idle' },
            },
          }
        })
      }, 800)
    }

    setTimeout(processNextEvent, 100)
  }, [])

  const queueEvent = useCallback((ev: WorkspaceEvent) => {
    eventQueue.current.push(ev)
    if (!processing.current) {
      processNextEvent()
    }
  }, [processNextEvent])

  const reset = useCallback(() => {
    eventQueue.current = []
    processing.current = false
    setState(initialState)
  }, [])

  // Progress animation for working agents
  useEffect(() => {
    const interval = setInterval(() => {
      setState(prev => {
        const updated = { ...prev.agents }
        let changed = false
        Object.values(updated).forEach(agent => {
          if (agent.status === 'working' && (agent.progress || 0) < 100) {
            updated[agent.id] = {
              ...agent,
              progress: Math.min((agent.progress || 0) + 2, 100),
            }
            changed = true
          }
        })
        return changed ? { ...prev, agents: updated } : prev
      })
    }, 200)
    return () => clearInterval(interval)
  }, [])

  return { state, queueEvent, reset }
}

export function buildDemoEvents(): { events: WorkspaceEvent[]; delays: number[] } {
  const events: WorkspaceEvent[] = [
    { type: 'workflow_start', total_waves: 3, wave: 0 },
    { type: 'wave_start', wave: 1, total_waves: 3 },
    { type: 'agent_spawn', agent_id: 'orch-1', agent_role: 'orchestrator', to_x: 500, to_y: 300 },
    { type: 'agent_spawn', agent_id: 'fe-1', agent_role: 'frontend', to_x: -60, to_y: 200 },
    { type: 'agent_spawn', agent_id: 'be-1', agent_role: 'backend', to_x: 1060, to_y: 200 },
    { type: 'agent_spawn', agent_id: 'db-1', agent_role: 'database', to_x: 500, to_y: 860 },
    { type: 'agent_spawn', agent_id: 'qa-1', agent_role: 'qa', to_x: -60, to_y: 650 },
    { type: 'agent_move', agent_id: 'fe-1', from_x: -60, from_y: 200, to_x: 200, to_y: 200 },
    { type: 'agent_move', agent_id: 'be-1', from_x: 1060, from_y: 200, to_x: 800, to_y: 200 },
    { type: 'agent_move', agent_id: 'db-1', from_x: 500, from_y: 860, to_x: 500, to_y: 500 },
    { type: 'agent_move', agent_id: 'qa-1', from_x: -60, from_y: 650, to_x: 300, to_y: 650 },
    { type: 'agent_work', agent_id: 'be-1', message: 'Designing REST API schema...' },
    { type: 'agent_work', agent_id: 'be-1', message: 'Writing API handlers...' },
    { type: 'agent_work', agent_id: 'db-1', message: 'Creating migration files...' },
    { type: 'agent_work', agent_id: 'fe-1', message: 'Building React components...' },
    { type: 'agent_handoff', agent_id: 'be-1', to_agent: 'fe-1', message: 'API spec ready' },
    { type: 'agent_idle', agent_id: 'be-1' },
    { type: 'agent_work', agent_id: 'fe-1', message: 'Integrating API into UI...' },
    { type: 'agent_review', agent_id: 'qa-1', to_agent: 'fe-1', message: 'Reviewing frontend code...' },
    { type: 'agent_move', agent_id: 'qa-1', from_x: 300, from_y: 650, to_x: 200, to_y: 200 },
    { type: 'agent_move', agent_id: 'qa-1', from_x: 200, from_y: 200, to_x: 800, to_y: 200 },
    { type: 'agent_move', agent_id: 'qa-1', from_x: 800, from_y: 200, to_x: 500, to_y: 500 },
    { type: 'agent_move', agent_id: 'qa-1', from_x: 500, from_y: 500, to_x: 300, to_y: 650 },
    { type: 'agent_idle', agent_id: 'qa-1' },
    { type: 'wave_complete', wave: 1, total_waves: 3 },
    { type: 'wave_start', wave: 2, total_waves: 3 },
    { type: 'wave_complete', wave: 2, total_waves: 3 },
    { type: 'wave_start', wave: 3, total_waves: 3 },
    { type: 'wave_complete', wave: 3, total_waves: 3 },
    { type: 'workflow_complete' },
  ]

  const delays = events.map((_, i) => {
    if (i <= 10) return i * 300
    if (i <= 14) return 4000 + (i - 10) * 600
    if (i <= 16) return 7000 + (i - 14) * 300
    if (i <= 17) return 8500
    if (i <= 22) return 10000 + (i - 18) * 1200
    if (i === 23) return 16000
    if (i === 24) return 17000
    if (i === 25) return 17500
    if (i === 26) return 18500
    if (i === 27) return 19000
    if (i === 28) return 19500
    if (i === 29) return 21000
    return 22000
  })

  return { events, delays }
}
