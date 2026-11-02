import { flushPromises, mount } from '@vue/test-utils'
import Antd, { FormItem, message, Tooltip } from 'ant-design-vue'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import BaselinePage from './index.vue'
import requestUtil from '@/util/request'
import store from '@/store'
import permission from '@/directives/permission'

vi.mock('@/store', () => ({ default: { state: { user: { timezone: 'Asia/Shanghai' }, perms: ['baseline:view', 'baseline:manage', 'baseline:scan'], role_codes: ['operator'] } } }))
vi.mock('vue-router', () => ({ useRouter: () => ({ push: vi.fn() }) }))
vi.mock('@/util/request', () => ({ default: { get: vi.fn(), patch: vi.fn(), post: vi.fn(), del: vi.fn() } }))

const baseline = { id: 7, name: '测试基线' }
const items = [
  { id: 88, category_id: 3, name: '策略一', severity: 'high', config: {} },
  { id: 89, category_id: 3, name: '策略二', severity: 'high', config: {} },
  { id: 90, category_id: 4, name: '其他类目策略', severity: 'low', config: {} },
]
const response = (data) => ({ data: { data } })
let wrapper
let savedItems

beforeEach(() => {
  vi.clearAllMocks()
  store.state.perms = ['baseline:view', 'baseline:manage', 'baseline:scan']
  store.state.role_codes = ['operator']
  savedItems = [...items]
  requestUtil.get.mockImplementation((url) => Promise.resolve(response(
    url.endsWith('/scan-hosts/')
      ? { hosts: [{ id: 10, name: 'host-a', ip: '10.0.0.1', online: true }, { id: 11, name: 'host-b', ip: '10.0.0.2', online: false }] }
      : url === 'sys/security/baseline/7/'
      ? { categories: [{ id: 3, name: '类目一' }, { id: 4, name: '类目二' }], items: savedItems }
      : { results: url === 'sys/security/baseline/' ? [baseline] : [] },
  )))
  requestUtil.post.mockResolvedValue(response({ targets: 1 }))
})

afterEach(() => {
  wrapper?.unmount()
  document.body.innerHTML = ''
  vi.restoreAllMocks()
})

async function loadPage() {
  wrapper = mount(BaselinePage, {
    global: { plugins: [Antd], stubs: { FontAwesomeIcon: true }, directives: { permission } },
    attachTo: document.body,
  })
  await flushPromises()
  wrapper.vm.selectBaseline(baseline)
  await flushPromises()
}

