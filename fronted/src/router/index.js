import { createRouter, createWebHistory } from 'vue-router'
import HomeView from '../views/HomeView.vue'

const router = createRouter({
  history: createWebHistory(import.meta.env.BASE_URL),
  routes: [
    {
        path: '/login2',
        name: 'login2',
        component: () => import('../views/Login2.vue')
    },
    {
      path: '/login',
      name: 'login',
      component: () => import('../views/Login.vue')
    },
    {
        path: '/',
        name: 'home',
        component: () => import('../layout/index.vue')
    },
    {
        path: '/test',
        name: 'test',
        component: () => import('../layout/test.vue')
    }
  ],
})

export default router
