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
        component: () => import('../views/assets/host/webssh.vue')
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
                name: '自动化执行中心',
                component: () => import('../views/sys/automation/index.vue'),
            },
            {
                path: '/sys/automation/logs',
                name: '任务运行记录',
                component: () => import('../views/sys/automation/logs.vue'),
            },
            {
                path: '/sys/automation/playbooks',
                name: 'Playbook模板',
                component: () => import('../views/sys/playbookTemplate/index.vue'),
            },
            {
                path: '/sys/automation/inventory',
                name: 'Inventory管理',
                component: () => import('../views/sys/automation/inventory.vue'),
            }
        ]
    },
]

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
        `../views/${normalized}`,
        `../views/${normalized}.vue`
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
                component,
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
        let component_url = `../views/${tree.component}.vue`;
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


export function getDynamicalRoutes2(menuList) {
    let indexRoute = staticRouterMap.filter(v => v.path === '/')[0];
    return addTree(indexRoute, menuList)
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
