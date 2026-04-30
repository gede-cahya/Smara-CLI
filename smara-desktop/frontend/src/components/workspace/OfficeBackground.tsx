import React from 'react'
import { DESK_POSITIONS } from '../../types/workspace'

const OfficeBackground: React.FC = () => {
  return (
    <svg
      viewBox="0 0 1000 800"
      className="w-full h-full"
      preserveAspectRatio="xMidYMid slice"
      style={{ position: 'absolute', inset: 0 }}
    >
      <defs>
        <pattern id="floorGrid" width="40" height="40" patternUnits="userSpaceOnUse">
          <path
            d="M 40 0 L 0 0 0 40"
            fill="none"
            stroke="currentColor"
            strokeOpacity="0.08"
            strokeWidth="1"
          />
        </pattern>
        <linearGradient id="floorGrad" x1="0" y1="0" x2="0" y2="1">
          <stop offset="0%" stopColor="hsl(var(--primary))" stopOpacity="0.03" />
          <stop offset="100%" stopColor="transparent" />
        </linearGradient>
      </defs>

      {/* Floor */}
      <rect width="1000" height="800" fill="url(#floorGrid)" />
      <rect width="1000" height="800" fill="url(#floorGrad)" />

      {/* Desks */}
      {DESK_POSITIONS.map((desk) => (
        <g key={desk.role}>
          {/* Desk shadow */}
          <rect
            x={desk.x - 62}
            y={desk.y - 42}
            width={124}
            height={84}
            rx={12}
            fill="black"
            opacity={0.15}
          />
          {/* Desk body */}
          <rect
            x={desk.x - 60}
            y={desk.y - 40}
            width={120}
            height={80}
            rx={12}
            fill="hsl(var(--card))"
            stroke="hsl(var(--border))"
            strokeWidth="1.5"
          />
          {/* Desk accent */}
          <rect
            x={desk.x - 60}
            y={desk.y - 40}
            width={120}
            height={6}
            rx={3}
            fill={desk.color}
            opacity={0.6}
          />
          {/* Monitor */}
          <rect
            x={desk.x - 25}
            y={desk.y - 55}
            width={50}
            height={32}
            rx={3}
            fill="#1a1a2e"
            stroke="hsl(var(--border))"
            strokeWidth="1"
          />
          {/* Monitor stand */}
          <rect
            x={desk.x - 3}
            y={desk.y - 25}
            width={6}
            height={8}
            fill="hsl(var(--muted-foreground))"
            opacity={0.5}
          />
          {/* Screen glow */}
          <rect
            x={desk.x - 22}
            y={desk.y - 52}
            width={44}
            height={26}
            rx={2}
            fill={desk.color}
            opacity={0.15}
          />
          {/* Keyboard */}
          <rect
            x={desk.x - 20}
            y={desk.y - 10}
            width={40}
            height={8}
            rx={2}
            fill="hsl(var(--muted))"
            opacity={0.6}
          />
          {/* Label */}
          <text
            x={desk.x}
            y={desk.y + 55}
            textAnchor="middle"
            className="text-[10px] font-bold uppercase tracking-wider"
            fill="hsl(var(--muted-foreground))"
          >
            {desk.label}
          </text>
        </g>
      ))}

      {/* Connection lines between desks */}
      <g stroke="hsl(var(--border))" strokeWidth="1" strokeDasharray="4 4" opacity={0.3}>
        <line x1={500} y1={300} x2={200} y2={200} />
        <line x1={500} y1={300} x2={800} y2={200} />
        <line x1={500} y1={300} x2={500} y2={500} />
        <line x1={200} y1={200} x2={300} y2={650} />
        <line x1={800} y1={200} x2={300} y2={650} />
        <line x1={500} y1={500} x2={300} y2={650} />
      </g>

      {/* Server rack behind Backend */}
      <g transform={`translate(${800 + 70}, ${200 - 30})`}>
        <rect x={0} y={0} width={40} height={60} rx={4} fill="hsl(var(--card))" stroke="hsl(var(--border))" strokeWidth="1.5" />
        <rect x={4} y={4} width={32} height={4} rx={1} fill="#ef4444" opacity={0.4} />
        <rect x={4} y={12} width={32} height={4} rx={1} fill="#ef4444" opacity={0.4} />
        <rect x={4} y={20} width={32} height={4} rx={1} fill="#ef4444" opacity={0.4} />
        <rect x={4} y={28} width={32} height={4} rx={1} fill="#ef4444" opacity={0.4} />
        <circle cx={8} cy={52} r={2} fill="#10b981" opacity={0.8}>
          <animate attributeName="opacity" values="0.8;0.3;0.8" dur="2s" repeatCount="indefinite" />
        </circle>
      </g>

      {/* Database icon behind Database desk */}
      <g transform={`translate(${500 + 65}, ${500 - 20})`}>
        <ellipse cx={15} cy={8} rx={12} ry={6} fill="#10b981" opacity={0.2} stroke="#10b981" strokeWidth="1" />
        <rect x={3} y={8} width={24} height={16} rx={2} fill="#10b981" opacity={0.15} stroke="#10b981" strokeWidth="1" />
        <ellipse cx={15} cy={24} rx={12} ry={6} fill="#10b981" opacity={0.15} stroke="#10b981" strokeWidth="1" />
      </g>

      {/* QA inspection area */}
      <rect
        x={150}
        y={620}
        width={300}
        height={120}
        rx={16}
        fill="hsl(var(--card))"
        stroke="#f59e0b"
        strokeWidth="1"
        strokeDasharray="8 4"
        opacity={0.1}
      />
      <text
        x={300}
        y={740}
        textAnchor="middle"
        className="text-[9px] font-bold uppercase tracking-widest"
        fill="#f59e0b"
        opacity={0.5}
      >
        QA Inspection Zone
      </text>
    </svg>
  )
}

export default React.memo(OfficeBackground)
