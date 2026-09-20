import { flushPromises, mount } from '@vue/test-utils'
import Antd from 'ant-design-vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'

vi.mock('@/api/assets/application', () => ({
  getBusinessSystemList: vi.fn(),
  getBusinessEnvironmentList: vi.fn(),
  getApplicationServiceList: vi.fn(),
  getApplicationDeploymentList: vi.fn(),
  getProjectList: vi.fn(),
}))

vi.mock('@/api/assets/host', () => ({
  getHostList: vi.fn(() => Promise.resolve({ data: { data: { results: [], count: 0, totalPages: 1 } } })),
}))

import * as applicationApi from '@/api/assets/application'
import ServiceTree from './ServiceTree.vue'

const listResponse = (results) => ({
  data: { data: { results, count: results.length, totalPages: 1 } },
})

describe('ServiceTree', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    applicationApi.getBusinessSystemList.mockResolvedValue(listResponse([
      { id: 7, name: '订单系统', code: 'order-system', enabled: true },
    ]))
    applicationApi.getBusinessEnvironmentList.mockResolvedValue(listResponse([
      { id: 71, name: '生产环境', code: 'production', enabled: true },
      { id: 72, name: '测试环境', code: 'testing', enabled: true },
    ]))
    applicationApi.getProjectList.mockResolvedValue(listResponse([
      { id: 301, name: '订单项目', code: 'order-project', business_systems: [7], enabled: true },
    ]))
    applicationApi.getApplicationServiceList.mockResolvedValue(listResponse([
      { id: 21, business_system: 7, environment: 71, environment_name: '生产环境', name: '订单 API', topology_type: 'cluster', cluster_profile_name: 'Redis 集群', log_collection_enabled: true },
      { id: 22, business_system: 7, environment: 72, environment_name: '测试环境', name: '订单任务', topology_type: 'standalone', log_collection_enabled: true },
    ]))
    applicationApi.getApplicationDeploymentList.mockResolvedValue(listResponse([
      { id: 11, application_service_ids: [21], instance_name: 'order-prod-1' },
      { id: 12, application_service_ids: [22], instance_name: 'order-test-1' },
    ]))
  })

  it('builds services directly under business systems and emits the selected service scope', async () => {
    const wrapper = mount(ServiceTree, { global: { plugins: [Antd], stubs: { FontAwesomeIcon: true } } })
    await flushPromises()

    expect(wrapper.text()).toContain('全部业务')
    expect(wrapper.text()).toContain('订单系统')
    expect(wrapper.text()).toContain('订单 API')
    expect(wrapper.text()).toContain('订单 API [生产环境]')
    expect(wrapper.text()).toContain('订单任务 [测试环境]')
    // 部署实例是懒加载子节点，展开服务节点后才挂载，初始渲染不包含实例名。

    const allBusinessNode = wrapper.findAll('.ant-tree-node-content-wrapper')
      .find((node) => node.text().includes('订单系统'))
    expect(allBusinessNode.exists()).toBe(true)

    const projectFilter = wrapper.findAllComponents({ name: 'ASelect' })
      .find((component) => component.classes().includes('service-tree-project-filter'))
    await projectFilter.vm.$emit('change', [301])
    await flushPromises()
    expect(wrapper.text()).toContain('订单 API [生产环境]')

    const serviceNode = wrapper.findAll('.ant-tree-node-content-wrapper')
      .find((node) => node.text().includes('订单任务'))
    await serviceNode.trigger('click')

    expect(wrapper.emitted('select').at(-1)).toEqual([{
      nodeType: 'service',
      applicationServiceId: 22,
      businessSystemId: 7,
      businessSystemName: '订单系统',
      environment: 72,
      environmentName: '测试环境',
      nodeTitle: '订单任务',
    }])

    await wrapper.setProps({
      selectedScope: {
        nodeType: 'service',
        applicationServiceId: 21,
      },
    })
    await flushPromises()
    expect(wrapper.find('.ant-tree-node-selected').text()).toContain('订单 API')

    const environmentFilter = wrapper.findAllComponents({ name: 'ASelect' })
      .find((component) => component.classes().includes('service-tree-environment-filter'))
    await environmentFilter.vm.$emit('change', [72])
    await flushPromises()
    expect(wrapper.text()).toContain('订单任务 [测试环境]')
    expect(wrapper.text()).not.toContain('订单 API [生产环境]')
  })

  it('groups business systems under their project and environment, and renders per-level icons when groupByProject is enabled', async () => {
    applicationApi.getBusinessSystemList.mockResolvedValue(listResponse([
      { id: 7, name: '订单系统', code: 'order-system', enabled: true, project: 301, project_name: '订单项目' },
    ]))
    const wrapper = mount(ServiceTree, { props: { groupByProject: true }, global: { plugins: [Antd], stubs: { FontAwesomeIcon: true } } })
    await flushPromises()

    const projectNode = wrapper.findAll('.ant-tree-node-content-wrapper')
      .find((node) => node.text().includes('订单项目'))
    expect(projectNode.exists()).toBe(true)
    expect(wrapper.text()).toContain('订单系统')
    // 环境层收拢同一环境下的多个服务，默认折叠，服务名此时还看不见。
    expect(wrapper.text()).toContain('生产环境 (1)')
    expect(wrapper.text()).not.toContain('订单 API')
    expect(wrapper.find('.service-tree-icon--project').exists()).toBe(true)
    expect(wrapper.find('.service-tree-icon--system').exists()).toBe(true)
    expect(wrapper.find('.service-tree-icon--environment').exists()).toBe(true)

    const environmentTreenode = wrapper.findAll('.ant-tree-treenode')
      .find((node) => node.text().includes('生产环境'))
    await environmentTreenode.find('.ant-tree-switcher').trigger('click')
    await flushPromises()

    // 展开环境节点后才能看到具体服务，且分了环境层级后服务名不再重复带 [环境] 后缀。
    expect(wrapper.text()).toContain('订单 API')
    expect(wrapper.text()).not.toContain('订单 API [生产环境]')
    expect(wrapper.find('.service-tree-icon--service').exists()).toBe(true)
    wrapper.unmount()
  })

  it('does not introduce a project or environment level when groupByProject is left at its default', async () => {
    const wrapper = mount(ServiceTree, { global: { plugins: [Antd], stubs: { FontAwesomeIcon: true } } })
    await flushPromises()

    expect(wrapper.find('.service-tree-icon--project').exists()).toBe(false)
    expect(wrapper.find('.service-tree-icon--environment').exists()).toBe(false)
    expect(wrapper.find('.service-tree-icon--system').exists()).toBe(true)
    expect(wrapper.text()).toContain('订单 API [生产环境]')
    wrapper.unmount()
  })

  // 2026-09-19 起树上不再按"是否开启日志采集"灰节点/加禁用标记：树是所有资产页共用的，
  // 而"采不采日志"只在日志域有意义；"为什么查不到日志"由日志中心的「采集链路」逐层回答。
  // 这条用例钉住"别再灰回去"——节点颜色不该承担"有没有日志"这种多因结论
  //（服务停用、配置没下发、规则没发布都会没日志，不只是采集开关）。
  it('does not grey out services with log collection disabled', async () => {
    applicationApi.getApplicationServiceList.mockResolvedValue(listResponse([
      { id: 21, business_system: 7, environment: 71, environment_name: '生产环境', name: '订单 API', topology_type: 'cluster', log_collection_enabled: true },
      { id: 22, business_system: 7, environment: 72, environment_name: '测试环境', name: '订单任务', topology_type: 'standalone', log_collection_enabled: false },
    ]))
    const wrapper = mount(ServiceTree, { global: { plugins: [Antd], stubs: { FontAwesomeIcon: true } } })
    await flushPromises()

    const labels = wrapper.findAll('.service-tree-node-label')
    const enabledLabel = labels.find((node) => node.text().includes('订单 API'))
    const disabledLabel = labels.find((node) => node.text().includes('订单任务'))
    expect(disabledLabel.classes()).toEqual(enabledLabel.classes())
    expect(wrapper.findAll('.service-tree-log-disabled-badge')).toHaveLength(0)
    wrapper.unmount()
  })

  // 回归（2026-09-20 现场）：项目节点、环境节点**第一次点不上**，光标弹回顶层，得再点一次。
  //
  // 机理：选中态由父组件回传的 scope 反推（见 scopeKey），而它原先只认业务系统/服务/实例三种，
  // 项目与环境两层都退回 'all'——第一次点击的选中态立刻被抹掉；第二次点因为父组件的 scope
  // 引用没变、watch 不再触发，才"显得"生效。各页都是 `@select="scope = $event"`，所以这个
  // 回传必然发生（服务树页与日志中心页用的是同一个组件）。
  it('keeps the selection on a project or environment node after the parent echoes the scope back', async () => {
    applicationApi.getBusinessSystemList.mockResolvedValue(listResponse([
      { id: 7, name: '订单系统', code: 'order-system', enabled: true, project: 301, project_name: '订单项目' },
    ]))
    const wrapper = mount(ServiceTree, { props: { groupByProject: true }, global: { plugins: [Antd], stubs: { FontAwesomeIcon: true } } })
    await flushPromises()

    const projectNode = wrapper.findAll('.ant-tree-node-content-wrapper')
      .find((node) => node.text().includes('订单项目'))
    await projectNode.trigger('click')
    const projectScope = { nodeType: 'project', projectId: 301, nodeTitle: '订单项目' }
    expect(wrapper.emitted('select').at(-1)).toEqual([projectScope])
    expect(wrapper.find('.ant-tree-node-selected').text()).toContain('订单项目')

    // 父组件原样回传 → 选中态必须还留在这个节点上（不能弹回「全部…」）。
    await wrapper.setProps({ selectedScope: projectScope })
    await flushPromises()
    expect(wrapper.find('.ant-tree-node-selected').text()).toContain('订单项目')

    const environmentNode = wrapper.findAll('.ant-tree-node-content-wrapper')
      .find((node) => node.text().includes('生产环境'))
    await environmentNode.trigger('click')
    const environmentScope = {
      nodeType: 'environment', businessSystemId: 7, businessSystemName: '订单系统',
      environment: 71, environmentName: '生产环境', nodeTitle: '生产环境',
    }
    expect(wrapper.emitted('select').at(-1)).toEqual([environmentScope])
    expect(wrapper.find('.ant-tree-node-selected').text()).toContain('生产环境')

    await wrapper.setProps({ selectedScope: environmentScope })
    await flushPromises()
    expect(wrapper.find('.ant-tree-node-selected').text()).toContain('生产环境')
    wrapper.unmount()
  })
})
