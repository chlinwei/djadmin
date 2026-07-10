import { describe, it, expect, vi, beforeEach } from 'vitest'
import { reactive } from 'vue'
import { shallowMount } from '@vue/test-utils'

vi.mock('@/api/assets/host/index.js', () => ({
  batchDeleteHost: vi.fn(() => Promise.resolve({ data: { code: 200, data: {} } })),
  collectHostInfo: vi.fn(() => Promise.resolve({ data: { code: 200, data: { status: 'collected' } } })),
  batchCollectHostInfo: vi.fn(() => Promise.resolve({ data: { code: 200, data: { results: [] } } })),
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
          credential: { id: 1, name: 'esb' },
          port: 22,
          remark: '',
        },
      },
    })
  ),
  getHostList: vi.fn(() => Promise.resolve({ data: { code: 200, data: { results: [], count: 0 } } })),
  saveOrCreateHost: vi.fn(() => Promise.resolve({ data: { code: 200, msg: 'success' } })),
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

describe('Host 编辑凭证保存', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('切换 SSH 凭证后保存应提交新 credential_id', async () => {
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

    expect(setupState.form.credential_id).toBe(1)

    // 模拟用户在下拉中选择 common（id=2）
    setupState.form.credential_id = 2
    setupState.formRef = {
      validate: vi.fn(() => Promise.resolve()),
    }

    setupState.handleOk()
    await flushPromises()

    expect(hostApi.saveOrCreateHost).toHaveBeenCalledTimes(1)
    const payload = hostApi.saveOrCreateHost.mock.calls[0][0]
    expect(payload.id).toBe(101)
    expect(payload.credential_id).toBe(2)
  })

  it('通过下拉事件切换凭证后，保存应提交新 credential_id', async () => {
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
    await setupState.onSaveOrCreate(101)
    await flushPromises()

    setupState.formRef = {
      validate: vi.fn(() => Promise.resolve()),
    }

    const selectStubs = wrapper.findAllComponents({ name: 'a-select' })
    expect(selectStubs.length).toBeGreaterThan(0)

    // 页面有多个 a-select，最后一个是编辑弹窗里的 SSH 凭证选择。
    const credentialSelect = selectStubs[selectStubs.length - 1]
    credentialSelect.vm.$emit('update:value', 2)
    credentialSelect.vm.$emit('change', 2)
    await flushPromises()

    expect(setupState.form.credential_id).toBe(2)

    setupState.handleOk()
    await flushPromises()

    expect(hostApi.saveOrCreateHost).toHaveBeenCalledTimes(1)
    const payload = hostApi.saveOrCreateHost.mock.calls[0][0]
    expect(payload.id).toBe(101)
    expect(payload.credential_id).toBe(2)
  })
})
