import { flushPromises, mount } from '@vue/test-utils'
import Antd, { message } from 'ant-design-vue'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import MediaPage from './index.vue'
import store from '@/store'
import { getAlertMediaList, updateAlertMedia } from '@/api/monitor'

vi.mock('@/store', async () => {
  const { reactive } = await import('vue')
  return { default: { state: reactive({ perms: [], role_codes: ['operator'] }) } }
})
vi.mock('vue-router', () => ({ useRoute: () => ({ query: {} }) }))
vi.mock('@/api/monitor', () => ({
  getAlertMediaList: vi.fn(),
  updateAlertMedia: vi.fn(),
  createAlertMedia: vi.fn(),
  testAlertMedia: vi.fn(),
  batchDeleteAlertMedias: vi.fn(),
}))

const records = [
  { id: 7, name: 'wuhan-smtp', media_type: 'email', enabled: false, remark: 'primary',
    config: { smtpServer: 'smtp.example.com', smtpPort: 587, email: 'from@example.com', password: '********' } },
  { id: 8, name: 'backup-smtp', media_type: 'email', enabled: true, remark: 'backup', config: {} },
]
let wrapper

beforeEach(() => {
  vi.resetAllMocks()
  store.state.perms = ['monitor:view', 'monitor:update']
  store.state.role_codes = ['operator']
  getAlertMediaList.mockResolvedValue({ data: { data: { results: records, count: 2 } } })
  updateAlertMedia.mockImplementation((id, data) => Promise.resolve({ data: { code: 200, data: { id, enabled: data.enabled } } }))
  vi.spyOn(message, 'success').mockImplementation(() => {})
  vi.spyOn(message, 'warning').mockImplementation(() => {})
  vi.spyOn(message, 'error').mockImplementation(() => {})
})

afterEach(() => {
  wrapper?.unmount()
  document.body.innerHTML = ''
  vi.restoreAllMocks()
})

async function loadPage() {
  wrapper = mount(MediaPage, {
    attachTo: document.body,
    global: { plugins: [Antd], stubs: { FontAwesomeIcon: true } },
  })
  await flushPromises()
}

function statusSwitch(id = 7) {
  return wrapper.get(`tr[data-row-key="${id}"] button[role="switch"]`)
}

