import { mount, flushPromises } from '@vue/test-utils'
import { createRouter, createMemoryHistory } from 'vue-router'
import Antd from 'ant-design-vue'
import { describe, expect, it, vi } from 'vitest'
import Menu from './index.vue'
import store from '@/store/index.js'

vi.mock('@/api/menu/index.js', () => ({ getMenuList: () => [] }))

describe('公共文档菜单', () => {
  it('无角色菜单时仍可见，并以正确标题打开标签', async () => {
    const router = createRouter({
      history: createMemoryHistory(),
      routes: [{ path: '/index', component: { template: '<div />' } }],
    })
    await router.push('/index')
    store.state.tabs = []
    store.state.selectedKeys = []
    const wrapper = mount(Menu, {
      global: {
        plugins: [Antd, store, router],
        components: { FontAwesomeIcon: { template: '<span />' } },
      },
    })
    try {
      await flushPromises()
      const docs = wrapper.findAll('.ant-menu-item').find((node) => node.text() === '文档中心')
      expect(docs).toBeDefined()
      await docs.trigger('click')
      expect(store.state.tabs).toContainEqual({ title: '文档中心', key: '/docs' })
    } finally {
      wrapper.unmount()
    }
  })
})
