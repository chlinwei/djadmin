import { flushPromises, mount } from '@vue/test-utils'
import Antd, { RadioGroup, Select, Table, Tooltip } from 'ant-design-vue'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

vi.mock('@/api/assets/application', () => ({
  getProjectList: vi.fn(),
  getBusinessEnvironmentList: vi.fn(),
  getBusinessSystemList: vi.fn(),
  getApplicationServiceList: vi.fn(),
}))
vi.mock('@/api/assets/serviceDependencies', () => ({
  getServiceDependencyTopology: vi.fn(),
  getServiceDependencyTypes: vi.fn(),
  deleteServiceDependencies: vi.fn(),
  deleteServiceDependencyTypes: vi.fn(),
  saveServiceDependency: vi.fn(),
  saveServiceDependencyType: vi.fn(),
}))
vi.mock('@/directives/permission/permission', () => ({ checkPermission: vi.fn(() => true) }))
vi.mock('@/util/deleteConfirm', async (original) => ({ ...await original(), openDeleteConfirm: vi.fn() }))
vi.mock('@/util/timezone', () => ({ formatTimeWithTimezone: vi.fn((value, tz) => `${value} [${tz}]`) }))

import * as catalogApi from '@/api/assets/application'
import * as api from '@/api/assets/serviceDependencies'
import { checkPermission } from '@/directives/permission/permission'
import { openDeleteConfirm } from '@/util/deleteConfirm'
import { formatTimeWithTimezone } from '@/util/timezone'
import store from '@/store'
import Workspace from './ServiceDependencyWorkspace.vue'

const envelope = (data) => ({ data: { code: 200, msg: 'success', data } })
const page = (results) => envelope({ results, totalPages: 1 })
const types = [{ id: 1, code: 'http', name: '服务调用', enabled: true }]
const makeNode = (id, name, business = 10) => ({
  id, name, code: `service-${id}`, enabled: true, business_system_id: business, business_system_name: `业务 ${business}`,
  runtime_summary: { total: 1, running: 0, stopped: 0, error: 1, unknown: 0 },
  instances: [{ id, instance_name: `实例 ${id}`, host_id: id, host_ip: '127.0.0.1', ha_role: 'primary', runtime_status: 'error', runtime_status_output: '检查脚本退出异常', last_status_check_time: '2026-04-01T00:00:00Z' }],
})
const edge = { id: 9, source_service_id: 1, target_service_id: 2, source_name: '订单', target_name: '缓存', interaction_type_id: 1, interaction_type_name: '服务调用', interaction_type_enabled: true, strength: 'strong', function_scope: '订单创建', protocol: 'HTTPS', failure_impact: '', fallback_plan: '' }
const topology = (nodes = [makeNode(1, '订单'), makeNode(2, '缓存', 20), makeNode(3, '孤立服务')], edges = [edge]) => envelope({ nodes, edges, snapshot_time: '2026-04-02T00:00:00Z', state_source: 'stored_runtime_snapshot' })
const graphStub = {
  name: 'ServiceDependencyGraph',
  props: ['nodes', 'edges', 'title', 'layoutKey'],
  emits: ['select-node', 'select-edge'],
  template: '<div class="graph-stub"><button v-for="node in nodes" :key="node.id" @click="$emit(\'select-node\', node)">{{node.name}}</button></div>',
}
const dialogStub = { name: 'ServiceDependencyDialog', props: ['open', 'record', 'services', 'types', 'initialSource'], emits: ['saved', 'update:open'], template: '<div class="dialog-stub" />' }
const typeStub = { name: 'ServiceDependencyTypes', props: ['open'], emits: ['changed', 'update:open'], template: '<div />' }
let wrappers = []
beforeEach(() => {
  vi.clearAllMocks()
  const computedStyle = window.getComputedStyle
  vi.spyOn(window, 'getComputedStyle').mockImplementation((element) => computedStyle(element))
  checkPermission.mockReturnValue(true)
  store.commit('add_user', { timezone: 'UTC' })
  catalogApi.getProjectList.mockResolvedValue(page([{ id: 1, name: '商城' }, { id: 2, name: '支付' }]))
  catalogApi.getBusinessEnvironmentList.mockResolvedValue(page([{ id: 3, name: '生产', enabled: true }, { id: 4, name: '预发', enabled: true }, { id: 5, name: 'poc', enabled: true }]))
  catalogApi.getBusinessSystemList.mockResolvedValue(page([{ id: 10, project: 1 }, { id: 20, project: 1 }, { id: 30, project: 2 }]))
  catalogApi.getApplicationServiceList.mockResolvedValue(page([
    { id: 1, business_system: 10, environment: 3 },
    { id: 2, business_system: 20, environment: 3 },
    { id: 3, business_system: 10, environment: 3 },
    { id: 4, business_system: 10, environment: 4 },
    { id: 5, business_system: 30, environment: 5 },
  ]))
  api.getServiceDependencyTypes.mockResolvedValue(page(types))
  api.getServiceDependencyTopology.mockResolvedValue(topology())
  api.deleteServiceDependencies.mockResolvedValue(envelope({ count: 1, results: [{ id: 9, ok: true }] }))
})
afterEach(() => {
  wrappers.forEach((wrapper) => wrapper.unmount())
  wrappers = []
  vi.restoreAllMocks()
  document.body.innerHTML = ''
})
async function create(scope = { nodeType: 'environment', businessSystemId: 10, environment: 3, nodeTitle: '订单业务' }) {
  const wrapper = mount(Workspace, {
    props: { scope },
    attachTo: document.body,
    global: { plugins: [Antd], stubs: { ServiceDependencyGraph: graphStub, ServiceDependencyDialog: dialogStub, ServiceDependencyTypes: typeStub, FontAwesomeIcon: true } },
  })
  wrappers.push(wrapper)
  await flushPromises()
  return wrapper
}
const clickText = async (wrapper, text) => {
  const button = wrapper.findAll('button').find((item) => item.text().replace(/\s+/g, '').includes(text))
  expect(button, `missing button ${text}`).toBeTruthy()
  await button.trigger('click')
  await flushPromises()
}

