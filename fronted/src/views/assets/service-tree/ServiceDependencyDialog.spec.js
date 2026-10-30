import { flushPromises, mount } from '@vue/test-utils'
import Antd, { Checkbox, Select } from 'ant-design-vue'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
vi.mock('@/api/assets/serviceDependencies', () => ({ saveServiceDependency: vi.fn() }))
import { saveServiceDependency } from '@/api/assets/serviceDependencies'
import DependencyFields from './DependencyFields.vue'
import Dialog from './ServiceDependencyDialog.vue'

const services = [
  { id: 1, name: '订单', business_system_name: '商城', code: 'order' },
  { id: 2, name: '缓存', business_system_name: '共享', code: 'cache' },
]
const types = [{ id: 1, name: '服务调用', enabled: true }, { id: 2, name: '旧类型', enabled: false }]
const details = { interaction_type_id: 1, strength: 'strong', function_scope: '订单创建', protocol: 'HTTPS', failure_impact: '', fallback_plan: '' }
const response = (results) => ({ data: { code: 200, msg: 'success', data: { results } } })
let wrapper
beforeEach(() => {
  vi.clearAllMocks()
  saveServiceDependency.mockResolvedValue(response([{ id: 9 }]))
})
afterEach(() => { wrapper?.unmount(); document.body.innerHTML = '' })
async function create(record = null) {
  wrapper = mount(Dialog, {
    props: { open: true, record, services, types, initialSource: 1 },
    global: { plugins: [Antd], stubs: { AModal: { props: ['open'], template: '<div v-if="open"><slot /><slot name="footer" /></div>' } } },
  })
  await flushPromises()
  return wrapper
}
async function fill() {
  const selects = wrapper.findAllComponents(Select)
  selects[1].vm.$emit('update:value', 2)
  Object.assign(wrapper.findComponent(DependencyFields).props('model'), details)
  await flushPromises()
}
async function submit() {
  await wrapper.findAll('button').find((button) => button.text().replace(/\s+/g, '') === '保存').trigger('click')
  await flushPromises()
}

describe('dependency editing dialog', () => {
  it('initializes on first open, defaults strength to unknown and excludes disabled types from new relationships', async () => {
    await create()
    expect(wrapper.findAllComponents(Select)[0].props('value')).toBe(1)
    const fields = wrapper.findComponent(DependencyFields)
    expect(fields.props('model').strength).toBe('unknown')
    expect(fields.props('typeOptions').map((item) => item.value)).toEqual([1])
    for (const select of wrapper.findAllComponents(Select)) expect(select.props('getPopupContainer')).toBeTypeOf('function')
  })
  it('creates a single directed relationship and closes only after persistence', async () => {
    await create()
    await fill()
    await submit()
    expect(saveServiceDependency).toHaveBeenCalledWith(undefined, { source_service_id: 1, target_service_id: 2, ...details })
    expect(wrapper.emitted('saved')).toHaveLength(1)
    expect(wrapper.emitted('update:open')).toEqual([[false]])
  })
  it('creates both directions atomically with independently entered properties', async () => {
    await create()
    await fill()
    wrapper.findComponent(Checkbox).vm.$emit('update:checked', true)
    await flushPromises()
    const reverse = { interaction_type_id: 1, strength: 'weak', function_scope: '异步补充', protocol: '', failure_impact: '', fallback_plan: '' }
    Object.assign(wrapper.findAllComponents(DependencyFields)[1].props('model'), reverse)
    saveServiceDependency.mockResolvedValueOnce(response([{ id: 9 }, { id: 10 }]))
    await submit()
    expect(saveServiceDependency).toHaveBeenCalledWith(undefined, { source_service_id: 1, target_service_id: 2, ...details, reverse })
  })
  it('edits only one direction while preserving its disabled original type', async () => {
    await create({ id: 9, source_service_id: 1, target_service_id: 2, ...details, interaction_type_id: 2 })
    expect(wrapper.findComponent(Checkbox).exists()).toBe(false)
    expect(wrapper.findComponent(DependencyFields).props('typeOptions').map((item) => item.value)).toEqual([1, 2])
    await submit()
    expect(saveServiceDependency).toHaveBeenCalledWith(9, { source_service_id: 1, target_service_id: 2, ...details, interaction_type_id: 2 })
    expect(saveServiceDependency.mock.calls[0][1]).not.toHaveProperty('reverse')
  })
  it('blocks self dependency and missing required functionality', async () => {
    await create()
    await fill()
    wrapper.findAllComponents(Select)[1].vm.$emit('update:value', 1)
    await submit()
    expect(wrapper.text()).toContain('不能让服务依赖自身')
    expect(saveServiceDependency).not.toHaveBeenCalled()
    wrapper.findAllComponents(Select)[1].vm.$emit('update:value', 2)
    wrapper.findComponent(DependencyFields).props('model').function_scope = ' '
    await submit()
    expect(saveServiceDependency).not.toHaveBeenCalled()
  })
  it('retains form input and displays failures so the user can correct and retry', async () => {
    await create()
    await fill()
    saveServiceDependency.mockRejectedValueOnce(new Error('依赖关系已存在'))
    await submit()
    expect(wrapper.text()).toContain('依赖关系已存在')
    expect(wrapper.findComponent(DependencyFields).props('model').function_scope).toBe('订单创建')
    expect(wrapper.emitted('update:open')).toBeUndefined()
    await submit()
    expect(wrapper.emitted('saved')).toHaveLength(1)
  })
})
