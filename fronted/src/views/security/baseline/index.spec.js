import { flushPromises, mount } from '@vue/test-utils'
import Antd, { message, Tooltip } from 'ant-design-vue'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import BaselinePage from './index.vue'
import requestUtil from '@/util/request'

vi.mock('@/store', () => ({ default: { state: { user: { timezone: 'Asia/Shanghai' } } } }))
vi.mock('vue-router', () => ({ useRouter: () => ({ push: vi.fn() }) }))
vi.mock('@/util/request', () => ({ default: { get: vi.fn(), patch: vi.fn(), post: vi.fn() } }))

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
  savedItems = [...items]
  requestUtil.get.mockImplementation((url) => Promise.resolve(response(
    url === 'sys/security/baseline/7/'
      ? { categories: [{ id: 3, name: '类目一' }, { id: 4, name: '类目二' }], items: savedItems }
      : { results: url === 'sys/security/baseline/' ? [baseline] : [] },
  )))
  requestUtil.post.mockResolvedValue(response({ targets: 1 }))
})

afterEach(() => {
  wrapper?.unmount()
  vi.restoreAllMocks()
})

async function loadPage() {
  wrapper = mount(BaselinePage, {
    global: { plugins: [Antd], stubs: { FontAwesomeIcon: true }, directives: { permission: {} } },
    attachTo: document.body,
  })
  await flushPromises()
  wrapper.vm.selectBaseline(baseline)
  await flushPromises()
}

describe('扫描项目与环境联动', () => {
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
    wrapper.vm.openScanModal(baseline)
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
    wrapper.vm.openScanModal(baseline)
    await flushPromises()
    wrapper.vm.scanForm.project_id = 301
    wrapper.vm.scanForm.environment_id = 71
    await wrapper.vm.submitScan()
    expect(requestUtil.post).toHaveBeenLastCalledWith('sys/security/baseline/7/scan/',
      { mount_type: 'environment', project_id: 301, environment_id: 71 })
    wrapper.vm.scanForm.project_id = 302
    expect(wrapper.vm.scanForm.environment_id).toBeUndefined()
    await wrapper.vm.submitScan()
    expect(requestUtil.post).toHaveBeenLastCalledWith('sys/security/baseline/7/scan/',
      { mount_type: 'project', project_id: 302, environment_id: null })
  })

  it('范围加载失败时提示错误并阻止发起扫描，重开弹窗可重试', async () => {
    await loadPage()
    const get = requestUtil.get.getMockImplementation()
    const error = vi.spyOn(message, 'error').mockImplementation(() => {})
    vi.spyOn(message, 'warning').mockImplementation(() => {})
    requestUtil.get.mockImplementation((url, params) => url === 'assets/application-services/'
      ? Promise.reject(new Error('服务名单加载失败')) : get(url, params))
    wrapper.vm.openScanModal(baseline)
    await flushPromises()
    expect(error).toHaveBeenCalledWith('服务名单加载失败')
    expect(wrapper.vm.scanScopeLoaded).toBe(false)
    wrapper.vm.scanForm.project_id = 301
    await wrapper.vm.submitScan()
    expect(requestUtil.post).not.toHaveBeenCalled()
    requestUtil.get.mockImplementation(get)
    wrapper.vm.openScanModal(baseline)
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
    wrapper.vm.openScanModal(baseline)
    await flushPromises()
    wrapper.vm.scanForm.project_id = 301
    await wrapper.vm.submitScan()
    expect(warning).toHaveBeenCalledWith('扫描范围尚未加载完成，请稍后重试')
    expect(requestUtil.post).not.toHaveBeenCalled()
    requestUtil.get.mockImplementation(get)
    wrapper.vm.openScanModal(baseline)
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
