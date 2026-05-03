const API_BASE = ''

export class APIError extends Error {
  status: number
  raw: string
  constructor(message: string, status: number, raw: string) {
    super(message)
    this.name = 'APIError'
    this.status = status
    this.raw = raw
  }
}

export async function fetchJSON<T = unknown>(path: string, options?: RequestInit): Promise<T> {
  const res = await fetch(`${API_BASE}${path}`, {
    headers: { 'Content-Type': 'application/json' },
    ...options,
  })
  if (!res.ok) {
    const raw = await res.text()
    let message = raw || res.statusText
    try {
      const parsed = JSON.parse(raw)
      if (parsed.error) message = parsed.error
    } catch { /* ignore */ }
    throw new APIError(message, res.status, raw)
  }
  return res.json() as Promise<T>
}

export interface Status {
  status: string
  mode: string
  mode_label: string
  mode_desc: string
  mode_emoji: string
  provider: string
  workspace: string
  version: string
}

export interface MemoryItem {
  id: number
  content: string
  tags: string[]
  source: string
  created_at: string
  category_id?: number
  workspace_id?: number
}

export interface WorkspaceItem {
  id: number
  name: string
  path: string
  created_at: string
}

export interface CategoryItem {
  id: number
  name: string
  description: string
  parent_id: number | null
  created_at: string
}

export interface MCPInfo {
  name: string
  connected: boolean
  tools: number
  error: string
}

export interface ChatMessage {
  role: 'user' | 'assistant' | 'error' | 'phase' | 'tool_call' | 'tool_result' | 'log'
  content: string
  timestamp: Date | string
  phase?: string
  tool?: string
  server?: string
  output?: string
}

export interface AgentSpec {
  role: string
  description: string
  skills: string[]
  tasks: Array<{
    id: string
    description: string
    type?: string
    mcp_server?: string
    tool_name?: string
  }>
  depends_on?: string[]
}

export interface Blueprint {
  project_name: string
  description: string
  domain: string
  prd: string
  architecture: string
  agents: AgentSpec[]
  thoughts?: string[]
}

export interface SkillItem {
  name: string
  description: string
  version: number
  tags: string[]
  parent_id?: string
  category_path?: string[]
  dependencies?: string[]
}

export interface ModeInfo {
  Name: string
  Label: string
  Emoji: string
  Description: string
}
