import { flushPromises, mount } from '@vue/test-utils'
import Antd from 'ant-design-vue'
import { afterEach, describe, expect, it, vi } from 'vitest'

vi.mock('@/router', async () => {
  const { createMemoryHistory, createRouter } = await import('vue-router')
  return { default: createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/index', name: '首页', component: { template: '<div />' } },
      { path: '/assets/service-tree/index', name: '服务树', component: { template: '<div />' } },
      { path: '/monitor/logging/center', name: '日志中心', component: { template: '<div />' } },
    ],
  }) }
})
vi.mock('@/api/menu/index.js', () => ({
  getMenuList: () => [
    { id: 1, name: '资产管理', path: '/assets', menu_type: 'M', location: 1, children: [
      { id: 2, name: '服务树', path: '/assets/service-tree/index', menu_type: 'C' },
    ] },
    { id: 3, name: '监控中心', path: '/monitor', menu_type: 'M', location: 1, children: [
      { id: 4, name: '日志中心', path: '/monitor/logging/center', menu_type: 'C' },
    ] },
  ],
}))

import router from '@/router'
import store from '@/store/index.js'
import Tabs from './index.vue'
import Menu from '@/layout/menu/index.vue'

let wrapper
afterEach(() => {
  wrapper?.unmount()
  vi.restoreAllMocks()
  store.commit('close_all_tabs')
})

describe('service tree tab navigation', () => {
  it.each([
    ['/assets/service-tree/index', '服务树', 'dependencies'],
    ['/monitor/logging/center', '日志中心', 'config'],
  ])('updates one %s tab without duplicate navigation and reopens its latest location', async (path, title, panel) => {
    await router.push(path)
    wrapper = mount({
      components: { Menu, Tabs },
      template: '<Menu /><Tabs />',
    }, { global: {
      plugins: [Antd, store, router],
      components: { FontAwesomeIcon: { template: '<span />' } },
    } })
    await flushPromises()
    expect(wrapper.find('.ant-menu-item-selected').text()).toBe(title)
    const push = vi.spyOn(router, 'push')
    const location = `${path}?application_service_id=21&panel=${panel}`
    await router.replace(location)
    await flushPromises()

    expect(store.state.tabs).toEqual([{ title, key: location }])
    expect(store.state.selectedKeys).toEqual([path])
    expect(wrapper.find('.ant-menu-item-selected').text()).toBe(title)
    expect(push).not.toHaveBeenCalled()
    await router.push('/index')
    await flushPromises()
    expect(wrapper.find('.ant-menu-item-selected').text()).toBe('首页')
    store.state.activeKey = location
    await flushPromises()
    expect(router.currentRoute.value.fullPath).toBe(location)
    expect(store.state.tabs.filter((tab) => tab.title === title)).toHaveLength(1)
    expect(wrapper.find('.ant-menu-item-selected').text()).toBe(title)
  })
})