describe('策略只读详情', () => {
  it('view 用户可查看完整策略，但不能编辑、排序、删除或发起扫描', async () => {
    store.state.perms = ['baseline:view']
    const policy = 'package baseline\n\nassertions contains {"name": "SSH", "pass": true} if {\n  input.sshd[0] == "PermitEmptyPasswords no"\n}'
    savedItems = [{
      ...items[0],
      description: 'sshd_config 中 PermitEmptyPasswords 必须设置为 no。\n这里应完整展示，不受列表截断影响。',
      config: {
        run_user: 'audit',
        remediation: '检查 /etc/ssh/sshd_config\n按运维流程更新配置',
        input_commands: [{ key: 'hostname', exec: 'printf "%s\\n" "{HOST_NAME}"', parse: 'raw' }],
        input_files: [
          { key: 'sshd', path: '/etc/ssh/sshd_config', parse: 'lines' },
          { key: 'settings', path: '/etc/example/settings.json', parse: 'json' },
        ],
        policy,
      },
    }, ...items.slice(1)]
    await loadPage()
    const originalItems = JSON.stringify(wrapper.vm.itemForm)
    const originalEditor = JSON.stringify(wrapper.vm.itemEditForm)
    const readsBeforeView = requestUtil.get.mock.calls.length
    const entries = wrapper.findAll('[data-view-policy]')
    expect(entries).toHaveLength(2)
    expect(entries.every((entry) => entry.isVisible())).toBe(true)
    const viewTooltip = wrapper.findAllComponents(Tooltip).find((tooltip) => tooltip.props('title') === '查看详情')
    expect(viewTooltip.props('placement')).toBe('top')
    await entries[0].trigger('mouseenter')
    await new Promise((resolve) => setTimeout(resolve, 150))
    await flushPromises()
    expect(document.body.querySelector('.ant-tooltip-inner')?.textContent).toBe('查看详情')
    await entries[0].trigger('click')
    await flushPromises()
    const dialog = document.body.querySelector('.ant-modal-content')
    expect(dialog.querySelector('.ant-modal-title').textContent).toBe('策略详情：策略一')
    for (const text of ['测试基线', '类目一', 'audit', '按运维流程更新配置', '这里应完整展示，不受列表截断影响。',
      'hostname', 'printf "%s\\n" "{HOST_NAME}"', '/etc/ssh/sshd_config', 'raw 原文', 'lines 按行', 'json 对象', '/etc/example/settings.json']) {
      expect(dialog.textContent).toContain(text)
    }
    expect(Array.from(dialog.querySelectorAll('pre')).some((element) => element.textContent === policy)).toBe(true)
    expect(dialog.querySelector('input, textarea, .ant-select, .ant-switch')).toBeNull()
    expect(dialog.querySelector('.ant-modal-footer')).toBeNull()
    expect(wrapper.findAll('button').filter((button) => button.isVisible() && /新增|添加策略|扫描|重命名/.test(button.text()))).toHaveLength(0)
    for (const title of ['上移', '下移', '编辑', '删除']) {
      const buttons = wrapper.findAllComponents(Tooltip).filter((tooltip) => tooltip.props('title') === title)
      expect(buttons.every((tooltip) => !tooltip.find('button').isVisible())).toBe(true)
    }
    wrapper.vm.openItemEditModal(0)
    await wrapper.vm.confirmItemEdit()
    await wrapper.vm.moveItem(wrapper.vm.categoryRows[0], 'down')
    wrapper.vm.openScanModal(baseline)
    expect(wrapper.vm.itemEditModalOpen).toBe(false)
    expect(wrapper.vm.scanModalOpen).toBe(false)
    expect(JSON.stringify(wrapper.vm.itemForm)).toBe(originalItems)
    expect(JSON.stringify(wrapper.vm.itemEditForm)).toBe(originalEditor)
    expect(requestUtil.get).toHaveBeenCalledTimes(readsBeforeView)
    expect(requestUtil.patch).not.toHaveBeenCalled()
    expect(requestUtil.post).not.toHaveBeenCalled()
    expect(requestUtil.del).not.toHaveBeenCalled()
    expect(wrapper.vm.itemColumns.find((column) => column.key === 'action')).toMatchObject({ fixed: 'right', width: 224 })
    expect(wrapper.findAllComponents({ name: 'ATable' })[0].props('scroll')).toEqual({ x: 800 })
  })

  it('空配置明确显示默认用户和未配置项，详情归属不随页面类目改变', async () => {
    store.state.perms = ['baseline:view']
    await loadPage()
    await wrapper.findAll('[data-view-policy]')[0].trigger('click')
    await flushPromises()
    wrapper.vm.selectedCategoryId = 4
    await flushPromises()
    const dialog = document.body.querySelector('.ant-modal-content')
    for (const text of ['类目一', 'root（默认）', '未配置采集命令', '未配置文件采集', '未配置 Rego 策略']) {
      expect(dialog.textContent).toContain(text)
    }
    expect(dialog.textContent).not.toContain('类目二')
    dialog.querySelector('.ant-modal-close').click()
    await flushPromises()
    expect(wrapper.vm.itemDetail).toBeNull()
    await wrapper.find('[data-view-policy]').trigger('click')
    await flushPromises()
    expect(document.body.querySelector('.ant-modal-title').textContent).toBe('策略详情：其他类目策略')
    expect(document.body.querySelector('.ant-descriptions').textContent).toContain('类目二')
  })

  it('缺少查看权限不能打开详情，管理员保留查看能力', async () => {
    store.state.perms = []
    await loadPage()
    expect(wrapper.findAll('[data-view-policy]').every((entry) => !entry.isVisible())).toBe(true)
    wrapper.vm.openItemDetails(wrapper.vm.categoryRows[0])
    expect(wrapper.vm.itemDetail).toBeNull()
    store.state.role_codes = ['admin']
    wrapper.vm.openItemDetails(wrapper.vm.categoryRows[0])
    await flushPromises()
    expect(document.body.querySelector('.ant-modal-title').textContent).toBe('策略详情：策略一')
    expect(document.body.querySelector('.ant-modal-footer')).toBeNull()
  })
})

