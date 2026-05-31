import { useEffect, useMemo, useState, type ReactNode } from 'react'
import { AlertCircle, CheckCircle2, Cpu, Database, Eye, EyeOff, Globe, Image, KeyRound, Mic, Plus, RefreshCw, Save, Settings, SlidersHorizontal, Trash2, Wrench } from 'lucide-react'
import { fetchJSON } from '../api'
import { getCachedSmaraConfig, loadSmaraConfig, SMARA_CONFIG_LOADED_EVENT } from '../configStore'

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
  { key: 'model', label: 'Default Model', placeholder: 'cx/gpt-5.5 / gpt-4o / llama3.1' },
  { key: 'reasoning_effort', label: 'Reasoning Effort', type: 'select', options: ['', 'low', 'medium', 'high', 'xhigh'], placeholder: 'default' },
  { key: 'custom_provider_name', label: 'Custom Provider Name', placeholder: '9router' },
  { key: 'custom_base_url', label: 'Custom Base URL', placeholder: 'http://localhost:20128/v1' },
  { key: 'custom_api_key', label: 'Custom API Key', type: 'password' },
  { key: 'custom_model', label: 'Custom Model' },
  { key: 'custom_disable_stream', label: 'Disable Custom Streaming', type: 'boolean' },
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

const imageFields: Field[] = [
  { key: 'image_provider', label: 'Image Provider', type: 'select', options: ['custom', 'openai'], placeholder: 'custom' },
  { key: 'image_model', label: 'Image Model', placeholder: 'gpt-image-2 / gemini-2.5-flash-image' },
  { key: 'image_base_url', label: 'Image Base URL', placeholder: 'https://cliproxyapi.example.com/v1' },
  { key: 'image_api_key', label: 'Image API Key', type: 'password' },
  { key: 'image_output_dir', label: 'Image Output Directory', placeholder: '~/.smara/images' },
]

const voiceFields: Field[] = [
  { key: 'voice_provider', label: 'Voice Provider', type: 'select', options: ['elevenlabs', 'browser', 'auto', 'piper'], placeholder: 'elevenlabs' },
  { key: 'voice_api_key', label: 'Voice API Key', type: 'password', placeholder: 'ElevenLabs API key' },
  { key: 'voice_base_url', label: 'Voice Base URL', placeholder: 'https://api.elevenlabs.io' },
  { key: 'voice_character', label: 'Voice ID / Character', placeholder: 'ngvNHfiCrXLPAHcTrZK1' },
  { key: 'voice_model_id', label: 'Voice Model', type: 'select', options: ['eleven_multilingual_v2', 'eleven_turbo_v2_5', 'eleven_flash_v2_5', 'eleven_v3'], placeholder: 'eleven_multilingual_v2' },
  { key: 'voice_language', label: 'Voice Language', placeholder: 'id-ID' },
  { key: 'voice_speed', label: 'Voice Speed', type: 'number', placeholder: '1' },
  { key: 'voice_volume', label: 'Voice Volume', type: 'number', placeholder: '1' },
  { key: 'voice_streaming', label: 'Voice Streaming', type: 'boolean' },
]

const allFields: Field[] = [
  ...providerFields,
  ...imageFields,
  ...voiceFields,
  ...agentFields,
  ...generalFields,
  ...cloudFields,
  ...smaraMCPFields,
]
function getValue(obj: any, path: string) {
  return path.split('.').reduce((acc, key) => acc?.[key], obj)
}

function setValueAtPath(obj: any, path: string, value: any) {
  const keys = path.split('.')
  const next = { ...obj }
  let cur: any = next
  keys.forEach((key, idx) => {
    if (idx === keys.length - 1) cur[key] = value
    else {
      cur[key] = { ...(cur[key] || {}) }
      cur = cur[key]
    }
  })
  return next
}

function normalizeValue(field: Field, value: any) {
  if (field.type === 'number') return Number(value || 0)
  if (field.type === 'boolean') return Boolean(value)
  if (Array.isArray(value)) return value.join(' ')
  return value ?? ''
}

function valuesEqual(a: any, b: any) {
  return JSON.stringify(a ?? '') === JSON.stringify(b ?? '')
}

