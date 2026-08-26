import { flushPromises, mount } from '@vue/test-utils'
import Antd from 'ant-design-vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'

vi.mock('@/api/assets/application', () => ({
  getBusinessSystem: vi.fn(),
  getBusinessSystemList: vi.fn(),
  getProjectList: vi.fn(),
  getBusinessEnvironmentList: vi.fn(),
  getApplicationService: vi.fn(),
  getApplicationServiceList: vi.fn(),
  getApplicationDeploymentList: vi.fn(),
  getApplicationDeployment: vi.fn(),
  getApplicationDeploymentBaselineHistory: vi.fn(),
}))
vi.mock('@/store', () => ({
  default: { state: { user: { timezone: 'Asia/Shanghai' } } },
}))
vi.mock('@/util/timezone', () => ({
  formatTimeWithTimezone: vi.fn((value, timezone) => `${value} @ ${timezone}`),
}))

import * as applicationApi from '@/api/assets/application'
import ServiceTreeNodeContent from './ServiceTreeNodeContent.vue'

const listResponse = (results) => ({
  data: { data: { results, count: results.length, totalPages: 1 } },
})

describe('ServiceTreeNodeContent', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    applicationApi.getBusinessSystemList.mockResolvedValue(listResponse([
      { id: 7, name: '订单系统', code: 'order-system', deployment_count: 2, enabled: true },
    ]))
    applicationApi.getProjectList.mockResolvedValue(listResponse([]))
    applicationApi.getBusinessSystem.mockResolvedValue({ data: { data: {
      id: 7, name: '订单系统', code: 'order-system', owner: '订单团队', enabled: true,
    } } })
    applicationApi.getApplicationServiceList.mockResolvedValue(listResponse([
      { id: 21, name: '订单 API', business_system: 7, environment: 72, environment_name: '测试环境', deployment_count: 2, topology_type: 'cluster', application_name: 'Order API' },
    ]))
    applicationApi.getApplicationService.mockResolvedValue({ data: { data: {
      id: 21, name: '订单 API', business_system: 7, business_system_name: '订单系统', environment: 72, environment_name: '测试环境',
      application_name: 'Order API', application_version_name: '1.0', deployment_template_name: 'Order Template', topology_type: 'cluster', cluster_profile_name: 'Redis Sentinel',
      cluster_type: 'ha',
      access_address: '10.0.0.10',
      ports: [{ name: 'main_port', protocol: 'tcp', port: 8080 }],
    } } })
    applicationApi.getApplicationDeploymentList.mockResolvedValue(listResponse([
      { id: 31, instance_name: 'order-api-1', runtime_status: 'running', health_status: 'healthy' },
    ]))
    applicationApi.getApplicationDeployment.mockResolvedValue({ data: { data: {
      id: 31, instance_name: 'order-api-1', service_name: '订单 API', business_system_name: '订单系统',
      environment: 72, environment_name: '测试环境', application_name: 'Order API', version: '1.0',
      host_name: 'node-1', host_ip: '10.0.0.1', runtime_status: 'running', health_status: 'healthy',
      baseline_pass_rate: 100, ports: [{ name: 'HTTP', protocol: 'tcp', port: 8080 }],
    } } })
    applicationApi.getApplicationDeploymentBaselineHistory.mockResolvedValue({ data: { data: [
      {
        id: 41, status: 'completed', passed: true, passed_count: 3, total_count: 3,
        requested_username: 'admin', start_time: '2026-08-23T04:30:00Z',
      },
    ] } })
  })

  it('shows the current summary, aggregate metrics, and direct children for every node level', async () => {
    const wrapper = mount(ServiceTreeNodeContent, {
      props: { scope: { nodeType: 'all', nodeTitle: '全部业务' } },
      global: { plugins: [Antd] },
    })
    await flushPromises()
    expect(wrapper.text()).toContain('业务系统1')
    expect(wrapper.text()).toContain('逻辑服务1')
    expect(wrapper.text()).toContain('订单系统')

    await wrapper.setProps({ scope: { nodeType: 'businessSystem', businessSystemId: 7, businessSystemName: '订单系统', nodeTitle: '订单系统' } })
    await flushPromises()
    expect(applicationApi.getApplicationServiceList).toHaveBeenLastCalledWith(expect.objectContaining({ business_system: 7 }))
    expect(applicationApi.getBusinessSystem).toHaveBeenCalledWith(7)
    expect(wrapper.text()).toContain('订单团队')
    expect(wrapper.text()).toContain('订单 API')
    expect(wrapper.find('.child-navigation-link').text()).toContain('订单 API')
    await wrapper.find('.child-navigation-link').trigger('click')
    expect(wrapper.emitted('navigate').at(-1)).toEqual([{
      nodeType: 'service',
      applicationServiceId: 21,
      businessSystemId: 7,
      businessSystemName: '订单系统',
      environment: 72,
      environmentName: '测试环境',
      nodeTitle: '订单 API',
    }])

    await wrapper.setProps({ scope: { nodeType: 'service', applicationServiceId: 21, businessSystemName: '订单系统', environmentName: '测试环境', nodeTitle: '订单 API' } })
    await flushPromises()
    expect(applicationApi.getApplicationDeploymentList).toHaveBeenLastCalledWith(expect.objectContaining({ application_service: 21 }))
    expect(applicationApi.getApplicationService).toHaveBeenCalledWith(21)
    expect(wrapper.text()).toContain('10.0.0.10')
    expect(wrapper.text()).toContain('1.0')
    expect(wrapper.text()).toContain('Order Template')
    expect(wrapper.text()).toContain('main_port · TCP 8080')
    expect(wrapper.text()).toContain('order-api-1')

    await wrapper.setProps({ scope: { nodeType: 'deployment', deploymentId: 31, businessSystemName: '订单系统', environmentName: '测试环境', serviceName: '订单 API', nodeTitle: 'order-api-1' } })
    await flushPromises()
    expect(applicationApi.getApplicationDeployment).toHaveBeenCalledWith(31)
    expect(wrapper.text()).not.toContain('逻辑服务')
    expect(wrapper.text()).not.toContain('业务系统')
    expect(wrapper.text()).not.toContain('部署模板')
    expect(wrapper.text()).toContain('10.0.0.1')
    expect(wrapper.text()).toContain('最近基线检查')
    expect(wrapper.text()).toContain('通过')
    expect(wrapper.text()).toContain('开始时间')
    expect(wrapper.text()).toContain('2026-08-23T04:30:00Z @ Asia/Shanghai')
  })
})
