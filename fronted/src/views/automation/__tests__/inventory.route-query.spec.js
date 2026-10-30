import { mount, flushPromises } from '@vue/test-utils'
import { nextTick, reactive } from 'vue'
import InventoryPage from '../inventory/index.vue'
import { message } from 'ant-design-vue'
import { batchDeleteInventories } from '@/api/sys/automation'

const routeState = reactive({
  query: {},
})

const routerPush = vi.fn()
const routerResolve = vi.fn(() => ({ matched: [] }))

vi.mock('vue-router', () => ({
  useRoute: () => routeState,
  useRouter: () => ({
    push: routerPush,
    resolve: routerResolve,
  }),
}))

const getInventoryList = vi.fn()
const getAutomationHostOptions = vi.fn()
const getAutomationGroupTree = vi.fn()

vi.mock('@/api/sys/automation', () => ({
  getInventoryList: (...args) => getInventoryList(...args),
  createInventory: vi.fn(),
  updateInventory: vi.fn(),
  batchDeleteInventories: vi.fn(),
  getAutomationHostOptions: (...args) => getAutomationHostOptions(...args),
  getAutomationGroupTree: (...args) => getAutomationGroupTree(...args),
}))

const componentStubs = {
  'a-row': true,
  'a-col': true,
  'a-input-search': true,
  'a-space': true,
  'a-button': true,
  'a-card': true,
  'a-table': true,
  'a-tag': true,
  'a-tooltip': true,
  'a-popconfirm': true,
  'a-modal': true,
  'a-form': true,
  'a-form-item': true,
  'a-input': true,
  'a-switch': true,
  'a-textarea': true,
  'a-tree': true,
  'a-empty': true,
  FontAwesomeIcon: true,
}

function mockCommonApiResponses() {
  getAutomationGroupTree.mockResolvedValue({
    data: { data: [] },
  })
  getAutomationHostOptions.mockResolvedValue({
    data: {
      data: {
        count: 0,
        next: null,
        results: [],
      },
    },
  })
  getInventoryList.mockResolvedValue({
    data: {
      data: {
        count: 0,
        results: [],
      },
    },
  })
}