describe('扫描项目与环境联动', () => {
  function openProjectScan() {
    wrapper.vm.openScanModal(baseline)
    wrapper.vm.scanMode = 'project'
  }
  beforeEach(() => {
    const originalGet = requestUtil.get.getMockImplementation()
    requestUtil.get.mockImplementation((url, params) => {
      const page = params?.page || 1
      if (url === 'assets/projects/') {
        return Promise.resolve(response({ totalPages: 2, results: page === 1
          ? [{ id: 301, name: '测试项目', business_systems: [7] }]
          : [{ id: 302, name: '生产项目', business_systems: [8] }, { id: 303, name: '空项目', business_systems: [] }] }))
      }
      if (url === 'assets/business-environments/') {
        return Promise.resolve(response({ totalPages: 2, results: page === 1
          ? [{ id: 71, name: 'test' }]
          : [{ id: 72, name: 'prod' }, { id: 73, name: '无关联环境' }] }))
      }
      if (url === 'assets/application-services/') {
        return Promise.resolve(response({ totalPages: 2, results: page === 1
          ? [{ id: 1, business_system: 7, environment: 72, enabled: false }, { id: 2, business_system: 8, environment: 72, enabled: true }]
          : [{ id: 3, business_system: 7, environment: 71, enabled: true }] }))
      }
      return originalGet(url, params)
    })
  })

  it('完整读取分页资产，只显示所选项目的启用服务环境，空项目不展示全局环境', async () => {
    await loadPage()
    openProjectScan()
    await flushPromises()
    expect(wrapper.vm.projectOptions.map((option) => option.value)).toEqual([301, 302, 303])
    expect(wrapper.vm.environmentOptions).toEqual([])
    wrapper.vm.scanForm.project_id = 301
    await flushPromises()
    expect(wrapper.vm.environmentOptions).toEqual([{ label: 'test', value: 71 }])
    expect(requestUtil.get).toHaveBeenCalledWith('assets/application-services/',
      expect.objectContaining({ enabled: true, page: 2 }))
    wrapper.vm.scanForm.project_id = 302
    await flushPromises()
    expect(wrapper.vm.environmentOptions).toEqual([{ label: 'prod', value: 72 }])
    wrapper.vm.scanForm.project_id = 303
    await flushPromises()
    expect(wrapper.vm.environmentOptions).toEqual([])
  })

  it('切换项目清掉无效环境，留空时仍提交项目全环境扫描', async () => {
    await loadPage()
    openProjectScan()
    await flushPromises()
    wrapper.vm.scanForm.project_id = 301
    wrapper.vm.scanForm.environment_id = 71
    await flushPromises()
    await wrapper.vm.submitScan()
    expect(requestUtil.post).toHaveBeenLastCalledWith('sys/security/baseline/7/scan/',
      { mount_type: 'environment', project_id: 301, environment_id: 71, host_ids: [] })
    wrapper.vm.scanForm.project_id = 302
    expect(wrapper.vm.scanForm.environment_id).toBeUndefined()
    await flushPromises()
    await wrapper.vm.submitScan()
    expect(requestUtil.post).toHaveBeenLastCalledWith('sys/security/baseline/7/scan/',
      { mount_type: 'project', project_id: 302, environment_id: null, host_ids: [] })
  })

  it('默认按主机且全部，支持部分主机，项目分支不携带主机选择', async () => {
    await loadPage()
    wrapper.vm.openScanModal(baseline)
    await flushPromises()
    expect(wrapper.vm.scanMode).toBe('host')
    const labels = () => wrapper.findAllComponents(FormItem).map((item) => item.props('label'))
    expect(labels()).toContain('主机（默认全部）')
    expect(labels()).not.toContain('项目')
    expect(labels()).not.toContain('环境（可选）')
    expect(wrapper.vm.scanForm.host_ids).toEqual([])
    expect(wrapper.vm.scanHostOptions).toHaveLength(2)
    wrapper.vm.scanForm.host_ids = [11]
    await wrapper.vm.submitScan()
    expect(requestUtil.post).toHaveBeenLastCalledWith('sys/security/baseline/7/scan/',
      { mount_type: 'host', host_ids: [11] })
    wrapper.vm.scanMode = 'project'
    expect(wrapper.vm.scanForm.host_ids).toEqual([])
    expect(wrapper.vm.scanHostsLoaded).toBe(false)
    await flushPromises()
    expect(wrapper.vm.scanHostsLoaded).toBe(false)
    expect(labels()).toContain('项目')
    expect(labels()).toContain('环境（可选）')
    expect(labels()).not.toContain('主机（默认全部）')
    wrapper.vm.scanForm.project_id = 301
    await flushPromises()
    await wrapper.vm.submitScan()
    expect(requestUtil.post).toHaveBeenLastCalledWith('sys/security/baseline/7/scan/',
      { mount_type: 'project', project_id: 301, environment_id: null, host_ids: [] })
    wrapper.vm.scanMode = 'host'
    await flushPromises()
    await wrapper.vm.submitScan()
    expect(requestUtil.post).toHaveBeenLastCalledWith('sys/security/baseline/7/scan/',
      { mount_type: 'host', host_ids: [] })
  })

  it('主机加载失败不发起扫描，旧范围的响应不能覆盖新范围', async () => {
    await loadPage()
    openProjectScan()
    await flushPromises()
    let resolveOld
    const get = requestUtil.get.getMockImplementation()
    requestUtil.get.mockImplementation((url, params) => url.endsWith('/scan-hosts/')
      ? new Promise((resolve) => { resolveOld = resolve })
      : get(url, params))
    wrapper.vm.scanForm.project_id = 301
    const error = vi.spyOn(message, 'error').mockImplementation(() => {})
    vi.spyOn(message, 'warning').mockImplementation(() => {})
    requestUtil.get.mockImplementation((url, params) => url.endsWith('/scan-hosts/')
      ? Promise.reject(new Error('主机加载失败'))
      : get(url, params))
    wrapper.vm.scanForm.project_id = 302
    await flushPromises()
    resolveOld(response({ hosts: [{ id: 99 }] }))
    await flushPromises()
    expect(wrapper.vm.scanHosts).toEqual([])
    expect(wrapper.vm.scanHostsLoaded).toBe(false)
    expect(error).toHaveBeenCalledWith('主机加载失败')
    requestUtil.post.mockClear()
    await wrapper.vm.submitScan()
    expect(requestUtil.post.mock.calls.some(([url]) => url.endsWith('/7/scan/'))).toBe(false)
  })

  it('按主机不加载项目资产，迟到项目响应不重置主机选择', async () => {
    await loadPage()
    requestUtil.get.mockClear()
    wrapper.vm.openScanModal(baseline)
    await flushPromises()
    expect(requestUtil.get).toHaveBeenCalledTimes(1)
    expect(requestUtil.get).toHaveBeenLastCalledWith('sys/security/baseline/scan-hosts/', { mount_type: 'host' })
    expect(requestUtil.post).not.toHaveBeenCalled()
    wrapper.vm.scanMode = 'project'
    await flushPromises()
    wrapper.vm.scanMode = 'host'
    await flushPromises()
    wrapper.vm.scanForm.host_ids = [11]
    wrapper.vm.scanScopeLoaded = false
    await flushPromises()
    expect(wrapper.vm.scanForm.host_ids).toEqual([11])
    await wrapper.vm.submitScan()
    expect(requestUtil.post).toHaveBeenLastCalledWith('sys/security/baseline/7/scan/', { mount_type: 'host', host_ids: [11] })
  })

  it('空主机范围禁止提交', async () => {
    await loadPage()
    openProjectScan()
    await flushPromises()
    const get = requestUtil.get.getMockImplementation()
    requestUtil.get.mockImplementation((url, params) => url.endsWith('/scan-hosts/')
      ? Promise.resolve(response({ hosts: [] }))
      : get(url, params))
    wrapper.vm.scanForm.project_id = 303
    await flushPromises()
    requestUtil.post.mockClear()
    const warning = vi.spyOn(message, 'warning').mockImplementation(() => {})
    await wrapper.vm.submitScan()
    expect(warning).toHaveBeenCalledWith('所选范围没有可扫描主机')
    expect(requestUtil.post.mock.calls.some(([url]) => url.endsWith('/7/scan/'))).toBe(false)
  })

  it('范围加载失败时提示错误并阻止发起扫描，重开弹窗可重试', async () => {
    await loadPage()
    const get = requestUtil.get.getMockImplementation()
    const error = vi.spyOn(message, 'error').mockImplementation(() => {})
    vi.spyOn(message, 'warning').mockImplementation(() => {})
    requestUtil.get.mockImplementation((url, params) => url === 'assets/application-services/'
      ? Promise.reject(new Error('服务名单加载失败')) : get(url, params))
    openProjectScan()
    await flushPromises()
    expect(error).toHaveBeenCalledWith('服务名单加载失败')
    expect(wrapper.vm.scanScopeLoaded).toBe(false)
    wrapper.vm.scanForm.project_id = 301
    await wrapper.vm.submitScan()
    expect(requestUtil.post.mock.calls.some(([url]) => url.endsWith('/7/scan/'))).toBe(false)
    requestUtil.get.mockImplementation(get)
    openProjectScan()
    await flushPromises()
    expect(wrapper.vm.scanScopeLoaded).toBe(true)
    expect(wrapper.vm.scanForm.project_id).toBeUndefined()
    expect(wrapper.vm.scanForm.environment_id).toBeUndefined()
  })

  it('弹窗重开后忽略旧资产响应，加载期间禁止提交', async () => {
    await loadPage()
    const get = requestUtil.get.getMockImplementation()
    const warning = vi.spyOn(message, 'warning').mockImplementation(() => {})
    let resolveOld
    requestUtil.get.mockImplementation((url, params) => url === 'assets/application-services/'
      ? new Promise((resolve) => { resolveOld = resolve }) : get(url, params))
    openProjectScan()
    await flushPromises()
    wrapper.vm.scanForm.project_id = 301
    await wrapper.vm.submitScan()
    expect(warning).toHaveBeenCalledWith('扫描范围尚未加载完成，请稍后重试')
    expect(requestUtil.post.mock.calls.some(([url]) => url.endsWith('/7/scan/'))).toBe(false)
    requestUtil.get.mockImplementation(get)
    openProjectScan()
    await flushPromises()
    wrapper.vm.scanForm.project_id = 301
    resolveOld(response({ results: [{ business_system: 7, environment: 72, enabled: true }] }))
    await flushPromises()
    expect(wrapper.vm.environmentOptions).toEqual([{ label: 'test', value: 71 }])
    expect(wrapper.vm.scanScopeLoading).toBe(false)
  })
})

