import { createRouter, createWebHistory } from 'vue-router'
import { getMenuList, getToken } from '@/api/user/index.js'
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
                component: () => import('../views/sys/user/index.vue'),
            }
        ]
    },
]

export function getDynamicalRoutes(menuList) {
    let indexRoute = staticRouterMap.filter(v => v.path === '/')[0];
    // indexRoute.children = [];
    if (menuList) {
        menuList.forEach(item => {
            let component_url_parent = `../views/${item.component}.vue`;
            let first_item = {
                path: item.path,
                name: item.name,
                component: modules[`${component_url_parent}`],
                children: []
            }
            item.children.forEach(item2 => {
                let component_url = `../views/${item2.component}.vue`;
                first_item.children.push({
                    path: item2.path,
                    name: item2.name,
                    component: modules[`${component_url}`]
                })
            })
            indexRoute.children.push(first_item);
        })
        return indexRoute;
    }
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
                component: modules[`${component_url}`],
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
        if (menuList.length >= 1) {
            let dyroutes = getDynamicalRoutes(menuList);
            let dyroutes2 = getDynamicalRoutes2(menuList);
            console.log(dyroutes);
            console.log("==========动态路由2=============")
            console.log(dyroutes2)
            router.addRoute(dyroutes);
        }
    } else {
        router.replace('/login');
    }
}


//路由守卫
router.beforeEach((to, from, next) => {
    if (to.path == '/login') {
        next()
    } else {
        //   检查是否已登录
        let token = getToken()
        if (token) {
            next();
        } else {
            router.replace('/login');
        }
    }
}) 
