

import { createApp } from 'vue'
// import "virtual:svg-icons-register"
import App from './App.vue'
import router from './router'
// ant design
import Antd from 'ant-design-vue'
import 'ant-design-vue/dist/reset.css'
import store from './store'
import '@/assets/styles/border.css'
import '@/assets/styles/reset.css'
import {addDynamicRoutes} from '@/router/index.js';

// fontawesome 组件注册,这里全局注册不起作用啊
import { library } from '@fortawesome/fontawesome-svg-core'
import { FontAwesomeIcon } from '@fortawesome/vue-fontawesome'
import { faEdit,faTrash,faUserSecret,faTree } from '@fortawesome/free-solid-svg-icons'

library.add(faTrash)
library.add(faEdit)
library.add(faTree)


//icon
import {setup as setupIcon} from './components/SvgIcon/index.js'

const app = createApp(App)
app.component('font-awesome-icon', FontAwesomeIcon)
// app.component('FontAwesomeIcon', FontAwesomeIcon)
app.use(store)
//解决刷新页面后，出现空白的情况，这里是直接加载动态路由
addDynamicRoutes()
app.use(router)
setupIcon(app)
app.use(Antd)
app.mount('#app')
