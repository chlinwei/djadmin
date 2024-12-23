import { createRouter, createWebHistory } from 'vue-router'

export const staticRouterMap = [
  {
    path: '/login',
    name: 'login',
    component: () => import('../views/Login.vue')
  },
]


export const dynamicRouterMap = [
  {
    path: '/',
    name: '主页',
    component: () => import('../layout/index.vue'),
    redirect: '/index',
    children: [
      {
        path: '/index',
        name: '首页',
        component: () => import('../views/index/index.vue')
      },
      {
        path: '/',
        name: '首页',
        component: () => import('../views/index/index.vue')
      },
      {
        path: '/sys/user',
        name: '用户管理',
        component: () => import('../views/sys/user/index.vue')
      },
      {
        path: '/sys/role',
        name: '角色管理',
        component: () => import('../views/sys/role/index.vue')
      },
      {
        path: '/sys/menu',
        name: '菜单管理',
        component: () => import('../views/sys/menu/index.vue')
      },
    ]
  },
]

const router = createRouter({
  history: createWebHistory(import.meta.env.BASE_URL),
  routes: staticRouterMap
})

export default router
