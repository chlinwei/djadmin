import { describe, it, expect, vi, beforeEach } from 'vitest'
import { reactive } from 'vue'
import { mount, shallowMount } from '@vue/test-utils'

vi.mock('@/api/assets/host/index.js', () => ({
  batchDeleteHost: vi.fn(() => Promise.resolve({ data: { code: 200, data: {} } })),
  getHostById: vi.fn(() =>
    Promise.resolve({
      data: {
        code: 200,
        data: {
          id: 101,
          instance_name: 'pvg-esb4-201',
          ip: '10.25.66.201',
          group: 1,
          port: 22,
          remark: '',
        },
      },
    })
  ),
  getHostList: vi.fn(() => Promise.resolve({ data: { code: 200, data: { results: [], count: 0 } } })),
  saveOrCreateHost: vi.fn(() => Promise.resolve({ data: { code: 200, msg: 'success' } })),
  installAgents: vi.fn(() => Promise.resolve({ data: { code: 200, data: { automation_job_id: 1, jobs: [1] } } })),
}))

vi.mock('@/api/assets/agentPackage.js', () => ({
  listAgentPackages: vi.fn(() => Promise.resolve({ data: { code: 200, data: { items: [] } } })),
  uploadAgentPackage: vi.fn(() => Promise.resolve({ data: { code: 200, data: {} } })),
  downloadAgentPackage: vi.fn(() => Promise.resolve({ data: new Blob() })),
  batchDeleteAgentPackages: vi.fn(() => Promise.resolve({ data: { code: 200, data: { count: 0, results: [] } } })),
}))

vi.mock('@/api/assets/hostgroup/index.js', () => ({
  getHostGroupTree: vi.fn(() => Promise.resolve({ data: { code: 200, data: [] } })),
  getHostGroupById: vi.fn(() => Promise.resolve({ data: { code: 200, data: { id: 7, name: 'testing', host_count: 0 } } })),
  batchDeleteHostGroups: vi.fn(() => Promise.resolve({ data: { code: 200, data: { results: [{ id: 7, ok: true }] } } })),
}))

vi.mock('@/util/deleteConfirm', async (importOriginal) => ({
  ...await importOriginal(),
  openDeleteConfirm: vi.fn(),
}))

vi.mock('@/api/assets/credential/index.js', () => ({
  getCredentailList: vi.fn(() =>
    Promise.resolve({
      data: {
        code: 200,
        data: {
          results: [
            { id: 1, name: 'esb', username: 'esb', port: 22 },
            { id: 2, name: 'common', username: 'root', port: 22 },
          ],
        },
      },
    })
  ),
}))

vi.mock('@/api/sys/sysconfig.js', () => ({
  getConfigByKey: vi.fn(() => Promise.resolve({ data: { value: '5' } })),
  CONFIG_KEYS: {
    HOST_MANAGE_REFRESH_INTERVAL_SECONDS: 'sys.assets.host.manage.refresh_interval_seconds',
  },
}))

vi.mock('@/api/user/index.js', () => ({
  getToken: vi.fn(() => 'test-token'),
}))

vi.mock('@/util/request', () => ({
  getWebSocketBaseUrl: vi.fn(() => 'ws://localhost:8000'),
  // index.vue 挂载时会经 api/assets/application 拉环境列表，缺 default 导出会让 onMounted 直接抛错。
  default: {
    get: vi.fn(() => Promise.resolve({ data: { code: 200, data: { results: [], count: 0 } } })),
    post: vi.fn(() => Promise.resolve({ data: { code: 200, data: {} } })),
    patch: vi.fn(() => Promise.resolve({ data: { code: 200, data: {} } })),
    put: vi.fn(() => Promise.resolve({ data: { code: 200, data: {} } })),
    del: vi.fn(() => Promise.resolve({ data: { code: 200, data: {} } })),
  },
}))

vi.mock('@/util/timezone', () => ({
  formatTimeWithTimezone: vi.fn((value) => value),
}))

