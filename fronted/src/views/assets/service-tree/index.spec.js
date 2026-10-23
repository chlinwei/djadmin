import { flushPromises, mount } from '@vue/test-utils'
import Antd from 'ant-design-vue'
import { createMemoryHistory, createRouter } from 'vue-router'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

vi.mock('@/api/assets/application', () => ({
  batchDeleteBusinessSystems: vi.fn(() => Promise.resolve({ data: { data: null } })),
  batchDeleteApplicationServices: vi.fn(() => Promise.resolve({ data: { data: { results: [{ id: 21, ok: true }] } } })),
}))

vi.mock('@/util/deleteConfirm', () => ({
  openDeleteConfirm: vi.fn(),
}))

vi.mock('echarts', () => ({
  init: vi.fn(() => ({ setOption: vi.fn(), resize: vi.fn(), dispose: vi.fn() })),
}))

import * as applicationApi from '@/api/assets/application'
import { openDeleteConfirm } from '@/util/deleteConfirm'
import { init as initChart } from 'echarts'
import ServiceTreePage from './index.vue'

let panelObserver
beforeEach(() => {
  vi.stubGlobal('ResizeObserver', class {
    constructor(callback) {
      this.callback = callback
      this.observe = vi.fn()
      this.disconnect = vi.fn()
      panelObserver = this
    }
  })
})
afterEach(() => vi.unstubAllGlobals())

const ServiceTreeStub = {
  name: 'ServiceTree',
  emits: ['select', 'stats-change'],
  template: '<div class="service-tree-stub" />',
  methods: { refresh: vi.fn() },
}
const ServiceTreeNodeContentStub = {
  name: 'ServiceTreeNodeContent',
  props: ['scope'],
  emits: ['navigate', 'edit-business-system', 'delete-business-system', 'edit-service', 'delete-service'],
  template: '<div class="node-content-stub" />',
  methods: { refresh: vi.fn() },
}
const BusinessSystemDialogStub = {
  name: 'BusinessSystemDialog',
  props: ['open', 'systemId', 'initialProjectId'],
  template: '<div class="business-system-dialog-stub" />',
}
const ApplicationServiceDialogStub = {
  name: 'ApplicationServiceDialog',
  props: ['open', 'serviceId', 'initialBusinessSystemId', 'initialEnvironmentId'],
  template: '<div class="application-service-dialog-stub" />',
}

async function mountPage(query = {}) {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [{ path: '/assets/service-tree', component: ServiceTreePage }],
  })
  router.push({ path: '/assets/service-tree', query })
  await router.isReady()
  return mount(ServiceTreePage, {
    global: {
      plugins: [Antd, router],
      stubs: {
        ServiceTree: ServiceTreeStub,
        ServiceTreeNodeContent: ServiceTreeNodeContentStub,
        BusinessSystemDialog: BusinessSystemDialogStub,
        ApplicationServiceDialog: ApplicationServiceDialogStub,
        FontAwesomeIcon: true,
      },
      directives: {
        permission: () => {},
      },
    },
  })
}

