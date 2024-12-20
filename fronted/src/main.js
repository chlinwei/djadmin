

import { createApp } from 'vue'
// import "virtual:svg-icons-register"
import App from './App.vue'
import router from './router'
// ant design
import Antd from 'ant-design-vue'
import 'ant-design-vue/dist/reset.css'

import '@/assets/styles/border.css'
import '@/assets/styles/reset.css'


//icon
import {setup as setupIcon} from './components/SvgIcon/index.js'

const app = createApp(App)
app.use(router)
setupIcon(app)
app.use(Antd)
app.mount('#app')
