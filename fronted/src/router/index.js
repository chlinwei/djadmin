import { createRouter, createWebHistory } from 'vue-router'
import { getToken } from '@/api/user/index.js'
import { getMenuList } from '@/api/menu/index.js'
export const modules = import.meta.glob("../views/**");

export const staticRouterMap = [
    {
        path: '/login',
        name: 'login',
        component: () => import('../views/Login.vue')
    },
    {
        path: '/login2',
        name: 'login2',
        component: () => import('../views/Login2.vue')
    },
    {
        path: '/test',
        name: 'test',
        component: () => import('../layout/test.vue')
    },
    {
        path: '/assets/hosts/webssh',
        name: 'webssh-page',
        component: () => import('../views/assets/host/webssh/index.vue')
    },
    {
        path: '/',
        name: 'dashbaord',
        component: () => import('../layout/index.vue'),
        redirect: '/index',
        children: [
            {
                path: '/index',
                name: '首页',
                component: () => import('../views/index/index.vue'),
            },
            {
                path: '/sys/userCenter',
                name: '个人中心',
                component: () => import('../views/userCenter/index.vue')
            },
            {
                path: '/sys/scheduler',
                name: '定时任务中心',
                component: () => import('../views/sys/scheduler/index.vue'),
            },
            {
                path: '/sys/automation',
                name: '自动化任务',
                component: () => import('../views/automation/automationtask/index.vue'),
            },
            {
                path: '/sys/inspection',
                name: '巡检中心',
                component: () => import('../views/inspection/index.vue'),
            },
            {
                path: '/sys/automation/logs',
                name: '运行记录中心',
                component: () => import('../views/automation/logs/index.vue'),
            },
            {
                path: '/sys/automation/templates',
                name: '模板',
                component: () => import('../views/automation/templates/index.vue'),
            },
            {
                path: '/sys/automation/inventory',
                name: 'Inventory管理',
                component: () => import('../views/automation/inventory/index.vue'),
            },
            {
                path: '/sys/automation/workflow',
                name: 'Workflow编排',
                component: () => import('../views/automation/workflow/list/index.vue'),
            },
            {
                path: '/sys/automation/workflow/create',
                name: 'Workflow创建',
                component: () => import('../views/automation/workflow/create.vue'),
            },
            {
                path: '/sys/automation/workflow/editor',
                name: 'Workflow编排编辑',
                component: () => import('../views/automation/workflow/editor/index.vue'),
            },
            {
                path: '/sys/automation/workflow/run',
                name: 'Workflow运行状态',
                component: () => import('../views/automation/workflow/run/index.vue'),
            },
            {
                path: '/monitor',
                name: '智能监控',
                component: () => import('../views/monitor/index.vue'),
            },
            {
                path: '/monitor/alert-rules',
                name: '告警规则',
                component: () => import('../views/monitor/alert-rules/index.vue'),
            },
            {
                path: '/monitor/alerts',
                name: '告警',
                component: () => import('../views/monitor/alerts/index.vue'),
            },
            {
                path: '/monitor/explore',
                name: 'Explore',
                component: () => import('../views/monitor/explore/index.vue'),
            },
            {
                path: '/monitor/media',
                name: '媒介',
                component: () => import('../views/monitor/media/index.vue'),
            },
            {
                path: '/monitor/alert-routes',
                name: '告警路由',
                component: () => import('../views/monitor/alert-routes/index.vue'),
            },
            {
                path: '/monitor/log-storage',
                name: '日志存储',
                component: () => import('../views/monitor/log-storage/index.vue'),
            },
            {
                path: '/monitor/log-parsers',
                name: '日志处理规则',
                component: () => import('../views/monitor/log-parsers/index.vue'),
            },
            {
                path: '/monitor/log-retention',
                name: '日志保留档位',
                component: () => import('../views/monitor/log-retention/index.vue'),
            },
            {
                path: '/assets/hosts',
                name: '主机管理',
                component: () => import('../views/assets/host/index.vue'),
                alias: ['/assets/host', '/assets/hosts/index', '/assets/host/index', '/assets'],
            },
            {
                path: '/assets/projects',
                name: '项目管理',
                component: () => import('../views/assets/projects/index.vue'),
                alias: ['/assets/projects/index'],
            },
            {
                path: '/assets/applications',
                name: '应用管理',
                component: () => import('../views/assets/application/index.vue'),
                alias: ['/assets/application', '/assets/applications/index', '/assets/application/index'],
            },
            {
                path: '/assets/environments',
                name: '环境管理',
                component: () => import('../views/assets/environments/index.vue'),
                alias: ['/assets/environments/index'],
            },
            {
                path: '/assets/credentials',
                name: '凭据管理',
                component: () => import('../views/assets/credential/index.vue'),
                alias: ['/assets/credential', '/assets/credentials/index', '/assets/credential/index'],
            },
            {
                path: '/assets/service-tree',
                name: '服务树管理',
                component: () => import('../views/assets/service-tree/index.vue'),
                alias: ['/assets/service-tree/index'],
            },
            {
                path: '/assets/hosts/detail/:id',
                name: '主机详情页',
                component: () => import('../views/assets/host/detail/index.vue'),
            },
            {
                path: '/assets/hosts/agent-runtime/:id',
                name: '主机 Agent 运行状态页',
                component: () => import('../views/assets/host/agent-runtime/index.vue'),
            },
        ]
    },
]