describe('service-tree capacity charts', () => {
  it('refreshes the new scope instead of requesting the deleted service', async () => {
    const wrapper = await mountPage()
    await flushPromises()
    const tree = wrapper.findComponent(ServiceTreeStub)
    const content = wrapper.findComponent(ServiceTreeNodeContentStub)
    tree.vm.$emit('select', { nodeType: 'service', applicationServiceId: 21, nodeTitle: '订单 API' })
    await flushPromises()
    const scopes = []
    ServiceTreeNodeContentStub.methods.refresh.mockImplementation(function () {
      scopes.push(this.scope.nodeType)
    })
    try {
      content.vm.$emit('delete-service', { id: 21, name: '订单 API' })
      await flushPromises()
      await openDeleteConfirm.mock.calls.at(-1)[0].onConfirm()
      expect(scopes).toEqual(['all'])
      expect(content.props('scope').nodeType).toBe('all')
    } finally {
      ServiceTreeNodeContentStub.methods.refresh.mockReset()
      wrapper.unmount()
    }
  })

  describe('service-tree panel layout', () => {
    async function mountSizedPage(width = 1200) {
      const wrapper = await mountPage()
      Object.defineProperty(wrapper.find('.service-tree-page').element, 'clientWidth', { configurable: true, value: width })
      panelObserver.callback()
      await flushPromises()
      return wrapper
    }

    it('enlarges and restores the manual width, clamping keyboard changes to available space', async () => {
      const wrapper = await mountSizedPage()
      const divider = wrapper.get('[role="separator"]')
      expect(divider.attributes('aria-valuenow')).toBe('260')
      await divider.trigger('keydown', { key: 'ArrowRight' })
      expect(divider.attributes('aria-valuenow')).toBe('280')
      await wrapper.get('[aria-label="放大面板"]').trigger('click')
      expect(divider.attributes('aria-valuenow')).toBe('640')
      await wrapper.get('[aria-label="还原宽度"]').trigger('click')
      expect(divider.attributes('aria-valuenow')).toBe('280')
      await divider.trigger('keydown', { key: 'End' })
      await divider.trigger('keydown', { key: 'ArrowRight' })
      expect(divider.attributes('aria-valuenow')).toBe('640')
      Object.defineProperty(wrapper.find('.service-tree-page').element, 'clientWidth', { value: 800 })
      panelObserver.callback()
      await flushPromises()
      expect(divider.attributes('aria-valuenow')).toBe('424')
      await divider.trigger('keydown', { key: 'Home' })
      await divider.trigger('keydown', { key: 'ArrowLeft' })
      expect(divider.attributes('aria-valuenow')).toBe('260')
      wrapper.unmount()
      expect(panelObserver.disconnect).toHaveBeenCalled()
    })

    it('hides without remounting the tree or losing the selected scope and width', async () => {
      const wrapper = await mountSizedPage()
      const tree = wrapper.findComponent(ServiceTreeStub)
      tree.vm.$emit('select', { nodeType: 'service', applicationServiceId: 21, nodeTitle: '订单 API' })
      await wrapper.get('[role="separator"]').trigger('keydown', { key: 'ArrowRight' })
      await wrapper.get('[aria-label="收起树面板"]').trigger('click')
      expect(wrapper.get('#service-tree-navigation').attributes('style')).toContain('display: none')
      expect(wrapper.find('[role="separator"]').exists()).toBe(false)
      expect(wrapper.findComponent(ServiceTreeStub).vm).toBe(tree.vm)
      expect(wrapper.findComponent(ServiceTreeNodeContentStub).props('scope').applicationServiceId).toBe(21)
      await wrapper.get('[aria-label="展开树面板"]').trigger('click')
      expect(wrapper.findComponent(ServiceTreeStub).vm).toBe(tree.vm)
      expect(wrapper.get('[role="separator"]').attributes('aria-valuenow')).toBe('280')
      wrapper.unmount()
    })

    it('drags with pointer capture, bounds width, and releases capture on unmount', async () => {
      const wrapper = await mountSizedPage()
      const divider = wrapper.get('[role="separator"]')
      divider.element.setPointerCapture = vi.fn()
      divider.element.hasPointerCapture = vi.fn(() => true)
      divider.element.releasePointerCapture = vi.fn()
      await divider.trigger('pointerdown', { button: 0, pointerId: 1, clientX: 260 })
      expect(divider.element.setPointerCapture).toHaveBeenCalledWith(1)
      await divider.trigger('pointermove', { pointerId: 1, clientX: 400 })
      expect(divider.attributes('aria-valuenow')).toBe('400')
      await divider.trigger('pointermove', { pointerId: 1, clientX: 2000 })
      expect(divider.attributes('aria-valuenow')).toBe('640')
      await divider.trigger('pointerup', { pointerId: 1 })
      expect(divider.element.releasePointerCapture).toHaveBeenCalledWith(1)
      await divider.trigger('pointermove', { pointerId: 1, clientX: 300 })
      expect(divider.attributes('aria-valuenow')).toBe('640')
      await divider.trigger('pointerdown', { button: 0, pointerId: 2, clientX: 640 })
      wrapper.unmount()
      expect(divider.element.releasePointerCapture).toHaveBeenCalledWith(2)
    })
  })

  it('preserves pie charts while showing deduplicated totals and explaining overlapping groups', async () => {
    const wrapper = await mountPage()
    await flushPromises()
    const tree = wrapper.findComponent(ServiceTreeStub)
    tree.vm.$emit('stats-change', {
      cpuRows: [{ name: 'API', value: 12 }, { name: 'DB', value: 12 }],
      memoryRows: [{ name: 'API', value: 16 }, { name: 'DB', value: 16 }],
      totalCpu: 12, totalMemory: 16,
    })
    await flushPromises()
    expect(wrapper.text()).toContain('去重总 CPU 12.00 核')
    expect(wrapper.text()).toContain('去重总内存 16.00 GB')
    expect(wrapper.text()).toContain('组间可能重叠')
    expect(wrapper.text()).toContain('饼图百分比以各组容量之和为分母')
    const charts = initChart.mock.results.slice(-2).map((result) => result.value)
    for (const [index, values] of [[0, [12, 12]], [1, [16, 16]]]) {
      const option = charts[index].setOption.mock.calls.at(-1)[0]
      expect(option.series[0].type).toBe('pie')
      expect(option.series[0].radius).toEqual(['40%', '72%'])
      expect(option.series[0].data).toEqual([
        { name: 'API', value: values[0] }, { name: 'DB', value: values[1] },
      ])
      expect(option.tooltip.formatter({ name: 'API', value: values[0], percent: 50 })).toContain('归集占比 50%')
    }
    tree.vm.$emit('stats-change', { cpuRows: [], memoryRows: [], totalCpu: 0, totalMemory: 0 })
    await flushPromises()
    expect(charts[0].setOption.mock.calls.at(-1)[0].series[0].data).toEqual([])
    expect(wrapper.text()).toContain('去重总 CPU 0.00 核')
    wrapper.unmount()
  })
})

