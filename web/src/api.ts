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
  const headers = new Headers(options?.headers || {})
  if (!(options?.body instanceof FormData) && !headers.has('Content-Type')) headers.set('Content-Type', 'application/json')
  const res = await fetch(`${API_BASE}${path}`, {
    ...options,
    headers,
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
  if (res.status === 204) return undefined as T
  const text = await res.text()
  if (!text) return undefined as T
  return JSON.parse(text) as T
}
export interface Status {
  status: string
  mode: string
  mode_label: string
  mode_desc: string
  mode_emoji: string
  provider: string
  provider_online?: boolean
  provider_endpoint?: string
  provider_error?: string
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
  args?: Record<string, unknown>
  // Tool-call cards collect streamed terminal output here so the UI can
  // render a single collapsible block instead of one row per stdout line.
  logs?: string[]
  status?: 'running' | 'done' | 'error'
  collapsed?: boolean
  requestPrompt?: string
  inputTokens?: number
  outputTokens?: number
  totalTokens?: number
  duration?: string
  durationMs?: number
  estimatedCostUSD?: number
  provider?: string
  model?: string
  attachments?: Array<{ path: string; size: number; kind: 'image' | 'file'; name?: string; preview?: string }>
}


export type WebSessionStatus = 'idle' | 'running' | 'cancelled' | 'error' | 'completed' | 'archived'

export interface WebSessionHistoryItem {
  role: string
  content: string
  timestamp: string
}

export interface WebSessionItem {
  id: string
  name: string
  mode: string
  workspace: string
  status: WebSessionStatus
  archived: boolean
  history: WebSessionHistoryItem[]
  total_history?: number
  history_limit?: number
  created_at: string
  updated_at: string
  error?: string
}

export interface WebSessionListResponse {
  sessions: WebSessionItem[]
}

export function fetchWebSessions(includeArchived = false, limit?: number): Promise<WebSessionListResponse> {
  const params = new URLSearchParams()
  if (includeArchived) params.set('archived', '1')
  if (limit !== undefined) params.set('limit', String(limit))
  const query = params.toString()
  return fetchJSON<WebSessionListResponse>(`/api/web-sessions${query ? `?${query}` : ''}`)
}

export function createWebSession(name?: string, mode?: string): Promise<WebSessionItem> {
  return fetchJSON<WebSessionItem>('/api/web-sessions', {
    method: 'POST',
    body: JSON.stringify({ name, mode }),
  })
}

export function getWebSession(id: string, limit?: number): Promise<WebSessionItem> {
  const params = new URLSearchParams()
  if (limit !== undefined) params.set('limit', String(limit))
  const query = params.toString()
  return fetchJSON<WebSessionItem>(`/api/web-sessions/${encodeURIComponent(id)}${query ? `?${query}` : ''}`)
}
export function renameWebSession(id: string, name: string): Promise<{ status: string }> {
  return fetchJSON<{ status: string }>(`/api/web-sessions/${encodeURIComponent(id)}/rename`, {
    method: 'POST',
    body: JSON.stringify({ name }),
  })
}


export function deleteWebSession(id: string): Promise<{ status: string }> {
  return fetchJSON<{ status: string }>(`/api/web-sessions/${encodeURIComponent(id)}`, { method: 'DELETE' })
}

export function archiveWebSession(id: string): Promise<{ status: string }> {
  return fetchJSON<{ status: string }>(`/api/web-sessions/${encodeURIComponent(id)}/archive`, { method: 'POST' })
}

export function unarchiveWebSession(id: string): Promise<{ status: string }> {
  return fetchJSON<{ status: string }>(`/api/web-sessions/${encodeURIComponent(id)}/unarchive`, { method: 'POST' })
}

export function cancelWebSession(id: string): Promise<{ status: string }> {
  return fetchJSON<{ status: string }>(`/api/web-sessions/${encodeURIComponent(id)}/cancel`, { method: 'POST' })
}

export interface RoadmapFileResponse {
  path: string
  relative_path: string
  name: string
  content: string
  size: number
  updated_at: string
  workspace: string
}

export function fetchRoadmapFile(path: string): Promise<RoadmapFileResponse> {
  return fetchJSON<RoadmapFileResponse>(`/api/roadmap?path=${encodeURIComponent(path)}`)
}

export interface ParallelOrchestrationConfig {
  enabled: boolean
  max_concurrency: number
  require_approval_high?: boolean
  require_approval_remote?: boolean
  dry_run?: boolean
  serial_fallback?: boolean
  auto_threshold?: string
}

export interface ParallelOrchestrationSummary {
  total: number
  running: number
  success: number
  failed: number
  skipped: number
  gated: number
}

export interface ParallelSubtask {
  id: string
  title: string
  description?: string
  status: string
  agent_id?: string
  kind?: string
  risk?: string
  depends_on?: string[]
  duration_ms?: number
  output?: string
  error?: string
  progress?: number
}

export interface ParallelBatch {
  id: string
  name: string
  mode: string
  status: string
  subtasks: ParallelSubtask[]
}

export interface ParallelAgent {
  id: string
  name: string
  role: string
  status: string
  current_task_id?: string
  completed: number
  total: number
}

export interface ParallelEvent {
  id: string
  time: string
  agent_id?: string
  task_id?: string
  type: string
  message: string
  status?: string
}

export interface ParallelOrchestrationSnapshot {
  active: boolean
  status: string
  run_id: string
  plan_id?: string
  title: string
  started_at?: string
  ended_at?: string
  updated_at: string
  config: ParallelOrchestrationConfig
  summary: ParallelOrchestrationSummary
  agents?: ParallelAgent[]
  tasks?: ParallelSubtask[]
  batches: ParallelBatch[]
  events?: ParallelEvent[]
  report_markdown?: string
  error?: string
}

export function fetchParallelTasksStatus(): Promise<ParallelOrchestrationSnapshot> {
  return fetchJSON<ParallelOrchestrationSnapshot>('/api/orchestration/status')
}

export function updateParallelTasksConfig(config: ParallelOrchestrationConfig): Promise<ParallelOrchestrationConfig> {
  return fetchJSON<ParallelOrchestrationConfig>('/api/orchestration/config', {
    method: 'POST',
    body: JSON.stringify(config),
  })
}

export interface UploadResponse {
  path: string
  size: number
  source: string
  kind: 'image' | 'file'
  ref: string
  name?: string
  mime?: string
}

export async function uploadClipboardImage(dataUrl: string): Promise<UploadResponse> {
  return fetchJSON<UploadResponse>('/api/clipboard/upload', {
    method: 'POST',
    body: JSON.stringify({ data_url: dataUrl }),
  })
}

export async function uploadAttachment(file: File): Promise<UploadResponse> {
  const form = new FormData()
  form.append('file', file)
  const res = await fetch('/api/attachments/upload', {
    method: 'POST',
    body: form,
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
  return res.json() as Promise<UploadResponse>
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

export interface SkillParam {
  name: string
  type: string
  description: string
  required: boolean
  default?: string | number | boolean
}

export interface SkillLineageEntry {
  version: number
  description?: string
  tags?: string[]
  step_count: number
  refined_at?: string
  refined_from?: string // "auto" | "manual" | "feedback"
}

export interface SkillItem {
  name: string
  description: string
  version: number
  tags: string[]
  parent_id?: string
  category_path?: string[]
  dependencies?: string[]
  params?: SkillParam[]
  lineage?: SkillLineageEntry[]
  run_count?: number
  success_rate?: number
  avg_duration_ms?: number
  last_run?: string
  needs_attention?: boolean
}

export interface SkillRecommendation {
  skill_name: string
  score: number
  confidence: 'high' | 'medium' | 'low'
  reasons: string[]
  clarify?: boolean
  success_rate?: number
  recently_used?: boolean
  skill?: SkillItem
}

export interface SkillRefineResponse {
  status: 'proposed' | 'applied'
  name?: string
  prompt?: string
  proposal?: string
  applied: boolean
  skill?: SkillItem
}

export interface ModeInfo {
  Name: string
  Label: string
  Emoji: string
  Description: string
}

export interface GraphInfo {
  graph_id: string
  root_path: string
  node_count: number
  edge_count: number
  languages: string[]
  created_at?: string
  updated_at?: string
  corpus_hash?: string
  version?: number
}

export interface GraphNode {
  id: string
  label: string
  type: string
  source_file: string
  source_line: number
  language: string
  content: string
  community: number
  god_score: number
  metadata?: Record<string, unknown>
}

export interface GraphEdge {
  id: string
  source: string
  target: string
  relation: string
  confidence: string
  confidence_score: number
  source_file: string
  inferred_reason?: string
}

export interface GraphData {
  graph_id: string
  root_path: string
  node_count: number
  edge_count: number
  truncated: boolean
  nodes: GraphNode[]
  edges: GraphEdge[]
}

export interface GraphListResponse {
  graphs: GraphInfo[]
}

export function fetchGraphList(): Promise<GraphListResponse> {
  return fetchJSON<GraphListResponse>('/api/graph/list')
}

export function fetchGraphData(id: string): Promise<GraphData> {
  return fetchJSON<GraphData>(`/api/graph/data?id=${encodeURIComponent(id)}`)
}

export function fetchGraphQuery(id: string, q: string, depth = 2): Promise<{ nodes: GraphNode[]; edges: GraphEdge[] }> {
  return fetchJSON(`/api/graph/query?id=${encodeURIComponent(id)}&q=${encodeURIComponent(q)}&depth=${depth}`)
}

export interface CustomWorkflowTask {
  id: string
  description: string
  type?: string
  mcp_server?: string
  tool_name?: string
}

export interface MemoryNodeConfig {
  action?: 'shared' | 'read' | 'search' | 'write' | 'read_write'
  query?: string
  content?: string
  limit?: number
}

export type LoopMode = 'count' | 'until_success' | 'until_condition' | 'while_condition' | 'for_each' | 'interval' | 'retry_backoff' | 'infinite_guarded'

export interface LoopRetryConfig {
  max_attempts?: number
  initial_delay_ms?: number
  multiplier?: number
  max_delay_ms?: number
}

export interface LoopNodeConfig {
  mode?: LoopMode
  max_iterations?: number
  delay_ms?: number
  timeout_ms?: number
  condition?: string
  items_source?: string
  retry?: LoopRetryConfig
  on_error?: 'stop' | 'continue' | 'retry' | 'skip'
}

export interface CustomWorkflowAgent {
  role: string
  description: string
  skills: string[]
  tasks: CustomWorkflowTask[]
  depends_on?: string[]
  inputs_from?: Record<string, string[]>
  memory?: MemoryNodeConfig
  loop?: LoopNodeConfig
}
export interface CustomWorkflowItem {
  name: string
  description: string
  project_dir?: string
  agents: CustomWorkflowAgent[]
  created_at?: string
  updated_at?: string
}

export interface CustomWorkflowSummary {
  name: string
  description: string
  agents: number
}

export interface CustomWorkflowListResponse {
  workflows: CustomWorkflowSummary[]
}

export interface BundledSkillItem {
  name: string
  description: string
  version: number
  tags: string[]
  params?: SkillParam[]
  category_path?: string[]
  dependencies?: string[]
}

export interface BundledSkillsResponse {
  skills: BundledSkillItem[]
}

export function fetchBundledSkills(): Promise<BundledSkillsResponse> {
  return fetchJSON<BundledSkillsResponse>('/api/skills/bundled')
}

export function installBundledSkill(name: string): Promise<{ status: string; name: string }> {
  return fetchJSON('/api/skills/install-bundled', {
    method: 'POST',
    body: JSON.stringify({ name }),
  })
}

export function fetchCustomWorkflowList(): Promise<CustomWorkflowListResponse> {
  return fetchJSON<CustomWorkflowListResponse>('/api/custom-workflow/list')
}

export function fetchCustomWorkflowGet(name: string): Promise<CustomWorkflowItem> {
  return fetchJSON<CustomWorkflowItem>(`/api/custom-workflow/get?name=${encodeURIComponent(name)}`)
}

export function saveCustomWorkflow(cw: CustomWorkflowItem): Promise<{ status: string; name: string }> {
  return fetchJSON('/api/custom-workflow/save', { method: 'POST', body: JSON.stringify(cw) })
}

export function deleteCustomWorkflow(name: string): Promise<{ status: string; name: string }> {
  return fetchJSON('/api/custom-workflow/delete', { method: 'POST', body: JSON.stringify({ name }) })
}

export function runCustomWorkflow(name: string, projectDir?: string): Promise<unknown> {
  return fetchJSON('/api/custom-workflow/run', {
    method: 'POST',
    body: JSON.stringify({ name, project_dir: projectDir || undefined }),
  })
}

export function importCustomWorkflow(name: string, json: string): Promise<{ status: string; name: string }> {
  return fetchJSON('/api/custom-workflow/import', {
    method: 'POST',
    body: JSON.stringify({ name, json }),
  })
}

export function getCwd(): Promise<{ path: string }> {
  return fetchJSON('/api/fs/cwd')
}

export interface FSEntry {
  name: string
  is_dir: boolean
}

export function listDir(path: string): Promise<{ path: string; entries: FSEntry[] }> {
  return fetchJSON(`/api/fs/list?path=${encodeURIComponent(path)}`)
}
