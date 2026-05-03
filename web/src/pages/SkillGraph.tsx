import { useState, useEffect, useCallback } from 'react'
import {
  ReactFlow,
  Background,
  Controls,
  MiniMap,
  type Node,
  type Edge,
} from '@xyflow/react'
import '@xyflow/react/dist/style.css'
import { fetchJSON } from '../api'

const categoryColors: Record<string, string> = {
  deploy: '#3b82f6',
  frontend: '#10b981',
  backend: '#8b5cf6',
  test: '#f59e0b',
  dev: '#6366f1',
  misc: '#6b7280',
}

function getColor(category?: string) {
  if (!category) return '#6b7280'
  return categoryColors[category.toLowerCase()] || '#6b7280'
}

export default function SkillGraph() {
  const [nodes, setNodes] = useState<Node[]>([])
  const [edges, setEdges] = useState<Edge[]>([])
  const [loading, setLoading] = useState(false)
  const [selected, setSelected] = useState<string | null>(null)

  const load = useCallback(async () => {
    setLoading(true)
    try {
      const data = await fetchJSON<{
        nodes: Array<{ id: string; label: string; category: string; version: number }>
        edges: Array<{ source: number; target: number; type: string }>
      }>('/api/skills/tree?format=graph')

      const flowNodes = (data.nodes || []).map((n, i): Node => ({
        id: n.id,
        position: { x: (i % 5) * 200, y: Math.floor(i / 5) * 150 },
        data: { label: n.label, category: n.category, version: n.version },
        style: {
          background: getColor(n.category),
          color: '#fff',
          border: 'none',
          borderRadius: 6,
          padding: '8px 12px',
          fontSize: 12,
          fontWeight: 500,
          minWidth: 100,
          textAlign: 'center',
          cursor: 'pointer',
        },
      }))

      const flowEdges = (data.edges || []).map((e, i): Edge => ({
        id: `e${i}`,
        source: data.nodes[e.source]?.id || '',
        target: data.nodes[e.target]?.id || '',
        animated: e.type === 'dependency',
        style: {
          stroke: e.type === 'parent' ? '#9ca3af' : '#3b82f6',
          strokeDasharray: e.type === 'parent' ? '5,5' : undefined,
          strokeWidth: e.type === 'dependency' ? 2 : 1,
        },
        label: e.type,
        labelStyle: { fontSize: 10, fill: '#9ca3af' },
      })).filter(e => e.source && e.target)

      setNodes(flowNodes)
      setEdges(flowEdges)
    } catch (e) {
      console.error(e)
    } finally {
      setLoading(false)
    }
  }, [setNodes, setEdges])

  useEffect(() => { load() }, [load])

  return (
    <div className="flex flex-col h-full p-4">
      <div className="flex items-center justify-between mb-2">
        <h3 className="text-sm font-medium text-gray-300">Skill Dependency Graph</h3>
        <button
          onClick={load}
          className="px-2 py-1 bg-smara-700 hover:bg-smara-600 rounded text-[10px] text-white transition-colors"
        >
          Refresh
        </button>
      </div>
      {loading && <div className="text-gray-500 text-xs mb-2">Loading graph...</div>}
      <div className="flex-1 border border-gray-800 rounded-lg overflow-hidden bg-gray-900/30">
        <ReactFlow
          nodes={nodes}
          edges={edges}
          onNodesChange={() => {}}
          onEdgesChange={() => {}}
          onNodeClick={(_, node) => setSelected(node.id)}
          fitView
        >
          <Background gap={16} size={1} color="#374151" />
          <Controls />
          <MiniMap nodeColor={(n) => (n.style?.background as string) || '#6b7280'} maskColor="rgba(0,0,0,0.4)" />
        </ReactFlow>
      </div>
      {selected && (
        <div className="mt-2 p-2 bg-gray-900/50 border border-gray-800 rounded text-xs text-gray-300">
          Selected: <span className="font-medium text-smara-300">{selected}</span>
        </div>
      )}
    </div>
  )
}