// KeepAlive 的 include 按“组件名”匹配，而本项目几乎所有页面都是 index.vue 且未声明 name，
// Vue 会把它们的名字统统推断成 index。同名会让“关闭一个标签”连带剪掉其他同名页面的缓存，
// 甚至卸载掉正在挂载的实例，表现为 unmount 阶段读 null 的 type/subTree/parentNode。
// 按路由记录路径盖一个唯一名字，使缓存项与 include 一一对应。
export function keepAliveNameOf(path) {
    const normalized = String(path || '').replace(/^\/+/, '').replace(/[^A-Za-z0-9]+/g, '_')
    return `view_${normalized || 'root'}`
}

function withKeepAliveName(loader, path) {
    if (typeof loader !== 'function') {
        return loader
    }
    const name = keepAliveNameOf(path)
    return async () => {
        const loaded = await loader()
        const component = loaded?.default ?? loaded
        if (component && typeof component === 'object') {
            component.name = name
        }
        return component
    }
}

function stampKeepAliveNames(routes) {
    routes.forEach((route) => {
        if (route.component) {
            route.component = withKeepAliveName(route.component, route.path)
        }
        if (Array.isArray(route.children)) {
            stampKeepAliveNames(route.children)
        }
    })
    return routes
}

stampKeepAliveNames(staticRouterMap)

function resolveMenuComponent(componentPath) {
    if (!componentPath) {
        return null
    }

    const normalized = String(componentPath)
        .trim()
        .replace(/^\/+/, '')
        .replace(/\.vue$/i, '')

    const candidateKeys = [
        `../views/${normalized}.vue`,
        `../views/${normalized}/index.vue`,
        `../views/${normalized}`,
        `../views/${normalized}.vue`
            .replace('sys/audit/', 'audit/')
            .replace('/applications/', '/application/')
            .replace('/credentials/', '/credential/')
            .replace('/usercenter/', '/userCenter/'),
        `../views/${normalized}/index.vue`
            .replace('sys/audit/', 'audit/')
            .replace('/applications/', '/application/')
            .replace('/credentials/', '/credential/')
            .replace('/usercenter/', '/userCenter/'),
    ]

    for (const key of candidateKeys) {
        if (modules[key]) {
            return modules[key]
        }
    }

    return null
}

function collectLeafRoutes(menuList, collector = []) {
    if (!Array.isArray(menuList)) {
        return collector
    }

    menuList.forEach((item) => {
        const children = Array.isArray(item?.children) ? item.children : []
        const component = resolveMenuComponent(item?.component)

        if (item?.path && component) {
            collector.push({
                path: item.path,
                name: item.name,
                component: withKeepAliveName(component, item.path),
            })
        }

        if (children.length) {
            collectLeafRoutes(children, collector)
        }
    })

    return collector
}

export function getDynamicalRoutes(menuList) {
    return collectLeafRoutes(menuList, [])
}

function addTree(indexRoute, treeList) {
    treeList.forEach(tree => {
        const normalizedComponent = String(tree.component || '')
            .replace('sys/audit/', 'audit/')
        let component_url = `../views/${normalizedComponent}.vue`;
        let treeRoute = {}
        if (tree.children) {
            treeRoute = {
                path: tree.path,
                name: tree.name,
                component: modules[`${component_url}`],
                children: []
            }

        } else {
            treeRoute = {
                path: tree.path,
                name: tree.name,
                component: modules[`${component_url}`]
            }
        }
        indexRoute.children.push(treeRoute)
        if (tree.children) {
            addTree(treeRoute, tree.children)
        }
    }
    )
    return indexRoute;

}

const router = createRouter({
    history: createWebHistory(import.meta.env.BASE_URL),
    routes: staticRouterMap
})

export default router


export function addDynamicRoutes() {
    //检查用户是否登录
    //获取用户权限列表
    //获取动态路由
    //添加动态路由
    if (getToken()) {
        //已经登录
        let menuList = getMenuList();
        if (Array.isArray(menuList) && menuList.length >= 1) {
            const dynamicChildren = getDynamicalRoutes(menuList)
            dynamicChildren.forEach((routeItem) => {
                const exists = router.getRoutes().some((r) => r.path === routeItem.path)
                if (!exists) {
                    router.addRoute('dashbaord', routeItem)
                }
            })
        }
    }
}


//路由守卫
router.beforeEach((to, from, next) => {
    const token = getToken()

    if (to.path === '/login') {
        if (token) {
            next('/index')
        } else if (String(to.query?.redirect || '').startsWith('/login')) {
            // 并发鉴权失败可能留下嵌套登录 redirect，登录页不允许再回跳到自身。
            next({ path: '/login', replace: true })
        } else {
            next()
        }
        return
    }

    if (token) {
        next()
    } else {
        next({
            path: '/login',
            query: { redirect: to.fullPath }
        })
    }
}) 
