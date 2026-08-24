import { flushPromises, mount } from '@vue/test-utils'
import Antd from 'ant-design-vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'

vi.mock('@/api/assets/application', () => ({
  getBusinessSystemList: vi.fn(),
  getBusinessEnvironmentList: vi.fn(),
  getApplicationServiceList: vi.fn(),
  getApplicationDeploymentList: vi.fn(),
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
      { id: 71, business_system: 7, name: '生产环境', code: 'production', order: 0, enabled: true },
      { id: 72, business_system: 7, name: '测试环境', code: 'testing', order: 1, enabled: true },
      { id: 73, business_system: 7, name: '开发环境', code: 'development', order: 2, enabled: true },
    ]))
    applicationApi.getApplicationServiceList.mockResolvedValue(listResponse([
      { id: 21, environment: 71, name: '订单 API', topology_type: 'cluster', cluster_profile_name: 'Redis 集群' },
      { id: 22, environment: 72, name: '订单任务', topology_type: 'standalone' },
    ]))
    applicationApi.getApplicationDeploymentList.mockResolvedValue(listResponse([
      { id: 11, application_service_ids: [21], instance_name: 'order-prod-1' },
      { id: 12, application_service_ids: [22], instance_name: 'order-test-1' },
    ]))
  })

  it('builds application environments and emits the selected service scope', async () => {
    const wrapper = mount(ServiceTree, { global: { plugins: [Antd] } })
    await flushPromises()

    expect(wrapper.text()).toContain('全部业务')
    expect(wrapper.text()).toContain('订单系统')
    expect(wrapper.text()).toContain('生产环境')
    expect(wrapper.text()).toContain('测试环境')
    // 没有逻辑服务的空环境也必须在服务树上可见
    expect(wrapper.text()).toContain('开发环境')
    expect(wrapper.text()).toContain('订单 API')
    expect(wrapper.text()).not.toContain('订单 API · Redis 集群')
    expect(wrapper.text()).toContain('order-prod-1')
    expect(wrapper.text()).toContain('order-test-1')

    const testingNode = wrapper.findAll('.ant-tree-node-content-wrapper')
      .find((node) => node.text().includes('测试环境'))
    await testingNode.trigger('click')

    expect(wrapper.emitted('select').at(-1)).toEqual([{
      nodeType: 'environment',
      businessSystemId: 7,
      businessSystemName: '订单系统',
      environment: 72,
      environmentName: '测试环境',
      nodeTitle: '测试环境',
    }])

    await wrapper.setProps({
      selectedScope: {
        nodeType: 'service',
        applicationServiceId: 21,
      },
    })
    await flushPromises()
    expect(wrapper.find('.ant-tree-node-selected').text()).toContain('订单 API')
  })
})
