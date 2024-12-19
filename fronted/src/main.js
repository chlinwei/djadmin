

import { createApp } from 'vue'
// import "virtual:svg-icons-register"
import App from './App.vue'
import router from './router'
// element
import ElementPlus from 'element-plus'
import 'element-plus/dist/index.css'

import '@/assets/styles/border.css'
import '@/assets/styles/reset.css'


//icon
import {setup as setupIcon} from './components/SvgIcon/index.js'

const app = createApp(App)
app.use(router)
setupIcon(app)
app.use(ElementPlus)
app.mount('#app')
