import { mount, flushPromises } from '@vue/test-utils'
import { nextTick, reactive } from 'vue'
import InventoryPage from '../inventory/index.vue'
import Antd, { message } from 'ant-design-vue'
import { batchDeleteInventories, createInventory, updateInventory } from '@/api/sys/automation'
import store from '@/store'
import permission from '@/directives/permission/index.js'

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

const getInventory = vi.fn()
const getInventoryList = vi.fn()
const getAutomationHostOptions = vi.fn()
const getAutomationGroupTree = vi.fn()

vi.mock('@/api/sys/automation', () => ({
  getInventory: (...args) => getInventory(...args),
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
  'a-descriptions': true,
  'a-descriptions-item': true,
  FontAwesomeIcon: true,
}

function hostOptionsPage(results, { count = results.length, page = 1, pageSize = 1000 } = {}) {
  return {
    data: { code: 200, data: {
      count, pageNumber: page, pageSize, totalPages: Math.ceil(count / pageSize), next: null, results,
    } },
  }
}

function mockCommonApiResponses() {
  getAutomationGroupTree.mockResolvedValue({
    data: { data: [] },
  })
  getAutomationHostOptions.mockResolvedValue(hostOptionsPage([]))
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
  const originalRoles = store.state.role_codes
  afterEach(() => { store.state.role_codes = originalRoles })
  beforeEach(() => {
    store.state.role_codes = ['admin']
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

  it('只读用户可通过操作列查看详情与范围，但不能编辑、删除或切换状态', async () => {
    const originalRoles = store.state.role_codes
    const originalPerms = store.state.perms
    store.state.role_codes = ['readonly']
    store.state.perms = ['automation:inventory:view']
    vi.clearAllMocks()
    getInventoryList.mockResolvedValue({
      data: { data: { count: 1, results: [{
        id: 9, name: 'Production Inventory', enabled: true, remark: '生产主机只读查看',
        selected_host_ids: [201], resolved_host_count: 1,
        health_status: { status: 'healthy', label: '正常', message: '已选主机可用' },
      }] } },
    })
    getAutomationGroupTree.mockResolvedValue({
      data: { data: [{ id: 1, name: 'production', children: [] }] },
    })
    getAutomationHostOptions.mockResolvedValue(hostOptionsPage([
      { id: 201, instance_name: 'prod-01', ip: '10.0.0.1', group_id: 1 },
    ]))
    const wrapper = mount(InventoryPage, {
      attachTo: document.body,
      global: { plugins: [Antd], directives: { permission }, stubs: { FontAwesomeIcon: true } },
    })
    try {
      await flushPromises()
      getInventory.mockResolvedValue({ data: { data: {
        ...wrapper.vm.records[0], selected_host_ids: [201, 202], selected_host_names: { 201: 'prod-01', 202: 'no-ip' },
        resolved_hosts: [{ host_id: 201, host_name: 'prod-01', host_ip: '10.0.0.1', group_id: 1, group_name: 'production' }],
      } } })
      await wrapper.get('[data-inventory-form]').trigger('click')
      await flushPromises()
      const dialog = document.body.querySelector('.ant-modal-content')
      expect(dialog.querySelector('.ant-modal-title').textContent).toBe('查看Inventory')
      expect(dialog.querySelector('input').value).toBe('Production Inventory')
      expect(dialog.querySelector('textarea').value).toContain('production / prod-01')
      expect(dialog.querySelector('.ant-switch').disabled).toBe(true)
      expect(dialog.querySelector('.ant-modal-footer').textContent.replace(/\s/g, '')).toBe('关闭')
      wrapper.vm.openScopeModal()
      expect(wrapper.vm.scopeModalVisible).toBe(true)
      expect(wrapper.vm.scopeEditTreeData[1].children[0].title).toBe('no-ip')
      wrapper.vm.onScopeCheck([])
      expect(wrapper.vm.form.selected_host_ids).toEqual([201, 202])
      await wrapper.vm.submitForm()
      await wrapper.vm.openScopeViewer(wrapper.vm.records[0])
      expect(wrapper.vm.scopeViewTreeData[0].children[0].title).toBe('prod-01(10.0.0.1)')
      expect(wrapper.vm.scopeViewTreeData).toHaveLength(1)
      expect(getAutomationHostOptions).not.toHaveBeenCalled()
      expect(getAutomationGroupTree).not.toHaveBeenCalled()
      expect(createInventory).not.toHaveBeenCalled()
      expect(updateInventory).not.toHaveBeenCalled()
      expect(batchDeleteInventories).not.toHaveBeenCalled()
    } finally {
      wrapper.unmount()
      store.state.role_codes = originalRoles
      store.state.perms = originalPerms
      document.body.innerHTML = ''
    }
  })

  it('实际主机清单也支持停用的空 Inventory', async () => {
    const roles = store.state.role_codes
    store.state.role_codes = ['admin']
    const wrapper = mountPage()
    try {
      await flushPromises()
      getInventory.mockResolvedValue({ data: { data: { name: 'Empty', enabled: false, resolved_hosts: [] } } })
      await wrapper.vm.openScopeViewer({ id: 9 })
      expect(wrapper.vm.scopeViewSummary).toBe('当前范围：0台主机')
      expect(wrapper.vm.scopeViewTreeData).toEqual([])
      expect(wrapper.vm.modalVisible).toBe(false)
      expect(getAutomationHostOptions).not.toHaveBeenCalled()
    } finally { wrapper.unmount(); store.state.role_codes = roles }
  })

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
    getAutomationHostOptions.mockResolvedValue(hostOptionsPage([
      { id: 201, instance_name: 'prod-01', ip: '10.0.0.1', group_id: 1 },
      { id: 202, instance_name: 'test-01', ip: '10.0.0.2', group_id: 2 },
    ]))
    const wrapper = mountPage()
    try {
      await flushPromises()
      await wrapper.vm.loadScopeCandidates()
      expect(wrapper.vm.scopeEditTreeData.map((group) => group.key)).toEqual(['group-1', 'group-2'])
      expect(wrapper.vm.scopeEditTreeData.map((group) => group.children[0].host_id)).toEqual([201, 202])
      getInventory.mockResolvedValue({ data: { data: { name: 'both groups', resolved_hosts: wrapper.vm.hostOptions.filter(h => [201, 202].includes(h.id)).map(h => ({
        host_id: h.id, host_name: h.instance_name, host_ip: h.ip, group_id: h.group_id, group_name: wrapper.vm.groupPathMap[h.group_id],
      })) } } })
      await wrapper.vm.openScopeViewer({ id: 9 })
      await nextTick()
      expect(wrapper.vm.scopeViewTreeData.map((group) => group.children[0].host_id)).toEqual([201, 202])
      expect(wrapper.vm.scopeViewSummary).toBe('当前范围：2台主机')
    } finally {
      wrapper.unmount()
    }
  })

  it('loads all host pages at the effective page size before selecting an entire group', async () => {
    getAutomationGroupTree.mockResolvedValue({
      data: { data: [{ id: 1, name: 'wuhan' }, { id: 2, name: 'other' }] },
    })
    const hosts = Array.from({ length: 32 }, (_, index) => ({
      id: index + 1, ip: `10.0.0.${index + 1}`, group_id: index === 29 ? 2 : 1,
    }))
    getAutomationHostOptions
      .mockResolvedValueOnce(hostOptionsPage(hosts.slice(0, 30), { count: 32, pageSize: 30 }))
      .mockResolvedValueOnce(hostOptionsPage(hosts.slice(30), { count: 32, pageSize: 30, page: 2 }))
    const wrapper = mountPage()
    try {
      await flushPromises()
      await wrapper.vm.loadScopeCandidates()
      expect(getAutomationHostOptions.mock.calls).toEqual([
        [{ page: 1, page_size: 1000 }], [{ page: 2, page_size: 30 }],
      ])
      const wuhan = wrapper.vm.scopeEditTreeData.find(({ key }) => key === 'group-1')
      expect(wuhan.children).toHaveLength(31)
      wrapper.vm.onScopeCheck([wuhan.key, ...wuhan.children.map(({ key }) => key)])
      expect(wrapper.vm.form.selected_host_ids).toEqual(hosts.filter(({ group_id }) => group_id === 1).map(({ id }) => id))
    } finally {
      wrapper.unmount()
    }
  })

  it('does not allow saving a scope when a later page is incomplete', async () => {
    getAutomationHostOptions
      .mockResolvedValueOnce(hostOptionsPage([{ id: 1, ip: '10.0.0.1', group_id: 1 }], { count: 2, pageSize: 1 }))
      .mockResolvedValueOnce(hostOptionsPage([], { count: 2, pageSize: 1, page: 2 }))
    const error = vi.spyOn(message, 'error').mockImplementation(() => {})
    const warning = vi.spyOn(message, 'warning').mockImplementation(() => {})
    const wrapper = mountPage()
    try {
      await flushPromises()
      await wrapper.vm.loadScopeCandidates()
      wrapper.vm.form.name = 'wuhan'
      wrapper.vm.openScopeModal()
      await wrapper.vm.submitForm()
      expect(error).toHaveBeenCalledWith(expect.stringContaining('主机范围加载失败'))
      expect(wrapper.vm.scopeModalVisible).toBe(false)
      expect(updateInventory).not.toHaveBeenCalled()
    } finally {
      error.mockRestore()
      warning.mockRestore()
      wrapper.unmount()
    }
  })

  it('uses string instance names in both scope trees, search and host navigation', async () => {
    getAutomationGroupTree.mockResolvedValue({
      data: { data: [{ id: 1, name: 'wuhan', children: [] }] },
    })
    getAutomationHostOptions.mockResolvedValue(hostOptionsPage([
      { id: 201, instance_name: 'hak-app-201', hostname: 'os-host-201', ip: '10.25.66.201', group_id: 1 },
      { id: 202, instance_name: null, hostname: null, ip: '10.25.66.202', group_id: 1 },
      { id: 203, instance_name: '', hostname: '', ip: '10.25.66.203', group_id: 1 },
      { id: 204, instance_name: 'not-selected', ip: '10.25.66.204', group_id: 1 },
    ]))
    const wrapper = mountPage()
    try {
      await flushPromises()
      await wrapper.vm.loadScopeCandidates()
      expect(wrapper.vm.scopeEditTreeData[0].children.map((node) => node.title)).toEqual([
        'hak-app-201(10.25.66.201)', '10.25.66.202', '10.25.66.203', 'not-selected(10.25.66.204)',
      ])
      getInventory.mockResolvedValue({ data: { data: { name: 'all', resolved_hosts: wrapper.vm.hostOptions.filter(h => [201, 202, 203].includes(h.id)).map(h => ({
        host_id: h.id, host_name: h.instance_name, host_ip: h.ip, group_id: h.group_id, group_name: wrapper.vm.groupPathMap[h.group_id],
      })) } } })
      await wrapper.vm.openScopeViewer({ id: 9 })
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
