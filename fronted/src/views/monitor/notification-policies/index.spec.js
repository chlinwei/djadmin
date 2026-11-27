import { mount, flushPromises } from '@vue/test-utils'
import Antd from 'ant-design-vue'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import Policies from './index.vue'
import store from '@/store'
import request from '@/util/request'

vi.mock('@/util/request', () => ({ default: { get: vi.fn(), post: vi.fn() } }))
const policy = {
  id: 1, is_root: true, name: '路由策略', matchers: [{ type: 'tree', node_type: 'service', id: 4 }],
  scope_names: { 'service:4': '订单 / 商城 / 生产 / 支付' },
  media_ids: [8], media_names: ['值班邮箱'], user_group_ids: [3], user_group_names: ['一线值班'],
}
let wrapper
beforeEach(() => {
  vi.clearAllMocks()
  store.state.role_codes = ['operator']
  store.state.perms = ['monitor:view']
  request.get.mockImplementation(async url => ({ data: { data: url === 'monitor/notification-policies/'
    ? { items: [policy] } : { items: [], results: [], totalPages: 1 } } }))
})
afterEach(() => { wrapper?.unmount(); document.body.innerHTML = '' })
it.each([false, true])('策略表单互斥入口与编辑候选，更新权限=%s', async editable => {
  if (editable) store.state.perms.push('monitor:update')
  wrapper = mount(Policies, { attachTo: document.body, global: { plugins: [Antd], stubs: { FontAwesomeIcon: true } } })
  await flushPromises()
  const buttons = wrapper.findAll('button').map(b => b.text().replace(/\s/g, ''))
  expect(buttons.includes('编辑')).toBe(editable)
  expect(buttons.includes('查看')).toBe(!editable)
  expect(request.get.mock.calls.map(([url]) => url)).toEqual(['monitor/notification-policies/'])
  await wrapper.findAll('button').find(b => b.text() === policy.name).trigger('click')
  await flushPromises()
  const dialog = document.querySelector('.ant-modal-content')
  for (const label of ['一线值班', '值班邮箱', policy.scope_names['service:4']]) expect(dialog.textContent).toContain(label)
  if (editable) {
    expect(request.get).toHaveBeenCalledWith('sys/user-groups/')
    expect(dialog.textContent).toContain('添加条件')
    expect(dialog.querySelector('.ant-form-item input[placeholder="例如：生产环境严重告警"]').disabled).toBe(false)
  } else {
    expect([...dialog.querySelectorAll('button')].some(b => /添加条件|移除/.test(b.textContent))).toBe(false)
    expect(dialog.querySelector('.modal-actions').textContent.replace(/\s/g, '')).toBe('关闭')
    expect([...dialog.querySelectorAll('input,textarea')].every(input => input.disabled)).toBe(true)
    await wrapper.vm.savePolicy()
    expect(request.get).toHaveBeenCalledTimes(1)
    expect(request.post).not.toHaveBeenCalled()
  }
})
