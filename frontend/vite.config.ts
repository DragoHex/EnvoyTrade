/// <reference types="vitest/config" />
import { defineConfig } from 'vite'
import solid from 'vite-plugin-solid'

export default defineConfig({
  plugins: [solid()],
  server: {
    allowedHosts: true,
    proxy: {
      '/api': 'http://localhost:8080',
      '/broker-callback': 'http://localhost:8080',
    },
  },
  test: {
    environment: 'jsdom',
    globals: true,
    setupFiles: ['./src/test-setup.ts'],
  },
  resolve: {
    conditions: ['development', 'browser'],
  },
})
