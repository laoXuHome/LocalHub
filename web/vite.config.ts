import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

export default defineConfig({
  plugins: [react()],
  build: {
    outDir: 'dist',
    emptyOutDir: true,
  },
  server: {
    // 本地开发时把健康检查与业务 API 代理到 Go 服务
    proxy: {
      '/health': { target: 'http://127.0.0.1:11030', changeOrigin: false },
      '/api': { target: 'http://127.0.0.1:11030', changeOrigin: false },
    },
  },
})
