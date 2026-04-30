import React, { useEffect, useCallback } from 'react'
import { EventsOn } from '../../wailsjs/runtime/runtime'
import OfficeBackground from './workspace/OfficeBackground'
import AgentAvatar from './workspace/AgentAvatar'
import WorkspaceHUD from './workspace/WorkspaceHUD'
import { useWorkspace, buildDemoEvents } from './workspace/useWorkspace'
import { Play, Square, Monitor } from 'lucide-react'
import { Button } from './ui/button'

const VirtualWorkspace: React.FC = () => {
  const { state, queueEvent, reset } = useWorkspace()

  // Listen for Wails events
  useEffect(() => {
    let off: any = null
    if (typeof EventsOn === 'function') {
      off = EventsOn('workspace:event', (data: any) => {
        queueEvent(data)
      })
    }
    return () => {
      if (typeof off === 'function') off()
    }
  }, [queueEvent])

  const runDemo = useCallback(() => {
    const { events, delays } = buildDemoEvents()
    events.forEach((ev, i) => {
      setTimeout(() => queueEvent(ev), delays[i])
    })
  }, [queueEvent])

  const stopDemo = useCallback(() => {
    reset()
  }, [reset])

  return (
    <div className="flex flex-col h-full w-full relative overflow-hidden bg-background">
      {/* Title bar */}
      <div className="h-14 flex items-center justify-between px-6 border-b border-border/30 glass z-10">
        <div className="flex items-center gap-3">
          <Monitor size={18} className="text-primary" />
          <h2 className="text-sm font-bold tracking-tight">Virtual Workspace</h2>
          <span className="text-[10px] text-muted-foreground uppercase tracking-wider">Workflow Visualization</span>
        </div>
        <div className="flex items-center gap-2">
          <Button
            variant="outline"
            size="sm"
            onClick={state.isActive ? stopDemo : runDemo}
            className={`rounded-xl text-xs gap-2 ${
              state.isActive
                ? 'border-red-500/30 text-red-500 hover:bg-red-500/10'
                : 'border-green-500/30 text-green-500 hover:bg-green-500/10'
            }`}
          >
            {state.isActive ? <Square size={12} /> : <Play size={12} />}
            {state.isActive ? 'Stop' : 'Run Demo'}
          </Button>
        </div>
      </div>

      {/* Office canvas */}
      <div className="flex-1 relative overflow-hidden">
        <div className="absolute inset-0 flex items-center justify-center">
          <div className="relative w-full h-full max-w-[1200px] max-h-[800px]">
            <OfficeBackground />

            {/* Agent layer */}
            <div className="absolute inset-0" style={{ zIndex: 10 }}>
              {Object.values(state.agents).map(agent => (
                <AgentAvatar key={agent.id} agent={agent} />
              ))}
            </div>
          </div>
        </div>

        {/* HUD overlay */}
        <WorkspaceHUD state={state} />
      </div>
    </div>
  )
}

export default React.memo(VirtualWorkspace)
