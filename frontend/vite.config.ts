import { resolve } from 'node:path'
import react from '@vitejs/plugin-react'
import { defineConfig } from 'vitest/config'

export default defineConfig({
  plugins: [react()],
  resolve: { alias: { '@': resolve(import.meta.dirname, 'src') } },
  server: {
    proxy: {
      '^/o/[^/]+/(login|callback)(?:\\?|$)': {
        target: process.env.API_PROXY_TARGET ?? 'http://localhost:8080',
        changeOrigin: true,
      },
      '/api/v1': {
        target: process.env.API_PROXY_TARGET ?? 'http://localhost:8080',
        changeOrigin: true,
        ws: true,
      },
    },
  },
  test: {
    server: { deps: { inline: ['@gravity-ui/uikit'] } },
    environment: 'jsdom',
    setupFiles: ['./src/app/test/setup-tests.ts'],
    clearMocks: true,
    restoreMocks: true,
  },
})