describe('service-tree/index.vue 业务系统 CRUD 入口', () => {
  it('全部业务/项目节点显示新增业务系统按钮，其他节点隐藏', async () => {
    const wrapper = await mountPage()
    await flushPromises()

    expect(wrapper.text()).toContain('新增业务系统')

    await wrapper.findComponent(ServiceTreeStub).vm.$emit('select', {
      nodeType: 'businessSystem', businessSystemId: 7, businessSystemName: '订单系统', nodeTitle: '订单系统',
    })
    await flushPromises()
    expect(wrapper.text()).not.toContain('新增业务系统')

    await wrapper.findComponent(ServiceTreeStub).vm.$emit('select', {
      nodeType: 'project', projectId: 5, nodeTitle: '示例项目',
    })
    await flushPromises()
    expect(wrapper.text()).toContain('新增业务系统')
  })

  it('新增业务系统在项目节点下会预填所属项目，其余场景不预填', async () => {
    const wrapper = await mountPage()
    await flushPromises()

    const createButton = wrapper.findAll('button').find((btn) => btn.text().includes('新增业务系统'))
    await createButton.trigger('click')
    await flushPromises()
    let dialog = wrapper.findComponent(BusinessSystemDialogStub)
    expect(dialog.props('open')).toBe(true)
    expect(dialog.props('systemId')).toBeNull()
    expect(dialog.props('initialProjectId')).toBeNull()

    await dialog.vm.$emit('update:open', false)
    await wrapper.findComponent(ServiceTreeStub).vm.$emit('select', { nodeType: 'project', projectId: 5, nodeTitle: '示例项目' })
    await flushPromises()
    await wrapper.findAll('button').find((btn) => btn.text().includes('新增业务系统')).trigger('click')
    await flushPromises()
    dialog = wrapper.findComponent(BusinessSystemDialogStub)
    expect(dialog.props('systemId')).toBeNull()
    expect(dialog.props('initialProjectId')).toBe(5)
  })

  it('子内容触发编辑事件后打开对话框并回填 systemId', async () => {
    const wrapper = await mountPage()
    await flushPromises()

    await wrapper.findComponent(ServiceTreeNodeContentStub).vm.$emit('edit-business-system', { id: 7, name: '订单系统' })
    await flushPromises()

    const dialog = wrapper.findComponent(BusinessSystemDialogStub)
    expect(dialog.props('open')).toBe(true)
    expect(dialog.props('systemId')).toBe(7)
  })

  it('子内容触发删除事件后调用删除确认，确认后请求删除接口并刷新树', async () => {
    const wrapper = await mountPage()
    await flushPromises()

    await wrapper.findComponent(ServiceTreeNodeContentStub).vm.$emit('delete-business-system', { id: 7, name: '订单系统' })
    await flushPromises()

    expect(openDeleteConfirm).toHaveBeenCalledWith(expect.objectContaining({
      title: '删除业务系统',
      items: ['订单系统'],
    }))

    const { onConfirm } = openDeleteConfirm.mock.calls.at(-1)[0]
    await onConfirm()

    expect(applicationApi.batchDeleteBusinessSystems).toHaveBeenCalledWith([7])
  })
})