vi.mock('@/store', () => ({
  default: {
    state: {
      user: {
        timezone: 'Asia/Shanghai',
      },
    },
  },
}))

vi.mock('@xterm/xterm', () => ({
  Terminal: class Terminal {
    loadAddon() {}
    open() {}
    focus() {}
    dispose() {}
    onData() {
      return { dispose() {} }
    }
    onResize() {
      return { dispose() {} }
    }
    attachCustomKeyEventHandler() {}
  },
}))

vi.mock('@xterm/addon-fit', () => ({
  FitAddon: class FitAddon {
    fit() {}
  },
}))

vi.mock('vue-router', async () => {
  const actual = await vi.importActual('vue-router')
  const route = reactive({ query: {}, path: '/assets/hosts/index', name: '主机管理' })
  return {
    ...actual,
    useRoute: () => route,
    useRouter: () => ({
      push: vi.fn(),
      resolve: vi.fn(() => ({ href: '/assets/hosts/webssh' })),
      replace: vi.fn(() => Promise.resolve()),
    }),
  }
})

import * as hostApi from '@/api/assets/host/index.js'
import * as hostGroupApi from '@/api/assets/hostgroup/index.js'
import { openDeleteConfirm } from '@/util/deleteConfirm'
import { Button, Col, Empty, Row, Spin, Tooltip, message } from 'ant-design-vue'
import HostPage from './index.vue'

const flushPromises = async () => {
  await Promise.resolve()
  await new Promise((resolve) => setTimeout(resolve, 0))
}

