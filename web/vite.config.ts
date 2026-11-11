/// <reference types="vitest/config" />
import react from '@vitejs/plugin-react'
import { defineConfig } from 'vite'

const backend = process.env.AGENT_DESK_BACKEND ?? 'http://127.0.0.1:7317'

export default defineConfig({
  plugins: [react()],
  server: {
    host: 'localhost',
    port: 5173,
    strictPort: true,
    proxy: {
      // changeOrigin makes Host match the backend's own address, which its
      // DNS rebinding check requires. Origin stays the dev server's.
      '/api': { target: backend, ws: true, changeOrigin: true },
    },
  },
  test: {
    include: ['src/**/*.test.ts', 'src/**/*.test.tsx'],
  },
})
