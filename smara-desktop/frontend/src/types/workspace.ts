export type AgentRole = 'orchestrator' | 'frontend' | 'backend' | 'database' | 'qa'

export type AgentStatus = 'idle' | 'walking' | 'working' | 'thinking' | 'celebrating'

export interface WorkspaceEvent {
  type: string
  agent_id?: string
  agent_role?: AgentRole
  from_x?: number
  from_y?: number
  to_x?: number
  to_y?: number
  to_agent?: string
  message?: string
  wave?: number
  total_waves?: number
}

export interface AgentState {
  id: string
  role: AgentRole
  x: number
  y: number
  status: AgentStatus
  currentTask?: string
  targetX?: number
  targetY?: number
  progress?: number
}

export interface DeskPosition {
  x: number
  y: number
  label: string
  role: AgentRole
  color: string
}

export const DESK_POSITIONS: DeskPosition[] = [
  { x: 500, y: 300, label: 'Orchestrator', role: 'orchestrator', color: '#8b5cf6' },
  { x: 200, y: 200, label: 'Frontend', role: 'frontend', color: '#3b82f6' },
  { x: 800, y: 200, label: 'Backend', role: 'backend', color: '#ef4444' },
  { x: 500, y: 500, label: 'Database', role: 'database', color: '#10b981' },
  { x: 300, y: 650, label: 'QA Zone', role: 'qa', color: '#f59e0b' },
]

export interface VirtualWorkspaceState {
  isActive: boolean
  wave: number
  totalWaves: number
  agents: Record<string, AgentState>
  events: WorkspaceEvent[]
  lastEvent?: string
}
