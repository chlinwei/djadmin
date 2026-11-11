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
  refreshApplicationServiceRuntimeStatus: vi.fn(),
}))
vi.mock('@/store', async () => {
  const { reactive } = await import('vue')
  return {
    default: { state: reactive({
      user: { timezone: 'Asia/Shanghai' },
      perms: ['assets:hosts:execute', 'assets:service-tree:manage'],
      role_codes: ['operator'],
    }) },
  }
})
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
import store from '@/store'
import permission from '@/directives/permission'
import ServiceTreeNodeContent from './ServiceTreeNodeContent.vue'

const listResponse = (results) => ({
  data: { data: { results, count: results.length, totalPages: 1 } },
})

describe('ServiceTreeNodeContent', () => {
  it('keeps service actions stable across runtime refresh feedback changes', async () => {
    const wrapper = mount(ServiceTreeNodeContent, {
      props: { scope: { nodeType: 'service', applicationServiceId: 21, nodeTitle: '订单 API' } },
      global: { plugins: [Antd] },
    })
    try {
      await flushPromises()
      for (const refreshing of [true, false, true, false]) {
        wrapper.vm.runtimeRefreshing = refreshing
        await flushPromises()
        const buttons = wrapper.findAll('.node-content-header button')
        expect(buttons).toHaveLength(3)
        expect(buttons[0].classes()).toContain('ant-btn-primary')
        expect(buttons[1].classes()).toContain('ant-btn-default')
        expect(buttons[1].classes()).not.toContain('ant-btn-dangerous')
        expect(buttons[2].classes()).toContain('delBtn')
        await buttons[1].trigger('click')
        expect(wrapper.emitted('copy-service').at(-1)[0].id).toBe(21)
      }
    } finally {
      wrapper.unmount()
    }
  })

  beforeEach(() => {
    vi.clearAllMocks()
    store.state.user.timezone = 'Asia/Shanghai'
    store.state.perms = ['assets:hosts:execute', 'assets:service-tree:manage']
    store.state.role_codes = ['operator']
    applicationApi.refreshApplicationServiceRuntimeStatus.mockResolvedValue({ data: { data: { summary: {} } } })
    applicationApi.getBusinessSystemList.mockResolvedValue(listResponse([
      { id: 7, name: '订单系统', code: 'order-system', deployment_count: 2, enabled: true },
    ]))
    applicationApi.getProjectList.mockResolvedValue(listResponse([]))
    applicationApi.getBusinessSystem.mockResolvedValue({ data: { data: {
      id: 7, name: '订单系统', code: 'order-system', owner: '订单团队', enabled: true,
      project: 5, project_name: '订单项目',
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

  it('在日志路径旁直接显示宏缺值错误而不是伪造空目录路径', async () => {
    applicationApi.getApplicationServiceLogConfig.mockResolvedValue({ data: { data: {
      logs: [{ log_definition: 5, name: 'error_log', path_pattern: '${APP_HOME}/logs/error.log',
        resolved_path: '${APP_HOME}/logs/error.log', pending_macros: ['APP_HOME'], resolution_error: '宏 IMF_USER 值为空' }],
    } } })
    const wrapper = mount(ServiceTreeNodeContent, {
      props: { scope: { nodeType: 'service', applicationServiceId: 21, nodeTitle: '订单 API' } },
      global: { plugins: [Antd], stubs: { FontAwesomeIcon: true } },
    })
    try {
      await flushPromises()
      const log = wrapper.find('.service-log-item')
      expect(log.text()).toContain('宏 IMF_USER 值为空')
      expect(log.text()).toContain('${APP_HOME}/logs/error.log')
      expect(log.text()).not.toContain('待展开')
      expect(log.find('.ant-tag-red').exists()).toBe(true)
    } finally {
      wrapper.unmount()
    }
  })

  it('shows resolved service ports and per-instance overrides without hiding resolution errors', async () => {
    applicationApi.getApplicationService.mockResolvedValue({ data: { data: {
      id: 21, name: '订单 API', ports: [{
        name: 'main_port', protocol: 'tcp', port: '${PORT}', resolved_port: '1414',
        instance_ports: [
          { deployment: 31, instance_name: 'instance-a', resolved_port: '1515' },
          { deployment: 32, instance_name: 'instance-b', resolved_port: '${PORT}', resolution_error: '宏 PORT 未定义' },
        ],
      }],
    } } })
    const wrapper = mount(ServiceTreeNodeContent, {
      props: { scope: { nodeType: 'service', serviceId: 21, nodeId: 21, nodeTitle: '订单 API' } },
      global: { plugins: [Antd], stubs: { FontAwesomeIcon: true } },
    })
    try {
      await flushPromises()
      const section = wrapper.find('.service-ports-section')
      expect(section.text()).toContain('TCP 1414')
      expect(section.text()).toContain('instance-a：1515')
      expect(section.text()).toContain('instance-b：${PORT}')
      expect(section.text()).toContain('宏 PORT 未定义')
      expect(section.findComponent({ name: 'ATooltip' }).props('title')).toContain('模板定义：${PORT}')
    } finally {
      wrapper.unmount()
    }
  })

  it('omits deployment counts from root and project business-system tables and keeps actions fixed', async () => {
    applicationApi.getBusinessSystemList.mockResolvedValue(listResponse([
      { id: 7, name: '订单系统', code: 'order-system', project: 5, project_name: '示例项目', enabled: true },
    ]))
    const wrapper = mount(ServiceTreeNodeContent, {
      props: { scope: { nodeType: 'all', nodeTitle: '全部业务' } },
      global: { plugins: [Antd] },
    })
    try {
      await flushPromises()
      const table = wrapper.findComponent({ name: 'ATable' })
      const columns = table.props('columns')
      expect(columns.slice(0, 2).map((column) => column.title)).toEqual(['项目', '业务系统'])
      expect(columns[0].dataIndex).toBe('project_name')
      expect(columns[0].customRender({ text: '' })).toBe('-')
      expect(table.text()).toContain('示例项目')
      expect(columns.some((column) => column.key === 'deployment_count')).toBe(false)
      expect(table.props('scroll').x).toBe(990)
      expect(columns.find((column) => column.key === 'business_system_action')).toMatchObject({ fixed: 'right', width: 110 })
      await wrapper.setProps({ scope: { nodeType: 'project', projectId: 5, nodeTitle: '示例项目' } })
      await flushPromises()
      expect(table.props('columns')[0].title).toBe('业务系统')
      expect(table.props('columns').some((column) => column.key === 'project_name')).toBe(false)
      expect(table.props('columns').some((column) => column.key === 'deployment_count')).toBe(false)
      expect(table.props('scroll').x).toBe(810)
      expect(table.props('columns').find((column) => column.key === 'business_system_action')).toMatchObject({ fixed: 'right', width: 110 })
      expect(wrapper.findAll('.metric-label').map((item) => item.text())).toContain('部署实例')
    } finally {
      wrapper.unmount()
    }
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

  it.each([
    { nodeType: 'all' },
    { nodeType: 'project', projectId: 5, projectName: '订单项目' },
    { nodeType: 'businessSystem', businessSystemId: 7, projectId: 999, projectName: '过期项目' },
    { nodeType: 'environment', businessSystemId: 7, environment: 72 },
    { nodeType: 'service', applicationServiceId: 21 },
  ])('异常数字与明细一致，可查看诊断并定位实例，且只读用户不触发检查 %j', async (scope) => {
    store.state.perms = ['assets:applications:view']
    applicationApi.getBusinessSystemList.mockResolvedValue(listResponse([
      { id: 7, name: '订单系统', project: 5, project_name: '订单项目' },
    ]))
    applicationApi.getApplicationServiceList.mockResolvedValue(listResponse([
      { id: 21, name: '订单 API', business_system: 7, business_system_name: '订单系统', environment: 72, environment_name: '测试环境' },
      { id: 22, name: '共享服务', business_system: 7, business_system_name: '订单系统', environment: 72, environment_name: '测试环境' },
    ]))
    const checkedAt = '2026-11-11T04:00:00Z'
    const deployments = Array.from({ length: 10 }, (_, index) => ({
      id: 31 + index, instance_name: `实例-${index + 1}`, host: 31, host_ip: '10.0.0.1',
      application_service_ids: index === 0 ? [21, 22] : [21],
      runtime_status: index < 8 ? (index % 2 ? 'stopped' : 'error') : index === 8 ? 'running' : 'unknown',
      runtime_status_output: index === 1 ? '' : 'Agent 未连接\n<b>完整诊断输出</b>',
      last_status_check_time: index === 1 ? null : checkedAt,
    }))
    applicationApi.getApplicationDeploymentList.mockResolvedValue(listResponse(deployments))
    const wrapper = mount(ServiceTreeNodeContent, {
      props: {
        scope, hostSnapshots: [{ id: 31, instance_name: 'dbserver', system: { hostname: 'dbserver.example' } }],
      },
      global: {
        plugins: [Antd], directives: { permission },
        stubs: {
          FontAwesomeIcon: true,
          AModal: {
            name: 'AModal',
            props: ['open', 'title', 'footer'],
            template: '<section v-if="open" class="abnormal-dialog"><h3>{{ title }}</h3><slot /></section>',
          },
        },
      },
    })
    try {
      await flushPromises()
      expect(wrapper.find('.abnormal-count-link').text()).toBe('8')
      expect(wrapper.find('.node-content-header').text()).not.toContain('8 个异常实例')
      expect(wrapper.findAll('.abnormal-count-link')).toHaveLength(1)
      const requestCount = applicationApi.getApplicationDeploymentList.mock.calls.length
      await wrapper.find('.abnormal-count-link').trigger('click')
      const modal = wrapper.findComponent({ name: 'AModal' })
      expect(modal.props('footer')).toBeNull()
      const table = wrapper.findAllComponents({ name: 'ATable' })
        .find((component) => component.classes().includes('abnormal-details-table'))
      expect(table.props('dataSource').map((item) => item.id)).toEqual([31, 32, 33, 34, 35, 36, 37, 38])
      expect(table.props('dataSource')[0].relatedServices).toHaveLength(scope.nodeType === 'service' ? 1 : 2)
      expect(table.props('columns').find((column) => column.key === 'related_services').title).toBe('项目 / 业务系统 / 环境 / 逻辑服务')
      expect(table.find('.abnormal-service-link').text()).toBe('订单项目 / 订单系统 / 测试环境 / 订单 API')
      expect(table.props('scroll').x).toBeGreaterThan(0)
      expect(table.text()).toContain('已停止')
      expect(table.text()).toContain('检查失败')
      expect(table.text()).toContain('订单 API')
      expect(table.text()).toContain('dbserver')
      expect(table.text()).toContain('10.0.0.1')
      expect(table.text()).toContain('未保存诊断输出')
      expect(table.text()).toContain('无检查时间记录')
      expect(table.text()).toContain(`${checkedAt} @ Asia/Shanghai`)
      expect(table.find('.abnormal-output').text()).toBe('Agent 未连接\n<b>完整诊断输出</b>')
      expect(table.find('.abnormal-output b').exists()).toBe(false)
      store.state.user.timezone = 'UTC'
      await flushPromises()
      expect(table.text()).toContain(`${checkedAt} @ UTC`)
      expect(applicationApi.getApplicationDeploymentList).toHaveBeenCalledTimes(requestCount)
      expect(applicationApi.refreshApplicationServiceRuntimeStatus).not.toHaveBeenCalled()
      const serviceLinks = table.findAll('.abnormal-service-link')
      await serviceLinks[scope.nodeType === 'service' ? 0 : 1].trigger('click')
      expect(wrapper.emitted('navigate').at(-1)[0]).toMatchObject({
        nodeType: 'deployment', deploymentId: 31, nodeTitle: '实例-1',
        applicationServiceId: scope.nodeType === 'service' ? 21 : 22,
        businessSystemId: 7, environment: 72, projectId: 5, projectName: '订单项目',
      })
      expect(modal.props('open')).toBe(false)
      await wrapper.find('.abnormal-count-link').trigger('click')
      await wrapper.find('.abnormal-instance-link').trigger('click')
      expect(wrapper.emitted('navigate').at(-1)[0]).toMatchObject({
        nodeType: 'deployment', deploymentId: 31, applicationServiceId: 21,
      })
      await wrapper.find('.abnormal-count-link').trigger('click')
      await wrapper.setProps({ scope: { nodeType: 'all', nodeTitle: '其他范围' } })
      await flushPromises()
      expect(modal.props('open')).toBe(false)
    } finally {
      wrapper.unmount()
    }
  })

  it.each([
    { project: 6, project_name: '另一项目', expectedName: '另一项目' },
    { project: null, project_name: '', expectedName: '未分配项目' },
  ])('共享实例的同名业务系统按实际项目区分，跳转保留所选归属 %j', async ({ project, project_name, expectedName }) => {
    applicationApi.getBusinessSystemList.mockResolvedValue(listResponse([
      { id: 7, name: '同名业务', project: 5, project_name: '订单项目' },
      { id: 8, name: '同名业务', project, project_name },
    ]))
    applicationApi.getApplicationServiceList.mockResolvedValue(listResponse([
      { id: 21, name: '服务一', business_system: 7, business_system_name: '同名业务', environment: 72 },
      { id: 22, name: '服务二', business_system: 8, business_system_name: '同名业务', environment: 72 },
    ]))
    applicationApi.getApplicationDeploymentList.mockResolvedValue(listResponse([
      { id: 31, instance_name: '共享实例', application_service_ids: [21, 22], runtime_status: 'error' },
    ]))
    const wrapper = mount(ServiceTreeNodeContent, {
      props: { scope: { nodeType: 'all', projectId: 999, projectName: '不能复用的项目' } },
      global: { plugins: [Antd], directives: { permission }, stubs: { FontAwesomeIcon: true } },
    })
    try {
      await flushPromises()
      expect(wrapper.vm.abnormalRows).toHaveLength(1)
      const row = wrapper.vm.abnormalRows[0]
      expect(row.relatedServices.map((service) => [service.project, service.project_name])).toEqual([
        [5, '订单项目'], [project, expectedName],
      ])
      wrapper.vm.navigateToAbnormalDeployment(row, row.relatedServices[1])
      expect(wrapper.emitted('navigate').at(-1)[0]).toMatchObject({
        nodeType: 'deployment', deploymentId: 31, applicationServiceId: 22,
        businessSystemId: 8, projectId: project, projectName: expectedName,
      })
    } finally {
      wrapper.unmount()
    }
  })

  it('异常明细遵循根节点的项目和环境查询范围，不混入其他范围的实例', async () => {
    applicationApi.getProjectList.mockResolvedValue(listResponse([
      { id: 5, name: '订单项目', business_systems: [7] },
      { id: 6, name: '其他项目', business_systems: [8] },
    ]))
    applicationApi.getApplicationServiceList.mockResolvedValue(listResponse([
      { id: 21, business_system: 7, environment: 72, name: '范围内服务' },
      { id: 22, business_system: 7, environment: 73, name: '其他环境服务' },
      { id: 23, business_system: 8, environment: 72, name: '其他项目服务' },
    ]))
    applicationApi.getApplicationDeploymentList.mockResolvedValue(listResponse([
      { id: 31, instance_name: '范围内实例', application_service_ids: [21, 22], runtime_status: 'error' },
      { id: 32, instance_name: '其他环境实例', application_service_ids: [22], runtime_status: 'error' },
      { id: 33, instance_name: '其他项目实例', application_service_ids: [23], runtime_status: 'stopped' },
    ]))
    const wrapper = mount(ServiceTreeNodeContent, {
      props: { scope: { nodeType: 'all', projectIds: [5], environmentIds: [72] } },
      global: { plugins: [Antd], directives: { permission }, stubs: { FontAwesomeIcon: true } },
    })
    try {
      await flushPromises()
      expect(wrapper.vm.abnormalCount).toBe(1)
      expect(wrapper.vm.abnormalRows.map((item) => item.id)).toEqual([31])
      expect(wrapper.vm.abnormalRows[0].relatedServices.map((item) => item.id)).toEqual([21])
    } finally {
      wrapper.unmount()
    }
  })

  it.each([
    ['只读', ['assets:applications:view', 'assets:hosts:view'], ['readonly'], false, false],
    ['仅管理', ['assets:service-tree:manage'], ['operator'], true, false],
    ['仅终端执行', ['assets:hosts:execute'], ['operator'], false, true],
    ['无权限', [], ['operator'], false, false],
    ['管理员', [], ['admin'], true, true],
  ])('%s 用户只显示具有可用操作的列，保留名称详情入口', async (_label, perms, roles, canManage, canExecute) => {
    store.state.perms = perms
    store.state.role_codes = roles
    const cases = [
      { scope: { nodeType: 'all' }, key: 'business_system_action', visible: canManage, target: 'businessSystem' },
      { scope: { nodeType: 'project', projectId: 5 }, key: 'business_system_action', visible: canManage, target: 'businessSystem' },
      { scope: { nodeType: 'environment', businessSystemId: 7, environment: 72 }, key: 'service_action', visible: canManage, target: 'service' },
      { scope: { nodeType: 'service', applicationServiceId: 21 }, key: 'deployment_action', visible: canExecute, target: 'deployment' },
    ]
    const wrapper = mount(ServiceTreeNodeContent, {
      props: { scope: cases[0].scope },
      global: { plugins: [Antd], directives: { permission }, stubs: { FontAwesomeIcon: true } },
    })
    try {
      for (const { scope, key, visible, target } of cases) {
        await wrapper.setProps({ scope })
        await flushPromises()
        const table = wrapper.findComponent({ name: 'ATable' })
        const actions = table.props('columns').filter((column) => column.title === '操作')
        expect(actions.map((column) => column.key)).toEqual(visible ? [key] : [])
        expect(table.findAll('thead th').some((header) => header.text() === '操作')).toBe(visible)
        if (visible) {
          expect(actions[0].fixed).toBe('right')
          expect(actions[0].width).toBeGreaterThan(0)
          expect(table.props('scroll').x).toBeGreaterThan(0)
          const buttons = table.findAll('button')
          expect(buttons.length).toBeGreaterThan(0)
          expect(buttons.every((button) => button.element.style.display !== 'none')).toBe(true)
        } else {
          expect(table.findAll('button:not([role="switch"])')).toHaveLength(0)
          expect(table.findAll('button[role="switch"]').every((button) => button.attributes('disabled') !== undefined)).toBe(true)
        }
        await table.find('.child-navigation-link').trigger('click')
        expect(wrapper.emitted('navigate').at(-1)[0].nodeType).toBe(target)
      }
    } finally {
      wrapper.unmount()
    }
  })

  it.each([
    [[], '暂无实例', 'default'],
    [['running'], '运行正常', 'success'],
    [['running', 'running'], '运行正常', 'success'],
    [['unknown'], '状态待确认', 'default'],
    [['running', 'unknown'], '状态待确认', 'default'],
    [[undefined], '状态待确认', 'default'],
    [[null], '状态待确认', 'default'],
    [['unrecognized'], '状态待确认', 'default'],
    [['stopped'], '1 个异常实例', 'error'],
    [['error', 'running', 'unknown'], '1 个异常实例', 'error'],
    [['stopped', 'error', 'unknown'], '2 个异常实例', 'error'],
  ])('各层级将运行状态 %j 汇总为「%s」', async (statuses, label, color) => {
    applicationApi.getApplicationDeploymentList.mockResolvedValue(listResponse(statuses.map((status, index) => ({
      id: index + 31, instance_name: `order-api-${index + 1}`,
      application_service_ids: [21], environment: 72, runtime_status: status, ha_role: 'unknown',
    }))))
    const scopes = [
      { nodeType: 'all' },
      { nodeType: 'project', projectId: 5 },
      { nodeType: 'businessSystem', businessSystemId: 7 },
      { nodeType: 'environment', businessSystemId: 7, environment: 72 },
      { nodeType: 'service', applicationServiceId: 21 },
    ]
    const wrapper = mount(ServiceTreeNodeContent, {
      props: { scope: scopes[0] },
      global: { plugins: [Antd], directives: { permission }, stubs: { FontAwesomeIcon: true } },
    })
    try {
      for (const scope of scopes) {
        await wrapper.setProps({ scope })
        await flushPromises()
        const tag = wrapper.find('.node-content-header').findComponent({ name: 'ATag' })
        const hasAbnormalInstances = statuses.some((status) => ['stopped', 'error'].includes(status))
        expect(tag.exists()).toBe(!hasAbnormalInstances)
        if (!hasAbnormalInstances) {
          expect(tag.text()).toBe(label)
          expect(tag.props('color')).toBe(color)
        }
        expect(wrapper.vm.summaryStatus).toEqual({ label, color })
        expect(wrapper.vm.descendants).toHaveLength(statuses.length)
        expect(wrapper.findAll('.abnormal-count-link')).toHaveLength(hasAbnormalInstances ? 1 : 0)
      }
      expect(applicationApi.refreshApplicationServiceRuntimeStatus).not.toHaveBeenCalled()
    } finally {
      wrapper.unmount()
    }
  })

  it('终端执行权限变动时同步显示或隐藏部署实例操作列', async () => {
    const wrapper = mount(ServiceTreeNodeContent, {
      props: { scope: { nodeType: 'service', applicationServiceId: 21 } },
      global: { plugins: [Antd], directives: { permission }, stubs: { FontAwesomeIcon: true } },
    })
    try {
      await flushPromises()
      const table = wrapper.findComponent({ name: 'ATable' })
      const hasAction = () => table.props('columns').some((column) => column.key === 'deployment_action')
      expect(hasAction()).toBe(true)
      store.state.perms = ['assets:applications:view', 'assets:hosts:view']
      await flushPromises()
      expect(hasAction()).toBe(false)
      expect(table.text()).toContain('order-api-1')
      store.state.perms = ['assets:hosts:execute']
      await flushPromises()
      expect(hasAction()).toBe(true)
    } finally {
      wrapper.unmount()
    }
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
