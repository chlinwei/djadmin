import { afterEach, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { defineComponent, h, KeepAlive, ref } from 'vue'
import Workspace from './index.vue'
import { getHomeOverview } from '@/api/home'
import store from '@/store'

vi.mock('@/api/home', () => ({ getHomeOverview: vi.fn() }))
vi.mock('@/store', async () => {
  const { reactive } = await import('vue')
  return { default: { state: reactive({ user: { username: 'operator', timezone: 'UTC' } }) } }
})

const generatedAt = '2026-11-26T14:00:00Z'
const response = (data) => ({ data: { code: 200, data: { generated_at: generatedAt, ...data } } })
let wrapper

function mountWorkspace() {
  const active = ref(true)
  wrapper = mount(defineComponent({
    setup: () => () => h(KeepAlive, null, () => active.value ? h(Workspace) : null),
  }), {
    global: {
      stubs: {
        RouterLink: { props: ['to'], template: '<a :href="to"><slot /></a>' },
        ACard: { props: ['title'], template: '<div>{{ title }}<slot name="extra" /><slot /></div>' },
        ATag: { template: '<span><slot /></span>' },
        AAlert: { props: ['message'], template: '<p>{{ message }}</p>' },
        AEmpty: { props: ['description'], template: '<p>{{ description }}</p>' },
        ASkeleton: true,
      },
    },
  })
  return active
}

afterEach(() => {
  wrapper?.unmount()
  vi.resetAllMocks()
  store.state.user.timezone = 'UTC'
})

it('distinguishes a real zero from a failed section and only links authorized modules', async () => {
  getHomeOverview.mockResolvedValue(response({
    hosts: { status: 'ok', count: 0 },
    inspections: { status: 'unavailable' },
  }))
  mountWorkspace()
  await flushPromises()

  expect(wrapper.findAll('.metric-value').map((node) => node.text())).toEqual(['0', '—'])
  expect(wrapper.text()).toContain('暂时无法获取记录')
  expect(wrapper.get('a[href="/assets/hosts"]').text()).toContain('主机管理')
  expect(wrapper.find('a[href="/monitor/alerting/alerts"]').exists()).toBe(false)
  expect(wrapper.get('a[href="/docs"]').text()).toContain('文档中心')
})

it('reloads when returning to the tab and reacts to a user timezone change', async () => {
  getHomeOverview.mockResolvedValueOnce(response({})).mockResolvedValueOnce(response({
    jobs: { status: 'ok', count: 1, recent: [{ id: 8, name: '发布配置', status: 'running', time: generatedAt }] },
  }))
  const active = mountWorkspace()
  await flushPromises()
  expect(wrapper.text()).toContain('暂无业务概览权限')
  active.value = false
  await flushPromises()
  expect(getHomeOverview).toHaveBeenCalledTimes(1)
  active.value = true
  await flushPromises()
  expect(getHomeOverview).toHaveBeenCalledTimes(2)
  expect(wrapper.text()).toContain('发布配置')
  expect(wrapper.get('.activity-list time').text()).toBe('11-26 14:00')
  store.state.user.timezone = 'Asia/Shanghai'
  await flushPromises()
  expect(wrapper.get('.activity-list time').text()).toBe('11-26 22:00')
})
