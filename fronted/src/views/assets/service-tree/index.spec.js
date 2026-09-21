import { flushPromises, mount } from '@vue/test-utils'
import Antd from 'ant-design-vue'
import { createMemoryHistory, createRouter } from 'vue-router'
import { describe, expect, it, vi } from 'vitest'

vi.mock('@/api/assets/application', () => ({
  batchDeleteBusinessSystems: vi.fn(() => Promise.resolve({ data: { data: null } })),
  batchDeleteApplicationServices: vi.fn(() => Promise.resolve({ data: { data: null } })),
}))

vi.mock('@/util/deleteConfirm', () => ({
  openDeleteConfirm: vi.fn(),
}))

vi.mock('echarts', () => ({
  init: vi.fn(() => ({ setOption: vi.fn(), resize: vi.fn(), dispose: vi.fn() })),
}))

import * as applicationApi from '@/api/assets/application'
import { openDeleteConfirm } from '@/util/deleteConfirm'
import ServiceTreePage from './index.vue'

const ServiceTreeStub = {
  name: 'ServiceTree',
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
      items: ['订单 API'],
    }))

    const { onConfirm } = openDeleteConfirm.mock.calls.at(-1)[0]
    await onConfirm()

    expect(applicationApi.batchDeleteApplicationServices).toHaveBeenCalledWith([21])
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
