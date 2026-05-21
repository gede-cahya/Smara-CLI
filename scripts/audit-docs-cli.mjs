#!/usr/bin/env node
import { readdirSync, readFileSync, statSync } from 'node:fs'
import { join, relative } from 'node:path'

const cwd = process.cwd()
const repoRoot = cwd.endsWith('docs-site') ? join(cwd, '..') : cwd
const root = repoRoot
const cmdDir = join(root, 'cmd', 'smara')
const docsDir = join(root, 'docs-site')
const skipDirs = new Set(['node_modules', 'dist', '.vitepress'])

function walk(dir, predicate = () => true) {
  const out = []
  for (const entry of readdirSync(dir)) {
    const path = join(dir, entry)
    const st = statSync(path)
    if (st.isDirectory()) {
      if (skipDirs.has(entry)) continue
      out.push(...walk(path, predicate))
    } else if (predicate(path)) {
      out.push(path)
    }
  }
  return out
}

function extractCommands() {
  const files = walk(cmdDir, p => p.endsWith('.go'))
  const commands = new Map()
  const useRe = /Use:\s*(?:`([^`]+)`|"([^"]+)")/g
  for (const file of files) {
    const text = readFileSync(file, 'utf8')
    for (const match of text.matchAll(useRe)) {
      const raw = (match[1] || match[2] || '').trim()
      if (!raw || raw.startsWith('{{')) continue
      const name = raw.split(/\s+/)[0]
      if (!name || name === 'smara') continue
      if (!commands.has(name)) commands.set(name, new Set())
      commands.get(name).add(relative(root, file))
    }
  }
  return commands
}

function readDocsText() {
  const files = walk(docsDir, p => p.endsWith('.md') || p.endsWith('.ts'))
  return files.map(file => ({ file: relative(root, file), text: readFileSync(file, 'utf8').toLowerCase() }))
}

function isCovered(doc, needle, sourceList) {
  if (doc.text.includes(`smara ${needle}`) || doc.text.includes(`\`${needle}`) || doc.text.includes(` ${needle} `)) return true
  if (sourceList.some(s => s.includes('provider.go')) && doc.text.includes(`provider ${needle}`)) return true
  if (sourceList.some(s => s.includes('memory_cloud.go'))) {
    return doc.text.includes(`memory cloud ${needle}`) ||
      doc.text.includes(`cloud ${needle}`) ||
      doc.text.includes(`encryption ${needle}`) ||
      doc.text.includes(`conflicts ${needle}`)
  }
  return false
}

const commands = extractCommands()
const docs = readDocsText()
const rows = []
for (const [cmd, sources] of [...commands.entries()].sort((a, b) => a[0].localeCompare(b[0]))) {
  const needle = cmd.toLowerCase()
  const sourceList = [...sources]
  const hits = docs.filter(d => isCovered(d, needle, sourceList))
  rows.push({ cmd, covered: hits.length > 0, sources: sourceList, hits: hits.slice(0, 5).map(h => h.file) })
}

const missing = rows.filter(r => !r.covered)
console.log(`# Smara Docs CLI Audit\n`)
console.log(`Commands found: ${rows.length}`)
console.log(`Covered: ${rows.length - missing.length}`)
console.log(`Missing: ${missing.length}\n`)

if (missing.length) {
  console.log(`## Missing or weak coverage\n`)
  for (const row of missing) {
    console.log(`- ${row.cmd} (${row.sources.join(', ')})`)
  }
  console.log('')
}

console.log(`## Command coverage table\n`)
console.log(`| Command | Status | Source | Docs hits |`)
console.log(`|---|---|---|---|`)
for (const row of rows) {
  console.log(`| \`${row.cmd}\` | ${row.covered ? 'covered' : 'missing'} | ${row.sources.join('<br>')} | ${row.hits.join('<br>') || '—'} |`)
}

if (missing.length) process.exitCode = 1
