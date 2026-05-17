import { useState, useEffect, useCallback, useRef, useMemo } from 'react'
import {
  ReactFlow,
  Background,
  Controls,
  MiniMap,
  type Node,
  type Edge,
  useNodesState,
  useEdgesState,
  Panel,
  type NodeProps,
  Handle,
  Position,
} from '@xyflow/react'
import '@xyflow/react/dist/style.css'
import {
  Network,
  Search,
  Upload,
  Database,
  AlertTriangle,
  Loader2,
  X,
  FileJson,
  ChevronRight,
  Filter,
} from 'lucide-react'
import { fetchGraphList, fetchGraphData, fetchGraphQuery, type GraphNode, type GraphEdge, type GraphInfo } from '../api'

const NODE_TYPE_COLORS: Record<string, string> = {
  function: '#bef264',
  method: '#bef264',
  class: '#60a5fa',
  type: '#60a5fa',
  struct: '#60a5fa',
  interface: '#a78bfa',
  variable: '#fbbf24',
  constant: '#fbbf24',
  package: '#34d399',
  import: '#6ee7b7',
  concept: '#f472b6',
  doc: '#94a3b8',
  file: '#64748b',
  default: '#9ca3af',
}

function getNodeColor(type?: string) {
  if (!type) return NODE_TYPE_COLORS.default
  return NODE_TYPE_COLORS[type.toLowerCase()] || NODE_TYPE_COLORS.default
}

function circularLayout(nodes: GraphNode[], radius = 300) {
  const count = nodes.length
  const angleStep = (2 * Math.PI) / Math.max(count, 1)
  return nodes.map((_n, i) => {
    const angle = i * angleStep
    return {
      x: Math.cos(angle) * radius,
      y: Math.sin(angle) * radius,
    }
  })
}

function CustomNode({ data }: NodeProps) {
  const label = (data?.label as string) || ''
  const type = (data?.type as string) || ''
  const color = getNodeColor(type)
  return (
    <div
      className="rounded-md px-2 py-1 text-[11px] font-medium text-center min-w-[80px] cursor-pointer select-none pointer-events-auto"
      style={{
        background: color + '22',
        border: `1px solid ${color}`,
        color: '#e5e7eb',
      }}
      title={label}
    >
      <Handle type="target" position={Position.Top} className="!w-1.5 !h-1.5 !bg-gray-500 !border-none pointer-events-none" />
      <div className="truncate max-w-[140px]" title={label}>{label}</div>
      <Handle type="source" position={Position.Bottom} className="!w-1.5 !h-1.5 !bg-gray-500 !border-none pointer-events-none" />
    </div>
  )
}

function toFlowNodes(graphNodes: GraphNode[]): Node[] {
  const positions = circularLayout(graphNodes, Math.max(300, graphNodes.length * 8))
  return graphNodes.map((n, i): Node => {
    const color = getNodeColor(n.type)
    return {
      id: n.id,
      type: 'custom',
      position: positions[i] || { x: 0, y: 0 },
      data: {
        label: n.label,
        type: n.type,
        sourceFile: n.source_file,
        sourceLine: n.source_line,
        content: n.content,
        community: n.community,
        language: n.language,
        godScore: n.god_score,
        original: n,
      },
      style: {
        background: color + '22',
        border: `1px solid ${color}`,
        color: '#e5e7eb',
        borderRadius: 6,
        padding: '6px 10px',
        fontSize: 11,
        fontWeight: 500,
        minWidth: 80,
        textAlign: 'center',
        cursor: 'pointer',
      },
    }
  })
}

function toFlowEdges(graphEdges: GraphEdge[]): Edge[] {
  return graphEdges.map((e): Edge => ({
    id: e.id,
    source: e.source,
    target: e.target,
    label: e.relation,
    animated: e.confidence === 'INFERRED',
    style: {
      stroke: e.confidence === 'INFERRED' ? '#a78bfa' : '#4b5563',
      strokeWidth: e.confidence === 'INFERRED' ? 1.5 : 1,
      strokeDasharray: e.confidence === 'INFERRED' ? '4,4' : undefined,
    },
    labelStyle: { fontSize: 9, fill: '#9ca3af' },
    labelBgStyle: { fill: '#1f2937', fillOpacity: 0.8 },
    labelBgPadding: [2, 4],
    labelBgBorderRadius: 4,
  }))
}

