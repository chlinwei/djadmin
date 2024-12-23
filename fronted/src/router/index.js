import { createRouter, createWebHistory } from 'vue-router'
import {getMenuList} from '@/api/user/index.js'
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
    path: '/index',
    name: '主页',
    component: () => import('../layout/index.vue'),
    children: [
    ]
  },
]
function getDynamicalRoutes(menuList) {
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
                            // component: () => import(`../views/Login2.vue`)
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



//路由守卫
router.beforeEach((to,from,next) =>{
    
    if(to.path == '/auth/login') {
        next()
    }else{
        // 获取动态路由
       const  menuList = getMenuList();
       let dynamicRoutes = getDynamicalRoutes(menuList)
       if(dynamicRoutes) {
        router.addRoute(dynamicRoutes);
       }
    //    console.log(router.getRoutes())
       next();
    }
}) 
