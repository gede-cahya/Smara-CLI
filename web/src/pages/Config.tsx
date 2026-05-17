import { useEffect, useMemo, useState } from 'react'
import { AlertCircle, CheckCircle2, Cpu, Database, Eye, EyeOff, Globe, KeyRound, Plus, RefreshCw, Save, Settings, SlidersHorizontal, Trash2, Wrench } from 'lucide-react'
import { fetchJSON } from '../api'

type MCPServer = {
  name: string
  type: 'local' | 'remote'
  command?: string
  args?: string[]
  url?: string
  enabled?: boolean
  headers?: Record<string, string>
  env?: Record<string, string>
}

type ConfigData = Record<string, any> & { mcp_servers?: MCPServer[] }

type Field = { key: string; label: string; type?: 'text' | 'password' | 'number' | 'boolean' | 'select'; options?: string[]; placeholder?: string }

const providerFields: Field[] = [
  { key: 'provider', label: 'Active Provider', type: 'select', options: ['custom', 'openai', 'anthropic', 'openrouter', 'ollama'], placeholder: 'custom' },
  { key: 'model', label: 'Default Model', placeholder: 'deepseek-v4-pro / gpt-4o / llama3.1' },
  { key: 'custom_provider_name', label: 'Custom Provider Name', placeholder: 'CLIProxyAPI' },
  { key: 'custom_base_url', label: 'Custom Base URL', placeholder: 'http://localhost:8317/v1' },
  { key: 'custom_api_key', label: 'Custom API Key', type: 'password' },
  { key: 'custom_model', label: 'Custom Model' },
  { key: 'openai_api_key', label: 'OpenAI API Key', type: 'password' },
  { key: 'openai_base_url', label: 'OpenAI Base URL', placeholder: 'https://api.openai.com/v1' },
  { key: 'openai_model', label: 'OpenAI Model', placeholder: 'gpt-4o' },
  { key: 'anthropic_api_key', label: 'Anthropic API Key', type: 'password' },
  { key: 'anthropic_model', label: 'Anthropic Model', placeholder: 'claude-sonnet-4-20250514' },
  { key: 'openrouter_api_key', label: 'OpenRouter API Key', type: 'password' },
  { key: 'openrouter_model', label: 'OpenRouter Model', placeholder: 'anthropic/claude-sonnet-4' },
  { key: 'ollama_host', label: 'Ollama Host', placeholder: 'http://localhost:11434' },
]

const generalFields: Field[] = [
  { key: 'active_workspace', label: 'Active Workspace' },
  { key: 'db_path', label: 'Database Path' },
  { key: 'sync_dir', label: 'Sync Directory' },
  { key: 'sync_interval', label: 'Sync Interval (minutes)', type: 'number' },
  { key: 'verbose', label: 'Verbose Logging', type: 'boolean' },
]

const agentFields: Field[] = [
  { key: 'agent_max_iterations', label: 'Agent Max Iterations', type: 'number' },
  { key: 'agent_request_timeout_sec', label: 'Web Agent Timeout (seconds)', type: 'number' },
  { key: 'platform_prompt_timeout', label: 'Platform Prompt Timeout (seconds)', type: 'number' },
  { key: 'auto_skill_detect', label: 'Auto Skill Detection', type: 'boolean' },
  { key: 'auto_skill_threshold', label: 'Auto Skill Threshold', type: 'number' },
]

const smaraMCPFields: Field[] = [
  { key: 'smara_mcp_enabled', label: 'Enable Smara MCP', type: 'boolean' },
  { key: 'smara_mcp_command', label: 'Smara MCP Command', placeholder: 'smara' },
  { key: 'smara_mcp_args', label: 'Smara MCP Args', placeholder: 'mcp serve' },
  { key: 'smara_mcp_api_key', label: 'Smara MCP API Key', type: 'password' },
]