describe('service-tree/index.vue 逻辑服务 CRUD 入口', () => {
  it('全部业务/项目/业务系统/环境节点显示新增逻辑服务按钮，service/deployment 节点隐藏', async () => {
    const wrapper = await mountPage()
    await flushPromises()

    expect(wrapper.text()).toContain('新增逻辑服务')

    await wrapper.findComponent(ServiceTreeStub).vm.$emit('select', {
      nodeType: 'businessSystem', businessSystemId: 7, businessSystemName: '订单系统', nodeTitle: '订单系统',
    })
    await flushPromises()
    expect(wrapper.text()).toContain('新增逻辑服务')

    await wrapper.findComponent(ServiceTreeStub).vm.$emit('select', {
      nodeType: 'service', applicationServiceId: 21, nodeTitle: '订单 API',
    })
    await flushPromises()
    expect(wrapper.text()).not.toContain('新增逻辑服务')
  })

  it('新增逻辑服务按当前树选中的业务系统/环境预填', async () => {
    const wrapper = await mountPage()
    await flushPromises()

    await wrapper.findComponent(ServiceTreeStub).vm.$emit('select', {
      nodeType: 'environment', businessSystemId: 7, businessSystemName: '订单系统', environment: 72, environmentName: '测试环境', nodeTitle: '测试环境',
    })
    await flushPromises()
    await wrapper.findAll('button').find((btn) => btn.text().includes('新增逻辑服务')).trigger('click')
    await flushPromises()

    const dialog = wrapper.findComponent(ApplicationServiceDialogStub)
    expect(dialog.props('open')).toBe(true)
    expect(dialog.props('serviceId')).toBeNull()
    expect(dialog.props('initialBusinessSystemId')).toBe(7)
    expect(dialog.props('initialEnvironmentId')).toBe(72)
  })

  it('子内容触发编辑事件后打开对话框并回填 serviceId', async () => {
    const wrapper = await mountPage()
    await flushPromises()

    await wrapper.findComponent(ServiceTreeNodeContentStub).vm.$emit('edit-service', { id: 21, name: '订单 API' })
    await flushPromises()

    const dialog = wrapper.findComponent(ApplicationServiceDialogStub)
    expect(dialog.props('open')).toBe(true)
    expect(dialog.props('serviceId')).toBe(21)
  })

  it('子内容触发删除事件后调用删除确认，确认后请求删除接口并刷新树', async () => {
    const wrapper = await mountPage()
    await flushPromises()

    await wrapper.findComponent(ServiceTreeNodeContentStub).vm.$emit('delete-service', { id: 21, name: '订单 API' })
    await flushPromises()

    expect(openDeleteConfirm).toHaveBeenCalledWith(expect.objectContaining({
      title: '删除逻辑服务',
      summary: expect.stringContaining('永久删除该服务所有档位的数据流（data stream）、底层索引及全部日志'),
      items: ['订单 API'],
    }))

    const { onConfirm } = openDeleteConfirm.mock.calls.at(-1)[0]
    await onConfirm()

    expect(applicationApi.batchDeleteApplicationServices).toHaveBeenCalledWith([21])
  })

  it('服务清理失败时保留选中服务并将失败抛给公共确认流程', async () => {
    const wrapper = await mountPage({ application_service_id: '21', service_name: '订单 API' })
    await flushPromises()
    applicationApi.batchDeleteApplicationServices.mockResolvedValueOnce({
      data: { data: { count: 0, results: [{ id: 21, ok: false, message: 'agent offline，请重试' }] } },
    })
    wrapper.findComponent(ServiceTreeNodeContentStub).vm.$emit('delete-service', { id: 21, name: '订单 API' })
    await flushPromises()
    const options = openDeleteConfirm.mock.calls.at(-1)[0]
    expect(options.summary).toContain('共享实例')
    expect(options.summary).toContain('原始日志文件保留')
    await expect(options.onConfirm()).rejects.toThrow('agent offline')
    expect(wrapper.findComponent(ServiceTreeNodeContentStub).props('scope').applicationServiceId).toBe(21)
    wrapper.unmount()
  })
})

describe('service-tree/index.vue 深链定位', () => {
  it('带 application_service_id query 挂载时直接定位到该服务节点', async () => {
    const wrapper = await mountPage({
      application_service_id: '21',
      service_name: '订单 API',
      business_system_id: '7',
      environment_id: '72',
      environment_name: '测试环境',
    })
    await flushPromises()
    // 内容面板收到的 scope 应是 service 节点（树的选中态由 scopeKey 反推，用同一份 scope）。
    const scope = wrapper.findComponent(ServiceTreeNodeContentStub).props('scope')
    expect(scope).toEqual(expect.objectContaining({
      nodeType: 'service',
      applicationServiceId: 21,
      nodeTitle: '订单 API',
      businessSystemId: 7,
      environment: 72,
      environmentName: '测试环境',
    }))
    wrapper.unmount()
  })

  it('无 query 挂载保持默认全部业务节点', async () => {
    const wrapper = await mountPage()
    await flushPromises()
    const scope = wrapper.findComponent(ServiceTreeNodeContentStub).props('scope')
    expect(scope.nodeType).toBe('all')
    wrapper.unmount()
  })
})
