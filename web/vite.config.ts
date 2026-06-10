import { createLogger, defineConfig, type LogErrorOptions, type Logger, type ProxyOptions } from 'vite'
import react from '@vitejs/plugin-react'

const defaultLogger = createLogger()

function isBackendProxyStartupNoise(message: string, error?: LogErrorOptions['error']) {
  const code = (error as { code?: string } | null | undefined)?.code
  const backendConnectionError =
    code === 'ECONNREFUSED' ||
    code === 'ECONNRESET' ||
    message.includes('ECONNREFUSED') ||
    message.includes('ECONNRESET')

  return backendConnectionError && (
    message.includes('http proxy error: /api') ||
    message.includes('ws proxy error:')
  )
}

const devLogger: Logger = {
  ...defaultLogger,
  error(message, options) {
    if (isBackendProxyStartupNoise(message, options?.error)) return
    defaultLogger.error(message, options)
  },
}

function getBackendTarget() {
  const raw = process.env.SMARA_BACKEND_URL || process.env.VITE_SMARA_BACKEND_URL || process.env.BACKEND_URL || 'http://127.0.0.1:8080'
  return raw.replace(/\/$/, '')
}

const backendTarget = getBackendTarget()
const backendWsTarget = backendTarget.replace(/^http:/, 'ws:').replace(/^https:/, 'wss:')

function quietBackendProxyErrors(proxy: Parameters<NonNullable<ProxyOptions['configure']>>[0]) {
  proxy.on('error', (err: Error & { code?: string }, _req, res) => {
    if (err.code !== 'ECONNREFUSED' && err.code !== 'ECONNRESET') {
      console.warn('[vite] proxy error:', err.message)
      return
    }

    const response = res as {
      headersSent?: boolean
      writeHead?: (status: number, headers: Record<string, string>) => void
      end?: (body?: string) => void
    } | undefined

    if (response?.writeHead && response?.end) {
      if (!response.headersSent) response.writeHead(503, { 'Content-Type': 'application/json' })
      response.end(JSON.stringify({ error: `backend unavailable (${backendTarget})` }))
    }
  })
}

export default defineConfig({
  customLogger: devLogger,
  plugins: [react()],
  build: {
    outDir: 'dist',
    emptyOutDir: true,
  },
  server: {
    host: '127.0.0.1',
    port: 5173,
    strictPort: true,
    proxy: {
      '/api': {
        target: backendTarget,
        changeOrigin: true,
        configure: quietBackendProxyErrors,
      },
      '/ws': {
        target: backendWsTarget,
        ws: true,
        changeOrigin: true,
        configure: quietBackendProxyErrors,
      },
    },
  },
})