function FieldInput({ field, config, onChange }: { field: Field; config: ConfigData; onChange: (key: string, value: any) => void }) {
  const value = normalizeValue(field, getValue(config, field.key))
  const [show, setShow] = useState(false)
  const inputClass = "w-full bg-[#26321f] shadow-inner shadow-black/10 rounded-lg px-3 py-2 pr-9 text-sm text-gray-100 placeholder:text-gray-500 focus:outline-none focus:ring-2 focus:ring-smara-300/20"

  return <div className="bg-[#2a3522]/78 rounded-xl p-3 shadow-sm shadow-black/10">
    <label className="block text-xs font-medium text-gray-300 mb-1">{field.label}</label>
    {field.type === 'boolean' ? (
      <button onClick={() => onChange(field.key, !value)} className={`px-3 py-2 rounded-lg text-sm shadow-sm ${value ? 'bg-smara-500/75 text-[#111807]' : 'bg-[#202a1a] text-gray-300'}`}>{value ? 'Enabled' : 'Disabled'}</button>
    ) : field.type === 'select' ? (
      <select value={value} onChange={e => onChange(field.key, e.target.value)} className="w-full bg-[#26321f] rounded-lg px-3 py-2 text-sm text-gray-100 focus:outline-none focus:ring-2 focus:ring-smara-300/20">
        {(field.options || []).map(o => <option key={o || 'default'} value={o}>{o || field.placeholder || 'default'}</option>)}
      </select>
    ) : (
      <div className="relative">
        <input type={field.type === 'password' && !show ? 'password' : field.type === 'number' ? 'number' : 'text'} value={value} onChange={e => onChange(field.key, field.type === 'number' ? Number(e.target.value) : e.target.value)} placeholder={field.placeholder} className={inputClass} />
        {field.type === 'password' && <button type="button" onClick={() => setShow(!show)} className="absolute right-2 top-2 text-gray-400 hover:text-gray-200">{show ? <EyeOff className="w-4 h-4"/> : <Eye className="w-4 h-4"/>}</button>}
      </div>
    )}
  </div>
}

