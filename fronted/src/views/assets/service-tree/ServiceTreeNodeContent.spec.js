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
  getApplicationServiceLogConfig: vi.fn(),
}))
vi.mock('@/store', () => ({
  default: { state: { user: { timezone: 'Asia/Shanghai' } } },
}))
vi.mock('@/api/assets/host', () => ({
  getHostById: vi.fn(() => Promise.resolve({ data: { data: {
    hostname: 'node-1.corp', os_type: 'Linux', os_version: 'Ubuntu 22.04', architecture: 'x86_64',
    cpu_cores: 8, cpu_model: 'Intel(R) Xeon(R) Gold', memory_gb: 32, disk_total_gb: 500, disk_used_percent: 72.5,
    disks: [
      { device: 'sda1', mount_point: '/', filesystem: 'ext4', size_gb: 400, used_gb: 280, usage_percent: 70 },
      { device: 'sdb1', mount_point: '/data', filesystem: 'xfs', size_gb: 100, used_gb: 25, usage_percent: 25 },
    ],
  } } })),
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
    applicationApi.getApplicationServiceLogConfig.mockResolvedValue({ data: { data: {
      logs: [{ log_definition: 5, name: 'message', path_pattern: '${APP_HOME}/logs/app.log', resolved_path: '/var/log/messages', pending_macros: [] }],
    } } })
    applicationApi.getApplicationDeploymentList.mockResolvedValue(listResponse([
      { id: 31, instance_name: 'order-api-1', runtime_status: 'running' },
    ]))
    applicationApi.getApplicationDeployment.mockResolvedValue({ data: { data: {
      id: 31, instance_name: 'order-api-1', service_name: '订单 API', business_system_name: '订单系统',
      environment: 72, environment_name: '测试环境', application_name: 'Order API', version: '1.0',
      host: 31, host_name: 'node-1', host_ip: '10.0.0.1', runtime_status: 'running',
      ports: [{ name: 'HTTP', protocol: 'tcp', port: 8080 }],
    } } })
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
    // 业务系统层级先按环境分组展示，跟左侧树的层级顺序保持一致
    expect(wrapper.text()).toContain('测试环境')
    expect(wrapper.find('.child-navigation-link').text()).toContain('测试环境')
    await wrapper.find('.child-navigation-link').trigger('click')
    expect(wrapper.emitted('navigate').at(-1)).toEqual([{
      nodeType: 'environment',
      businessSystemId: 7,
      businessSystemName: '订单系统',
      environment: 72,
      environmentName: '测试环境',
      nodeTitle: '测试环境',
    }])

    await wrapper.setProps({
      scope: {
        nodeType: 'environment', businessSystemId: 7, businessSystemName: '订单系统',
        environment: 72, environmentName: '测试环境', nodeTitle: '测试环境',
      },
    })
    await flushPromises()
    expect(applicationApi.getApplicationServiceList).toHaveBeenLastCalledWith(expect.objectContaining({ business_system: 7 }))
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
    expect(wrapper.text()).toContain('message')
    expect(wrapper.text()).toContain('/var/log/messages')
    expect(wrapper.text()).toContain('order-api-1')

    await wrapper.setProps({ scope: { nodeType: 'deployment', deploymentId: 31, businessSystemName: '订单系统', environmentName: '测试环境', serviceName: '订单 API', nodeTitle: 'order-api-1' } })
    await flushPromises()
    expect(applicationApi.getApplicationDeployment).toHaveBeenCalledWith(31)
    expect(wrapper.text()).not.toContain('逻辑服务')
    expect(wrapper.text()).not.toContain('业务系统')
    expect(wrapper.text()).not.toContain('部署模板')
    expect(wrapper.text()).toContain('10.0.0.1')
    expect(wrapper.text()).toContain('运行中')

    // 主机信息区块：部署详情带 host id → 拉主机详情渲染硬件/系统与磁盘明细 + WebSSH 入口。
    const { getHostById } = await import('@/api/assets/host')
    expect(getHostById).toHaveBeenCalledWith(31)
    expect(wrapper.text()).toContain('主机信息')
    expect(wrapper.text()).toContain('node-1.corp')
    expect(wrapper.text()).toContain('Ubuntu 22.04')
    expect(wrapper.text()).toContain('Intel(R) Xeon(R) Gold')
    expect(wrapper.text()).toContain('32 GB')
    expect(wrapper.text()).toContain('72.5%')
    expect(wrapper.text()).toContain('/data')
    // 按钮已改为图标形态（tooltip 提供文字说明），断言按钮容器存在。
    expect(wrapper.find('.host-info-section button').exists()).toBe(true)
  })

  it('renders project and environment node types used when the tree groups by project', async () => {
    const wrapper = mount(ServiceTreeNodeContent, {
      props: { scope: { nodeType: 'project', projectId: 5, nodeTitle: '示例项目' } },
      global: { plugins: [Antd] },
    })
    await flushPromises()
    expect(applicationApi.getBusinessSystemList).toHaveBeenLastCalledWith(expect.objectContaining({ project: 5 }))
    expect(wrapper.text()).toContain('订单系统')
    expect(wrapper.find('.child-navigation-link').text()).toContain('订单系统')
    await wrapper.find('.child-navigation-link').trigger('click')
    expect(wrapper.emitted('navigate').at(-1)).toEqual([{
      nodeType: 'businessSystem',
      businessSystemId: 7,
      businessSystemName: '订单系统',
      nodeTitle: '订单系统',
      projectId: 5,
      projectName: '示例项目',
    }])

    await wrapper.setProps({
      scope: {
        nodeType: 'environment', businessSystemId: 7, businessSystemName: '订单系统',
        environment: 72, environmentName: '测试环境', nodeTitle: '测试环境',
      },
    })
    await flushPromises()
    expect(applicationApi.getApplicationServiceList).toHaveBeenLastCalledWith(expect.objectContaining({ business_system: 7 }))
    expect(wrapper.text()).toContain('订单 API')
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
  })

  it('业务系统的编辑/删除操作只对外抛事件，不会触发导航', async () => {
    const wrapper = mount(ServiceTreeNodeContent, {
      props: { scope: { nodeType: 'all', nodeTitle: '全部业务' } },
      global: { plugins: [Antd] },
    })
    await flushPromises()

    // 子表格里的编辑/删除按钮嵌在可点击跳转的整行里，点击必须 stop 冒泡，不能连带触发 navigate。
    const rowButtons = wrapper.findAll('.ant-btn')
    await rowButtons[0].trigger('click')
    expect(wrapper.emitted('edit-business-system')?.at(-1)).toEqual([
      expect.objectContaining({ id: 7, name: '订单系统' }),
    ])
    await wrapper.find('.delBtn').trigger('click')
    expect(wrapper.emitted('delete-business-system')?.at(-1)).toEqual([
      expect.objectContaining({ id: 7, name: '订单系统' }),
    ])
    expect(wrapper.emitted('navigate')).toBeUndefined()

    await wrapper.setProps({ scope: { nodeType: 'businessSystem', businessSystemId: 7, businessSystemName: '订单系统', nodeTitle: '订单系统' } })
    await flushPromises()

    const headerButtons = wrapper.findAll('.ant-btn')
    await headerButtons[0].trigger('click')
    expect(wrapper.emitted('edit-business-system')?.at(-1)).toEqual([
      expect.objectContaining({ id: 7 }),
    ])
    await wrapper.find('.delBtn').trigger('click')
    expect(wrapper.emitted('delete-business-system')?.at(-1)).toEqual([
      expect.objectContaining({ id: 7 }),
    ])
  })

  it('逻辑服务的编辑/删除操作只对外抛事件，不会触发导航', async () => {
    const wrapper = mount(ServiceTreeNodeContent, {
      props: {
        scope: {
          nodeType: 'environment', businessSystemId: 7, businessSystemName: '订单系统',
          environment: 72, environmentName: '测试环境', nodeTitle: '测试环境',
        },
      },
      global: { plugins: [Antd] },
    })
    await flushPromises()

    // 环境层级没有自己的头部编辑/删除按钮，子表格的服务操作按钮就是第一组；
    // 子表格里的编辑/删除按钮嵌在可点击跳转的整行里，点击必须 stop 冒泡，不能连带触发 navigate。
    const rowButtons = wrapper.findAll('.ant-btn')
    await rowButtons[0].trigger('click')
    expect(wrapper.emitted('edit-service')?.at(-1)).toEqual([
      expect.objectContaining({ id: 21, name: '订单 API' }),
    ])
    await wrapper.find('.delBtn').trigger('click')
    expect(wrapper.emitted('delete-service')?.at(-1)).toEqual([
      expect.objectContaining({ id: 21, name: '订单 API' }),
    ])
    expect(wrapper.emitted('navigate')).toBeUndefined()

    await wrapper.setProps({ scope: { nodeType: 'service', applicationServiceId: 21, businessSystemName: '订单系统', environmentName: '测试环境', nodeTitle: '订单 API' } })
    await flushPromises()

    const headerButtons = wrapper.findAll('.ant-btn')
    await headerButtons[0].trigger('click')
    expect(wrapper.emitted('edit-service')?.at(-1)).toEqual([
      expect.objectContaining({ id: 21 }),
    ])
    await wrapper.find('.delBtn').trigger('click')
    expect(wrapper.emitted('delete-service')?.at(-1)).toEqual([
      expect.objectContaining({ id: 21 }),
    ])
  })
})
