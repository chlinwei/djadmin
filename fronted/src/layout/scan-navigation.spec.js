import { flushPromises, mount } from '@vue/test-utils'
import { h } from 'vue'
import { RouterView } from 'vue-router'
import Antd from 'ant-design-vue'
import { describe, expect, it, vi } from 'vitest'
import router from '@/router'
import store from '@/store'
import requestUtil from '@/util/request'

vi.mock('@/layout/menu/index.vue', () => ({ default: { template: '<div />' } }))
vi.mock('@/layout/header/index.vue', () => ({ default: { template: '<div />' } }))
vi.mock('@/layout/footer/index.vue', () => ({ default: { template: '<div />' } }))
vi.mock('@/util/request', () => ({ default: { get: vi.fn() } }))

describe('扫描详情的真实页签与缓存切换', () => {
  it('重复过滤、切换不同扫描、关闭当前页签和全部页签不产生渲染异常', async () => {
    const warnings = []
    requestUtil.get.mockImplementation((url) => Promise.resolve({ data: { data: {
      scan: { id: Number(url.match(/scans\/(\d+)/)?.[1]), status: 'failed', baseline: 'test', environments: [] },
      targets: [{ host_id: 1, host_name: 'host', host_ip: '127.0.0.1', status: 'failed', compliance_rate: 50, environments: [] }],
      items: [
        { host_id: 1, host_name: 'host', status: 'pass', severity: 'low', item_name: 'pass', environments: [] },
        { host_id: 1, host_name: 'host', status: 'fail', severity: 'high', item_name: 'fail', message: 'failed', remediation: 'fix', environments: [] },
      ],
    } } }))
    await router.push('/sys/security/baseline/scans/33')
    await router.isReady()
    const wrapper = mount({ render: () => h(RouterView) }, {
      attachTo: document.body,
      global: {
        plugins: [Antd, store, router],
        stubs: { FontAwesomeIcon: true },
        config: { warnHandler: (message) => { warnings.push(message) } },
      },
    })
    try {
      await flushPromises()
      for (let i = 0; i < 4; i++) {
        for (const id of [34, 33]) {
          await router.push(`/sys/security/baseline/scans/${id}`)
          await flushPromises()
          for (const label of ['通过（1）', '不符合（1）', '全部（2）']) {
            const button = wrapper.findAll('.ant-radio-button-wrapper').find((node) => node.text() === label)
            expect(button).toBeDefined()
            await button.trigger('click')
            await flushPromises()
          }
        }
      }
      const currentTab = wrapper.find('.app-tabs .ant-tabs-tab-active')
      await currentTab.find('.ant-tabs-tab-remove').trigger('click')
      await flushPromises()
      expect(router.currentRoute.value.path).toBe('/sys/security/baseline/scans/34')
      await wrapper.find('.tabs-action-btn').trigger('click')
      await flushPromises()
      await vi.waitFor(() => expect(router.currentRoute.value.path).toBe('/index'))
      expect(warnings.filter((message) => /outside of the render|Unhandled error/.test(message))).toEqual([])
    } finally {
      wrapper.unmount()
    }
  })
})
