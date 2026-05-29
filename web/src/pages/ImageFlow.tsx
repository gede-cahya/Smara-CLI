import { useCallback, useEffect, useMemo, useRef, useState, type PointerEvent } from 'react'
import {
  AlertCircle,
  CheckCircle,
  Download,
  Eye,
  FileImage,
  Image as ImageIcon,
  Images,
  Play,
  Plus,
  Save,
  SlidersHorizontal,
  Sparkles,
  Trash2,
  Upload,
  Wand2,
  XCircle,
  Square,
} from 'lucide-react'
import {
  addEdge,
  applyEdgeChanges,
  applyNodeChanges,
  Background,
  Controls,
  Handle,
  MiniMap,
  Position,
  ReactFlow,
  type Connection,
  type Edge,
  type EdgeChange,
  type Node,
  type NodeChange,
} from '@xyflow/react'
import '@xyflow/react/dist/style.css'
import { fetchJSON, uploadAttachment, uploadClipboardImage } from '../api'

const STORAGE_KEY = 'smara-image-flow-draft'

type PortType = 'text' | 'image' | 'mask' | 'model' | 'number' | 'seed' | 'latent' | 'boolean' | 'conditioning' | 'json'
type ImageNodeKind = 'text_prompt' | 'batch_prompt' | 'model_loader' | 'image_input' | 'mask_input' | 'random_seed' | 'generate_image' | 'image_to_image' | 'inpaint' | 'outpaint' | 'upscale' | 'image_preview' | 'image_output'
type NodeStatus = 'idle' | 'queued' | 'running' | 'success' | 'failed'

type Port = { name: string; type: PortType; required?: boolean }
type RegistryNode = {
  type: ImageNodeKind
  label: string
  category: string
  description: string
  inputs: Port[]
  outputs: Port[]
  defaults: Record<string, string | number>
}

type ImageNodeData = {
  kind: ImageNodeKind
  label: string
  description: string
  inputs: Port[]
  outputs: Port[]
  config: Record<string, string | number>
  status: NodeStatus
  lastResult?: string
}

type ImageFlowWorkflow = {
  version: '1.0'
  name: string
  nodes: Array<{
    id: string
    type: ImageNodeKind
    position: { x: number; y: number }
    config: Record<string, string | number>
  }>
  edges: Array<{
    id: string
    source: string
    sourcePort?: string | null
    target: string
    targetPort?: string | null
  }>
  metadata: { updatedAt: string }
}

type ImageFlowSummary = { name: string; nodes: number; edges: number; updatedAt?: string }
type ImageFlowRunResult = { status: string; path?: string; image_url?: string; model?: string; logs?: string[] }
type ImageFlowJob = {
  id: string
  workflow: string
  status: 'queued' | 'running' | 'success' | 'failed' | 'canceled'
  nodes: Array<{ node_id: string; type: ImageNodeKind; status: NodeStatus | 'skipped'; result?: string }>
  logs: string[]
  result?: ImageFlowRunResult
  error?: string
}

type ImageFlowAsset = {
  id: string
  workflow: string
  job_id: string
  path: string
  image_url?: string
  model?: string
  mode?: string
  provider?: string
  prompt?: string
  width?: number
  height?: number
  seed?: number
  size_bytes?: number
  archived?: boolean
  created_at: string
}

type ImageFlowModelStatus = {
  provider: string
  model: string
  quality: string
  output_dir: string
  base_url?: string
  api_key_configured: boolean
  image_capable: boolean
  status: string
  message: string
}

type ImageFlowModelsResponse = {
  current: ImageFlowModelStatus
  available?: ImageFlowModelStatus[]
}

type ImageFlowTemplate = {
  id: string
  name: string
  description: string
  tags?: string[]
  workflow: ImageFlowWorkflow
}

type AgentIssue = { level: string; node_id?: string; edge_id?: string; message: string }
type AgentExplanation = { summary: string; steps?: string[]; nodes?: string[]; warnings?: string[] }
type AgentSuggestion = { level: string; message: string }

const registry: RegistryNode[] = [
  {
    type: 'text_prompt',
    label: 'Text Prompt',
    category: 'Input',
    description: 'Prompt teks utama untuk generasi/edit gambar.',
    inputs: [],
    outputs: [{ name: 'prompt', type: 'text', required: true }],
    defaults: { prompt: 'cinematic portrait, soft light, high detail' },
  },
  {
    type: 'batch_prompt',
    label: 'Batch Prompt',
    category: 'Input',
    description: 'Daftar prompt multi-baris untuk menghasilkan banyak output.',
    inputs: [],
    outputs: [{ name: 'prompt', type: 'text', required: true }],
    defaults: { prompts: 'cinematic landscape at sunrise\neditorial portrait on muted backdrop', mode: 'random', index: 1 },
  },
  {
    type: 'model_loader',
    label: 'Model Loader',
    category: 'Model',
    description: 'Provider, model, dan metadata model image.',
    inputs: [],
    outputs: [{ name: 'model', type: 'model', required: true }],
    defaults: { provider: 'openai', model: 'gpt-image-2', quality: 'high' },
  },
  {
    type: 'image_input',
    label: 'Image Input',
    category: 'Input',
    description: 'Path gambar lokal untuk image-to-image atau editing.',
    inputs: [],
    outputs: [{ name: 'image', type: 'image', required: true }],
    defaults: { image_path: '', source: 'local file' },
  },
  {
    type: 'mask_input',
    label: 'Mask Input',
    category: 'Input',
    description: 'Path mask PNG/WebP untuk area edit inpaint.',
    inputs: [],
    outputs: [{ name: 'mask', type: 'mask', required: true }],
    defaults: { mask_path: '', source: 'local mask' },
  },
  {
    type: 'random_seed',
    label: 'Random Seed',
    category: 'Utility',
    description: 'Seed tetap atau random untuk variasi output workflow.',
    inputs: [],
    outputs: [{ name: 'seed', type: 'seed', required: true }],
    defaults: { mode: 'random', seed: 1024, min: 1, max: 2147483647 },
  },
  {
    type: 'generate_image',
    label: 'Generate Image',
    category: 'Generation',
    description: 'Node sampler/generator untuk text-to-image MVP.',
    inputs: [
      { name: 'prompt', type: 'text', required: true },
      { name: 'model', type: 'model', required: true },
      { name: 'seed', type: 'seed' },
    ],
    outputs: [{ name: 'image', type: 'image', required: true }],
    defaults: { seed: 1024, steps: 28, cfg: 7, width: 1024, height: 1024 },
  },
  {
    type: 'image_to_image',
    label: 'Image Edit',
    category: 'Image Processing',
    description: 'Edit/style transfer dari input image memakai prompt.',
    inputs: [
      { name: 'image', type: 'image', required: true },
      { name: 'prompt', type: 'text', required: true },
      { name: 'model', type: 'model', required: true },
      { name: 'seed', type: 'seed' },
    ],
    outputs: [{ name: 'image', type: 'image', required: true }],
    defaults: { prompt: 'preserve composition, improve lighting and detail', width: 1024, height: 1024 },
  },
  {
    type: 'inpaint',
    label: 'Inpaint',
    category: 'Image Processing',
    description: 'Edit area ter-mask pada input image memakai instruksi teks.',
    inputs: [
      { name: 'image', type: 'image', required: true },
      { name: 'mask', type: 'mask', required: true },
      { name: 'prompt', type: 'text', required: true },
      { name: 'model', type: 'model', required: true },
      { name: 'seed', type: 'seed' },
    ],
    outputs: [{ name: 'image', type: 'image', required: true }],
    defaults: { prompt: 'replace masked area naturally, match lighting and perspective', width: 1024, height: 1024 },
  },
  {
    type: 'outpaint',
    label: 'Outpaint',
    category: 'Image Processing',
    description: 'Perluas kanvas/komposisi dari input image memakai prompt.',
    inputs: [
      { name: 'image', type: 'image', required: true },
      { name: 'prompt', type: 'text', required: true },
      { name: 'model', type: 'model', required: true },
      { name: 'seed', type: 'seed' },
    ],
    outputs: [{ name: 'image', type: 'image', required: true }],
    defaults: { prompt: 'extend the scene beyond the original frame, keep style consistent', direction: 'all', width: 1536, height: 1024 },
  },
  {
    type: 'upscale',
    label: 'Upscale',
    category: 'Image Processing',
    description: 'Perbesar dan pertajam input image melalui image edit provider.',
    inputs: [
      { name: 'image', type: 'image', required: true },
      { name: 'prompt', type: 'text' },
      { name: 'model', type: 'model', required: true },
      { name: 'seed', type: 'seed' },
    ],
    outputs: [{ name: 'image', type: 'image', required: true }],
    defaults: { prompt: 'upscale this image, preserve identity and composition, improve crisp detail', scale: 2, width: 2048, height: 2048 },
  },
  {
    type: 'image_preview',
    label: 'Image Preview',
    category: 'Output',
    description: 'Preview hasil gambar dan metadata run terakhir.',
    inputs: [{ name: 'image', type: 'image', required: true }],
    outputs: [{ name: 'image', type: 'image' }],
    defaults: { zoom: 100, compare: 'off' },
  },
  {
    type: 'image_output',
    label: 'Image Output',
    category: 'Output',
    description: 'Simpan hasil ke gallery atau ekspor file.',
    inputs: [{ name: 'image', type: 'image', required: true }],
    outputs: [{ name: 'asset', type: 'json' }],
    defaults: { destination: 'gallery', filename: 'image-flow-output.png' },
  },
]

const statusLabel: Record<NodeStatus, string> = {
  idle: 'Idle',
  queued: 'Queued',
  running: 'Running',
  success: 'Success',
  failed: 'Failed',
}

function registryByType(type: ImageNodeKind) {
  return registry.find(item => item.type === type) || registry[0]
}

function portColor(type: PortType) {
  if (type === 'image') return '#38bdf8'
  if (type === 'model') return '#c084fc'
  if (type === 'number' || type === 'seed') return '#f59e0b'
  if (type === 'mask') return '#60a5fa'
  if (type === 'latent' || type === 'conditioning') return '#fb7185'
  if (type === 'boolean') return '#f472b6'
  if (type === 'json') return '#34d399'
  return '#bef264'
}

