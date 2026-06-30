

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
import '@/assets/styles/button-theme.css'
import {addDynamicRoutes} from '@/router/index.js';

// fontawesome 组件注册,这里全局注册不起作用啊
import { library } from '@fortawesome/fontawesome-svg-core'
import { FontAwesomeIcon } from '@fortawesome/vue-fontawesome'
import { fas } from '@fortawesome/free-solid-svg-icons'
// import {checkPermission} from '@/directives/permission/permission'
import Directives from '@/directives'

// 初始化
import dj_init from '@/init'

library.add(fas)

//icon
import {setup as setupIcon} from './components/SvgIcon/index.js'

const ICON_TOOLTIP_MAP = {
	plus: '新增',
	'plus-circle': '新增',
	edit: '编辑',
	pen: '编辑',
	'pen-to-square': '编辑',
	'pen-to-squar': '编辑',
	'square-pen': '编辑',
	trash: '删除',
	'trash-alt': '删除',
	'trash-can': '删除',
	download: '下载/采集',
	upload: '上传',
	refresh: '刷新',
	'arrows-rotate': '刷新',
	search: '搜索',
	save: '保存',
	close: '关闭',
	'xmark': '关闭',
	'circle-info': '查看详情',
	eye: '查看',
	'open-to-square': '打开/跳转',
	'arrow-up-right-from-square': '打开/跳转',
}

function setButtonTitle(buttonEl) {
	if (!buttonEl || buttonEl.getAttribute('title')) {
		return
	}

	const text = (buttonEl.textContent || '').replace(/\s+/g, ' ').trim()
	if (text) {
		buttonEl.setAttribute('title', text)
		return
	}

	const iconName = buttonEl.querySelector('svg[data-icon]')?.getAttribute('data-icon')
	if (iconName) {
		buttonEl.setAttribute('title', ICON_TOOLTIP_MAP[iconName] || '操作')
	}
}

function applyButtonTitles(root = document) {
	root.querySelectorAll('.ant-btn').forEach((btn) => setButtonTitle(btn))
}

function initGlobalButtonTooltips() {
	applyButtonTitles(document)

	const observer = new MutationObserver((mutations) => {
		mutations.forEach((mutation) => {
			mutation.addedNodes.forEach((node) => {
				if (!(node instanceof HTMLElement)) {
					return
				}
				if (node.matches && node.matches('.ant-btn')) {
					setButtonTitle(node)
				}
				applyButtonTitles(node)
			})
		})
	})

	observer.observe(document.body, {
		childList: true,
		subtree: true,
	})
}

const app = createApp(App)
dj_init()


app.use(Directives)
// app.component('font-awesome-icon', FontAwesomeIcon)
app.component('FontAwesomeIcon', FontAwesomeIcon)
app.use(store)
//解决刷新页面后，出现空白的情况，这里是直接加载动态路由
addDynamicRoutes()
app.use(router)
setupIcon(app)
app.use(Antd)
app.mount('#app')
initGlobalButtonTooltips()
