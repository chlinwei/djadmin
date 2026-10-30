import { flushPromises, mount } from '@vue/test-utils'
import Antd, { Form, Input, Switch, Table, Tooltip } from 'ant-design-vue'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
vi.mock('@/api/assets/serviceDependencies', () => ({
  getServiceDependencyTypes: vi.fn(),
  saveServiceDependencyType: vi.fn(),
  deleteServiceDependencyTypes: vi.fn(),
}))
vi.mock('@/directives/permission/permission', () => ({ checkPermission: () => true }))
vi.mock('@/util/deleteConfirm', async (original) => ({ ...await original(), openDeleteConfirm: vi.fn() }))
import * as api from '@/api/assets/serviceDependencies'
import { openDeleteConfirm } from '@/util/deleteConfirm'
import Types from './ServiceDependencyTypes.vue'

const record = { id: 1, code: 'http', name: '服务调用', enabled: true, description: 'HTTP 请求' }
const envelope = (data) => ({ data: { code: 200, msg: 'success', data } })
let wrapper
beforeEach(() => {
  vi.clearAllMocks()
  const computedStyle = window.getComputedStyle
  vi.spyOn(window, 'getComputedStyle').mockImplementation((element) => computedStyle(element))
  api.getServiceDependencyTypes.mockResolvedValue(envelope({ results: [record], totalPages: 1 }))
  api.saveServiceDependencyType.mockResolvedValue(envelope(record))
})
afterEach(() => { wrapper?.unmount(); vi.restoreAllMocks(); document.body.innerHTML = '' })
async function create() {
  wrapper = mount(Types, {
    props: { open: true },
    global: { plugins: [Antd], stubs: { FontAwesomeIcon: true, AModal: { props: ['open'], template: '<div v-if="open"><slot /><slot name="footer" /></div>' } } },
  })
  await flushPromises()
}
async function clickText(text) {
  await wrapper.findAll('button').find((button) => button.text().replace(/\s+/g, '') === text).trigger('click')
  await flushPromises()
}
describe('dependency interaction dictionary', () => {
  it('supports creating a custom type with validated stable code and optional description', async () => {
    await create()
    await clickText('新增类型')
    const model = wrapper.findComponent(Form).props('model')
    Object.assign(model, { code: 'grpc', name: 'RPC 调用', enabled: true, description: '' })
    await clickText('保存')
    expect(api.saveServiceDependencyType).toHaveBeenCalledWith(null, { code: 'grpc', name: 'RPC 调用', enabled: true, description: '' })
    expect(wrapper.emitted('changed')).toHaveLength(1)
  })
  it('locks existing codes, allows disabling, and keeps action layout and tooltips consistent', async () => {
    await create()
    const table = wrapper.findComponent(Table)
    expect(table.props('columns').at(-1)).toMatchObject({ fixed: 'right', width: 120 })
    expect(table.props('scroll').x).toBeGreaterThan(0)
    const edit = wrapper.findAllComponents(Tooltip).find((tooltip) => tooltip.props('title') === '编辑')
    expect(edit.props('placement')).toBe('top')
    await edit.get('button').trigger('click')
    await flushPromises()
    expect(wrapper.findComponent(Input).props('disabled')).toBe(true)
    wrapper.findComponent(Switch).vm.$emit('update:checked', false)
    await clickText('保存')
    expect(api.saveServiceDependencyType).toHaveBeenCalledWith(1, { code: 'http', name: '服务调用', enabled: false, description: 'HTTP 请求' })
  })
  it('shows referenced-type deletion rejection and refreshes instead of reporting success', async () => {
    await create()
    api.deleteServiceDependencyTypes.mockResolvedValueOnce(envelope({ count: 0, results: [{ id: 1, ok: false, message: '该类型已被依赖关系引用' }] }))
    const remove = wrapper.findAllComponents(Tooltip).find((tooltip) => tooltip.props('title') === '删除')
    await remove.get('button').trigger('click')
    const confirm = openDeleteConfirm.mock.calls.at(-1)[0]
    expect(confirm.items).toEqual(['服务调用（http，#1）'])
    await expect(confirm.onConfirm()).rejects.toThrow('已被依赖关系引用')
    expect(api.getServiceDependencyTypes).toHaveBeenCalledTimes(2)
    expect(wrapper.text()).toContain('已被依赖关系引用')
  })
})