describe('策略上下移动', () => {
  it('保存移动方向并重新读取排序，刷新后保持顺序且不影响其他类目', async () => {
    await loadPage()
    requestUtil.patch.mockImplementation(() => {
      savedItems = [items[1], items[0], items[2]]
      return Promise.resolve(response({ id: 89 }))
    })
    const upButton = wrapper.findAllComponents(Tooltip).filter((tooltip) => tooltip.props('title') === '上移')[1]
    expect(upButton.props('placement')).toBe('top')
    await upButton.find('button').trigger('click')
    await flushPromises()
    expect(requestUtil.patch).toHaveBeenCalledWith('sys/security/baseline/7/items/89/', { direction: 'up' })
    expect(wrapper.vm.categoryRows.map((item) => item.id)).toEqual([89, 88])
    wrapper.vm.selectBaseline(baseline)
    await flushPromises()
    expect(wrapper.vm.categoryRows.map((item) => item.id)).toEqual([89, 88])
    wrapper.vm.selectCategoryById('4')
    expect(wrapper.vm.categoryRows.map((item) => item.id)).toEqual([90])
  })

  it('首尾按钮禁用，移动失败提示错误且不改变顺序', async () => {
    await loadPage()
    const buttons = wrapper.findAllComponents(Tooltip)
    const upButtons = buttons.filter((button) => button.props('title') === '上移')
    const downButtons = buttons.filter((button) => button.props('title') === '下移')
    expect(upButtons).toHaveLength(2)
    expect(upButtons[0].find('button').attributes('disabled')).toBeDefined()
    expect(downButtons[1].find('button').attributes('disabled')).toBeDefined()
    const error = vi.spyOn(message, 'error').mockImplementation(() => {})
    requestUtil.patch.mockRejectedValue(new Error('移动失败'))
    await wrapper.vm.moveItem(wrapper.vm.categoryRows[1], 'up')
    expect(error).toHaveBeenCalledWith('移动失败')
    expect(wrapper.vm.categoryRows.map((item) => item.id)).toEqual([88, 89])
    expect(wrapper.vm.itemsMoving).toBe(false)
  })

  it('跨分页移动按完整类目列表定位，移动期间禁止重复请求', async () => {
    savedItems = Array.from({ length: 12 }, (_, index) => ({
      ...items[0], id: index + 100, name: `策略${index + 1}`,
    }))
    await loadPage()
    let complete
    requestUtil.patch.mockImplementation(() => new Promise((resolve) => { complete = resolve }))
    const record = wrapper.vm.categoryRows[9]
    const pending = wrapper.vm.moveItem(record, 'down')
    expect(wrapper.vm.itemsMoving).toBe(true)
    await wrapper.vm.moveItem(record, 'down')
    expect(requestUtil.patch).toHaveBeenCalledTimes(1)
    ;[savedItems[9], savedItems[10]] = [savedItems[10], savedItems[9]]
    complete(response({ id: record.id }))
    await pending
    expect(wrapper.vm.categoryRows[10].id).toBe(record.id)
    expect(wrapper.vm.itemsMoving).toBe(false)
  })
})