function ImageFlowNode({ data }: { data: ImageNodeData }) {
  const Icon = data.kind === 'text_prompt' || data.kind === 'batch_prompt' ? Sparkles : data.kind === 'model_loader' || data.kind === 'random_seed' ? SlidersHorizontal : data.kind === 'image_input' || data.kind === 'mask_input' ? Images : data.kind === 'generate_image' || data.kind === 'image_to_image' || data.kind === 'inpaint' || data.kind === 'outpaint' || data.kind === 'upscale' ? Wand2 : data.kind === 'image_preview' ? Eye : FileImage
  const statusClass = data.status === 'success' ? 'text-emerald-300 bg-emerald-400/10' : data.status === 'failed' ? 'text-red-300 bg-red-400/10' : data.status === 'running' ? 'text-sky-300 bg-sky-400/10' : data.status === 'queued' ? 'text-amber-300 bg-amber-400/10' : 'text-neutral-500 bg-neutral-900/65'

  return (
    <div className="image-flow-node min-w-[230px] max-w-[270px] overflow-hidden rounded-xl border border-neutral-800 bg-[#0b1008] shadow-xl shadow-black/35">
      {data.inputs.map((port, idx) => (
        <Handle
          key={port.name}
          id={port.name}
          type="target"
          position={Position.Left}
          style={{ top: 62 + idx * 24, background: portColor(port.type), borderColor: '#080c06' }}
          className="!h-3 !w-3"
        />
      ))}
      <div className="flex items-center gap-2 border-b border-neutral-900 bg-neutral-900/50 px-3 py-2">
        <span className="grid h-7 w-7 place-items-center rounded-lg bg-smara-300/10 text-smara-200">
          <Icon className="h-3.5 w-3.5" />
        </span>
        <div className="min-w-0 flex-1">
          <div className="truncate text-xs font-semibold text-neutral-100">{data.label}</div>
          <div className="text-[10px] uppercase tracking-[0.16em] text-neutral-600">{data.kind.replace('_', ' ')}</div>
        </div>
        <span className={`rounded-full px-2 py-0.5 text-[9px] ${statusClass}`}>{statusLabel[data.status]}</span>
      </div>
      <div className="space-y-2 px-3 py-2.5">
        <p className="line-clamp-2 text-[11px] leading-4 text-neutral-400">{data.description}</p>
        <div className="grid gap-1 text-[10px] text-neutral-500">
          {data.inputs.length > 0 && <div>In: {data.inputs.map(p => `${p.name}:${p.type}`).join(', ')}</div>}
          {data.outputs.length > 0 && <div>Out: {data.outputs.map(p => `${p.name}:${p.type}`).join(', ')}</div>}
        </div>
        {data.lastResult && <div className="rounded-md bg-neutral-900/65 px-2 py-1 text-[10px] text-neutral-300">{data.lastResult}</div>}
      </div>
      {data.outputs.map((port, idx) => (
        <Handle
          key={port.name}
          id={port.name}
          type="source"
          position={Position.Right}
          style={{ top: 62 + idx * 24, background: portColor(port.type), borderColor: '#080c06' }}
          className="!h-3 !w-3"
        />
      ))}
    </div>
  )
}

const nodeTypes = { imageFlowNode: ImageFlowNode }

function defaultNodes(): Node<ImageNodeData>[] {
  const layout: Array<[ImageNodeKind, number, number]> = [
    ['text_prompt', 40, 90],
    ['model_loader', 40, 280],
    ['generate_image', 380, 170],
    ['image_preview', 720, 90],
    ['image_output', 720, 300],
  ]
  return layout.map(([kind, x, y], idx) => makeNode(kind, `${kind}_${idx + 1}`, { x, y }))
}

function editTemplateNodes(): Node<ImageNodeData>[] {
  const layout: Array<[ImageNodeKind, number, number]> = [
    ['image_input', 40, 70],
    ['text_prompt', 40, 260],
    ['model_loader', 40, 450],
    ['image_to_image', 390, 230],
    ['image_preview', 740, 150],
    ['image_output', 740, 360],
  ]
  return layout.map(([kind, x, y], idx) => makeNode(kind, `${kind}_${idx + 1}`, { x, y }))
}

function inpaintTemplateNodes(): Node<ImageNodeData>[] {
  const layout: Array<[ImageNodeKind, number, number]> = [
    ['image_input', 40, 50],
    ['mask_input', 40, 210],
    ['batch_prompt', 40, 370],
    ['model_loader', 40, 560],
    ['inpaint', 400, 250],
    ['image_preview', 760, 170],
    ['image_output', 760, 380],
  ]
  return layout.map(([kind, x, y], idx) => makeNode(kind, `${kind}_${idx + 1}`, { x, y }))
}

function defaultEdges(): Edge[] {
  return [
    { id: 'prompt-generate', source: 'text_prompt_1', sourceHandle: 'prompt', target: 'generate_image_3', targetHandle: 'prompt', animated: true, style: { stroke: '#bef264', strokeWidth: 2 } },
    { id: 'model-generate', source: 'model_loader_2', sourceHandle: 'model', target: 'generate_image_3', targetHandle: 'model', style: { stroke: '#c084fc', strokeWidth: 2 } },
    { id: 'generate-preview', source: 'generate_image_3', sourceHandle: 'image', target: 'image_preview_4', targetHandle: 'image', style: { stroke: '#38bdf8', strokeWidth: 2 } },
    { id: 'generate-output', source: 'generate_image_3', sourceHandle: 'image', target: 'image_output_5', targetHandle: 'image', style: { stroke: '#38bdf8', strokeWidth: 2 } },
  ]
}

function editTemplateEdges(): Edge[] {
  return [
    { id: 'image-edit', source: 'image_input_1', sourceHandle: 'image', target: 'image_to_image_4', targetHandle: 'image', style: { stroke: '#38bdf8', strokeWidth: 2 } },
    { id: 'prompt-edit', source: 'text_prompt_2', sourceHandle: 'prompt', target: 'image_to_image_4', targetHandle: 'prompt', animated: true, style: { stroke: '#bef264', strokeWidth: 2 } },
    { id: 'model-edit', source: 'model_loader_3', sourceHandle: 'model', target: 'image_to_image_4', targetHandle: 'model', style: { stroke: '#c084fc', strokeWidth: 2 } },
    { id: 'edit-preview', source: 'image_to_image_4', sourceHandle: 'image', target: 'image_preview_5', targetHandle: 'image', style: { stroke: '#38bdf8', strokeWidth: 2 } },
    { id: 'edit-output', source: 'image_to_image_4', sourceHandle: 'image', target: 'image_output_6', targetHandle: 'image', style: { stroke: '#38bdf8', strokeWidth: 2 } },
  ]
}

function inpaintTemplateEdges(): Edge[] {
  return [
    { id: 'image-inpaint', source: 'image_input_1', sourceHandle: 'image', target: 'inpaint_5', targetHandle: 'image', style: { stroke: '#38bdf8', strokeWidth: 2 } },
    { id: 'mask-inpaint', source: 'mask_input_2', sourceHandle: 'mask', target: 'inpaint_5', targetHandle: 'mask', style: { stroke: '#38bdf8', strokeWidth: 2 } },
    { id: 'batch-inpaint', source: 'batch_prompt_3', sourceHandle: 'prompt', target: 'inpaint_5', targetHandle: 'prompt', animated: true, style: { stroke: '#bef264', strokeWidth: 2 } },
    { id: 'model-inpaint', source: 'model_loader_4', sourceHandle: 'model', target: 'inpaint_5', targetHandle: 'model', style: { stroke: '#c084fc', strokeWidth: 2 } },
    { id: 'inpaint-preview', source: 'inpaint_5', sourceHandle: 'image', target: 'image_preview_6', targetHandle: 'image', style: { stroke: '#38bdf8', strokeWidth: 2 } },
    { id: 'inpaint-output', source: 'inpaint_5', sourceHandle: 'image', target: 'image_output_7', targetHandle: 'image', style: { stroke: '#38bdf8', strokeWidth: 2 } },
  ]
}

function edgeColorFromHandle(handle?: string | null) {
  if (handle === 'image' || handle === 'mask') return '#38bdf8'
  if (handle === 'model') return '#c084fc'
  if (handle === 'seed') return '#f59e0b'
  if (handle === 'mask') return '#60a5fa'
  if (handle === 'asset') return '#34d399'
  return '#bef264'
}

function makeNode(kind: ImageNodeKind, id: string, position: { x: number; y: number }): Node<ImageNodeData> {
  const def = registryByType(kind)
  return {
    id,
    type: 'imageFlowNode',
    position,
    data: {
      kind,
      label: def.label,
      description: def.description,
      inputs: def.inputs,
      outputs: def.outputs,
      config: { ...def.defaults },
      status: 'idle',
    },
  }
}

function toWorkflow(name: string, nodes: Node<ImageNodeData>[], edges: Edge[]): ImageFlowWorkflow {
  return {
    version: '1.0',
    name,
    nodes: nodes.map(node => ({
      id: node.id,
      type: node.data.kind,
      position: node.position,
      config: node.data.config,
    })),
    edges: edges.map(edge => ({
      id: edge.id,
      source: edge.source,
      sourcePort: edge.sourceHandle,
      target: edge.target,
      targetPort: edge.targetHandle,
    })),
    metadata: { updatedAt: new Date().toISOString() },
  }
}

function fromWorkflow(workflow: ImageFlowWorkflow): { nodes: Node<ImageNodeData>[]; edges: Edge[]; name: string } {
  const nodes = workflow.nodes.map(item => {
    const node = makeNode(item.type, item.id, item.position)
    node.data = { ...node.data, config: { ...node.data.config, ...item.config } }
    return node
  })
  const edges = workflow.edges.map(edge => ({
    id: edge.id,
    source: edge.source,
    sourceHandle: edge.sourcePort || undefined,
    target: edge.target,
    targetHandle: edge.targetPort || undefined,
    animated: edge.sourcePort === 'prompt',
    style: { stroke: edgeColorFromHandle(edge.sourcePort), strokeWidth: 2 },
  }))
  return { nodes, edges, name: workflow.name || 'Basic Text to Image' }
}