describe('inventory route query auto search', () => {
  beforeEach(() => {
    routeState.query = {}
    routerPush.mockReset()
    routerResolve.mockClear()
    getInventoryList.mockReset()
    getAutomationHostOptions.mockReset()
    getAutomationGroupTree.mockReset()
    mockCommonApiResponses()
  })

  function mountPage() {
    return mount(InventoryPage, {
      global: {
        stubs: componentStubs,
        directives: {
          permission: {},
        },
      },
    })
  }

  it.each([true, false])('删除逐项结果 ok=%s 时正确提示', async (ok) => {
    const success = vi.spyOn(message, 'success').mockImplementation(() => {})
    const error = vi.spyOn(message, 'error').mockImplementation(() => {})
    batchDeleteInventories.mockResolvedValueOnce({
      data: { data: { results: [{ id: 9, ok, message: 'Inventory 被任务引用' }] } },
    })
    const wrapper = mountPage()
    await flushPromises()
    const loads = getInventoryList.mock.calls.length
    try {
      const deletion = wrapper.vm.removeRecord({ id: 9 })
      if (ok) {
        await deletion
        expect(success).toHaveBeenCalledWith('Inventory 已删除')
        expect(getInventoryList.mock.calls.length).toBe(loads + 1)
      } else {
        await expect(deletion).rejects.toThrow('Inventory 被任务引用')
        expect(error).toHaveBeenCalledWith(expect.stringContaining('Inventory 被任务引用'))
        expect(success).not.toHaveBeenCalled()
        expect(getInventoryList.mock.calls.length).toBe(loads)
      }
    } finally {
      wrapper.unmount()
      success.mockRestore()
      error.mockRestore()
    }
  })

  it('uses search query when opening page', async () => {
    routeState.query = {
      search: 'inventory-from-search',
      inventory_name: 'inventory-from-name',
    }

    const wrapper = mountPage()

    await flushPromises()

    expect(getInventoryList).toHaveBeenCalled()
    const firstCallPayload = getInventoryList.mock.calls[0][0]
    expect(firstCallPayload.search).toBe('inventory-from-search')
    wrapper.unmount()
  })

  it('falls back to inventory_name when search is missing', async () => {
    routeState.query = {
      inventory_name: 'inventory-from-name',
    }

    const wrapper = mountPage()

    await flushPromises()

    expect(getInventoryList).toHaveBeenCalled()
    const lastCallPayload = getInventoryList.mock.calls.at(-1)[0]
    expect(lastCallPayload.search).toBe('inventory-from-name')
    wrapper.unmount()
  })

  it('reloads list when route query changes after mount', async () => {
    routeState.query = { search: 'before-change' }

    const wrapper = mountPage()

    await flushPromises()
    getInventoryList.mockClear()

    routeState.query = { search: 'after-change' }
    await nextTick()
    await flushPromises()

    expect(getInventoryList.mock.calls.length).toBeGreaterThan(0)
    const payload = getInventoryList.mock.calls.at(-1)[0]
    expect(payload.search).toBe('after-change')
    wrapper.unmount()
  })

  it('keeps independent groups and host selections in both scope trees', async () => {
    getAutomationGroupTree.mockResolvedValue({
      data: { data: [{ id: 1, name: 'production', children: [] }, { id: 2, name: 'testing', children: [] }] },
    })
    getAutomationHostOptions.mockResolvedValue({
      data: { data: { count: 2, next: null, results: [
        { id: 201, instance_name: 'prod-01', ip: '10.0.0.1', group_id: 1 },
        { id: 202, instance_name: 'test-01', ip: '10.0.0.2', group_id: 2 },
      ] } },
    })
    const wrapper = mountPage()
    try {
      await flushPromises()
      expect(wrapper.vm.scopeEditTreeData.map((group) => group.key)).toEqual(['group-1', 'group-2'])
      expect(wrapper.vm.scopeEditTreeData.map((group) => group.children[0].host_id)).toEqual([201, 202])
      wrapper.vm.openScopeViewer({ name: 'both groups', selected_host_ids: [201, 202] })
      await nextTick()
      expect(wrapper.vm.scopeViewTreeData.map((group) => group.children[0].host_id)).toEqual([201, 202])
      expect(wrapper.vm.scopeViewSummary).toBe('当前范围：2台主机')
    } finally {
      wrapper.unmount()
    }
  })

  it('uses string instance names in both scope trees, search and host navigation', async () => {
    getAutomationGroupTree.mockResolvedValue({
      data: { data: [{ id: 1, name: 'wuhan', children: [] }] },
    })
    getAutomationHostOptions.mockResolvedValue({
      data: { data: { count: 4, next: null, results: [
        { id: 201, instance_name: 'hak-app-201', hostname: 'os-host-201', ip: '10.25.66.201', group_id: 1 },
        { id: 202, instance_name: null, hostname: null, ip: '10.25.66.202', group_id: 1 },
        { id: 203, instance_name: '', hostname: '', ip: '10.25.66.203', group_id: 1 },
        { id: 204, instance_name: 'not-selected', ip: '10.25.66.204', group_id: 1 },
      ] } },
    })
    const wrapper = mountPage()
    try {
      await flushPromises()
      expect(wrapper.vm.scopeEditTreeData[0].children.map((node) => node.title)).toEqual([
        'hak-app-201(10.25.66.201)', '10.25.66.202', '10.25.66.203', 'not-selected(10.25.66.204)',
      ])
      wrapper.vm.openScopeViewer({ name: 'all', selected_host_ids: [201, 202, 203] })
      await nextTick()
      expect(wrapper.vm.scopeViewTreeData[0].title).toBe('wuhan（3台）')
      expect(wrapper.vm.scopeViewTreeData[0].children.map((node) => node.title)).toEqual([
        'hak-app-201(10.25.66.201)', '10.25.66.202', '10.25.66.203',
      ])
      expect(wrapper.vm.scopeViewSummary).toBe('当前范围：3台主机')
      wrapper.vm.scopeViewKeyword = 'hak-app-201'
      await nextTick()
      const matches = wrapper.vm.filteredScopeViewTreeData[0].children
      expect(matches).toHaveLength(1)
      expect(matches[0].host_id).toBe(201)
      routerResolve.mockReturnValueOnce({ matched: [{}] })
      wrapper.vm.goToAssetHostByNode(matches[0])
      expect(routerPush).toHaveBeenLastCalledWith({
        path: '/assets/hosts',
        query: { host_id: '201', search: 'hak-app-201' },
      })
    } finally {
      wrapper.unmount()
    }
  })
})
