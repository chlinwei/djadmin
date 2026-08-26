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
      { id: 21, business_system: 7, environment: 71, environment_name: '生产环境', name: '订单 API', topology_type: 'cluster', cluster_profile_name: 'Redis 集群' },
      { id: 22, business_system: 7, environment: 72, environment_name: '测试环境', name: '订单任务', topology_type: 'standalone' },
    ]))
    applicationApi.getApplicationDeploymentList.mockResolvedValue(listResponse([
      { id: 11, application_service_ids: [21], instance_name: 'order-prod-1' },
      { id: 12, application_service_ids: [22], instance_name: 'order-test-1' },
    ]))
  })

  it('builds services directly under business systems and emits the selected service scope', async () => {
    const wrapper = mount(ServiceTree, { global: { plugins: [Antd] } })
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
})
