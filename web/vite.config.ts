import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import path from 'path'

export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: {
      '@': path.resolve(__dirname, './src'),
    },
  },
  build: {
    rollupOptions: {
      output: {
        manualChunks(id) {
          if (!id.includes('node_modules')) return

          if (id.includes('/react/') || id.includes('/react-dom/')) {
            return 'react-vendor'
          }
          if (id.includes('/@radix-ui/')) {
            return 'radix-vendor'
          }
          if (id.includes('/recharts/')) {
            return 'charts-vendor'
          }
          if (
            id.includes('/motion/') ||
            id.includes('/framer-motion/') ||
            id.includes('/motion-dom/') ||
            id.includes('/motion-utils/')
          ) {
            return 'motion-vendor'
          }
          return 'vendor'
        },
      },
    },
  },
  server: {
    port: 5274,
    proxy: {
      '/api': {
        target: 'http://localhost:16823',
        changeOrigin: true,
      },
    },
  },
})