describe('service dependency workspace', () => {
  it('derives project from business, displays direct boundary nodes and uses the full environment for edit candidates', async () => {
    const wrapper = await create({ nodeType: 'service', businessSystemId: 10, environment: 3, applicationServiceId: 1, nodeTitle: '订单服务' })
    expect(api.getServiceDependencyTopology).toHaveBeenCalledWith({ project: 1, environment: 3 })
    expect(wrapper.findAllComponents(Select)[1].props('options')).toEqual([{ value: 3, label: '生产' }])
    const graph = wrapper.findComponent(graphStub)
    expect(graph.props('layoutKey')).toBe('[1,3,10,1]')
    expect(graph.props('nodes').map((node) => [node.id, node.external_to_selection])).toEqual([[1, false], [2, true]])
    expect(graph.props('edges')).toHaveLength(1)
    await clickText(wrapper, '新增依赖')
    const dialog = wrapper.findComponent(dialogStub)
    expect(dialog.props('services')).toHaveLength(3)
    expect(dialog.props('initialSource')).toBe(1)
    expect(dialog.props('types')).toEqual(types)
  })
  it('does not guess project/environment at the root and waits for both explicit selections', async () => {
    const wrapper = await create({ nodeType: 'all', nodeTitle: '全部业务' })
    expect(wrapper.text()).toContain('请先选择项目与环境')
    expect(api.getServiceDependencyTopology).not.toHaveBeenCalled()
    const selects = wrapper.findAllComponents(Select)
    expect(selects[1].props('options')).toEqual([])
    expect(selects[1].props('disabled')).toBe(true)
    selects[0].vm.$emit('update:value', 1)
    await flushPromises()
    expect(api.getServiceDependencyTopology).not.toHaveBeenCalled()
    selects[1].vm.$emit('update:value', 3)
    await flushPromises()
    expect(api.getServiceDependencyTopology).toHaveBeenCalledWith({ project: 1, environment: 3 })
    for (const select of wrapper.findAllComponents(Select)) expect(select.props('getPopupContainer')).toBeTypeOf('function')
  })
  it('automatically selects the sole project environment and loads its graph, not the global dictionary', async () => {
    const wrapper = await create({ nodeType: 'project', projectId: 2, nodeTitle: 'yilake' })
    const environment = wrapper.findAllComponents(Select)[1]
    expect(environment.props('options')).toEqual([{ value: 5, label: 'poc' }])
    expect(environment.props('value')).toBe(5)
    expect(api.getServiceDependencyTopology).toHaveBeenCalledTimes(1)
    expect(api.getServiceDependencyTopology).toHaveBeenCalledWith({ project: 2, environment: 5 })
    expect(wrapper.findComponent(graphStub).exists()).toBe(true)
  })
  it('narrows business candidates while retaining the whole project environment scope for cross-business dependencies', async () => {
    const wrapper = await create({ nodeType: 'businessSystem', businessSystemId: 20, projectId: 1 })
    expect(wrapper.findAllComponents(Select)[1].props('options')).toEqual([{ value: 3, label: '生产' }])
    expect(wrapper.findAllComponents(Select)[1].props('value')).toBe(3)
    expect(api.getServiceDependencyTopology).toHaveBeenCalledWith({ project: 1, environment: 3 })
    await wrapper.setProps({ scope: { nodeType: 'project', projectId: 1 } })
    await flushPromises()
    expect(wrapper.findAllComponents(Select)[1].props('options').map((option) => option.value)).toEqual([3, 4])
    expect(wrapper.findAllComponents(Select)[1].props('value')).toBeUndefined()
    expect(wrapper.findComponent(graphStub).exists()).toBe(false)
  })
  it('clears an invalid old environment when projects change and does not request the mismatched pair', async () => {
    const wrapper = await create({ nodeType: 'all', nodeTitle: '全部业务' })
    const [project, environment] = wrapper.findAllComponents(Select)
    project.vm.$emit('update:value', 1)
    await flushPromises()
    environment.vm.$emit('update:value', 3)
    await flushPromises()
    expect(api.getServiceDependencyTopology).toHaveBeenCalledTimes(1)
    project.vm.$emit('update:value', 2)
    await flushPromises()
    expect(environment.props('options')).toEqual([{ value: 5, label: 'poc' }])
    expect(environment.props('value')).toBe(5)
    expect(api.getServiceDependencyTopology).toHaveBeenCalledTimes(2)
    expect(api.getServiceDependencyTopology).toHaveBeenLastCalledWith({ project: 2, environment: 5 })
    expect(api.getServiceDependencyTopology).not.toHaveBeenCalledWith({ project: 2, environment: 3 })
    expect(wrapper.findComponent(graphStub).exists()).toBe(true)
    environment.vm.$emit('update:value', 3)
    await flushPromises()
    expect(environment.props('value')).toBe(5)
    expect(api.getServiceDependencyTopology).toHaveBeenCalledTimes(2)
    project.vm.$emit('update:value', undefined)
    await flushPromises()
    expect(environment.props('value')).toBeUndefined()
    expect(environment.props('options')).toEqual([])
    expect(wrapper.findComponent(graphStub).exists()).toBe(false)
    expect(api.getServiceDependencyTopology).toHaveBeenCalledTimes(2)
  })
  it('preserves an explicitly selected tree environment among multiple candidates', async () => {
    const wrapper = await create({ nodeType: 'environment', businessSystemId: 10, environment: 4 })
    expect(wrapper.findAllComponents(Select)[1].props('value')).toBe(4)
    expect(api.getServiceDependencyTopology).toHaveBeenCalledTimes(1)
    expect(api.getServiceDependencyTopology).toHaveBeenCalledWith({ project: 1, environment: 4 })
  })
  it('does not replace an unavailable explicit tree environment with the sole remaining candidate', async () => {
    const wrapper = await create({ nodeType: 'environment', projectId: 2, businessSystemId: 30, environment: 3 })
    expect(wrapper.findAllComponents(Select)[1].props('options')).toEqual([{ value: 5, label: 'poc' }])
    expect(wrapper.findAllComponents(Select)[1].props('value')).toBeUndefined()
    expect(wrapper.text()).toContain('当前节点环境不在可用范围')
    expect(api.getServiceDependencyTopology).not.toHaveBeenCalled()
  })
  it('does not omit later service pages or include disabled and unassigned environments', async () => {
    catalogApi.getBusinessEnvironmentList.mockResolvedValue(page([
      { id: 3, name: '生产', enabled: true }, { id: 4, name: '停用环境', enabled: false }, { id: 5, name: 'poc', enabled: true },
    ]))
    catalogApi.getApplicationServiceList.mockImplementation(async ({ page: number }) => envelope({
      results: number === 1
        ? [{ id: 1, business_system: 30, environment: 4 }, { id: 2, business_system: 30, environment: null }]
        : [{ id: 3, business_system: 30, environment: 5 }],
      totalPages: 2,
    }))
    const wrapper = await create({ nodeType: 'project', projectId: 2 })
    expect(catalogApi.getApplicationServiceList).toHaveBeenCalledWith({ page: 2, page_size: 100 })
    expect(wrapper.findAllComponents(Select)[1].props('options')).toEqual([{ value: 5, label: 'poc' }])
    expect(wrapper.findAllComponents(Select)[1].props('value')).toBe(5)
    expect(api.getServiceDependencyTopology).toHaveBeenCalledWith({ project: 2, environment: 5 })
  })
  it('reports an empty or failed service environment catalog instead of falling back to all environments', async () => {
    catalogApi.getApplicationServiceList.mockResolvedValueOnce(page([]))
    const wrapper = await create({ nodeType: 'project', projectId: 2 })
    expect(wrapper.findAllComponents(Select)[1].props('options')).toEqual([])
    expect(wrapper.text()).toContain('当前范围暂无可用的服务环境')
    catalogApi.getApplicationServiceList.mockRejectedValueOnce(new Error('服务目录不可用'))
    const failed = await create({ nodeType: 'project', projectId: 2 })
    expect(failed.text()).toContain('服务目录不可用')
    expect(api.getServiceDependencyTopology).not.toHaveBeenCalled()
  })
  it('invalidates an old environment response instead of showing it in the new scope', async () => {
    let resolveOld
    api.getServiceDependencyTopology.mockReturnValueOnce(new Promise((resolve) => { resolveOld = resolve }))
    const wrapper = await create()
    api.getServiceDependencyTopology.mockResolvedValueOnce(topology([makeNode(4, '预发服务')], []))
    await wrapper.setProps({ scope: { nodeType: 'environment', businessSystemId: 10, environment: 4 } })
    await flushPromises()
    expect(wrapper.findComponent(graphStub).props('nodes')[0].name).toBe('预发服务')
    resolveOld(topology([makeNode(1, '过期生产服务')], []))
    await flushPromises()
    expect(wrapper.text()).not.toContain('过期生产服务')
  })
  it('shows load errors explicitly and retries without displaying an empty success graph', async () => {
    api.getServiceDependencyTopology.mockRejectedValueOnce(new Error('数据库不可用'))
    const wrapper = await create()
    expect(wrapper.text()).toContain('数据库不可用')
    expect(wrapper.findComponent(graphStub).exists()).toBe(false)
    await clickText(wrapper, '重试')
    expect(wrapper.findComponent(graphStub).exists()).toBe(true)
  })
  it('shows instance output and user-timezone check timestamps, distinct from snapshot time', async () => {
    const wrapper = await create()
    wrapper.findComponent(graphStub).vm.$emit('select-node', makeNode(1, '订单'))
    await flushPromises()
    expect(document.body.textContent).toContain('检查异常')
    expect(document.body.textContent).toContain('检查脚本退出异常')
    expect(document.body.textContent).toContain('上次检查')
    expect(formatTimeWithTimezone).toHaveBeenCalledWith('2026-04-01T00:00:00Z', 'UTC')
    expect(formatTimeWithTimezone).toHaveBeenCalledWith('2026-04-02T00:00:00Z', 'UTC')
  })
  it('uses a fixed action column, scroll, consistent tooltips and clickable edit controls', async () => {
    const wrapper = await create()
    wrapper.findComponent(RadioGroup).vm.$emit('update:value', 'list')
    await flushPromises()
    const table = wrapper.findComponent(Table)
    expect(table.props('columns').at(-1)).toMatchObject({ key: 'action', fixed: 'right', width: 120 })
    expect(table.props('scroll').x).toBeGreaterThan(0)
    const edit = wrapper.findAllComponents(Tooltip).find((item) => item.props('title') === '编辑')
    expect(edit.props('placement')).toBe('top')
    await edit.get('button').trigger('mouseenter')
    await new Promise((resolve) => setTimeout(resolve, 200))
    expect(document.body.querySelector('.ant-tooltip-inner')?.textContent).toBe('编辑')
    await edit.get('button').trigger('click')
    await flushPromises()
    expect(wrapper.findComponent(dialogStub).props('record').id).toBe(9)
    expect(wrapper.findAllComponents(Tooltip).filter((item) => item.props('title') === '删除').every((item) => item.props('placement') === 'top')).toBe(true)
  })
  it('checks per-item deletion results and reloads even after partial failure', async () => {
    const wrapper = await create()
    wrapper.findComponent(RadioGroup).vm.$emit('update:value', 'list')
    await flushPromises()
    const remove = wrapper.findAllComponents(Tooltip).filter((item) => item.props('title') === '删除').at(-1)
    await remove.get('button').trigger('click')
    const confirm = openDeleteConfirm.mock.calls.at(-1)[0]
    expect(confirm.items[0]).toContain('订单 → 缓存')
    api.deleteServiceDependencies.mockResolvedValueOnce(envelope({ count: 0, results: [{ id: 9, ok: false, message: '删除失败' }] }))
    api.getServiceDependencyTopology.mockResolvedValueOnce(topology())
    await expect(confirm.onConfirm()).rejects.toThrow('删除失败')
    await flushPromises()
    expect(api.deleteServiceDependencies).toHaveBeenCalledWith([9])
    expect(api.getServiceDependencyTopology).toHaveBeenCalledTimes(2)
    expect(wrapper.text()).toContain('删除失败')
  })
  it('reports refresh failure after a successful save without reopening a resubmittable form', async () => {
    const wrapper = await create()
    await clickText(wrapper, '新增依赖')
    api.getServiceDependencyTopology.mockRejectedValueOnce(new Error('网络不可用'))
    wrapper.findComponent(dialogStub).vm.$emit('saved')
    wrapper.findComponent(dialogStub).vm.$emit('update:open', false)
    await flushPromises()
    expect(wrapper.text()).toContain('关系已保存，但刷新失败：网络不可用')
    expect(wrapper.findComponent(dialogStub).exists()).toBe(false)
  })
  it('refreshes embedded edge type labels and enabled flags after dictionary changes', async () => {
    const wrapper = await create()
    await clickText(wrapper, '交互类型管理')
    api.getServiceDependencyTypes.mockResolvedValueOnce(page([{ ...types[0], name: '接口调用', enabled: false }]))
    api.getServiceDependencyTopology.mockResolvedValueOnce(topology(undefined, [{ ...edge, interaction_type_name: '接口调用', interaction_type_enabled: false }]))
    wrapper.findComponent(typeStub).vm.$emit('changed')
    await flushPromises()
    expect(api.getServiceDependencyTopology).toHaveBeenCalledTimes(2)
    expect(wrapper.findComponent(graphStub).props('edges')[0]).toMatchObject({ interaction_type_name: '接口调用', interaction_type_enabled: false })
  })
  it('hides write actions when only view permission is granted', async () => {
    checkPermission.mockImplementation((permission) => permission.endsWith(':view'))
    const wrapper = await create()
    expect(wrapper.text()).not.toContain('新增依赖')
    wrapper.findComponent(RadioGroup).vm.$emit('update:value', 'list')
    await flushPromises()
    expect(wrapper.findAllComponents(Tooltip)).toHaveLength(0)
  })
})
