import { fetchJSON } from './api'

export type SmaraConfigData = Record<string, unknown>

export const SMARA_CONFIG_STORAGE_KEY = 'smara_config'
export const SMARA_CONFIG_LOADED_EVENT = 'smara:config-loaded'
export const SMARA_CONFIG_ERROR_EVENT = 'smara:config-error'

let cachedConfig: SmaraConfigData | null = null
let pendingLoad: Promise<SmaraConfigData> | null = null

function publishLoaded(config: SmaraConfigData) {
  try { localStorage.setItem(SMARA_CONFIG_STORAGE_KEY, JSON.stringify(config)) } catch { /* ignore */ }
  window.dispatchEvent(new CustomEvent(SMARA_CONFIG_LOADED_EVENT, { detail: config }))
}

export function getCachedSmaraConfig(): SmaraConfigData {
  if (cachedConfig) return cachedConfig
  try {
    const raw = localStorage.getItem(SMARA_CONFIG_STORAGE_KEY)
    if (raw) {
      cachedConfig = JSON.parse(raw) as SmaraConfigData
      return cachedConfig
    }
  } catch { /* ignore */ }
  return {}
}

export async function loadSmaraConfig(force = false): Promise<SmaraConfigData> {
  if (!force && cachedConfig) return cachedConfig
  if (!force && pendingLoad) return pendingLoad
  pendingLoad = fetchJSON<SmaraConfigData>('/api/config')
    .then(config => {
      cachedConfig = config || {}
      publishLoaded(cachedConfig)
      return cachedConfig
    })
    .catch(err => {
      window.dispatchEvent(new CustomEvent(SMARA_CONFIG_ERROR_EVENT, { detail: err }))
      throw err
    })
    .finally(() => { pendingLoad = null })
  return pendingLoad
}

export function setCachedSmaraConfig(config: SmaraConfigData) {
  cachedConfig = config || {}
  publishLoaded(cachedConfig)
}
