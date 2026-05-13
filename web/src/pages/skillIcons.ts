// skillIcons.ts — shared icon resolution for the skill tree views.
//
// Icons are returned as emoji glyphs because SVG <text> can render them
// natively without needing inline SVG paths or foreignObject. The mapping
// is keyed by tag and category_path, with a sensible fallback.

import { type SkillItem } from '../api'

// Ordered priority list. First hit wins.
const MAPPINGS: Array<{ match: (s: string) => boolean; icon: string }> = [
  // Remote ops
  { match: s => /(ssh|remote|vps|server|cloud)/.test(s), icon: '🖥️' },
  // Deploy & release
  { match: s => /(deploy|release|publish|ship|rollout|ci)/.test(s), icon: '🚀' },
  // Monitoring
  { match: s => /(monitor|alert|log|metric|observability|health)/.test(s), icon: '📊' },
  // Database
  { match: s => /(database|db|sql|postgres|mysql|sqlite|redis|backup|restore)/.test(s), icon: '🗄️' },
  // Frontend / web
  { match: s => /(frontend|web|react|vue|nextjs|ui|css|html|bundle)/.test(s), icon: '🎨' },
  // Testing / QA
  { match: s => /(test|qa|e2e|unit|integration|smoke|contract)/.test(s), icon: '🧪' },
  // Build
  { match: s => /(build|compile|bundler|webpack|vite)/.test(s), icon: '🔨' },
  // File operations
  { match: s => /(file|edit|read|view|list|grep)/.test(s), icon: '📄' },
  // Web browse
  { match: s => /(browse|fetch|scrape|crawl)/.test(s), icon: '🌐' },
  // Memory
  { match: s => /(memory|remember|recall|note)/.test(s), icon: '🧠' },
  // MCP / integration
  { match: s => /(mcp|integrat|connect|bridge)/.test(s), icon: '🔌' },
  // Graphify / analysis
  { match: s => /(graph|graphify|analy|reverse|extract)/.test(s), icon: '🔍' },
  // Skill composition
  { match: s => /(skill|compose|automate|workflow|routine)/.test(s), icon: '⚡' },
  // Shell
  { match: s => /(shell|command|bash|run)/.test(s), icon: '⌨️' },
  // Security
  { match: s => /(auth|secure|cert|ssl|key|token)/.test(s), icon: '🔐' },
  // Auto-captured marker
  { match: s => /(auto|captured|pattern)/.test(s), icon: '🤖' },
  // DevOps utilities
  { match: s => /(docker|container|k8s|kube)/.test(s), icon: '📦' },
  { match: s => /(git|commit|push|pull)/.test(s), icon: '🌿' },
  // Data science
  { match: s => /(data|analyt|science|pandas|ml|ai)/.test(s), icon: '📈' },
  // Notifications
  { match: s => /(notif|message|alert|email|telegram)/.test(s), icon: '📬' },
]

const DEFAULT_ICON = '✨'
const CATEGORY_ICON = '📁'

/** Returns an emoji that best describes the skill. */
export function getSkillIcon(sk: SkillItem | null | undefined, synthetic = false): string {
  if (synthetic) return CATEGORY_ICON
  if (!sk) return DEFAULT_ICON

  // Check tags first, then category_path, then name as last resort.
  const haystacks = [
    ...(sk.tags ?? []),
    ...(sk.category_path ?? []),
    sk.name,
  ].map(s => s.toLowerCase())

  for (const needle of haystacks) {
    for (const m of MAPPINGS) {
      if (m.match(needle)) return m.icon
    }
  }
  return DEFAULT_ICON
}

/** Returns an emoji for a raw category label (used by synthetic nodes). */
export function getCategoryIcon(label: string): string {
  const l = label.toLowerCase()
  for (const m of MAPPINGS) {
    if (m.match(l)) return m.icon
  }
  return CATEGORY_ICON
}
