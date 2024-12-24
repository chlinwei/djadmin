import { createRouter, createWebHistory } from 'vue-router'
import {getMenuList,getToken} from '@/api/user/index.js'
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
//   {
//     path: '/userCenter',
//     name: '个人中心',
//     component: () => import('../views/userCenter/index.vue')
//   },
  {
    path: '/index',
    name: '主页',
    alias: ['/'],
    component: () => import('../layout/index.vue'),
    children: [
        // {
        //     path: '/sys/userCenter',
        //     name: '个人中心',
        //     component: () => import('../views/userCenter/index.vue')
        //   },
    ]
  },
]
export  function getDynamicalRoutes(menuList) {
    let indexRoute = staticRouterMap.filter(v=>v.path === '/index')[0]; 
    indexRoute.children = [];
    if(menuList) { 
        menuList.forEach(item => {
                item.children.forEach(item2 => {
                    if(item2.component) {
                        let component_url = `../views/${item2.component}.vue`;
                        indexRoute.children.push({
                            path: item2.path,
                            name: item2.name,
                            component: modules[`${component_url}`]
                    })
                    }
                })
        })  
        return indexRoute;
    }
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
    if(getToken()) {
        //已经登录
        let menuList = getMenuList();
        if(menuList.length >=1) {
            let dyroutes = getDynamicalRoutes(menuList);
            let component_url = "../views/userCenter/index.vue"
            dyroutes.children.push({
                path: '/userCenter',
                name: '个人中心',
                component: modules[`${component_url}`]
            })
            console.log(dyroutes);
            router.addRoute(dyroutes);
        }
    }else {
        router.replace('/login');
    }
}


//路由守卫
router.beforeEach((to,from,next) =>{
    if(to.path == '/login') {
        next()
    }else{
    //   检查是否已登录
     let token = getToken()
     if(token) {
        next();
     }else {
        router.replace('/login');
     }
    }
}) 