describe('Host Agent 凭证安装与主机编辑保存', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('主机分组可收起展开，列表占满宽度且筛选、分页和勾选不变', async () => {
    const wrapper = mount(HostPage, {
      global: {
        components: { AButton: Button, ACol: Col, ARow: Row, ASpin: Spin, ATooltip: Tooltip, AEmpty: Empty },
        stubs: { Dialog: true, FontAwesomeIcon: true },
        directives: { permission: () => {} },
      },
    })
    try {
      await flushPromises()
      const state = wrapper.vm.$.setupState
      state.selectedGroupId = 7
      state.selectedGroupName = 'testing'
      state.groupSearchText = 'test'
      state.searchText = 'host-01'
      state.activeSearchText = 'host-01'
      state.pagination.current = 3
      state.state.selectedRowKeys = [101]
      state.groupContextMenu.visible = true
      await wrapper.vm.$nextTick()
      const hostRequests = hostApi.getHostList.mock.calls.length
      const groupRequests = hostGroupApi.getHostGroupTree.mock.calls.length
      const panel = wrapper.find('.host-group-panel')
      const list = wrapper.find('.host-list-panel')
      const groupScroll = panel.get('.group-list-scroll .ant-spin-container')
      expect(groupScroll.element.closest('.host-group-panel')).toBe(panel.element)
      expect(list.element.closest('.group-list-scroll')).toBeNull()
      const button = wrapper.find(`button[aria-controls="${state.groupPanelId}"]`)
      const tooltip = wrapper.findAllComponents(Tooltip).find((item) => item.props('title') === '收起主机分组')
      expect(panel.isVisible()).toBe(true)
      expect(button.classes()).toContain('group-panel-toggle')
      expect(button.element.parentElement).toBe(wrapper.find('.host-page').element)
      expect(panel.element.nextElementSibling).toBe(button.element)
      expect(button.element.nextElementSibling).toBe(list.element)
      expect(list.element.style.flex).toBe('1')
      expect(button.text()).toBe('◀')
      expect(button.attributes('aria-expanded')).toBe('true')
      expect(button.attributes('aria-label')).toBe('收起主机分组')
      expect(tooltip.props('placement')).toBe('top')
      await button.trigger('mouseenter')
      await new Promise((resolve) => setTimeout(resolve, 200))
      expect(document.querySelector('.ant-tooltip-inner')?.textContent).toBe('收起主机分组')
      await button.trigger('click')
      expect(panel.isVisible()).toBe(false)
      expect(list.element.style.flex).toBe('1')
      expect(button.isVisible()).toBe(true)
      expect(button.text()).toBe('▶')
      expect(button.attributes('aria-expanded')).toBe('false')
      expect(button.attributes('aria-label')).toBe('展开主机分组')
      expect(tooltip.props('title')).toBe('展开主机分组')
      expect(state.groupContextMenu.visible).toBe(false)
      await button.trigger('click')
      expect(panel.isVisible()).toBe(true)
      expect(button.text()).toBe('◀')
      expect(state.selectedGroupId).toBe(7)
      expect(state.selectedGroupName).toBe('testing')
      expect(state.groupSearchText).toBe('test')
      expect(state.searchText).toBe('host-01')
      expect(state.activeSearchText).toBe('host-01')
      expect(state.pagination.current).toBe(3)
      expect(state.state.selectedRowKeys).toEqual([101])
      expect(hostApi.getHostList).toHaveBeenCalledTimes(hostRequests)
      expect(hostGroupApi.getHostGroupTree).toHaveBeenCalledTimes(groupRequests)
    } finally {
      wrapper.unmount()
    }
  })

  it('使用真实 Empty 组件渲染空分组和搜索无结果，加载时不展示空状态', async () => {
    const wrapper = mount(HostPage, {
      global: {
        components: { AEmpty: Empty },
        stubs: { Dialog: true, FontAwesomeIcon: true },
        directives: { permission: () => {} },
      },
    })
    try {
      await flushPromises()
      const state = wrapper.vm.$.setupState
      expect(wrapper.find('.ant-empty-description').text()).toBe('暂无分组')
      state.groupLoading = true
      await wrapper.vm.$nextTick()
      expect(wrapper.findComponent(Empty).exists()).toBe(false)
      state.groupLoading = false
      state.groups = [{ key: 7, name: 'testing', host_count: 2 }]
      await wrapper.vm.$nextTick()
      expect(wrapper.findComponent(Empty).exists()).toBe(false)
      state.groupSearchText = 'no-match'
      await wrapper.vm.$nextTick()
      expect(wrapper.find('.ant-empty-description').text()).toBe('暂无分组')
      state.groupSearchText = ''
      await wrapper.vm.$nextTick()
      expect(wrapper.findComponent(Empty).exists()).toBe(false)
    } finally {
      wrapper.unmount()
    }
  })

  it('分组为单层列表，支持搜索、准确筛选和全部分组', async () => {
    hostGroupApi.getHostGroupTree.mockResolvedValueOnce({ data: { code: 200, data: [
      { id: 7, name: 'Testing', host_count: 2 },
      { id: 8, name: 'Production', host_count: 3 },
    ] } })
    const wrapper = shallowMount(HostPage, {
      global: { stubs: { Dialog: true, FontAwesomeIcon: true }, directives: { permission: () => {} } },
    })
    await flushPromises()
    const state = wrapper.vm.$.setupState
    expect(state.groups).toHaveLength(2)
    expect(state.groupOptions).toEqual([{ label: 'Testing', value: 7 }, { label: 'Production', value: 8 }])
    state.groupSearchText = ' test '
    expect(state.filteredGroups.map((group) => group.key)).toEqual([7])
    await state.onGroupSelect(state.groups[0])
    expect(state.selectedGroupName).toBe('Testing')
    expect(hostApi.getHostList).toHaveBeenLastCalledWith(expect.objectContaining({ group_id: 7, page: 1 }))
    await state.selectAllGroups()
    expect(state.selectedGroupId).toBe(0)
    expect(hostApi.getHostList.mock.calls.at(-1)[0]).not.toHaveProperty('group_id')
    wrapper.unmount()
  })

  it('非空分组不可删除，空分组通过公共确认并检查逐项结果', async () => {
    const warning = vi.spyOn(message, 'warning').mockImplementation(() => {})
    const wrapper = shallowMount(HostPage, {
      global: { stubs: { Dialog: true, FontAwesomeIcon: true }, directives: { permission: () => {} } },
    })
    await flushPromises()
    const state = wrapper.vm.$.setupState
    hostGroupApi.getHostGroupById.mockResolvedValueOnce({ data: { code: 200, data: { id: 7, name: 'testing', host_count: 2 } } })
    await state.openGroupDeleteConfirm({ key: 7 })
    expect(warning).toHaveBeenCalledWith(expect.stringContaining('2 台主机'))
    expect(openDeleteConfirm).not.toHaveBeenCalled()
    await state.openGroupDeleteConfirm({ key: 7 })
    expect(openDeleteConfirm).toHaveBeenCalledWith(expect.objectContaining({
      items: ['分组: testing (ID: 7)'], onConfirm: expect.any(Function),
    }))
    state.selectedGroupId = 7
    await openDeleteConfirm.mock.calls[0][0].onConfirm()
    expect(hostGroupApi.batchDeleteHostGroups).toHaveBeenCalledWith([7])
    expect(state.selectedGroupId).toBe(0)
    hostGroupApi.batchDeleteHostGroups.mockResolvedValueOnce({
      data: { code: 200, data: { results: [{ id: 7, ok: false, message: '分组仍被引用' }] } },
    })
    await expect(state.deleteGroup(7)).rejects.toThrow('分组仍被引用')
    warning.mockRestore()
    wrapper.unmount()
  })

  it('主机列表不展示 ID，保留内部选中 ID 和创建时间列', () => {
    const wrapper = shallowMount(HostPage, {
      global: {
        stubs: { Dialog: true, FontAwesomeIcon: true },
        directives: { permission: () => {} },
      },
    })
    const state = wrapper.vm.$.setupState
    expect(state.columns.some((column) => column.key === 'id' || column.dataIndex === 'id')).toBe(false)
    expect(state.columns[0]).toMatchObject({ title: '实例名', dataIndex: 'instance_name' })
    state.onSelectChange([101])
    expect(state.state.selectedRowKeys).toEqual([101])
    expect(state.columns.at(-2)).toMatchObject({
      title: '创建时间', dataIndex: 'create_time', key: 'create_time', width: 190,
      sorter: true,
    })
    expect(state.columns.at(-1)).toMatchObject({ key: 'action', fixed: 'right' })
    expect(state.formatDateTime('2026-10-22T00:00:00Z')).toBe('2026-10-22T00:00:00Z')
    expect(state.formatDateTime(null)).toBe('-')
    wrapper.unmount()
  })

  it('创建时间排序跨页保留，切换及取消排序回到第一页', async () => {
    const wrapper = shallowMount(HostPage, {
      global: {
        stubs: { Dialog: true, FontAwesomeIcon: true },
        directives: { permission: () => {} },
      },
    })
    await flushPromises()
    const state = wrapper.vm.$.setupState
    for (const [order, expected] of [['ascend', 'create_time'], ['descend', '-create_time'], [undefined, '']]) {
      await state.handleTableChange({ current: 3, pageSize: 10 }, {}, { field: 'create_time', order })
      expect(hostApi.getHostList).toHaveBeenLastCalledWith(expect.objectContaining({
        page: 1, ordering: expected,
      }))
      await state.handleTableChange({ current: 2, pageSize: 10 }, {}, { field: 'create_time', order })
      expect(hostApi.getHostList).toHaveBeenLastCalledWith(expect.objectContaining({
        page: 2, ordering: expected,
      }))
    }
    wrapper.unmount()
  })

  it('编辑主机并保存应提交正确的表单字段', async () => {
    const wrapper = shallowMount(HostPage, {
      global: {
        stubs: {
          Dialog: true,
          FontAwesomeIcon: true,
        },
        directives: {
          permission: () => {},
        },
      },
    })

    await flushPromises()

    const setupState = wrapper.vm.$.setupState

    await setupState.onSaveOrCreate(101)
    await flushPromises()

    expect(setupState.form.id).toBe(101)
    expect(setupState.form.instance_name).toBe('pvg-esb4-201')
    expect(setupState.form.ip).toBe('10.25.66.201')

    setupState.form.instance_name = 'pvg-esb4-201-renamed'
    setupState.formRef = {
      validate: vi.fn(() => Promise.resolve()),
    }

    setupState.handleOk()
    await flushPromises()

    expect(hostApi.saveOrCreateHost).toHaveBeenCalledTimes(1)
    const payload = hostApi.saveOrCreateHost.mock.calls[0][0]
    expect(payload.id).toBe(101)
    expect(payload.instance_name).toBe('pvg-esb4-201-renamed')
  })

  it('Agent 安装弹窗选择 SSH 凭证后提交应携带 credential_id', async () => {
    const wrapper = shallowMount(HostPage, {
      global: {
        stubs: {
          'a-modal': { template: '<div><slot /></div>' },
          'a-form': { template: '<form><slot /></form>' },
          'a-form-item': { template: '<div><slot /></div>' },
          'a-select': true,
          Dialog: true,
          FontAwesomeIcon: true,
        },
        directives: {
          permission: () => {},
        },
      },
    })

    await flushPromises()

    const setupState = wrapper.vm.$.setupState
    setupState.state.selectedRowKeys = [101]
    setupState.datasources = [{ id: 101, instance_name: 'host-101', agent_online: false }]

    setupState.openAgentManage()
    await flushPromises()

    expect(setupState.agentManageVisible).toBe(true)
    setupState.agentManageCredentialId = 2

    await setupState.submitAgentManage()
    await flushPromises()

    expect(hostApi.installAgents).toHaveBeenCalledTimes(1)
    const payload = hostApi.installAgents.mock.calls[0][0]
    expect(payload.host_ids).toEqual([101])
    expect(payload.credential_id).toBe(2)
    expect(payload.operation).toBe('install')
  })

  it('blocks update submission when the selected host agent is offline', async () => {
    const wrapper = shallowMount(HostPage, {
      global: {
        stubs: {
          'a-modal': { template: '<div><slot /></div>' },
          'a-form': { template: '<form><slot /></form>' },
          'a-form-item': { template: '<div><slot /></div>' },
          'a-select': true,
          Dialog: true,
          FontAwesomeIcon: true,
        },
        directives: {
          permission: () => {},
        },
      },
    })

    await flushPromises()

    const setupState = wrapper.vm.$.setupState
    setupState.state.selectedRowKeys = [101]
    setupState.datasources = [{ id: 101, instance_name: 'host-101', agent_online: false }]

    setupState.openAgentManage()
    await flushPromises()

    // 离线主机的默认操作是 install；这里显式切到 update 以验证提交守卫。
    setupState.agentManageOperation = 'update'
    setupState.agentManageCredentialId = 2

    await setupState.submitAgentManage()
    await flushPromises()

    expect(hostApi.installAgents).not.toHaveBeenCalled()
  })

  it('updates an online host without requiring an SSH credential', async () => {
    const wrapper = shallowMount(HostPage, {
      global: {
        stubs: {
          'a-modal': { template: '<div><slot /></div>' },
          'a-form': { template: '<form><slot /></form>' },
          'a-form-item': { template: '<div><slot /></div>' },
          'a-select': true,
          Dialog: true,
          FontAwesomeIcon: true,
        },
        directives: {
          permission: () => {},
        },
      },
    })

    await flushPromises()

    const setupState = wrapper.vm.$.setupState
    setupState.state.selectedRowKeys = [101]
    setupState.datasources = [{ id: 101, instance_name: 'host-101', agent_online: true }]

    setupState.openAgentManage()
    await flushPromises()

    expect(setupState.agentManageOperation).toBe('update')
    // 更新不需要选 SSH 凭证：不设置 agentManageCredentialId 也应该能提交成功。

    await setupState.submitAgentManage()
    await flushPromises()

    expect(hostApi.installAgents).toHaveBeenCalledTimes(1)
    const payload = hostApi.installAgents.mock.calls[0][0]
    expect(payload.operation).toBe('update')
    expect(payload.credential_id).toBeUndefined()
  })
})
