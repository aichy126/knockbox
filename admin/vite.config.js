import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import { fileURLToPath } from 'node:url'

const at = (p) => fileURLToPath(new URL(p, import.meta.url))

export default defineConfig({
  // 产物挂在 /admin/ 下，由 Go 的 NoRoute 从 embed 里发出去；见 admin/embed.go。
  base: '/admin/',
  plugins: [vue()],
  resolve: {
    alias: {
      '@': at('./src'),
      // 语料与令牌都从服务端那边拿：文案和颜色只有一份真源。
      '@web': at('../internal/api/web'),
    },
  },
  server: {
    fs: { allow: ['..'] },
    // 本地开发：vite 起在 5173，接口打到本机跑着的 knockbox serve。
    proxy: { '/admin/api': 'http://127.0.0.1:8080', '/api': 'http://127.0.0.1:8080', '/favicon.png': 'http://127.0.0.1:8080' },
  },
  build: { outDir: 'dist', emptyOutDir: true },
  test: { environment: 'jsdom' },
})
