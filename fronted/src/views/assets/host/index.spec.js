import { describe, it, expect, vi, beforeEach } from 'vitest'
import { reactive } from 'vue'
import { shallowMount } from '@vue/test-utils'

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
  batchDeleteHostGroups: vi.fn(() => Promise.resolve({ data: { code: 200 } })),
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
    HOSTGROUP_MAX_TREE_DEPTH: 'sys.assets.hostgroup.max_tree_depth',
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
import HostPage from './index.vue'

const flushPromises = async () => {
  await Promise.resolve()
  await new Promise((resolve) => setTimeout(resolve, 0))
}

describe('Host Agent 凭证安装与主机编辑保存', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('主机创建时间为最后一个数据列并按用户时区格式化', () => {
    const wrapper = shallowMount(HostPage, {
      global: {
        stubs: { Dialog: true, FontAwesomeIcon: true },
        directives: { permission: () => {} },
      },
    })
    const state = wrapper.vm.$.setupState
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
