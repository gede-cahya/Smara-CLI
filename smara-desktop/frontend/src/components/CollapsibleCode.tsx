import { useState } from 'react'
import { ChevronRight, ChevronDown, Copy, Check } from 'lucide-react'
import { cn } from '@/lib/utils'

interface CollapsibleCodeProps {
  language?: string
  content: string
  defaultOpen?: boolean
}

export default function CollapsibleCode({ language, content, defaultOpen = false }: CollapsibleCodeProps) {
  const [isOpen, setIsOpen] = useState(defaultOpen)
  const [copied, setCopied] = useState(false)

  const lineCount = content.split('\n').length
  const displayLang = language || 'text'

  const handleCopy = async () => {
    try {
      await navigator.clipboard.writeText(content)
      setCopied(true)
      setTimeout(() => setCopied(false), 2000)
    } catch {
      // Fallback
    }
  }

  return (
    <div className="my-3 rounded-xl border border-border/60 overflow-hidden bg-muted/40">
      {/* Header — always visible */}
      <button
        onClick={() => setIsOpen(!isOpen)}
        className={cn(
          "w-full flex items-center justify-between px-3 py-2 text-xs font-mono",
          "bg-muted/60 hover:bg-muted/80 transition-colors cursor-pointer select-none",
          isOpen && "border-b border-border/40"
        )}
      >
        <div className="flex items-center gap-2">
          {isOpen ? (
            <ChevronDown size={14} className="text-muted-foreground" />
          ) : (
            <ChevronRight size={14} className="text-muted-foreground" />
          )}
          <span className="text-primary font-semibold uppercase tracking-wider">
            {displayLang}
          </span>
          <span className="text-muted-foreground">
            ({lineCount} {lineCount === 1 ? 'line' : 'lines'})
          </span>
        </div>
        <div className="flex items-center gap-2">
          <button
            onClick={(e) => {
              e.stopPropagation()
              handleCopy()
            }}
            className={cn(
              "p-1 rounded-md transition-colors",
              copied
                ? "text-green-500 bg-green-500/10"
                : "text-muted-foreground hover:text-foreground hover:bg-muted"
            )}
            title="Copy code"
          >
            {copied ? <Check size={12} /> : <Copy size={12} />}
          </button>
        </div>
      </button>

      {/* Body — collapsible */}
      <div
        className={cn(
          "overflow-hidden transition-all duration-200",
          isOpen ? "max-h-[60vh] opacity-100" : "max-h-0 opacity-0"
        )}
      >
        <pre className="p-4 overflow-x-auto text-sm leading-relaxed">
          <code className="font-mono text-sm">{content}</code>
        </pre>
      </div>
    </div>
  )
}