const cloudFields: Field[] = [
  { key: 'cloud_memory.enabled', label: 'Cloud Memory Enabled', type: 'boolean' },
  { key: 'cloud_memory.provider', label: 'Cloud Memory Provider', type: 'select', options: ['turso', 'libsql', 'custom'] },
  { key: 'cloud_memory.db_name_pattern', label: 'DB Name Pattern' },
  { key: 'cloud_memory.sync_interval_sec', label: 'Cloud Sync Interval (seconds)', type: 'number' },
  { key: 'cloud_memory.conflict_policy', label: 'Conflict Policy', type: 'select', options: ['lww', 'remote_wins', 'local_wins', 'manual'] },
  { key: 'cloud_memory.offline_mode', label: 'Offline Mode', type: 'select', options: ['auto', 'always', 'never'] },
  { key: 'cloud_memory.encrypt_at_rest', label: 'Encrypt at Rest', type: 'boolean' },
  { key: 'cloud_memory.max_rows_per_hour', label: 'Max Rows / Hour', type: 'number' },
  { key: 'cloud_memory.max_storage_mb', label: 'Max Storage MB', type: 'number' },
  { key: 'cloud_memory.embeddings_cloud', label: 'Cloud Embeddings', type: 'boolean' },
]

function getValue(obj: any, path: string) {
  return path.split('.').reduce((acc, k) => acc?.[k], obj)
}

function normalizeValue(field: Field, value: any) {
  if (field.type === 'number') return Number(value || 0)
  if (field.type === 'boolean') return Boolean(value)
  if (Array.isArray(value)) return value.join(' ')
  return value ?? ''
}

function FieldInput({ field, config, onSave, saving }: { field: Field; config: ConfigData; onSave: (key: string, value: any) => void; saving: boolean }) {
  const [value, setValue] = useState<any>(normalizeValue(field, getValue(config, field.key)))
  const [show, setShow] = useState(false)
  useEffect(() => setValue(normalizeValue(field, getValue(config, field.key))), [config, field.key])
  const dirty = String(value) !== String(normalizeValue(field, getValue(config, field.key)))

  return <div className="bg-gray-900/60 border border-gray-800 rounded-xl p-3">
    <label className="block text-xs font-medium text-gray-400 mb-1">{field.label}</label>
    <div className="flex gap-2">
      {field.type === 'boolean' ? (
        <button onClick={() => setValue(!value)} className={`px-3 py-2 rounded-lg text-sm border ${value ? 'bg-smara-700/30 border-smara-600 text-smara-200' : 'bg-gray-800 border-gray-700 text-gray-400'}`}>{value ? 'Enabled' : 'Disabled'}</button>
      ) : field.type === 'select' ? (
        <select value={value} onChange={e => setValue(e.target.value)} className="flex-1 bg-gray-950 border border-gray-700 rounded-lg px-3 py-2 text-sm focus:outline-none focus:border-smara-500">
          {(field.options || []).map(o => <option key={o} value={o}>{o}</option>)}
        </select>
      ) : (
        <div className="relative flex-1">
          <input type={field.type === 'password' && !show ? 'password' : field.type === 'number' ? 'number' : 'text'} value={value} onChange={e => setValue(e.target.value)} placeholder={field.placeholder} className="w-full bg-gray-950 border border-gray-700 rounded-lg px-3 py-2 pr-9 text-sm focus:outline-none focus:border-smara-500" />
          {field.type === 'password' && <button onClick={() => setShow(!show)} className="absolute right-2 top-2 text-gray-500 hover:text-gray-300">{show ? <EyeOff className="w-4 h-4"/> : <Eye className="w-4 h-4"/>}</button>}
        </div>
      )}
      <button disabled={!dirty || saving} onClick={() => onSave(field.key, field.type === 'number' ? Number(value) : value)} className="px-3 py-2 bg-smara-700 hover:bg-smara-600 disabled:bg-gray-800 disabled:text-gray-600 rounded-lg text-sm flex items-center gap-1"><Save className="w-4 h-4"/> Save</button>
    </div>
  </div>
}

function Section({ icon, title, desc, children }: { icon: any; title: string; desc?: string; children: React.ReactNode }) {
  const Icon = icon
  return <section className="bg-gray-950/40 border border-gray-800 rounded-2xl p-4 space-y-4">
    <div className="flex items-start gap-3"><Icon className="w-5 h-5 text-smara-400 mt-0.5"/><div><h3 className="font-semibold text-gray-100">{title}</h3>{desc && <p className="text-xs text-gray-500 mt-1">{desc}</p>}</div></div>
    {children}
  </section>
}

