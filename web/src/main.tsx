import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import App from './App'
import './index.css'

// Escape hatch: visit /?reset to wipe all Smara-owned localStorage keys.
// Useful when the page is stuck on a blank screen because the previous
// session blew the quota and React can't mount.
if (typeof window !== 'undefined' && window.location.search.includes('reset')) {
  try {
    Object.keys(localStorage).forEach(k => {
      if (k.startsWith('smara_')) localStorage.removeItem(k)
    })
  } catch { /* ignore */ }
  // Drop the query so a refresh doesn't clear again.
  window.history.replaceState(null, '', window.location.pathname)
}

// Last-resort guard: if rendering itself throws (e.g. quota error during
// initial localStorage read), show a minimal recovery UI instead of a
// black void. We don't surface React error boundaries here because the
// crash can happen *before* the boundary mounts.
window.addEventListener('error', (ev) => {
  const msg = String(ev.error?.message || ev.message || '')
  if (msg.includes('quota') || msg.includes('QuotaExceeded')) {
    try {
      Object.keys(localStorage).forEach(k => {
        if (k.startsWith('smara_')) localStorage.removeItem(k)
      })
    } catch { /* ignore */ }
  }
})

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <App />
  </StrictMode>,
)
