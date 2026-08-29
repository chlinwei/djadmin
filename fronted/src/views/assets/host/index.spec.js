import { describe, it, expect, vi, beforeEach } from 'vitest'
import { reactive } from 'vue'
import { shallowMount } from '@vue/test-utils'

vi.mock('@/api/assets/host/index.js', () => ({
  batchDeleteHost: vi.fn(() => Promise.resolve({ data: { code: 200, data: {} } })),
  deleteHostById: vi.fn(() => Promise.resolve({ data: { code: 200 } })),
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

vi.mock('@/api/assets/hostgroup/index.js', () => ({
  getHostGroupTree: vi.fn(() => Promise.resolve({ data: { code: 200, data: [] } })),
  deleteHostGroupById: vi.fn(() => Promise.resolve({ data: { code: 200 } })),
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
    setupState.datasources = [{ id: 101, agent_id: '' }]

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
})