describe('媒介列表状态开关', () => {
  it.each([false, true])('当前状态为%s时可直接切换，仅提交enabled并保留SMTP配置', async (enabled) => {
    getAlertMediaList.mockResolvedValueOnce({
      data: { data: { results: [{ ...records[0], enabled }, records[1]], count: 2 } },
    })
    await loadPage()
    expect(statusSwitch().attributes('aria-checked')).toBe(String(enabled))
    expect(statusSwitch().attributes('aria-label')).toBe('wuhan-smtp启用状态')
    await statusSwitch().trigger('click')
    await flushPromises()
    expect(updateAlertMedia).toHaveBeenCalledWith(7, { enabled: !enabled })
    expect(statusSwitch().attributes('aria-checked')).toBe(String(!enabled))
    expect(wrapper.vm.mediaList[0].config).toEqual(records[0].config)
    expect(message.success).toHaveBeenCalledWith(enabled ? '媒介已停用' : '媒介已启用')
    expect(getAlertMediaList).toHaveBeenCalledTimes(1)
    const table = wrapper.findComponent({ name: 'ATable' })
    expect(table.props('columns').at(-1)).toMatchObject({ fixed: 'right', width: 180 })
    expect(table.props('scroll')).toEqual({ x: 1100 })
  })

  it.each(['operator', 'readonly'])('%s角色按动作权限控制开关，可响应授权与撤权', async (role) => {
    store.state.perms = ['monitor:view']
    store.state.role_codes = [role]
    await loadPage()
    expect(statusSwitch().attributes('disabled')).toBeDefined()
    await statusSwitch().trigger('click')
    await wrapper.vm.changeMediaEnabled(wrapper.vm.mediaList[0], true)
    expect(updateAlertMedia).not.toHaveBeenCalled()
    expect(message.warning).toHaveBeenCalledWith('没有修改媒介状态的权限')
    store.state.perms.push('monitor:update')
    await flushPromises()
    expect(statusSwitch().attributes('disabled')).toBeUndefined()
    await statusSwitch().trigger('click')
    await flushPromises()
    expect(updateAlertMedia).toHaveBeenCalledWith(7, { enabled: true })
    store.state.perms = ['monitor:view']
    await flushPromises()
    expect(statusSwitch().attributes('disabled')).toBeDefined()
    expect(statusSwitch().attributes('aria-checked')).toBe('true')
  })

  it('单行保存期间显示loading并防止重复提交，其他行仍可独立切换', async () => {
    let complete
    updateAlertMedia.mockImplementationOnce(() => new Promise((resolve) => { complete = resolve }))
    await loadPage()
    await statusSwitch().trigger('click')
    await flushPromises()
    expect(statusSwitch().classes()).toContain('ant-switch-loading')
    expect(statusSwitch().attributes('disabled')).toBeDefined()
    expect(statusSwitch().attributes('aria-checked')).toBe('false')
    await wrapper.vm.changeMediaEnabled(wrapper.vm.mediaList[0], true)
    expect(updateAlertMedia).toHaveBeenCalledTimes(1)
    expect(statusSwitch(8).attributes('disabled')).toBeUndefined()
    await statusSwitch(8).trigger('click')
    await flushPromises()
    expect(statusSwitch(8).attributes('aria-checked')).toBe('false')
    expect(updateAlertMedia).toHaveBeenCalledTimes(2)
    const previousRow = wrapper.vm.mediaList[0]
    getAlertMediaList.mockResolvedValueOnce({
      data: { data: { results: [records[0], { ...records[1], enabled: false }], count: 2 } },
    })
    await wrapper.vm.loadMedia()
    await flushPromises()
    expect(wrapper.vm.mediaList[0]).not.toBe(previousRow)
    complete({ data: { code: 200, data: { id: 7, enabled: true } } })
    await flushPromises()
    expect(statusSwitch().attributes('aria-checked')).toBe('true')
    expect(statusSwitch().classes()).not.toContain('ant-switch-loading')
    expect(statusSwitch().attributes('disabled')).toBeUndefined()
  })

  it.each([
    { name: '网络错误', error: new Error('network down'), expected: 'network down' },
    { name: '服务端拒绝', error: { response: { data: { msg: '更新被拒绝' } } }, expected: '更新被拒绝' },
    { name: '响应缺少状态', invalidResponse: true, expected: '媒介状态响应无效，请刷新后重试' },
  ])('$name时保留原状态，结束loading并明确报错', async ({ error, invalidResponse, expected }) => {
    if (invalidResponse) updateAlertMedia.mockResolvedValueOnce({ data: { code: 200, data: {} } })
    else updateAlertMedia.mockRejectedValueOnce(error)
    await loadPage()
    await statusSwitch().trigger('click')
    await flushPromises()
    expect(statusSwitch().attributes('aria-checked')).toBe('false')
    expect(statusSwitch().attributes('disabled')).toBeUndefined()
    expect(statusSwitch().classes()).not.toContain('ant-switch-loading')
    expect(message.error).toHaveBeenCalledWith(expected)
    expect(message.success).not.toHaveBeenCalled()
  })

  it('以服务端确认的实际状态为准，不把请求值冒充保存结果', async () => {
    updateAlertMedia.mockResolvedValueOnce({ data: { code: 200, data: { id: 7, enabled: false } } })
    await loadPage()
    await statusSwitch().trigger('click')
    await flushPromises()
    expect(statusSwitch().attributes('aria-checked')).toBe('false')
    expect(message.success).toHaveBeenCalledWith('媒介已停用')
  })
})