function validateGraph(nodes: Node<ImageNodeData>[], edges: Edge[]) {
  const issues: string[] = []
  const nodeMap = new Map(nodes.map(node => [node.id, node]))
  edges.forEach(edge => {
    const source = nodeMap.get(edge.source)
    const target = nodeMap.get(edge.target)
    const sourcePort = source?.data.outputs.find(port => port.name === edge.sourceHandle)
    const targetPort = target?.data.inputs.find(port => port.name === edge.targetHandle)
    if (!source || !target) issues.push(`Edge ${edge.id} references missing node.`)
    else if (!sourcePort || !targetPort) issues.push(`Edge ${edge.id} uses unknown port.`)
    else if (sourcePort.type !== targetPort.type) issues.push(`${source.data.label}.${sourcePort.name} (${sourcePort.type}) tidak kompatibel dengan ${target.data.label}.${targetPort.name} (${targetPort.type}).`)
  })
  nodes.forEach(node => {
    node.data.inputs.filter(port => port.required).forEach(port => {
      const connected = edges.some(edge => edge.target === node.id && edge.targetHandle === port.name)
      if (!connected) issues.push(`${node.data.label} membutuhkan input ${port.name}:${port.type}.`)
    })
  })
  return issues
}

export default function ImageFlow() {
  const [name, setName] = useState('Basic Text to Image')
  const [nodes, setNodes] = useState<Node<ImageNodeData>[]>(defaultNodes)
  const [edges, setEdges] = useState<Edge[]>(defaultEdges)
  const [savedFlows, setSavedFlows] = useState<ImageFlowSummary[]>([])
  const [selectedNodeId, setSelectedNodeId] = useState<string | null>('generate_image_3')
  const [issues, setIssues] = useState<string[]>([])
  const [running, setRunning] = useState(false)
  const [saving, setSaving] = useState(false)
  const [notice, setNotice] = useState('Draft lokal siap. Simpan untuk menyimpan ke backend.')
  const [previewUrl, setPreviewUrl] = useState('')
  const [activeJobId, setActiveJobId] = useState('')
  const [runLogs, setRunLogs] = useState<string[]>([])
  const [gallery, setGallery] = useState<ImageFlowAsset[]>([])
  const [galleryQuery, setGalleryQuery] = useState('')
  const [galleryMode, setGalleryMode] = useState('')
  const [showArchivedAssets, setShowArchivedAssets] = useState(false)
  const [modelStatus, setModelStatus] = useState<ImageFlowModelStatus | null>(null)
  const [availableModels, setAvailableModels] = useState<ImageFlowModelStatus[]>([])
  const [maskBrushSize, setMaskBrushSize] = useState(48)
  const [maskMode, setMaskMode] = useState<'paint' | 'erase'>('paint')
  const [maskHistory, setMaskHistory] = useState<string[]>([])
  const [maskRedoHistory, setMaskRedoHistory] = useState<string[]>([])
  const [maskOpacity, setMaskOpacity] = useState(70)
  const [maskFeather, setMaskFeather] = useState(0)
  const [maskZoom, setMaskZoom] = useState(100)
  const [maskCanvasSize, setMaskCanvasSize] = useState({ width: 1024, height: 1024 })
  const [templates, setTemplates] = useState<ImageFlowTemplate[]>([])
  const [agentInstruction, setAgentInstruction] = useState('')
  const [agentPrompt, setAgentPrompt] = useState('')
  const [agentIssues, setAgentIssues] = useState<AgentIssue[]>([])
  const [agentExplanation, setAgentExplanation] = useState<AgentExplanation | null>(null)
  const [agentSuggestions, setAgentSuggestions] = useState<AgentSuggestion[]>([])
  const [agentActions, setAgentActions] = useState<string[]>([])
  const [agentBusy, setAgentBusy] = useState(false)
  const importRef = useRef<HTMLInputElement | null>(null)
  const imageUploadRef = useRef<HTMLInputElement | null>(null)
  const maskCanvasRef = useRef<HTMLCanvasElement | null>(null)
  const maskDrawingRef = useRef(false)

  const selectedNode = useMemo(() => nodes.find(node => node.id === selectedNodeId) || null, [nodes, selectedNodeId])
  const inputImagePath = useMemo(() => {
    const imageInput = nodes.find(node => node.data.kind === 'image_input' && String(node.data.config.image_path || '').trim())
    return imageInput ? String(imageInput.data.config.image_path || '').trim() : ''
  }, [nodes])
  const inputPreviewUrl = inputImagePath ? `/api/local-image?path=${encodeURIComponent(inputImagePath)}` : ''
  const selectedAssetPath = selectedNode?.data.kind === 'mask_input'
    ? String(selectedNode.data.config.mask_path || '').trim()
    : selectedNode?.data.kind === 'image_input'
      ? String(selectedNode.data.config.image_path || '').trim()
      : ''
  const selectedAssetPreviewUrl = selectedAssetPath ? `/api/local-image?path=${encodeURIComponent(selectedAssetPath)}` : ''
  const workflowJSON = useMemo(() => JSON.stringify(toWorkflow(name, nodes, edges), null, 2), [name, nodes, edges])
  const workflow = useMemo(() => toWorkflow(name, nodes, edges), [name, nodes, edges])

  useEffect(() => {
    if (!inputPreviewUrl || selectedNode?.data.kind !== 'mask_input') return
    const image = new Image()
    image.onload = () => {
      const width = Math.max(64, image.naturalWidth || 1024)
      const height = Math.max(64, image.naturalHeight || 1024)
      setMaskCanvasSize({ width, height })
      const canvas = maskCanvasRef.current
      if (canvas) {
        canvas.width = width
        canvas.height = height
        canvas.getContext('2d')?.clearRect(0, 0, width, height)
      }
    }
    image.src = inputPreviewUrl
  }, [inputPreviewUrl, selectedNode?.data.kind])

  const loadSavedFlows = useCallback(async () => {
    try {
      const data = await fetchJSON<{ workflows: ImageFlowSummary[] }>('/api/image-flow/list')
      setSavedFlows(data.workflows || [])
    } catch {
      setSavedFlows([])
    }
  }, [])

  const loadGallery = useCallback(async () => {
    try {
      const params = new URLSearchParams()
      if (galleryQuery.trim()) params.set('q', galleryQuery.trim())
      if (galleryMode.trim()) params.set('mode', galleryMode.trim())
      if (showArchivedAssets) params.set('archived', '1')
      const suffix = params.toString() ? `?${params.toString()}` : ''
      const data = await fetchJSON<{ assets: ImageFlowAsset[] }>(`/api/image-flow/assets${suffix}`)
      setGallery(data.assets || [])
    } catch {
      setGallery([])
    }
  }, [galleryQuery, galleryMode, showArchivedAssets])

  const loadModelStatus = useCallback(async () => {
    try {
      const data = await fetchJSON<ImageFlowModelsResponse>('/api/image-flow/models')
      setModelStatus(data.current)
      setAvailableModels(data.available || [])
    } catch {
      setModelStatus(null)
      setAvailableModels([])
    }
  }, [])

  const loadTemplates = useCallback(async () => {
    try {
      const data = await fetchJSON<{ templates: ImageFlowTemplate[] }>('/api/image-flow/templates')
      setTemplates(data.templates || [])
    } catch {
      setTemplates([])
    }
  }, [])

  useEffect(() => {
    loadSavedFlows()
    loadGallery()
    loadModelStatus()
    loadTemplates()
    const saved = localStorage.getItem(STORAGE_KEY)
    if (!saved) {
      setIssues(validateGraph(defaultNodes(), defaultEdges()))
      return
    }
    try {
      const parsed = JSON.parse(saved) as ImageFlowWorkflow
      const restored = fromWorkflow(parsed)
      setName(restored.name)
      setNodes(restored.nodes)
      setEdges(restored.edges)
      setIssues(validateGraph(restored.nodes, restored.edges))
    } catch {
      setIssues(validateGraph(defaultNodes(), defaultEdges()))
    }
  }, [loadSavedFlows, loadGallery, loadModelStatus, loadTemplates])

  useEffect(() => {
    const workflow = toWorkflow(name, nodes, edges)
    localStorage.setItem(STORAGE_KEY, JSON.stringify(workflow))
    setIssues(validateGraph(nodes, edges))
  }, [name, nodes, edges])

  useEffect(() => {
    if (!activeJobId) return
    const events = new EventSource(`/api/image-flow/events?id=${encodeURIComponent(activeJobId)}`)
    events.addEventListener('job', event => {
      const job = JSON.parse((event as MessageEvent).data) as ImageFlowJob
      applyJob(job)
      if (['success', 'failed', 'canceled'].includes(job.status)) {
        setActiveJobId('')
        setRunning(false)
        loadGallery()
        events.close()
      }
    })
    events.addEventListener('error', () => {
      fetchJSON<ImageFlowJob>(`/api/image-flow/status?id=${encodeURIComponent(activeJobId)}`)
        .then(job => {
          applyJob(job)
          if (['success', 'failed', 'canceled'].includes(job.status)) {
            setActiveJobId('')
            setRunning(false)
            loadGallery()
          }
        })
        .catch(error => {
          setNotice(`Gagal membaca status job: ${error instanceof Error ? error.message : 'unknown error'}.`)
          setActiveJobId('')
          setRunning(false)
        })
      events.close()
    })
    return () => events.close()
  }, [activeJobId, loadGallery])

  const onNodesChange = useCallback((changes: NodeChange[]) => {
    setNodes(current => applyNodeChanges(changes, current) as Node<ImageNodeData>[])
  }, [])

  const onEdgesChange = useCallback((changes: EdgeChange[]) => {
    setEdges(current => applyEdgeChanges(changes, current))
  }, [])

  const onConnect = useCallback((connection: Connection) => {
    const source = nodes.find(node => node.id === connection.source)
    const target = nodes.find(node => node.id === connection.target)
    const sourcePort = source?.data.outputs.find(port => port.name === connection.sourceHandle)
    const targetPort = target?.data.inputs.find(port => port.name === connection.targetHandle)
    if (!source || !target || !sourcePort || !targetPort || sourcePort.type !== targetPort.type) {
      setNotice('Koneksi ditolak karena tipe port tidak cocok.')
      return
    }
    setEdges(current => addEdge({
      ...connection,
      id: `${connection.source}-${connection.sourceHandle}-${connection.target}-${connection.targetHandle}-${Date.now()}`,
      animated: sourcePort.type === 'text',
      style: { stroke: portColor(sourcePort.type), strokeWidth: 2 },
    }, current))
  }, [nodes])

  const addNode = (kind: ImageNodeKind) => {
    const count = nodes.filter(node => node.data.kind === kind).length + 1
    const id = `${kind}_${Date.now()}`
    setNodes(current => [...current, makeNode(kind, id, { x: 120 + count * 34, y: 120 + count * 42 })])
    setSelectedNodeId(id)
  }

  const deleteSelected = () => {
    if (!selectedNodeId) return
    setNodes(current => current.filter(node => node.id !== selectedNodeId))
    setEdges(current => current.filter(edge => edge.source !== selectedNodeId && edge.target !== selectedNodeId))
    setSelectedNodeId(null)
  }

  const updateConfig = (key: string, value: string) => {
    if (!selectedNodeId) return
    setNodes(current => current.map(node => {
      if (node.id !== selectedNodeId) return node
      const previous = node.data.config[key]
      const parsed = typeof previous === 'number' ? Number(value) : value
      return { ...node, data: { ...node.data, config: { ...node.data.config, [key]: Number.isNaN(parsed) ? 0 : parsed } } }
    }))
  }

  const uploadImageAsset = async (file: File | undefined) => {
    if (!file || !selectedNode || (selectedNode.data.kind !== 'image_input' && selectedNode.data.kind !== 'mask_input')) return
    const pathKey = selectedNode.data.kind === 'mask_input' ? 'mask_path' : 'image_path'
    setNotice(`Mengupload gambar ke ${selectedNode.data.label}.`)
    try {
      const uploaded = await uploadAttachment(file)
      if (uploaded.kind !== 'image') throw new Error('file yang diupload bukan gambar')
      setNodes(current => current.map(node => {
        if (node.id !== selectedNode.id) return node
        return {
          ...node,
          data: {
            ...node.data,
            config: { ...node.data.config, [pathKey]: uploaded.path, source: uploaded.name || 'uploaded file' },
          },
        }
      }))
      setNotice(`${selectedNode.data.label} memakai ${uploaded.path}.`)
    } catch (error) {
      setNotice(`Upload ${selectedNode.data.label} gagal: ${error instanceof Error ? error.message : 'unknown error'}.`)
    } finally {
      if (imageUploadRef.current) imageUploadRef.current.value = ''
    }
  }

  const drawMaskPoint = (event: PointerEvent<HTMLCanvasElement>) => {
    const canvas = maskCanvasRef.current
    if (!canvas || !maskDrawingRef.current) return
    const rect = canvas.getBoundingClientRect()
    const x = ((event.clientX - rect.left) / rect.width) * canvas.width
    const y = ((event.clientY - rect.top) / rect.height) * canvas.height
    const ctx = canvas.getContext('2d')
    if (!ctx) return
    ctx.globalCompositeOperation = maskMode === 'erase' ? 'destination-out' : 'source-over'
    ctx.fillStyle = `rgba(255,255,255,${Math.max(5, maskOpacity) / 100})`
    ctx.shadowColor = maskMode === 'erase' ? 'rgba(0,0,0,1)' : 'rgba(255,255,255,1)'
    ctx.shadowBlur = maskFeather
    ctx.beginPath()
    ctx.arc(x, y, maskBrushSize / 2, 0, Math.PI * 2)
    ctx.fill()
    ctx.shadowBlur = 0
    ctx.globalCompositeOperation = 'source-over'
  }

  const pushMaskHistory = () => {
    const canvas = maskCanvasRef.current
    if (!canvas) return
    setMaskHistory(current => [...current.slice(-19), canvas.toDataURL('image/png')])
    setMaskRedoHistory([])
  }

  const startMaskDraw = (event: PointerEvent<HTMLCanvasElement>) => {
    event.currentTarget.setPointerCapture(event.pointerId)
    pushMaskHistory()
    maskDrawingRef.current = true
    drawMaskPoint(event)
  }

  const stopMaskDraw = (event: PointerEvent<HTMLCanvasElement>) => {
    if (event.currentTarget.hasPointerCapture(event.pointerId)) event.currentTarget.releasePointerCapture(event.pointerId)
    maskDrawingRef.current = false
  }

  const clearMaskCanvas = () => {
    const canvas = maskCanvasRef.current
    if (!canvas) return
    pushMaskHistory()
    canvas.getContext('2d')?.clearRect(0, 0, canvas.width, canvas.height)
  }

  const restoreMaskCanvas = (dataUrl: string) => {
    const canvas = maskCanvasRef.current
    if (!canvas) return
    const image = new Image()
    image.onload = () => {
      const ctx = canvas.getContext('2d')
      if (!ctx) return
      ctx.clearRect(0, 0, canvas.width, canvas.height)
      ctx.drawImage(image, 0, 0)
    }
    image.src = dataUrl
  }

  const undoMaskCanvas = () => {
    const canvas = maskCanvasRef.current
    const last = maskHistory[maskHistory.length - 1]
    if (!canvas || !last) return
    setMaskRedoHistory(current => [...current.slice(-19), canvas.toDataURL('image/png')])
    restoreMaskCanvas(last)
    setMaskHistory(current => current.slice(0, -1))
  }

  const redoMaskCanvas = () => {
    const canvas = maskCanvasRef.current
    const next = maskRedoHistory[maskRedoHistory.length - 1]
    if (!canvas || !next) return
    setMaskHistory(current => [...current.slice(-19), canvas.toDataURL('image/png')])
    restoreMaskCanvas(next)
    setMaskRedoHistory(current => current.slice(0, -1))
  }

  const saveMaskCanvas = async () => {
    if (!selectedNode || selectedNode.data.kind !== 'mask_input') return
    const canvas = maskCanvasRef.current
    if (!canvas) return
    const output = document.createElement('canvas')
    output.width = canvas.width
    output.height = canvas.height
    const ctx = output.getContext('2d')
    if (!ctx) return
    ctx.fillStyle = '#000'
    ctx.fillRect(0, 0, output.width, output.height)
    ctx.drawImage(canvas, 0, 0)
    setNotice('Menyimpan mask dari canvas.')
    try {
      const uploaded = await uploadClipboardImage(output.toDataURL('image/png'))
      setNodes(current => current.map(node => node.id === selectedNode.id
        ? { ...node, data: { ...node.data, config: { ...node.data.config, mask_path: uploaded.path, source: 'mask editor' } } }
        : node))
      setNotice(`Mask tersimpan: ${uploaded.path}.`)
    } catch (error) {
      setNotice(`Simpan mask gagal: ${error instanceof Error ? error.message : 'unknown error'}.`)
    }
  }

  const saveDraft = async () => {
    setSaving(true)
    localStorage.setItem(STORAGE_KEY, workflowJSON)
    try {
      await fetchJSON('/api/image-flow/save', { method: 'POST', body: JSON.stringify(workflow) })
      setNotice('Workflow tersimpan ke backend dan draft lokal.')
      await loadSavedFlows()
    } catch (error) {
      setNotice(`Backend save gagal, draft lokal tetap tersimpan: ${error instanceof Error ? error.message : 'unknown error'}.`)
    } finally {
      setSaving(false)
    }
  }

  const loadWorkflow = async (flowName: string) => {
    try {
      const data = await fetchJSON<ImageFlowWorkflow>(`/api/image-flow/get?name=${encodeURIComponent(flowName)}`)
      const restored = fromWorkflow(data)
      setName(restored.name)
      setNodes(restored.nodes)
      setEdges(restored.edges)
      setSelectedNodeId(restored.nodes[0]?.id || null)
      setPreviewUrl('')
      setNotice(`Workflow '${flowName}' dibuka dari backend.`)
    } catch (error) {
      setNotice(`Gagal load workflow: ${error instanceof Error ? error.message : 'unknown error'}.`)
    }
  }

  const deleteWorkflow = async (flowName: string) => {
    try {
      await fetchJSON('/api/image-flow/delete', { method: 'POST', body: JSON.stringify({ name: flowName }) })
      setNotice(`Workflow '${flowName}' dihapus.`)
      await loadSavedFlows()
    } catch (error) {
      setNotice(`Gagal hapus workflow: ${error instanceof Error ? error.message : 'unknown error'}.`)
    }
  }

  const resetWorkflow = () => {
    const ns = defaultNodes()
    const es = defaultEdges()
    setName('Basic Text to Image')
    setNodes(ns)
    setEdges(es)
    setSelectedNodeId('generate_image_3')
    setIssues(validateGraph(ns, es))
    setPreviewUrl('')
    setNotice('Workflow di-reset ke template text-to-image MVP.')
  }

  const resetEditWorkflow = () => {
    const ns = editTemplateNodes()
    const es = editTemplateEdges()
    setName('Basic Image Edit')
    setNodes(ns)
    setEdges(es)
    setSelectedNodeId('image_input_1')
    setIssues(validateGraph(ns, es))
    setPreviewUrl('')
    setNotice('Workflow di-reset ke template image-to-image. Isi image_path di Image Input.')
  }

  const resetInpaintWorkflow = () => {
    const ns = inpaintTemplateNodes()
    const es = inpaintTemplateEdges()
    setName('Basic Inpaint')
    setNodes(ns)
    setEdges(es)
    setSelectedNodeId('image_input_1')
    setIssues(validateGraph(ns, es))
    setPreviewUrl('')
    setNotice('Workflow di-reset ke template inpaint. Upload image dan mask sebelum Run.')
  }

  const applyWorkflowToCanvas = (wf: ImageFlowWorkflow, message: string) => {
    const restored = fromWorkflow(wf)
    setName(restored.name)
    setNodes(restored.nodes)
    setEdges(restored.edges)
    setSelectedNodeId(restored.nodes[0]?.id || null)
    setPreviewUrl('')
    setAgentIssues([])
    setAgentExplanation(null)
    setAgentSuggestions([])
    setAgentActions([])
    setNotice(message)
  }

  const loadTemplateToCanvas = (template: ImageFlowTemplate) => {
    applyWorkflowToCanvas(template.workflow, `Template ${template.name} dimuat ke canvas.`)
  }

  const runTemplate = async (template: ImageFlowTemplate) => {
    setRunning(true)
    setPreviewUrl('')
    setRunLogs([])
    setNotice(`Menjalankan template ${template.name}.`)
    try {
      const prompt = agentPrompt.trim() || agentInstruction.trim()
      const job = await fetchJSON<ImageFlowJob>('/api/image-flow/template/run', {
        method: 'POST',
        body: JSON.stringify({ template_id: template.id, params: { prompt } }),
      })
      setActiveJobId(job.id)
      applyJob(job)
    } catch (error) {
      setNotice(`Run template gagal: ${error instanceof Error ? error.message : 'unknown error'}.`)
      setRunning(false)
    }
  }

  const agentCreateWorkflow = async () => {
    setAgentBusy(true)
    setNotice('Agent membuat workflow dari instruksi.')
    try {
      const data = await fetchJSON<{ workflow: ImageFlowWorkflow; issues: AgentIssue[] }>('/api/image-flow/agent/create', {
        method: 'POST',
        body: JSON.stringify({ name: name || 'Agent Generated Image Flow', instruction: agentInstruction, prompt: agentPrompt, save: false }),
      })
      applyWorkflowToCanvas(data.workflow, 'Agent berhasil membuat workflow baru.')
      setAgentIssues(data.issues || [])
    } catch (error) {
      setNotice(`Agent create gagal: ${error instanceof Error ? error.message : 'unknown error'}.`)
    } finally {
      setAgentBusy(false)
    }
  }

  const agentLintWorkflow = async () => {
    setAgentBusy(true)
    try {
      const data = await fetchJSON<{ issues: AgentIssue[] }>('/api/image-flow/agent/lint', { method: 'POST', body: JSON.stringify(workflow) })
      setAgentIssues(data.issues || [])
      setNotice(`Agent lint selesai: ${(data.issues || []).length} issue.`)
    } catch (error) {
      setNotice(`Agent lint gagal: ${error instanceof Error ? error.message : 'unknown error'}.`)
    } finally {
      setAgentBusy(false)
    }
  }

  const agentExplainWorkflow = async () => {
    setAgentBusy(true)
    try {
      const data = await fetchJSON<AgentExplanation>('/api/image-flow/agent/explain', { method: 'POST', body: JSON.stringify(workflow) })
      setAgentExplanation(data)
      setNotice('Agent explain selesai.')
    } catch (error) {
      setNotice(`Agent explain gagal: ${error instanceof Error ? error.message : 'unknown error'}.`)
    } finally {
      setAgentBusy(false)
    }
  }

  const agentFixWorkflow = async () => {
    setAgentBusy(true)
    try {
      const data = await fetchJSON<{ workflow: ImageFlowWorkflow; actions: string[]; issues: AgentIssue[] }>('/api/image-flow/agent/fix', { method: 'POST', body: JSON.stringify(workflow) })
      applyWorkflowToCanvas(data.workflow, 'Agent fix diterapkan ke canvas.')
      setAgentActions(data.actions || [])
      setAgentIssues(data.issues || [])
    } catch (error) {
      setNotice(`Agent fix gagal: ${error instanceof Error ? error.message : 'unknown error'}.`)
    } finally {
      setAgentBusy(false)
    }
  }

  const agentOptimizeWorkflow = async () => {
    setAgentBusy(true)
    try {
      const data = await fetchJSON<{ suggestions: AgentSuggestion[] }>('/api/image-flow/agent/optimize', { method: 'POST', body: JSON.stringify(workflow) })
      setAgentSuggestions(data.suggestions || [])
      setNotice(`Agent optimize selesai: ${(data.suggestions || []).length} saran.`)
    } catch (error) {
      setNotice(`Agent optimize gagal: ${error instanceof Error ? error.message : 'unknown error'}.`)
    } finally {
      setAgentBusy(false)
    }
  }

  const exportWorkflow = () => {
    const blob = new Blob([workflowJSON], { type: 'application/json' })
    const url = URL.createObjectURL(blob)
    const link = document.createElement('a')
    link.href = url
    link.download = `${name.trim().toLowerCase().replace(/[^a-z0-9]+/g, '-') || 'image-flow'}.json`
    link.click()
    URL.revokeObjectURL(url)
  }

  const importWorkflow = async (file: File | undefined) => {
    if (!file) return
    try {
      const parsed = JSON.parse(await file.text()) as ImageFlowWorkflow
      if (parsed.version !== '1.0' || !Array.isArray(parsed.nodes) || !Array.isArray(parsed.edges)) {
        throw new Error('schema tidak valid')
      }
      const restored = fromWorkflow(parsed)
      setName(restored.name)
      setNodes(restored.nodes)
      setEdges(restored.edges)
      setSelectedNodeId(restored.nodes[0]?.id || null)
      setPreviewUrl('')
      setNotice('Workflow JSON berhasil di-import.')
    } catch (error) {
      setNotice(`Import gagal: ${error instanceof Error ? error.message : 'file tidak valid'}.`)
    }
    if (importRef.current) importRef.current.value = ''
  }

  const useAssetAsInput = (asset: ImageFlowAsset) => {
    setNodes(current => current.map(node => node.data.kind === 'image_input'
      ? { ...node, data: { ...node.data, config: { ...node.data.config, image_path: asset.path }, lastResult: asset.path } }
      : node))
    setPreviewUrl(asset.image_url || `/api/generated-image?path=${encodeURIComponent(asset.path)}`)
    setNotice(`Asset dari ${asset.workflow || asset.job_id} dipakai sebagai Image Input.`)
  }

  const useAssetAsMask = (asset: ImageFlowAsset) => {
    setNodes(current => current.map(node => node.data.kind === 'mask_input'
      ? { ...node, data: { ...node.data, config: { ...node.data.config, mask_path: asset.path }, lastResult: asset.path } }
      : node))
    setNotice(`Asset ${asset.id} dipakai sebagai Mask Input.`)
  }

  const archiveAsset = async (asset: ImageFlowAsset, archived: boolean) => {
    try {
      await fetchJSON('/api/image-flow/assets/archive', { method: 'POST', body: JSON.stringify({ id: asset.id, archived }) })
      setNotice(archived ? 'Asset di-archive.' : 'Asset dikembalikan dari archive.')
      loadGallery()
    } catch (error) {
      setNotice(`Archive gagal: ${error instanceof Error ? error.message : 'unknown error'}.`)
    }
  }

  const deleteAsset = async (asset: ImageFlowAsset) => {
    if (!confirm(`Hapus asset ${asset.id} dari gallery? File gambar tidak dihapus.`)) return
    try {
      await fetchJSON('/api/image-flow/assets/delete', { method: 'POST', body: JSON.stringify({ id: asset.id, delete_file: false }) })
      setNotice('Asset dihapus dari gallery.')
      loadGallery()
    } catch (error) {
      setNotice(`Delete asset gagal: ${error instanceof Error ? error.message : 'unknown error'}.`)
    }
  }

  const applyJob = (job: ImageFlowJob) => {
    setRunLogs(job.logs || [])
    if (job.result?.image_url) setPreviewUrl(job.result.image_url)
    setNodes(current => current.map(node => {
      const state = job.nodes.find(item => item.node_id === node.id)
      if (!state) return node
      const status = state.status === 'skipped' ? 'idle' : state.status
      return { ...node, data: { ...node.data, status, lastResult: state.result || node.data.lastResult } }
    }))
    if (job.status === 'queued') setNotice(`Job ${job.id} masuk queue.`)
    else if (job.status === 'running') setNotice(`Job ${job.id} sedang berjalan.`)
    else if (job.status === 'success') setNotice(job.result?.image_url ? `Run selesai: ${job.result.path || job.result.image_url}` : 'Run selesai.')
    else if (job.status === 'failed') setNotice(`Run gagal: ${job.error || 'unknown error'}.`)
    else if (job.status === 'canceled') setNotice('Run dibatalkan.')
  }

  const runWorkflow = async () => {
    if (issues.length) {
      setNotice(`Workflow belum valid: ${issues[0]}`)
      return
    }
    const workflow = toWorkflow(name, nodes, edges)
    setRunning(true)
    setPreviewUrl('')
    setRunLogs([])
    setNotice('Mengirim Image Flow ke queue.')
    setNodes(current => current.map(node => ({ ...node, data: { ...node.data, status: 'queued', lastResult: undefined } })))
    try {
      const job = await fetchJSON<ImageFlowJob>('/api/image-flow/run', { method: 'POST', body: JSON.stringify(workflow) })
      setActiveJobId(job.id)
      applyJob(job)
    } catch (error) {
      setNodes(current => current.map(node => node.data.kind === 'generate_image' || node.data.kind === 'image_to_image' || node.data.kind === 'inpaint' || node.data.kind === 'outpaint' || node.data.kind === 'upscale' ? { ...node, data: { ...node.data, status: 'failed', lastResult: 'failed' } } : node))
      setNotice(`Run gagal: ${error instanceof Error ? error.message : 'unknown error'}.`)
      setRunning(false)
    }
  }

  const cancelRun = async () => {
    if (!activeJobId) return
    try {
      const job = await fetchJSON<ImageFlowJob>('/api/image-flow/cancel', { method: 'POST', body: JSON.stringify({ id: activeJobId }) })
      applyJob(job)
    } catch (error) {
      setNotice(`Cancel gagal: ${error instanceof Error ? error.message : 'unknown error'}.`)
    }
  }

  const retryRun = async () => {
    if (!activeJobId) return
    try {
      const job = await fetchJSON<ImageFlowJob>('/api/image-flow/retry', { method: 'POST', body: JSON.stringify({ id: activeJobId }) })
      setActiveJobId(job.id)
      setRunning(true)
      applyJob(job)
    } catch (error) {
      setNotice(`Retry gagal: ${error instanceof Error ? error.message : 'unknown error'}.`)
    }
  }

  return (
    <div className="flex h-full flex-col overflow-hidden p-4">
      <div className="mb-3 flex items-center gap-3">
        <div className="grid h-9 w-9 place-items-center rounded-xl bg-smara-300/10 text-smara-200">
          <ImageIcon className="h-5 w-5" />
        </div>
        <div className="min-w-0">
          <h2 className="text-lg font-semibold text-neutral-100">Image Flow</h2>
          <p className="text-xs text-neutral-500">Node-based workflow MVP untuk text-to-image, schema, validasi port, dan run dummy.</p>
        </div>
        <div className="ml-auto flex items-center gap-2">
          <button onClick={saveDraft} disabled={saving} className="flex items-center gap-1.5 rounded-lg bg-neutral-900/70 px-3 py-2 text-xs text-neutral-200 transition-colors hover:bg-neutral-800 disabled:cursor-not-allowed disabled:opacity-60">
            <Save className="h-3.5 w-3.5" /> {saving ? 'Saving' : 'Simpan'}
          </button>
          <button onClick={exportWorkflow} className="flex items-center gap-1.5 rounded-lg bg-neutral-900/70 px-3 py-2 text-xs text-neutral-200 transition-colors hover:bg-neutral-800">
            <Download className="h-3.5 w-3.5" /> Export
          </button>
          <input ref={importRef} type="file" accept=".json,application/json" className="hidden" onChange={event => importWorkflow(event.target.files?.[0])} />
          <button onClick={() => importRef.current?.click()} className="flex items-center gap-1.5 rounded-lg bg-neutral-900/70 px-3 py-2 text-xs text-neutral-200 transition-colors hover:bg-neutral-800">
            <Upload className="h-3.5 w-3.5" /> Import
          </button>
          {!running && activeJobId ? (
            <button onClick={retryRun} className="flex items-center gap-1.5 rounded-lg bg-amber-700 px-3 py-2 text-xs font-medium text-amber-50 transition-colors hover:bg-amber-600">
              <Play className="h-3.5 w-3.5" /> Retry
            </button>
          ) : null}
          {running && activeJobId ? (
            <button onClick={cancelRun} className="flex items-center gap-1.5 rounded-lg bg-red-800 px-3 py-2 text-xs font-medium text-red-100 transition-colors hover:bg-red-700">
              <Square className="h-3.5 w-3.5" /> Cancel
            </button>
          ) : null}
          <button onClick={runWorkflow} disabled={running} className="flex items-center gap-1.5 rounded-lg bg-smara-500 px-3 py-2 text-xs font-medium text-black transition-colors hover:bg-smara-400 disabled:cursor-not-allowed disabled:opacity-60">
            <Play className="h-3.5 w-3.5" /> {running ? 'Running' : 'Run'}
          </button>
        </div>
      </div>

      <div className="grid min-h-0 flex-1 grid-cols-[250px_minmax(0,1fr)_340px] gap-3">
        <aside className="min-h-0 overflow-y-auto rounded-xl bg-[#0f1a0f]/60 p-3 ring-1 ring-black/35">
          <div className="mb-3">
            <label className="mb-1 block text-[10px] uppercase tracking-[0.2em] text-neutral-600">Workflow</label>
            <input value={name} onChange={event => setName(event.target.value)} className="w-full rounded-lg bg-neutral-950/60 px-3 py-2 text-sm text-neutral-100 ring-1 ring-black/35 focus:outline-none focus:ring-2 focus:ring-smara-300/25" />
          </div>
          <div className="mb-3 rounded-xl border border-smara-300/10 bg-neutral-950/25 p-2">
            <div className="mb-2 flex items-center justify-between">
              <span className="text-[10px] font-semibold uppercase tracking-[0.2em] text-neutral-600">Templates & Agent</span>
              <button onClick={loadTemplates} className="text-[10px] text-smara-200 hover:text-smara-100">Reload</button>
            </div>
            <div className="mb-2 grid grid-cols-2 gap-1.5">
              {templates.slice(0, 6).map(template => (
                <div key={template.id} className="rounded-lg bg-neutral-900/45 p-1.5">
                  <button onClick={() => loadTemplateToCanvas(template)} className="block w-full truncate text-left text-[10px] font-medium text-neutral-200 hover:text-smara-100">{template.name}</button>
                  <button onClick={() => runTemplate(template)} disabled={running} className="mt-1 w-full rounded bg-smara-500/15 px-1.5 py-1 text-[9px] text-smara-100 ring-1 ring-smara-300/15 disabled:opacity-50">Run</button>
                </div>
              ))}
              {templates.length === 0 ? <div className="col-span-2 rounded-lg bg-neutral-950/35 p-2 text-[10px] text-neutral-500">Template belum termuat.</div> : null}
            </div>
            <textarea value={agentInstruction} onChange={event => setAgentInstruction(event.target.value)} rows={3} placeholder="Instruksi agent: buat workflow inpaint untuk ganti background..." className="mb-2 w-full resize-y rounded-lg bg-neutral-950/60 px-2 py-1.5 text-[11px] leading-4 text-neutral-100 ring-1 ring-black/35 focus:outline-none" />
            <input value={agentPrompt} onChange={event => setAgentPrompt(event.target.value)} placeholder="Prompt optional / parameter template..." className="mb-2 w-full rounded-lg bg-neutral-950/60 px-2 py-1.5 text-[11px] text-neutral-100 ring-1 ring-black/35 focus:outline-none" />
            <div className="grid grid-cols-2 gap-1.5">
              <button onClick={agentCreateWorkflow} disabled={agentBusy} className="rounded bg-smara-500/15 px-2 py-1.5 text-[10px] text-smara-100 ring-1 ring-smara-300/15 disabled:opacity-50">Create</button>
              <button onClick={agentLintWorkflow} disabled={agentBusy} className="rounded bg-neutral-900 px-2 py-1.5 text-[10px] text-neutral-200 disabled:opacity-50">Lint</button>
              <button onClick={agentFixWorkflow} disabled={agentBusy} className="rounded bg-neutral-900 px-2 py-1.5 text-[10px] text-neutral-200 disabled:opacity-50">Fix</button>
              <button onClick={agentOptimizeWorkflow} disabled={agentBusy} className="rounded bg-neutral-900 px-2 py-1.5 text-[10px] text-neutral-200 disabled:opacity-50">Optimize</button>
              <button onClick={agentExplainWorkflow} disabled={agentBusy} className="col-span-2 rounded bg-neutral-900 px-2 py-1.5 text-[10px] text-neutral-200 disabled:opacity-50">Explain Workflow</button>
            </div>
            {(agentIssues.length > 0 || agentActions.length > 0 || agentSuggestions.length > 0 || agentExplanation) ? (
              <details className="mt-2 rounded-lg bg-neutral-950/45 p-2" open>
                <summary className="cursor-pointer text-[10px] text-neutral-400">Agent Output</summary>
                <div className="mt-2 space-y-1 text-[10px] leading-4 text-neutral-500">
                  {agentExplanation ? <div className="text-neutral-300">{agentExplanation.summary}</div> : null}
                  {agentExplanation?.steps?.map(step => <div key={step}>Step: {step}</div>)}
                  {agentActions.map(action => <div key={action} className="text-sky-200">Action: {action}</div>)}
                  {agentIssues.map((issue, idx) => <div key={`${issue.message}-${idx}`} className={issue.level === 'error' ? 'text-red-200' : issue.level === 'warning' ? 'text-amber-100' : 'text-neutral-500'}>{issue.level}: {issue.message}</div>)}
                  {agentSuggestions.map((item, idx) => <div key={`${item.message}-${idx}`} className={item.level === 'warning' ? 'text-amber-100' : 'text-neutral-400'}>{item.level}: {item.message}</div>)}
                </div>
              </details>
            ) : null}
          </div>
          <div className="mb-2 flex items-center justify-between">
            <span className="text-[10px] font-semibold uppercase tracking-[0.2em] text-neutral-600">Node Library</span>
            <div className="flex gap-2">
              <button onClick={resetWorkflow} className="text-[10px] text-neutral-500 hover:text-neutral-200">T2I</button>
              <button onClick={resetEditWorkflow} className="text-[10px] text-neutral-500 hover:text-neutral-200">Edit</button>
              <button onClick={resetInpaintWorkflow} className="text-[10px] text-neutral-500 hover:text-neutral-200">Inpaint</button>
            </div>
          </div>
          <div className="space-y-2">
            {registry.map(item => (
              <button key={item.type} onClick={() => addNode(item.type)} className="w-full rounded-lg border border-neutral-900 bg-neutral-950/35 p-2 text-left transition-colors hover:border-smara-300/30 hover:bg-neutral-900/55">
                <div className="flex items-center gap-2">
                  <Plus className="h-3.5 w-3.5 text-smara-300" />
                  <span className="text-xs font-medium text-neutral-200">{item.label}</span>
                  <span className="ml-auto text-[9px] text-neutral-600">{item.category}</span>
                </div>
                <p className="mt-1 line-clamp-2 text-[10px] leading-4 text-neutral-500">{item.description}</p>
              </button>
            ))}
          </div>
          <div className="mt-3 rounded-lg bg-neutral-950/35 p-2 text-[10px] leading-4 text-neutral-500">
            Drag dari handle kanan ke handle kiri. Koneksi hanya diterima kalau tipe port sama.
          </div>
          <div className="mt-3">
            <div className="mb-2 text-[10px] font-semibold uppercase tracking-[0.2em] text-neutral-600">Saved</div>
            <div className="space-y-1.5">
              {savedFlows.length === 0 ? (
                <div className="rounded-lg bg-neutral-950/35 p-2 text-[10px] text-neutral-500">Belum ada workflow backend.</div>
              ) : savedFlows.map(flow => (
                <div key={flow.name} className="flex items-center gap-1 rounded-lg bg-neutral-950/35 p-2">
                  <button onClick={() => loadWorkflow(flow.name)} className="min-w-0 flex-1 text-left">
                    <div className="truncate text-xs text-neutral-200">{flow.name}</div>
                    <div className="text-[9px] text-neutral-600">{flow.nodes} node · {flow.edges} edge</div>
                  </button>
                  <button onClick={() => deleteWorkflow(flow.name)} className="rounded-md p-1 text-neutral-600 hover:bg-red-950/40 hover:text-red-300">
                    <Trash2 className="h-3.5 w-3.5" />
                  </button>
                </div>
              ))}
            </div>
          </div>
        </aside>

        <main className="min-w-0 overflow-hidden rounded-xl bg-[#080c06]/80 ring-1 ring-black/35">
          <ReactFlow
            nodes={nodes}
            edges={edges}
            nodeTypes={nodeTypes}
            onNodesChange={onNodesChange}
            onEdgesChange={onEdgesChange}
            onConnect={onConnect}
            onNodeClick={(_, node) => setSelectedNodeId(node.id)}
            onPaneClick={() => setSelectedNodeId(null)}
            fitView
          >
            <Background gap={22} size={1} color="#334155" />
            <Controls />
            <MiniMap nodeColor={node => portColor((node.data as ImageNodeData).outputs[0]?.type || 'text')} maskColor="rgba(0,0,0,0.48)" />
          </ReactFlow>
        </main>

        <aside className="flex min-h-0 flex-col gap-3 overflow-y-auto rounded-xl bg-[#0f1a0f]/60 p-3 ring-1 ring-black/35">
          <div>
            <div className="mb-2 flex items-center justify-between">
              <span className="text-xs font-medium text-neutral-300">Inspector</span>
              {selectedNode && <button onClick={deleteSelected} className="rounded-md p-1 text-neutral-500 hover:bg-red-950/40 hover:text-red-300"><Trash2 className="h-4 w-4" /></button>}
            </div>
            {selectedNode ? (
              <div className="space-y-3">
                <div>
                  <div className="text-sm font-semibold text-neutral-100">{selectedNode.data.label}</div>
                  <div className="mt-1 text-[11px] leading-4 text-neutral-500">{selectedNode.id}</div>
                </div>
                <div className="space-y-2">
                  {Object.entries(selectedNode.data.config).map(([key, value]) => (
                    <label key={key} className="block">
                      <span className="mb-1 block text-[10px] uppercase tracking-[0.16em] text-neutral-600">{key}</span>
                      {key === 'prompts' || key === 'prompt' || key === 'positive' || key === 'negative' ? (
                        <textarea
                          value={value}
                          rows={key === 'prompts' ? 5 : 3}
                          onChange={event => updateConfig(key, event.target.value)}
                          className="w-full resize-y rounded-lg bg-neutral-950/60 px-3 py-2 text-xs leading-5 text-neutral-100 ring-1 ring-black/35 focus:outline-none focus:ring-2 focus:ring-smara-300/25"
                        />
                      ) : (
                        <input
                          type={typeof value === 'number' ? 'number' : 'text'}
                          value={value}
                          onChange={event => updateConfig(key, event.target.value)}
                          className="w-full rounded-lg bg-neutral-950/60 px-3 py-2 text-xs text-neutral-100 ring-1 ring-black/35 focus:outline-none focus:ring-2 focus:ring-smara-300/25"
                        />
                      )}
                    </label>
                  ))}
                </div>
                {selectedNode.data.kind === 'image_input' || selectedNode.data.kind === 'mask_input' ? (
                  <div className="space-y-2">
                    <input ref={imageUploadRef} type="file" accept="image/png,image/jpeg,image/webp" className="hidden" onChange={event => uploadImageAsset(event.target.files?.[0])} />
                    <button onClick={() => imageUploadRef.current?.click()} className="flex w-full items-center justify-center gap-1.5 rounded-lg bg-sky-500/15 px-3 py-2 text-xs font-medium text-sky-100 ring-1 ring-sky-300/20 transition-colors hover:bg-sky-500/25">
                      <Upload className="h-3.5 w-3.5" /> {selectedNode.data.kind === 'mask_input' ? 'Upload Mask' : 'Upload Image'}
                    </button>
                    {selectedAssetPreviewUrl ? (
                      <div className="aspect-video rounded-lg border border-neutral-800 bg-neutral-950/55 p-2">
                        <img src={selectedAssetPreviewUrl} alt="Image Flow input" className="h-full w-full rounded-md object-contain" />
                      </div>
                    ) : null}
                    {selectedNode.data.kind === 'mask_input' ? (
                      <div className="space-y-2 rounded-lg border border-neutral-800 bg-neutral-950/45 p-2">
                        {inputPreviewUrl ? (
                          <>
                            <div
                              className="relative w-full overflow-hidden rounded-md bg-neutral-950"
                              style={{ aspectRatio: `${maskCanvasSize.width} / ${maskCanvasSize.height}` }}
                            >
                              <img src={inputPreviewUrl} alt="Mask source" className="absolute inset-0 h-full w-full object-fill opacity-70" draggable={false} />
                              <canvas
                                ref={maskCanvasRef}
                                width={maskCanvasSize.width}
                                height={maskCanvasSize.height}
                                className="absolute inset-0 h-full w-full touch-none"
                                style={{ transform: `scale(${maskZoom / 100})`, transformOrigin: 'center' }}
                                onPointerDown={startMaskDraw}
                                onPointerMove={drawMaskPoint}
                                onPointerUp={stopMaskDraw}
                                onPointerCancel={stopMaskDraw}
                                onPointerLeave={() => { maskDrawingRef.current = false }}
                              />
                            </div>
                            <label className="block">
                              <span className="mb-1 block text-[10px] uppercase tracking-[0.16em] text-neutral-600">brush · {maskBrushSize}px</span>
                              <input
                                type="range"
                                min={8}
                                max={180}
                                value={maskBrushSize}
                                onChange={event => setMaskBrushSize(Number(event.target.value))}
                                className="w-full accent-sky-300"
                              />
                            </label>
                            <div className="grid grid-cols-3 gap-2">
                              <label className="block">
                                <span className="mb-1 block text-[10px] uppercase tracking-[0.16em] text-neutral-600">opacity · {maskOpacity}%</span>
                                <input type="range" min={5} max={100} value={maskOpacity} onChange={event => setMaskOpacity(Number(event.target.value))} className="w-full accent-sky-300" />
                              </label>
                              <label className="block">
                                <span className="mb-1 block text-[10px] uppercase tracking-[0.16em] text-neutral-600">feather · {maskFeather}px</span>
                                <input type="range" min={0} max={60} value={maskFeather} onChange={event => setMaskFeather(Number(event.target.value))} className="w-full accent-sky-300" />
                              </label>
                              <label className="block">
                                <span className="mb-1 block text-[10px] uppercase tracking-[0.16em] text-neutral-600">zoom · {maskZoom}%</span>
                                <input type="range" min={50} max={250} value={maskZoom} onChange={event => setMaskZoom(Number(event.target.value))} className="w-full accent-sky-300" />
                              </label>
                            </div>
                            <div className="grid grid-cols-2 gap-2">
                              <button onClick={() => setMaskMode(maskMode === 'paint' ? 'erase' : 'paint')} className="rounded-lg bg-neutral-900 px-3 py-2 text-xs text-neutral-200 transition-colors hover:bg-neutral-800">{maskMode === 'paint' ? 'Paint' : 'Erase'}</button>
                              <button onClick={undoMaskCanvas} disabled={maskHistory.length === 0} className="rounded-lg bg-neutral-900 px-3 py-2 text-xs text-neutral-200 transition-colors hover:bg-neutral-800 disabled:opacity-50">Undo</button>
                              <button onClick={redoMaskCanvas} disabled={maskRedoHistory.length === 0} className="rounded-lg bg-neutral-900 px-3 py-2 text-xs text-neutral-200 transition-colors hover:bg-neutral-800 disabled:opacity-50">Redo</button>
                              <button onClick={clearMaskCanvas} className="rounded-lg bg-neutral-900 px-3 py-2 text-xs text-neutral-200 transition-colors hover:bg-neutral-800">Clear</button>
                              <button onClick={saveMaskCanvas} className="rounded-lg bg-sky-500/15 px-3 py-2 text-xs font-medium text-sky-100 ring-1 ring-sky-300/20 transition-colors hover:bg-sky-500/25">Save Mask</button>
                            </div>
                          </>
                        ) : (
                          <div className="rounded-md border border-amber-500/20 bg-amber-950/20 p-2 text-[11px] leading-4 text-amber-100">Upload Image Input dulu untuk menggambar mask di atas gambar sumber.</div>
                        )}
                      </div>
                    ) : null}
                  </div>
                ) : null}
              </div>
            ) : (
              <div className="rounded-lg bg-neutral-950/35 p-3 text-xs text-neutral-500">Pilih node di canvas untuk mengedit config.</div>
            )}
          </div>

          <div>
            <div className="mb-2 flex items-center gap-2 text-xs font-medium text-neutral-300">
              {issues.length === 0 ? <CheckCircle className="h-4 w-4 text-emerald-300" /> : <AlertCircle className="h-4 w-4 text-amber-300" />}
              Validation
            </div>
            <div className="space-y-1.5">
              {issues.length === 0 ? (
                <div className="rounded-lg bg-emerald-950/25 p-2 text-[11px] text-emerald-200">Graph valid untuk MVP text-to-image.</div>
              ) : issues.map(issue => (
                <div key={issue} className="flex gap-2 rounded-lg bg-amber-950/20 p-2 text-[11px] leading-4 text-amber-100">
                  <XCircle className="mt-0.5 h-3.5 w-3.5 shrink-0" />
                  <span>{issue}</span>
                </div>
              ))}
            </div>
          </div>

          <div>
            <div className="mb-2 text-xs font-medium text-neutral-300">Preview</div>
            <div className="rounded-xl border border-neutral-800 bg-[radial-gradient(circle_at_30%_20%,rgba(190,242,100,.18),transparent_34%),linear-gradient(135deg,#11170d,#071008_60%,#101820)] p-3">
              {previewUrl || inputPreviewUrl ? (
                <div className={previewUrl && inputPreviewUrl ? 'grid gap-2 sm:grid-cols-2' : 'grid gap-2'}>
                  {inputPreviewUrl ? (
                    <div className="min-w-0">
                      <div className="mb-1 text-[10px] uppercase tracking-[0.18em] text-neutral-500">Input</div>
                      <div className="aspect-square rounded-lg border border-neutral-800/70 bg-neutral-950/35 p-2">
                        <img src={inputPreviewUrl} alt="Image Flow input" className="h-full w-full rounded-md object-contain" />
                      </div>
                    </div>
                  ) : null}
                  {previewUrl ? (
                    <div className="min-w-0">
                      <div className="mb-1 text-[10px] uppercase tracking-[0.18em] text-neutral-500">Output</div>
                      <div className="aspect-square rounded-lg border border-neutral-800/70 bg-neutral-950/35 p-2">
                        <img src={previewUrl} alt="Image Flow output" className="h-full w-full rounded-md object-contain" />
                      </div>
                    </div>
                  ) : null}
                </div>
              ) : (
                <div className="flex aspect-square flex-col justify-end rounded-lg border border-neutral-800/70 p-3">
                  <div className="text-[10px] uppercase tracking-[0.18em] text-neutral-500">Output</div>
                  <div className="mt-1 text-sm font-medium text-neutral-100">Belum ada gambar</div>
                  <div className="mt-1 text-[11px] text-neutral-500">Klik Run untuk memanggil provider image dari node Generate Image atau Image Edit.</div>
                </div>
              )}
            </div>
          </div>

          <div>
            <div className="mb-2 flex items-center justify-between text-xs font-medium text-neutral-300">
              <span>Model / Provider</span>
              <button onClick={loadModelStatus} className="text-[10px] text-smara-200 hover:text-smara-100">Check</button>
            </div>
            <div className="space-y-2 rounded-xl border border-neutral-800 bg-neutral-950/25 p-2 text-[11px] leading-4 text-neutral-400">
              {modelStatus ? (
                <>
                  <div className="flex items-center justify-between gap-2">
                    <span className="truncate text-neutral-200">{modelStatus.provider} · {modelStatus.model}</span>
                    <span className={`rounded-full px-2 py-0.5 text-[9px] ${modelStatus.status === 'ready' ? 'bg-emerald-500/15 text-emerald-200' : 'bg-amber-500/15 text-amber-100'}`}>{modelStatus.status}</span>
                  </div>
                  <div className="text-neutral-500">{modelStatus.message}</div>
                  <div className="grid grid-cols-2 gap-1 text-[10px] text-neutral-500">
                    <div>Quality: {modelStatus.quality || 'default'}</div>
                    <div>API key: {modelStatus.api_key_configured ? 'configured' : 'missing'}</div>
                    <div className="col-span-2 truncate">Output: {modelStatus.output_dir || '-'}</div>
                  </div>
                  {availableModels.length > 0 ? (
                    <details>
                      <summary className="cursor-pointer text-neutral-300">Available models ({availableModels.length})</summary>
                      <div className="mt-2 space-y-1">
                        {availableModels.map(item => (
                          <button
                            key={`${item.provider}-${item.model}`}
                            onClick={() => {
                              setNodes(current => current.map(node => node.data.kind === 'model_loader'
                                ? { ...node, data: { ...node.data, config: { ...node.data.config, provider: item.provider, model: item.model, quality: item.quality || node.data.config.quality } } }
                                : node))
                              setNotice(`Model Loader memakai ${item.provider}/${item.model}.`)
                            }}
                            className="w-full rounded-md bg-neutral-900/70 px-2 py-1 text-left text-[10px] text-neutral-300 hover:bg-neutral-800"
                          >
                            {item.provider} · {item.model} · {item.status}
                          </button>
                        ))}
                      </div>
                    </details>
                  ) : null}
                </>
              ) : (
                <div>Model/provider belum terbaca.</div>
              )}
            </div>
          </div>

          <div>
            <div className="mb-2 flex items-center justify-between text-xs font-medium text-neutral-300">
              <span>Image Flow Gallery</span>
              <button onClick={loadGallery} className="text-[10px] text-smara-200 hover:text-smara-100">Refresh</button>
            </div>
            <div className="mb-2 grid grid-cols-[1fr_96px] gap-2">
              <input value={galleryQuery} onChange={event => setGalleryQuery(event.target.value)} placeholder="Search asset..." className="rounded-lg bg-neutral-950/60 px-2 py-1.5 text-[11px] text-neutral-100 ring-1 ring-black/35 focus:outline-none" />
              <select value={galleryMode} onChange={event => setGalleryMode(event.target.value)} className="rounded-lg bg-neutral-950/60 px-2 py-1.5 text-[11px] text-neutral-100 ring-1 ring-black/35 focus:outline-none">
                <option value="">All</option>
                <option value="text-to-image">T2I</option>
                <option value="image-to-image">I2I</option>
                <option value="inpaint">Inpaint</option>
                <option value="outpaint">Outpaint</option>
                <option value="upscale">Upscale</option>
              </select>
            </div>
            <label className="mb-2 flex items-center gap-2 text-[10px] text-neutral-500">
              <input type="checkbox" checked={showArchivedAssets} onChange={event => setShowArchivedAssets(event.target.checked)} className="accent-smara-300" /> Show archived
            </label>
            <div className="max-h-72 space-y-2 overflow-auto rounded-xl border border-neutral-800 bg-neutral-950/25 p-2">
              {gallery.length === 0 ? (
                <div className="p-2 text-[11px] text-neutral-500">Belum ada history output Image Flow.</div>
              ) : gallery.slice(0, 24).map(asset => (
                <details key={asset.id} className="rounded-lg border border-neutral-800/70 bg-neutral-900/35 p-2" open={false}>
                  <summary className="grid cursor-pointer grid-cols-[56px_1fr] gap-2">
                    <img src={asset.image_url || `/api/generated-image?path=${encodeURIComponent(asset.path)}`} alt={asset.workflow} className="h-14 w-14 rounded-md object-cover" />
                    <div className="min-w-0">
                      <div className="truncate text-[11px] font-medium text-neutral-200">{asset.workflow || asset.mode || 'Image Flow output'} {asset.archived ? '· archived' : ''}</div>
                      <div className="truncate text-[10px] text-neutral-500">{asset.mode || 'output'} · {asset.model || 'model default'}</div>
                      <div className="mt-1 flex flex-wrap gap-1">
                        <button onClick={event => { event.preventDefault(); useAssetAsInput(asset) }} className="rounded bg-smara-500/15 px-2 py-1 text-[10px] text-smara-100 ring-1 ring-smara-300/15">Input</button>
                        <button onClick={event => { event.preventDefault(); useAssetAsMask(asset) }} className="rounded bg-sky-500/15 px-2 py-1 text-[10px] text-sky-100 ring-1 ring-sky-300/15">Mask</button>
                        <button onClick={event => { event.preventDefault(); setPreviewUrl(asset.image_url || `/api/generated-image?path=${encodeURIComponent(asset.path)}`) }} className="rounded bg-white/5 px-2 py-1 text-[10px] text-neutral-300">Preview</button>
                      </div>
                    </div>
                  </summary>
                  <div className="mt-2 space-y-1 border-t border-neutral-800/70 pt-2 text-[10px] leading-4 text-neutral-500">
                    <div>ID: {asset.id}</div>
                    <div>Job: {asset.job_id || '-'}</div>
                    <div>Size: {asset.width || '?'}×{asset.height || '?'} · {asset.size_bytes ? `${Math.round(asset.size_bytes / 1024)} KB` : 'unknown'}</div>
                    <div className="break-all">Path: {asset.path}</div>
                    {asset.prompt ? <div className="line-clamp-3">Prompt: {asset.prompt}</div> : null}
                    {asset.seed ? <div>Seed: {asset.seed}</div> : null}
                    <div className="flex gap-1 pt-1">
                      <button onClick={() => archiveAsset(asset, !asset.archived)} className="rounded bg-neutral-800 px-2 py-1 text-neutral-300">{asset.archived ? 'Unarchive' : 'Archive'}</button>
                      <button onClick={() => deleteAsset(asset)} className="rounded bg-red-500/15 px-2 py-1 text-red-200 ring-1 ring-red-300/15">Delete</button>
                    </div>
                  </div>
                </details>
              ))}
            </div>
          </div>

          <div className="rounded-lg bg-neutral-950/35 p-2 text-[11px] leading-4 text-neutral-400">{notice}</div>
          <details className="rounded-lg bg-neutral-950/35 p-2" open={runLogs.length > 0}>
            <summary className="cursor-pointer text-xs text-neutral-400">Run Logs ({runLogs.length})</summary>
            <div className="mt-2 max-h-40 space-y-1 overflow-auto text-[10px] leading-4 text-neutral-500">
              {runLogs.length === 0 ? <div>Belum ada log.</div> : runLogs.map((line, idx) => <div key={`${idx}-${line}`}>{line}</div>)}
            </div>
          </details>
          <details className="rounded-lg bg-neutral-950/35 p-2">
            <summary className="cursor-pointer text-xs text-neutral-400">Workflow JSON</summary>
            <pre className="mt-2 max-h-52 overflow-auto text-[10px] leading-4 text-neutral-500">{workflowJSON}</pre>
          </details>
        </aside>
      </div>
    </div>
  )
}
