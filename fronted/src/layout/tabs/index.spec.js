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

import router from '@/router'
import store from '@/store/index.js'
import Tabs from './index.vue'

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
    wrapper = mount(Tabs, { global: { plugins: [Antd, store, router] } })
    await flushPromises()
    const push = vi.spyOn(router, 'push')
    const location = `${path}?application_service_id=21&panel=${panel}`
    await router.replace(location)
    await flushPromises()

    expect(store.state.tabs).toEqual([{ title, key: location }])
    expect(push).not.toHaveBeenCalled()
    await router.push('/index')
    await flushPromises()
    store.state.activeKey = location
    await flushPromises()
    expect(router.currentRoute.value.fullPath).toBe(location)
    expect(store.state.tabs.filter((tab) => tab.title === title)).toHaveLength(1)
  })
})