function Section({ icon, title, desc, children }: { icon: any; title: string; desc?: string; children: ReactNode }) {
  const Icon = icon
  return <section className="bg-[#24301d]/82 rounded-2xl p-4 space-y-4 shadow-lg shadow-black/10">
    <div className="flex items-start gap-3"><Icon className="w-5 h-5 text-smara-400 mt-0.5"/><div><h3 className="font-semibold text-gray-100">{title}</h3>{desc && <p className="text-xs text-gray-400 mt-1">{desc}</p>}</div></div>
    {children}
  </section>
}
export default function Config() {
  const [config, setConfig] = useState<ConfigData>(() => getCachedSmaraConfig() as ConfigData)
  const [draftConfig, setDraftConfig] = useState<ConfigData>(() => getCachedSmaraConfig() as ConfigData)
  const [loading, setLoading] = useState(false)
  const [saving, setSaving] = useState(false)
  const [notice, setNotice] = useState<{type:'ok'|'err', text:string} | null>(null)
  const [mcpDraft, setMcpDraft] = useState<MCPServer>({ name: '', type: 'local', command: '', args: [], url: '', enabled: true })
  const [rawKey, setRawKey] = useState('')
  const [rawValue, setRawValue] = useState('')

  const load = async () => {
    setLoading(true)
    try {
      const next = await loadSmaraConfig(true) as ConfigData
      setConfig(next)
      setDraftConfig(next)
      setNotice(null)
    }
    catch (e) { setNotice({ type: 'err', text: 'Gagal load config: ' + e }) }
    finally { setLoading(false) }
  }
  useEffect(() => { load() }, [])

  const updateDraft = (key: string, value: any) => setDraftConfig(prev => setValueAtPath(prev, key, value))
  const changedFields = allFields.filter(f => !valuesEqual(normalizeValue(f, getValue(draftConfig, f.key)), normalizeValue(f, getValue(config, f.key))))
  const dirty = changedFields.length > 0 || (!!rawKey && rawValue !== '')

  useEffect(() => {
    const handler = (event: Event) => {
      const next = (event as CustomEvent<ConfigData>).detail || (getCachedSmaraConfig() as ConfigData)
      setConfig(next)
      if (!dirty) setDraftConfig(next)
    }
    window.addEventListener(SMARA_CONFIG_LOADED_EVENT, handler)
    return () => window.removeEventListener(SMARA_CONFIG_LOADED_EVENT, handler)
  }, [dirty])

  const saveKey = async (key: string, value: any) => fetchJSON('/api/config', { method: 'POST', body: JSON.stringify({ key, value }) })
  const saveAll = async () => {
    if (!dirty || saving) return
    setSaving(true); setNotice(null)
    try {
      for (const f of changedFields) await saveKey(f.key, getValue(draftConfig, f.key))
      if (rawKey && rawValue !== '') { await saveKey(rawKey, rawValue); setRawKey(''); setRawValue('') }
      setNotice({ type: 'ok', text: `${changedFields.length + (rawKey && rawValue !== '' ? 1 : 0)} perubahan tersimpan` })
      const next = await loadSmaraConfig(true) as ConfigData
      setConfig(next)
      setDraftConfig(next)
    } catch (e) { setNotice({ type: 'err', text: 'Gagal simpan: ' + e }) }
    finally { setSaving(false) }
  }

  const mcpServers = useMemo(() => Array.isArray(draftConfig.mcp_servers) ? draftConfig.mcp_servers : [], [draftConfig])
  const setMCP = (servers: MCPServer[]) => updateDraft('mcp_servers', servers)
  const addMCP = () => {
    if (!mcpDraft.name.trim()) return setNotice({ type: 'err', text: 'Nama MCP wajib diisi' })
    const next = [...mcpServers.filter(s => s.name !== mcpDraft.name.trim()), { ...mcpDraft, name: mcpDraft.name.trim(), args: typeof mcpDraft.args === 'string' ? String(mcpDraft.args).split(/\s+/).filter(Boolean) : mcpDraft.args }]
    setMCP(next)
    setMcpDraft({ name: '', type: 'local', command: '', args: [], url: '', enabled: true })
  }

  return <div className="flex flex-col h-full overflow-y-auto bg-[#1d2718]">
    <div className="sticky top-0 z-10 bg-[#1d2718]/95 backdrop-blur p-4 flex items-center justify-between">
      <div className="flex items-center gap-3"><Settings className="w-6 h-6 text-smara-400"/><div><h2 className="text-xl font-semibold">Settings</h2><p className="text-xs text-gray-400">Atur config, MCP, model provider, agent runtime, memory, dan advanced settings.</p></div></div>
      <div className="flex items-center gap-2">
        {dirty && <span className="text-xs text-amber-200">Unsaved changes</span>}
        <button onClick={load} disabled={loading || saving} className="px-3 py-2 bg-[#2a3522] hover:bg-[#303d27] disabled:opacity-50 rounded-lg text-sm flex items-center gap-2"><RefreshCw className={`w-4 h-4 ${loading ? 'animate-spin' : ''}`}/> Refresh</button>
        <button onClick={saveAll} disabled={!dirty || saving} className="px-4 py-2 bg-smara-500 hover:bg-smara-400 text-[#111807] disabled:bg-[#202a1a] disabled:text-gray-500 rounded-lg text-sm flex items-center gap-2 shadow-sm"><Save className="w-4 h-4"/> {saving ? 'Saving...' : 'Save'}</button>
      </div>
    </div>

    <div className="p-4 space-y-4">
      {notice && <div className={`flex items-center gap-2 px-3 py-2 rounded-lg text-sm shadow-sm ${notice.type === 'ok' ? 'bg-smara-700/35 text-smara-200' : 'bg-red-950/35 text-red-200'}`}>{notice.type === 'ok' ? <CheckCircle2 className="w-4 h-4"/> : <AlertCircle className="w-4 h-4"/>}{notice.text}</div>}

      <Section icon={Cpu} title="Model Provider" desc="Pilih provider utama, model default, API key, base URL, OpenRouter, OpenAI, Anthropic, Ollama, atau custom provider.">
        <div className="grid grid-cols-1 lg:grid-cols-2 gap-3">{providerFields.map(f => <FieldInput key={f.key} field={f} config={draftConfig} onChange={updateDraft}/>)}</div>
      </Section>

      <Section icon={Image} title="Image Generation" desc="Provider, model, API key, base URL, dan direktori output untuk image flow.">
        <div className="grid grid-cols-1 lg:grid-cols-2 gap-3">{imageFields.map(f => <FieldInput key={f.key} field={f} config={draftConfig} onChange={updateDraft}/>)}</div>
      </Section>

      <Section icon={Mic} title="Voice AI" desc="Atur provider suara, Voice ID, model ElevenLabs, bahasa, speed, dan volume untuk mode Voice di chat.">
        <div className="grid grid-cols-1 lg:grid-cols-2 gap-3">{voiceFields.map(f => <FieldInput key={f.key} field={f} config={draftConfig} onChange={updateDraft}/>)}</div>
      </Section>

      <Section icon={Wrench} title="MCP Servers" desc="Kelola server MCP lokal atau remote. Klik Save di atas untuk menyimpan semua perubahan sekaligus.">
        <div className="grid grid-cols-1 md:grid-cols-6 gap-2">
          <input value={mcpDraft.name} onChange={e => setMcpDraft({...mcpDraft, name:e.target.value})} placeholder="name" className="bg-[#26321f] rounded-lg px-3 py-2 text-sm text-gray-100 placeholder:text-gray-500 focus:outline-none focus:ring-2 focus:ring-smara-300/20" />
          <select value={mcpDraft.type} onChange={e => setMcpDraft({...mcpDraft, type:e.target.value as any})} className="bg-[#26321f] rounded-lg px-3 py-2 text-sm text-gray-100 placeholder:text-gray-500 focus:outline-none focus:ring-2 focus:ring-smara-300/20"><option value="local">local</option><option value="remote">remote</option></select>
          <input value={mcpDraft.command || ''} onChange={e => setMcpDraft({...mcpDraft, command:e.target.value})} placeholder="command" className="bg-[#26321f] rounded-lg px-3 py-2 text-sm text-gray-100 placeholder:text-gray-500 focus:outline-none focus:ring-2 focus:ring-smara-300/20 md:col-span-2" />
          <input value={Array.isArray(mcpDraft.args) ? mcpDraft.args.join(' ') : (mcpDraft.args as any)} onChange={e => setMcpDraft({...mcpDraft, args:e.target.value.split(/\s+/).filter(Boolean)})} placeholder="args" className="bg-[#26321f] rounded-lg px-3 py-2 text-sm text-gray-100 placeholder:text-gray-500 focus:outline-none focus:ring-2 focus:ring-smara-300/20" />
          <button onClick={addMCP} className="bg-smara-500 hover:bg-smara-400 text-[#111807] rounded-lg px-3 py-2 text-sm flex items-center justify-center gap-1"><Plus className="w-4 h-4"/> Add</button>
          <input value={mcpDraft.url || ''} onChange={e => setMcpDraft({...mcpDraft, url:e.target.value})} placeholder="remote url (optional)" className="bg-[#26321f] rounded-lg px-3 py-2 text-sm text-gray-100 placeholder:text-gray-500 focus:outline-none focus:ring-2 focus:ring-smara-300/20 md:col-span-6" />
        </div>
        <div className="space-y-2">{mcpServers.map(s => <div key={s.name} className="flex items-center justify-between bg-[#2a3522]/78 shadow-sm shadow-black/10 rounded-xl p-3"><div><div className="font-mono text-sm text-smara-300">{s.name} <span className="text-xs text-gray-400">({s.type})</span></div><div className="text-xs text-gray-400 mt-1">{s.type === 'remote' ? s.url : `${s.command || ''} ${(s.args || []).join(' ')}`}</div></div><button onClick={() => setMCP(mcpServers.filter(x => x.name !== s.name))} className="p-2 text-gray-400 hover:text-red-300"><Trash2 className="w-4 h-4"/></button></div>)}</div>
      </Section>

      <Section icon={SlidersHorizontal} title="Agent Runtime" desc="Batas iterasi, timeout, verbose, dan auto skill capture."><div className="grid grid-cols-1 lg:grid-cols-2 gap-3">{agentFields.map(f => <FieldInput key={f.key} field={f} config={draftConfig} onChange={updateDraft}/>)}</div></Section>
      <Section icon={Database} title="General Config & Storage"><div className="grid grid-cols-1 lg:grid-cols-2 gap-3">{generalFields.map(f => <FieldInput key={f.key} field={f} config={draftConfig} onChange={updateDraft}/>)}</div></Section>
      <Section icon={Globe} title="Cloud Memory"><div className="grid grid-cols-1 lg:grid-cols-2 gap-3">{cloudFields.map(f => <FieldInput key={f.key} field={f} config={draftConfig} onChange={updateDraft}/>)}</div></Section>
      <Section icon={KeyRound} title="Built-in Smara MCP"><div className="grid grid-cols-1 lg:grid-cols-2 gap-3">{smaraMCPFields.map(f => <FieldInput key={f.key} field={f} config={draftConfig} onChange={updateDraft}/>)}</div></Section>

      <Section icon={Settings} title="Advanced Raw Config" desc="Untuk key lain yang belum dibuatkan field khusus. Akan ikut tersimpan saat klik tombol Save di atas.">
        <div className="flex gap-2"><input value={rawKey} onChange={e => setRawKey(e.target.value)} placeholder="config key" className="w-56 bg-[#26321f] rounded-lg px-3 py-2 text-sm text-gray-100 placeholder:text-gray-500 focus:outline-none focus:ring-2 focus:ring-smara-300/20"/><input value={rawValue} onChange={e => setRawValue(e.target.value)} placeholder="value" className="flex-1 bg-[#26321f] rounded-lg px-3 py-2 text-sm text-gray-100 placeholder:text-gray-500 focus:outline-none focus:ring-2 focus:ring-smara-300/20"/></div>
        <details className="text-xs text-gray-400"><summary className="cursor-pointer hover:text-gray-200">Lihat semua config raw</summary><pre className="mt-3 overflow-auto bg-[#26321f] rounded-lg p-3 max-h-96 text-gray-200 shadow-inner shadow-black/10">{JSON.stringify(draftConfig, null, 2)}</pre></details>
      </Section>
    </div>
  </div>
}