export default function Config() {
  const [config, setConfig] = useState<ConfigData>({})
  const [loading, setLoading] = useState(false)
  const [saving, setSaving] = useState(false)
  const [notice, setNotice] = useState<{type:'ok'|'err', text:string} | null>(null)
  const [mcpDraft, setMcpDraft] = useState<MCPServer>({ name: '', type: 'local', command: '', args: [], url: '', enabled: true })
  const [rawKey, setRawKey] = useState('')
  const [rawValue, setRawValue] = useState('')

  const load = async () => {
    setLoading(true)
    try { setConfig(await fetchJSON<ConfigData>('/api/config') || {}) }
    catch (e) { setNotice({ type: 'err', text: 'Gagal load config: ' + e }) }
    finally { setLoading(false) }
  }
  useEffect(() => { load() }, [])

  const save = async (key: string, value: any) => {
    setSaving(true); setNotice(null)
    try { await fetchJSON('/api/config', { method: 'POST', body: JSON.stringify({ key, value }) }); setNotice({ type: 'ok', text: `${key} tersimpan` }); await load() }
    catch (e) { setNotice({ type: 'err', text: 'Gagal simpan: ' + e }) }
    finally { setSaving(false) }
  }

  const mcpServers = useMemo(() => Array.isArray(config.mcp_servers) ? config.mcp_servers : [], [config])
  const saveMCP = async (servers: MCPServer[]) => save('mcp_servers', JSON.stringify(servers))
  const addMCP = () => {
    if (!mcpDraft.name.trim()) return setNotice({ type: 'err', text: 'Nama MCP wajib diisi' })
    const next = [...mcpServers.filter(s => s.name !== mcpDraft.name.trim()), { ...mcpDraft, name: mcpDraft.name.trim(), args: typeof mcpDraft.args === 'string' ? String(mcpDraft.args).split(/\s+/).filter(Boolean) : mcpDraft.args }]
    saveMCP(next)
    setMcpDraft({ name: '', type: 'local', command: '', args: [], url: '', enabled: true })
  }

  return <div className="flex flex-col h-full overflow-y-auto bg-gray-950">
    <div className="sticky top-0 z-10 bg-gray-950/95 backdrop-blur border-b border-gray-800 p-4 flex items-center justify-between">
      <div className="flex items-center gap-3"><Settings className="w-6 h-6 text-smara-400"/><div><h2 className="text-xl font-semibold">Settings</h2><p className="text-xs text-gray-500">Atur config, MCP, model provider, agent runtime, memory, dan advanced settings.</p></div></div>
      <button onClick={load} className="px-3 py-2 bg-gray-900 hover:bg-gray-800 border border-gray-700 rounded-lg text-sm flex items-center gap-2"><RefreshCw className={`w-4 h-4 ${loading ? 'animate-spin' : ''}`}/> Refresh</button>
    </div>

    <div className="p-4 space-y-4">
      {notice && <div className={`flex items-center gap-2 px-3 py-2 rounded-lg border text-sm ${notice.type === 'ok' ? 'bg-green-900/20 border-green-700/50 text-green-300' : 'bg-red-900/20 border-red-700/50 text-red-300'}`}>{notice.type === 'ok' ? <CheckCircle2 className="w-4 h-4"/> : <AlertCircle className="w-4 h-4"/>}{notice.text}</div>}

      <Section icon={Cpu} title="Model Provider" desc="Pilih provider utama, model default, API key, base URL, OpenRouter, OpenAI, Anthropic, Ollama, atau custom provider.">
        <div className="grid grid-cols-1 lg:grid-cols-2 gap-3">{providerFields.map(f => <FieldInput key={f.key} field={f} config={config} onSave={save} saving={saving}/>)}</div>
      </Section>

      <Section icon={Wrench} title="MCP Servers" desc="Kelola server MCP lokal atau remote. Perubahan tersimpan ke config.yaml; restart Smara Web jika perlu re-connect runtime.">
        <div className="grid grid-cols-1 md:grid-cols-6 gap-2">
          <input value={mcpDraft.name} onChange={e => setMcpDraft({...mcpDraft, name:e.target.value})} placeholder="name" className="bg-gray-900 border border-gray-700 rounded-lg px-3 py-2 text-sm" />
          <select value={mcpDraft.type} onChange={e => setMcpDraft({...mcpDraft, type:e.target.value as any})} className="bg-gray-900 border border-gray-700 rounded-lg px-3 py-2 text-sm"><option value="local">local</option><option value="remote">remote</option></select>
          <input value={mcpDraft.command || ''} onChange={e => setMcpDraft({...mcpDraft, command:e.target.value})} placeholder="command" className="bg-gray-900 border border-gray-700 rounded-lg px-3 py-2 text-sm md:col-span-2" />
          <input value={Array.isArray(mcpDraft.args) ? mcpDraft.args.join(' ') : (mcpDraft.args as any)} onChange={e => setMcpDraft({...mcpDraft, args:e.target.value.split(/\s+/).filter(Boolean)})} placeholder="args" className="bg-gray-900 border border-gray-700 rounded-lg px-3 py-2 text-sm" />
          <button onClick={addMCP} className="bg-smara-700 hover:bg-smara-600 rounded-lg px-3 py-2 text-sm flex items-center justify-center gap-1"><Plus className="w-4 h-4"/> Add</button>
          <input value={mcpDraft.url || ''} onChange={e => setMcpDraft({...mcpDraft, url:e.target.value})} placeholder="remote url (optional)" className="bg-gray-900 border border-gray-700 rounded-lg px-3 py-2 text-sm md:col-span-6" />
        </div>
        <div className="space-y-2">{mcpServers.map(s => <div key={s.name} className="flex items-center justify-between bg-gray-900/60 border border-gray-800 rounded-xl p-3"><div><div className="font-mono text-sm text-smara-300">{s.name} <span className="text-xs text-gray-500">({s.type})</span></div><div className="text-xs text-gray-500 mt-1">{s.type === 'remote' ? s.url : `${s.command || ''} ${(s.args || []).join(' ')}`}</div></div><button onClick={() => saveMCP(mcpServers.filter(x => x.name !== s.name))} className="p-2 text-gray-500 hover:text-red-400"><Trash2 className="w-4 h-4"/></button></div>)}</div>
      </Section>

      <Section icon={SlidersHorizontal} title="Agent Runtime" desc="Batas iterasi, timeout, verbose, dan auto skill capture."><div className="grid grid-cols-1 lg:grid-cols-2 gap-3">{agentFields.map(f => <FieldInput key={f.key} field={f} config={config} onSave={save} saving={saving}/>)}</div></Section>
      <Section icon={Database} title="General Config & Storage"><div className="grid grid-cols-1 lg:grid-cols-2 gap-3">{generalFields.map(f => <FieldInput key={f.key} field={f} config={config} onSave={save} saving={saving}/>)}</div></Section>
      <Section icon={Globe} title="Cloud Memory"><div className="grid grid-cols-1 lg:grid-cols-2 gap-3">{cloudFields.map(f => <FieldInput key={f.key} field={f} config={config} onSave={save} saving={saving}/>)}</div></Section>
      <Section icon={KeyRound} title="Built-in Smara MCP"><div className="grid grid-cols-1 lg:grid-cols-2 gap-3">{smaraMCPFields.map(f => <FieldInput key={f.key} field={f} config={config} onSave={save} saving={saving}/>)}</div></Section>

      <Section icon={Settings} title="Advanced Raw Config" desc="Untuk key lain yang belum dibuatkan field khusus.">
        <div className="flex gap-2"><input value={rawKey} onChange={e => setRawKey(e.target.value)} placeholder="config key" className="w-56 bg-gray-900 border border-gray-700 rounded-lg px-3 py-2 text-sm"/><input value={rawValue} onChange={e => setRawValue(e.target.value)} placeholder="value" className="flex-1 bg-gray-900 border border-gray-700 rounded-lg px-3 py-2 text-sm"/><button onClick={() => rawKey && save(rawKey, rawValue)} className="px-3 py-2 bg-smara-700 hover:bg-smara-600 rounded-lg text-sm flex items-center gap-1"><Save className="w-4 h-4"/> Save</button></div>
        <details className="text-xs text-gray-500"><summary className="cursor-pointer hover:text-gray-300">Lihat semua config raw</summary><pre className="mt-3 overflow-auto bg-gray-900 border border-gray-800 rounded-lg p-3 max-h-96">{JSON.stringify(config, null, 2)}</pre></details>
      </Section>
    </div>
  </div>
}
