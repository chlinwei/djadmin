import { describe, expect, it, vi } from 'vitest'
import { getToken } from '@/api/user/index.js'
import { modules, staticRouterMap } from './index'

vi.mock('@/api/user/index.js', () => ({ getToken: vi.fn() }))
vi.mock('@/api/menu/index.js', () => ({ getMenuList: () => [] }))

describe('文档中心路由', () => {
  const route = staticRouterMap.find((entry) => entry.path === '/').children.find((entry) => entry.path === '/docs')

  it('存在独立的顶级页面，不依赖数据库动态路由', () => {
    expect(route.name).toBe('文档中心')
    expect(route.component).toBeTypeOf('function')
  })

  it('动态页面入口保留服务树与依赖组件，但不把测试及测试框架打入生产包', () => {
    const paths = Object.keys(modules)
    expect(paths).toContain('../views/assets/service-tree/index.vue')
    expect(paths).toContain('../views/assets/service-tree/ServiceDependencyWorkspace.vue')
    expect(paths.some((path) => /\.(spec|test)\.(js|ts)$/.test(path))).toBe(false)
  })

  it('未登录跳转登录页，已登录不要求额外角色权限', () => {
    getToken.mockReturnValue(null)
    expect(route.beforeEnter()).toEqual({ path: '/login' })
    getToken.mockReturnValue('signed-in-token')
    expect(route.beforeEnter()).toBe(true)
  })
})
