import { useMemo } from 'react'
import { marked } from 'marked'
import CollapsibleCode from './CollapsibleCode'

// Configure marked for performance
marked.setOptions({
  breaks: true,
  gfm: true
})

interface Segment {
  type: 'text' | 'code'
  content: string
  language?: string
}

function parseMarkdownSegments(markdown: string): Segment[] {
  const segments: Segment[] = []
  const codeFenceRegex = /```(\w+)?\n([\s\S]*?)```/g
  let lastIndex = 0
  let match

  while ((match = codeFenceRegex.exec(markdown)) !== null) {
    // Text before code block
    if (match.index > lastIndex) {
      segments.push({
        type: 'text',
        content: markdown.slice(lastIndex, match.index),
      })
    }

    // Code block
    segments.push({
      type: 'code',
      language: match[1] || undefined,
      content: match[2],
    })

    lastIndex = match.index + match[0].length
  }

  // Remaining text after last code block
  if (lastIndex < markdown.length) {
    segments.push({
      type: 'text',
      content: markdown.slice(lastIndex),
    })
  }

  // No code blocks found — treat entire content as text
  if (segments.length === 0) {
    segments.push({ type: 'text', content: markdown })
  }

  return segments
}

interface CollapsibleMarkdownProps {
  content: string
}

export default function CollapsibleMarkdown({ content }: CollapsibleMarkdownProps) {
  const segments = useMemo(() => parseMarkdownSegments(content), [content])

  return (
    <div className="markdown-content leading-relaxed">
      {segments.map((segment, i) => {
        if (segment.type === 'code') {
          return (
            <CollapsibleCode
              key={i}
              language={segment.language}
              content={segment.content}
              defaultOpen={false}
            />
          )
        }

        // Render text segment with marked
        const html = marked.parse(segment.content, { async: false }) as string
        return (
          <div
            key={i}
            dangerouslySetInnerHTML={{ __html: html }}
          />
        )
      })}
    </div>
  )
}
