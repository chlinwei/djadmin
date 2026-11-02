import { flushPromises, mount } from '@vue/test-utils'
import Antd from 'ant-design-vue'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import SysConfig from './index.vue'
import store from '@/store'
import { getConfigList, resetConfigDefault, updateConfig } from '@/api/sys/sysconfig'

vi.mock('@/store', async () => {
  const { reactive } = await import('vue')
  return { default: { state: reactive({
    user: { timezone: 'Asia/Shanghai' }, perms: [], role_codes: [],
  }) } }
})
vi.mock('@/api/sys/sysconfig', () => ({
  getConfigList: vi.fn(),
  updateConfig: vi.fn(),
  resetConfigDefault: vi.fn(),
}))
vi.mock('@/util/timezone', () => ({ formatTimeWithTimezone: vi.fn((value) => value) }))

const configs = [
  { id: 1, name: '日志保留天数', key: 'sys.logs.retention', value: '7', default_value: '30', value_type: 'int', is_readonly: false, description: '保留日志的天数' },
  { id: 2, name: '系统版本', key: 'sys.version', value: '1.0', default_value: '1.0', value_type: 'string', is_readonly: true, description: '构建版本' },
]
let wrapper

beforeEach(() => {
  vi.clearAllMocks()
  store.state.perms = ['sys:config:list']
  store.state.role_codes = ['operator']
  getConfigList.mockResolvedValue({ data: { data: { results: configs, count: 2, totalPages: 1 } } })
  updateConfig.mockResolvedValue({ data: { code: 200 } })
  resetConfigDefault.mockResolvedValue({ data: { code: 200 } })
})

afterEach(() => {
  wrapper?.unmount()
  document.body.innerHTML = ''
})

async function loadPage() {
  wrapper = mount(SysConfig, {
    attachTo: document.body,
    global: { plugins: [Antd], stubs: { FontAwesomeIcon: true } },
  })
  await flushPromises()
}

function table() {
  return wrapper.findComponent({ name: 'ATable' })
}

function hasActions() {
  return table().props('columns').some((column) => column.key === 'action')
}

describe('参数设置操作列', () => {
  it.each(['readonly', 'operator'])('%s 角色仅有查看权限时不显示空操作列，参数内容仍可查看', async (role) => {
    store.state.role_codes = [role]
    await loadPage()
    expect(hasActions()).toBe(false)
    expect(table().findAll('thead th').map((header) => header.text())).not.toContain('操作')
    expect(table().findAll('tbody button')).toHaveLength(0)
    for (const text of ['日志保留天数', 'sys.logs.retention', '30', '可编辑', '构建版本']) {
      expect(table().text()).toContain(text)
    }
    wrapper.vm.openEdit(configs[0])
    wrapper.vm.saveEdit()
    wrapper.vm.handleResetDefault(configs[0])
    await flushPromises()
    expect(wrapper.vm.editVisible).toBe(false)
    expect(updateConfig).not.toHaveBeenCalled()
    expect(resetConfigDefault).not.toHaveBeenCalled()
  })

  it.each([false, true])('有更新权限或管理员（admin=%s）保留编辑、重置确认及参数只读限制', async (admin) => {
    store.state.role_codes = admin ? ['admin'] : ['operator']
    if (!admin) store.state.perms.push('sys:config:update')
    await loadPage()
    expect(table().props('columns').at(-1)).toMatchObject({ key: 'action', fixed: 'right', width: 180 })
    expect(table().props('scroll')).toEqual({ x: 1600 })
    const editable = wrapper.get('tr[data-row-key="1"]')
    const readonly = wrapper.get('tr[data-row-key="2"]')
    expect(editable.findAll('button').every((button) => button.attributes('disabled') === undefined)).toBe(true)
    expect(readonly.findAll('button')).toHaveLength(2)
    expect(readonly.findAll('button').every((button) => button.attributes('disabled') !== undefined)).toBe(true)
    await editable.findAll('button')[0].trigger('click')
    await flushPromises()
    expect(wrapper.vm.editVisible).toBe(true)
    const input = document.body.querySelector('.ant-modal-content input:not(:disabled)')
    input.value = '11'
    input.dispatchEvent(new Event('input', { bubbles: true }))
    await flushPromises()
    document.body.querySelector('.ant-modal-footer .ant-btn-primary').click()
    await flushPromises()
    expect(updateConfig).toHaveBeenCalledWith(1, { value: '11', default_value: '30' })
    await wrapper.get('tr[data-row-key="1"]').findAll('button')[1].trigger('click')
    await new Promise((resolve) => setTimeout(resolve, 150))
    await flushPromises()
    expect(document.body.querySelector('.ant-popconfirm').textContent).toContain('确认重置为默认值吗')
    expect(resetConfigDefault).not.toHaveBeenCalled()
    document.body.querySelector('.ant-popconfirm-buttons .ant-btn-primary').click()
    await flushPromises()
    expect(resetConfigDefault).toHaveBeenCalledWith(1)
  })

  it('权限变化时同步隐藏或恢复操作列，不重新请求参数', async () => {
    await loadPage()
    expect(hasActions()).toBe(false)
    store.state.perms.push('sys:config:update')
    await flushPromises()
    expect(hasActions()).toBe(true)
    store.state.perms = ['sys:config:list']
    await flushPromises()
    expect(hasActions()).toBe(false)
    expect(table().findAll('thead th').map((header) => header.text())).not.toContain('操作')
    expect(getConfigList).toHaveBeenCalledTimes(1)
    expect(updateConfig).not.toHaveBeenCalled()
    expect(resetConfigDefault).not.toHaveBeenCalled()
  })

  it('隐藏操作列时保留仅已修改筛选和搜索', async () => {
    await loadPage()
    await wrapper.findAllComponents({ name: 'ARadioButton' })[1].trigger('click')
    await flushPromises()
    expect(table().props('dataSource').map((item) => item.id)).toEqual([1])
    expect(hasActions()).toBe(false)
    wrapper.vm.filterText = '日志'
    await wrapper.vm.loadConfigs()
    expect(getConfigList).toHaveBeenLastCalledWith({ page_size: 30, search: '日志', page: 1 })
  })
})