export default function Graphify() {
  const [graphs, setGraphs] = useState<GraphInfo[]>([])
  const [selectedGraph, setSelectedGraph] = useState<string | null>(null)
  const [loadingList, setLoadingList] = useState(false)
  const [loadingData, setLoadingData] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [searchText, setSearchText] = useState('')
  const [filterType, setFilterType] = useState('')
  const [selectedNode, setSelectedNode] = useState<GraphNode | null>(null)
  const [uploadData, setUploadData] = useState<{ nodes: GraphNode[]; edges: GraphEdge[] } | null>(null)
  const [dragOver, setDragOver] = useState(false)
  const [warning, setWarning] = useState<string | null>(null)
  const fileRef = useRef<HTMLInputElement>(null)

  const [nodes, setNodes, onNodesChange] = useNodesState<Node>([])
  const [edges, setEdges, onEdgesChange] = useEdgesState<Edge>([])
  const [selectedNodeId, setSelectedNodeId] = useState<string | null>(null)

  const highlightedNodes = useMemo(() => {
    if (!selectedNodeId) return nodes
    const connected = new Set<string>([selectedNodeId])
    edges.forEach(edge => {
      if (edge.source === selectedNodeId) connected.add(edge.target)
      if (edge.target === selectedNodeId) connected.add(edge.source)
    })
    return nodes.map(node => {
      const isSelected = node.id === selectedNodeId
      const isConnected = connected.has(node.id)
      const type = (node.data?.type as string) || ''
      const color = getNodeColor(type)
      return {
        ...node,
        selected: isSelected,
        draggable: true,
        style: {
          ...node.style,
          border: isSelected ? `2px solid #facc15` : isConnected ? `2px solid ${color}` : `1px solid ${color}`,
          boxShadow: isSelected
            ? '0 0 22px rgba(250, 204, 21, 0.75)'
            : isConnected
              ? `0 0 14px ${color}77`
              : 'none',
          opacity: !selectedNodeId || isConnected ? 1 : 0.35,
        },
      }
    })
  }, [nodes, edges, selectedNodeId])

  const highlightedEdges = useMemo(() => {
    if (!selectedNodeId) return edges
    return edges.map(edge => {
      const isConnected = edge.source === selectedNodeId || edge.target === selectedNodeId
      return {
        ...edge,
        animated: isConnected || edge.animated,
        style: {
          ...edge.style,
          stroke: isConnected ? '#facc15' : '#374151',
          strokeWidth: isConnected ? 3 : 1,
          opacity: isConnected ? 1 : 0.25,
        },
        labelStyle: {
          ...edge.labelStyle,
          fill: isConnected ? '#fde68a' : '#6b7280',
        },
      }
    })
  }, [edges, selectedNodeId])

  const loadGraphList = useCallback(async () => {
    setLoadingList(true)
    setError(null)
    try {
      const res = await fetchGraphList()
      setGraphs(res.graphs || [])
    } catch (e: any) {
      setError(e.message || 'Failed to load graph list')
    } finally {
      setLoadingList(false)
    }
  }, [])

  useEffect(() => {
    loadGraphList()
  }, [loadGraphList])

  const loadGraph = useCallback(async (id: string) => {
    if (!id) return
    setLoadingData(true)
    setError(null)
    setWarning(null)
    setSelectedNode(null)
    setSelectedNodeId(null)
    setUploadData(null)
    try {
      const data = await fetchGraphData(id)
      setNodes(toFlowNodes(data.nodes))
      setEdges(toFlowEdges(data.edges))
      if (data.truncated) {
        setWarning(`Graph has ${data.node_count} nodes — showing first 500. Use search to narrow results.`)
      }
    } catch (e: any) {
      setError(e.message || 'Failed to load graph data')
    } finally {
      setLoadingData(false)
    }
  }, [setNodes, setEdges])

  const handleGraphSelect = useCallback((id: string) => {
    if (!id) return
    setSelectedGraph(id)
    loadGraph(id)
  }, [loadGraph])

  const handleSearch = useCallback(async () => {
    if (!selectedGraph || !searchText.trim()) return
    setLoadingData(true)
    setError(null)
    setWarning(null)
    setSelectedNodeId(null)
    setSelectedNode(null)
    try {
      const result = await fetchGraphQuery(selectedGraph, searchText.trim(), 2)
      setNodes(toFlowNodes(result.nodes))
      setEdges(toFlowEdges(result.edges))
      if (result.nodes.length === 0) {
        setWarning('No nodes matched your search.')
      }
    } catch (e: any) {
      setError(e.message || 'Search failed')
    } finally {
      setLoadingData(false)
    }
  }, [selectedGraph, searchText, setNodes, setEdges])

  const handleFilterType = useCallback((type: string) => {
    setFilterType(type)
    if (!uploadData) return
    setSelectedNodeId(null)
    setSelectedNode(null)
    const baseNodes = uploadData.nodes || []
    const baseEdges = uploadData.edges || []
    if (type === '') {
      setNodes(toFlowNodes(baseNodes))
      setEdges(toFlowEdges(baseEdges))
      return
    }
    const filtered = baseNodes.filter(n => n.type === type)
    const filteredIds = new Set(filtered.map(n => n.id))
    const filteredEdges = baseEdges.filter(e => filteredIds.has(e.source) && filteredIds.has(e.target))
    setNodes(toFlowNodes(filtered))
    setEdges(toFlowEdges(filteredEdges))
  }, [uploadData, setNodes, setEdges])

  const handleFile = useCallback((file: File) => {
    if (!file.name.endsWith('.json')) {
      setError('Only .json files are supported')
      return
    }
    const reader = new FileReader()
    reader.onload = () => {
      try {
        const parsed = JSON.parse(reader.result as string)
        const dataNodes: GraphNode[] = parsed.nodes || []
        const dataEdges: GraphEdge[] = parsed.edges || []
        if (dataNodes.length === 0) {
          setError('No nodes found in uploaded file')
          return
        }
        setSelectedGraph(null)
        setUploadData({ nodes: dataNodes, edges: dataEdges })
        setSelectedNodeId(null)
        setSelectedNode(null)
        setNodes(toFlowNodes(dataNodes))
        setEdges(toFlowEdges(dataEdges))
        setError(null)
        setWarning(dataNodes.length > 500 ? `Uploaded file has ${dataNodes.length} nodes — showing first 500.` : null)
      } catch {
        setError('Invalid JSON file')
      }
    }
    reader.readAsText(file)
  }, [setNodes, setEdges])

  const onDrop = useCallback((e: React.DragEvent) => {
    e.preventDefault()
    setDragOver(false)
    const file = e.dataTransfer.files[0]
    if (file) handleFile(file)
  }, [handleFile])

  const onDragOver = useCallback((e: React.DragEvent) => {
    e.preventDefault()
    setDragOver(true)
  }, [])

  const onDragLeave = useCallback(() => setDragOver(false), [])

  const nodeTypesSet = new Set<string>()
  if (uploadData) {
    uploadData.nodes.forEach(n => nodeTypesSet.add(n.type))
  } else if (selectedGraph) {
    // We don't have easy access to original graph nodes here without storing them,
    // so we extract from flow nodes data instead
    nodes.forEach(n => {
      if (n.data?.type) nodeTypesSet.add(n.data.type as string)
    })
  }
  const nodeTypes = Array.from(nodeTypesSet).sort()

  return (
    <div className="flex h-full w-full overflow-hidden bg-gray-950 text-gray-100">
      {/* Sidebar */}
      <aside className="w-72 bg-gray-900 border-r border-gray-800 flex flex-col overflow-hidden">
        <div className="p-4 border-b border-gray-800">
          <div className="flex items-center gap-2 mb-3">
            <Network className="w-4 h-4 text-smara-300" />
            <h2 className="font-semibold text-sm">Graphify</h2>
          </div>

          {/* Search / Query */}
          <div className="space-y-2">
            <div className="flex gap-1">
              <input
                type="text"
                value={searchText}
                onChange={e => setSearchText(e.target.value)}
                onKeyDown={e => e.key === 'Enter' && handleSearch()}
                placeholder="Search nodes..."
                className="flex-1 bg-gray-800 border border-gray-700 rounded px-2 py-1 text-xs text-gray-200 placeholder-gray-500 focus:outline-none focus:border-smara-500"
              />
              <button
                onClick={handleSearch}
                disabled={!selectedGraph || !searchText.trim() || loadingData}
                className="px-2 py-1 bg-smara-700 hover:bg-smara-600 disabled:bg-gray-700 disabled:text-gray-500 rounded text-xs text-white transition-colors"
              >
                <Search className="w-3 h-3" />
              </button>
            </div>

            {nodeTypes.length > 0 && (
              <div className="flex items-center gap-1">
                <Filter className="w-3 h-3 text-gray-500" />
                <select
                  value={filterType}
                  onChange={e => handleFilterType(e.target.value)}
                  className="flex-1 bg-gray-800 border border-gray-700 rounded px-2 py-1 text-xs text-gray-200 focus:outline-none focus:border-smara-500"
                >
                  <option value="">All types</option>
                  {nodeTypes.map(t => (
                    <option key={t} value={t}>{t}</option>
                  ))}
                </select>
              </div>
            )}
          </div>
        </div>

        {/* Graph list */}
        <div className="flex-1 overflow-y-auto p-3 space-y-2">
          <div className="flex items-center gap-2 text-xs text-gray-400 mb-1">
            <Database className="w-3 h-3" />
            <span className="font-medium">Stored Graphs</span>
            <button
              onClick={loadGraphList}
              disabled={loadingList}
              className="ml-auto text-gray-500 hover:text-gray-300 transition-colors"
            >
              {loadingList ? <Loader2 className="w-3 h-3 animate-spin" /> : <ChevronRight className="w-3 h-3 rotate-90" />}
            </button>
          </div>

          {graphs.length === 0 && !loadingList && (
            <div className="text-xs text-gray-500 italic px-1">No graphs stored.</div>
          )}

          {graphs.map(g => (
            <button
              key={g.graph_id}
              onClick={() => handleGraphSelect(g.graph_id)}
              className={`w-full text-left px-3 py-2 rounded-lg text-xs transition-colors ${
                selectedGraph === g.graph_id
                  ? 'bg-smara-700/20 border border-smara-700/40 text-smara-300'
                  : 'bg-gray-800/50 hover:bg-gray-800 text-gray-300 border border-transparent'
              }`}
            >
              <div className="font-medium truncate">{g.graph_id}</div>
              <div className="text-gray-500 mt-0.5 flex gap-2">
                <span>{g.node_count} nodes</span>
                <span>{g.edge_count} edges</span>
              </div>
            </button>
          ))}
        </div>

        {/* Upload */}
        <div className="p-3 border-t border-gray-800">
          <div
            onDrop={onDrop}
            onDragOver={onDragOver}
            onDragLeave={onDragLeave}
            className={`border-2 border-dashed rounded-lg p-3 text-center transition-colors ${
              dragOver ? 'border-smara-500 bg-smara-900/10' : 'border-gray-700 hover:border-gray-600'
            }`}
          >
            <Upload className="w-4 h-4 text-gray-500 mx-auto mb-1" />
            <div className="text-[10px] text-gray-500">
              Drop .json file or{' '}
              <button
                onClick={() => fileRef.current?.click()}
                className="text-smara-400 hover:text-smara-300 underline"
              >
                browse
              </button>
            </div>
            <input
              ref={fileRef}
              type="file"
              accept=".json"
              className="hidden"
              onChange={e => {
                const file = e.target.files?.[0]
                if (file) handleFile(file)
                e.currentTarget.value = ''
              }}
            />
          </div>
          {uploadData && (
            <div className="mt-2 flex items-center justify-between text-xs text-gray-400">
              <span className="flex items-center gap-1">
                <FileJson className="w-3 h-3" />
                Uploaded file
              </span>
              <button onClick={() => { setUploadData(null); setNodes([]); setEdges([]); setFilterType('') }} className="text-gray-500 hover:text-gray-300">
                <X className="w-3 h-3" />
              </button>
            </div>
          )}
        </div>
      </aside>

      {/* Main canvas */}
      <main className="flex-1 flex flex-col overflow-hidden relative">
        {/* Warnings / errors */}
        {(error || warning) && (
          <div className="absolute top-3 left-1/2 -translate-x-1/2 z-20 max-w-lg w-full">
            {error && (
              <div className="bg-red-900/40 border border-red-700/50 text-red-200 px-3 py-2 rounded-lg text-xs flex items-center gap-2 mb-2">
                <AlertTriangle className="w-3 h-3 shrink-0" />
                {error}
                <button onClick={() => setError(null)} className="ml-auto text-red-400 hover:text-red-200">
                  <X className="w-3 h-3" />
                </button>
              </div>
            )}
            {warning && (
              <div className="bg-yellow-900/30 border border-yellow-700/40 text-yellow-200 px-3 py-2 rounded-lg text-xs flex items-center gap-2">
                <AlertTriangle className="w-3 h-3 shrink-0" />
                {warning}
                <button onClick={() => setWarning(null)} className="ml-auto text-yellow-400 hover:text-yellow-200">
                  <X className="w-3 h-3" />
                </button>
              </div>
            )}
          </div>
        )}

        {loadingData && (
          <div className="absolute inset-0 z-10 bg-gray-950/60 flex items-center justify-center">
            <div className="flex items-center gap-2 text-sm text-gray-400">
              <Loader2 className="w-4 h-4 animate-spin" />
              Loading graph...
            </div>
          </div>
        )}

        {/* Empty state */}
        {nodes.length === 0 && !loadingData && !uploadData && !selectedGraph && (
          <div className="flex-1 flex items-center justify-center">
            <div className="text-center text-gray-500">
              <Network className="w-10 h-10 mx-auto mb-3 text-gray-600" />
              <p className="text-sm font-medium mb-1">No graph selected</p>
              <p className="text-xs">Select a stored graph from the sidebar or upload a JSON export.</p>
            </div>
          </div>
        )}

        {/* React Flow canvas */}
        {(nodes.length > 0 || edges.length > 0 || selectedGraph || uploadData) && (
          <div className="flex-1 overflow-hidden bg-gray-950 relative">
            <ReactFlow
              nodes={highlightedNodes}
              edges={highlightedEdges}
              onNodesChange={onNodesChange}
              onEdgesChange={onEdgesChange}
              nodesDraggable
              nodesConnectable={false}
              elementsSelectable
              onPaneClick={() => {
                setSelectedNodeId(null)
                setSelectedNode(null)
              }}
              onNodeClick={(_, node) => {
                const original = (node.data?.original as GraphNode) || null
                setSelectedNodeId(node.id)
                setSelectedNode(original)
              }}
              nodeTypes={{ custom: CustomNode }}
              fitView
              fitViewOptions={{ padding: 0.2 }}
              minZoom={0.1}
              maxZoom={2}
            >
              <Background gap={20} size={1} color="#374151" />
              <Controls />
              <MiniMap
                nodeColor={(n) => {
                  const type = (n.data?.type as string) || ''
                  return getNodeColor(type)
                }}
                maskColor="rgba(0,0,0,0.4)"
                className="bg-gray-900/80 border border-gray-700 rounded-lg"
              />
              <Panel position="top-left">
                <div className="bg-gray-900/80 border border-gray-700 rounded-lg px-2 py-1 text-[10px] text-gray-400">
                  {nodes.length} nodes · {edges.length} edges
                </div>
              </Panel>
            </ReactFlow>
          </div>
        )}

        {/* Node details panel */}
        {selectedNode && (
          <div className="absolute bottom-3 right-3 z-20 w-80 bg-gray-900 border border-gray-700 rounded-lg shadow-lg overflow-hidden">
            <div className="flex items-center justify-between px-3 py-2 border-b border-gray-800">
              <div className="flex items-center gap-2 min-w-0">
                <span
                  className="inline-block w-2 h-2 rounded-full shrink-0"
                  style={{ background: getNodeColor(selectedNode.type) }}
                />
                <span className="text-xs font-medium text-gray-200 truncate" title={selectedNode.label}>
                  {selectedNode.label}
                </span>
                <span className="text-[10px] px-1.5 py-0.5 rounded bg-gray-800 text-gray-400 shrink-0">
                  {selectedNode.type}
                </span>
              </div>
              <button onClick={() => setSelectedNode(null)} className="text-gray-500 hover:text-gray-300 shrink-0 ml-2">
                <X className="w-3 h-3" />
              </button>
            </div>
            <div className="p-3 space-y-2 max-h-72 overflow-y-auto text-xs">
              <div>
                <span className="text-gray-500">Label</span>
                <p className="text-gray-200 font-medium mt-0.5">{selectedNode.label}</p>
              </div>
              <div className="flex gap-3">
                <div>
                  <span className="text-gray-500">Type</span>
                  <p className="text-gray-200 mt-0.5">{selectedNode.type}</p>
                </div>
                <div>
                  <span className="text-gray-500">Community</span>
                  <p className="text-gray-200 mt-0.5">{selectedNode.community}</p>
                </div>
              </div>
              {selectedNode.source_file && (
                <div>
                  <span className="text-gray-500">Source</span>
                  <p className="text-gray-300 mt-0.5 font-mono truncate">{selectedNode.source_file}{selectedNode.source_line > 0 ? `:${selectedNode.source_line}` : ''}</p>
                </div>
              )}
              {selectedNode.content && (
                <div>
                  <span className="text-gray-500">Content</span>
                  <pre className="mt-0.5 bg-gray-800 rounded p-2 text-gray-300 overflow-x-auto whitespace-pre-wrap text-[10px] leading-relaxed">
                    {selectedNode.content}
                  </pre>
                </div>
              )}
            </div>
          </div>
        )}
      </main>
    </div>
  )
}
