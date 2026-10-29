import { fileURLToPath, URL } from 'node:url'

import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import vueDevTools from 'vite-plugin-vue-devtools'
import { createSvgIconsPlugin } from 'vite-plugin-svg-icons'
import path from 'path'

export default defineConfig({
  server: {
    host: '0.0.0.0',
  },
  resolve: {
    alias: {
      '@': fileURLToPath(new URL('./src', import.meta.url))
    },
  },
  plugins: [
    vue(),
    vueDevTools(),
    createSvgIconsPlugin({
        iconDirs: [
            path.resolve(__dirname,"src/assets/icons/svg")
        ],
        symbolId: "icon-[dir]-[name]"
    })
  ]
 
})
