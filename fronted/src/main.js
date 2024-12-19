

import { createApp } from 'vue'
import App from './App.vue'
import router from './router'
// element
import ElementPlus from 'element-plus'
import 'element-plus/dist/index.css'

import '@/assets/styles/border.css'
import '@/assets/styles/reset.css'

const app = createApp(App).use(ElementPlus)
app.use(router)
app.mount('#app')
