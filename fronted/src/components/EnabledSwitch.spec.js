import { flushPromises, mount } from '@vue/test-utils'
import Antd, { Form, Switch, message } from 'ant-design-vue'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { reactive } from 'vue'
import EnabledSwitch from './EnabledSwitch.vue'
import request from '@/util/request'
import store from '@/store'

vi.mock('@/util/request', () => ({ default: { patch: vi.fn() } }))
vi.mock('@/store', async () => {
  const { reactive } = await import('vue')
  return { default: { state: reactive({ perms: [], role_codes: [] }) } }
})

const endpoint = 'assets/business-environments/7/'
const envelope = (enabled) => ({ data: { code: 200, msg: 'success', data: { enabled } } })
let wrapper
beforeEach(() => {
  vi.resetAllMocks()
  store.state.perms = ['assets:applications:update']
  store.state.role_codes = ['operator']
  vi.spyOn(message, 'success').mockImplementation(() => {})
  vi.spyOn(message, 'error').mockImplementation(() => {})
  request.patch.mockImplementation((_url, body) => Promise.resolve(envelope(body.enabled)))
})
afterEach(() => { wrapper?.unmount(); vi.restoreAllMocks() })
function create(props = {}) {
  wrapper = mount(EnabledSwitch, {
    props: { checked: true, ...props },
    global: { plugins: [Antd] },
  })
  return wrapper.get('button[role="switch"]')
}
describe('shared enabled status control', () => {
  it('uses the inspection baseline: default size and enabled/disabled labels', () => {
    const button = create()
    expect(button.classes()).not.toContain('ant-switch-small')
    expect(button.attributes('aria-checked')).toBe('true')
    expect(button.text()).toContain('启用')
    expect(button.text()).toContain('停用')
  })
  it('changes form drafts without sending a request and remains controlled', async () => {
    const button = create()
    await button.trigger('click')
    expect(wrapper.emitted('update:checked')).toEqual([[false]])
    expect(wrapper.emitted('change')[0][0]).toBe(false)
    expect(request.patch).not.toHaveBeenCalled()
    expect(button.attributes('aria-checked')).toBe('true')
    await wrapper.setProps({ checked: false })
    expect(button.attributes('aria-checked')).toBe('false')
  })
  it.each([{ readOnly: true }, { disabled: true }, { loading: true }])('prevents interaction for %j', async (props) => {
    const button = create(props)
    await button.trigger('click')
    expect(wrapper.emitted('update:checked')).toBeUndefined()
    expect(request.patch).not.toHaveBeenCalled()
  })
  it('inherits disabled form context', async () => {
    wrapper = mount({
      components: { EnabledSwitch, AForm: Form },
      template: '<a-form disabled><EnabledSwitch :checked="true" /></a-form>',
    }, { global: { plugins: [Antd] } })
    expect(wrapper.get('button[role="switch"]').attributes('disabled')).toBeDefined()
  })
  it('checks action permissions reactively and requires every permission', async () => {
    const button = create({ endpoint, permission: ['assets:applications:update', 'assets:service-tree:manage'] })
    expect(button.attributes('disabled')).toBeDefined()
    store.state.perms.push('assets:service-tree:manage')
    await flushPromises()
    expect(button.attributes('disabled')).toBeUndefined()
    store.state.perms = []
    await flushPromises()
    await button.trigger('click')
    expect(request.patch).not.toHaveBeenCalled()
  })
  it('does not make a remote endpoint writable without explicit authorization', async () => {
    const button = create({ endpoint })
    expect(button.attributes('disabled')).toBeDefined()
    await button.trigger('click')
    expect(request.patch).not.toHaveBeenCalled()
  })
  it('supports explicitly scoped self-service endpoints without management permissions', async () => {
    store.state.perms = []
    const button = create({ endpoint: 'sys/usercenter/alertMediaBindings/9/', selfService: true })
    await button.trigger('click')
    await flushPromises()
    expect(request.patch).toHaveBeenCalledWith('sys/usercenter/alertMediaBindings/9/', { enabled: false })
    expect(button.attributes('aria-checked')).toBe('false')
  })
  it('only submits enabled, blocks duplicate clicks, and confirms after server success', async () => {
    let resolve
    request.patch.mockImplementation(() => new Promise(done => { resolve = done }))
    const button = create({ endpoint, permission: 'assets:applications:update' })
    await button.trigger('click')
    await flushPromises()
    expect(request.patch).toHaveBeenCalledWith(endpoint, { enabled: false })
    expect(button.attributes('aria-checked')).toBe('true')
    expect(button.classes()).toContain('ant-switch-loading')
    await button.trigger('click')
    expect(request.patch).toHaveBeenCalledTimes(1)
    resolve(envelope(false))
    await flushPromises()
    expect(button.attributes('aria-checked')).toBe('false')
    expect(wrapper.emitted('changed')).toEqual([[false]])
    expect(message.success).toHaveBeenCalledWith('已停用')
  })
  it.each([
    () => Promise.reject(new Error('保存失败')),
    () => Promise.resolve({ data: { code: 400, msg: '禁止修改', data: null } }),
    () => Promise.resolve({ data: { code: 200, msg: 'success', data: {} } }),
    () => Promise.resolve(envelope(true)),
  ])('keeps the previous state on failure and permits retry', async (result) => {
    request.patch.mockImplementationOnce(result)
    const button = create({ endpoint, permission: 'assets:applications:update' })
    await button.trigger('click')
    await flushPromises()
    expect(button.attributes('aria-checked')).toBe('true')
    expect(wrapper.emitted('changed')).toBeUndefined()
    expect(message.error).toHaveBeenCalledTimes(1)
    expect(message.success).not.toHaveBeenCalled()
    await button.trigger('click')
    await flushPromises()
    expect(button.attributes('aria-checked')).toBe('false')
  })
  it('updates the current row after a refresh rather than a stale row reference', async () => {
    let resolve
    request.patch.mockImplementation(() => new Promise(done => { resolve = done }))
    const state = reactive({ row: { enabled: true } })
    const old = state.row
    wrapper = mount({
      components: { EnabledSwitch },
      setup: () => ({ state, endpoint }),
      template: '<EnabledSwitch v-model:checked="state.row.enabled" :endpoint="endpoint" permission="assets:applications:update" />',
    }, { global: { plugins: [Antd] } })
    await wrapper.get('button').trigger('click')
    await flushPromises()
    state.row = { enabled: true }
    await flushPromises()
    resolve(envelope(false))
    await flushPromises()
    expect(state.row.enabled).toBe(false)
    expect(old.enabled).toBe(true)
  })
  it('does not navigate a parent row or fail on Ant Design custom click arguments', async () => {
    const click = vi.fn()
    wrapper = mount({
      components: { EnabledSwitch },
      setup: () => ({ click }),
      template: '<div @click="click"><EnabledSwitch :checked="true" /></div>',
    }, { global: { plugins: [Antd] } })
    await wrapper.get('button').trigger('click')
    expect(click).not.toHaveBeenCalled()
    expect(wrapper.findComponent(Switch).exists()).toBe(true)
  })
  it('does not apply an old response to a row reused for another resource', async () => {
    let resolve
    request.patch.mockImplementation(() => new Promise(done => { resolve = done }))
    const button = create({ endpoint, permission: 'assets:applications:update' })
    await button.trigger('click')
    await flushPromises()
    await wrapper.setProps({ endpoint: 'assets/business-environments/8/', checked: true })
    resolve(envelope(false))
    await flushPromises()
    expect(button.attributes('aria-checked')).toBe('true')
    expect(wrapper.emitted('changed')).toBeUndefined()
    expect(message.success).not.toHaveBeenCalled()
  })
})
