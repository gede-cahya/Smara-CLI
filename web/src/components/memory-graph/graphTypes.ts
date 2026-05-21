export interface MemGraphNode { id: number; label: string; content: string; tags: string[]; source: string; category_id?: number; degree: number }
export interface MemGraphEdge { from: number; to: number; relation: string; weight: number; auto: boolean }
export interface MemGraphMeta { mode?: string; node_limit?: number; edge_limit?: number; min_weight?: number; total_nodes?: number; total_edges?: number; truncated_nodes?: boolean; truncated_edges?: boolean }
export interface MemGraphData { nodes: MemGraphNode[]; edges: MemGraphEdge[]; meta?: MemGraphMeta }
