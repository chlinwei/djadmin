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
  'a-descriptions': true,
  'a-descriptions-item': true,
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
    getAutomationHostOptions.mockResolvedValue({
      data: { data: { count: 1, next: null, results: [
        { id: 201, instance_name: 'prod-01', ip: '10.0.0.1', group_id: 1 },
      ] } },
    })
    const wrapper = mount(InventoryPage, {
      attachTo: document.body,
      global: { plugins: [Antd], directives: { permission }, stubs: { FontAwesomeIcon: true } },
    })
    try {
      await flushPromises()
      const view = wrapper.get('[data-view-inventory]')
      expect(view.isVisible()).toBe(true)
      expect(view.classes()).toContain('ant-btn-link')
      expect(view.classes()).toContain('detail-action')
      expect(view.text()).toBe('详情')
      const tooltip = wrapper.findAllComponents({ name: 'ATooltip' })
        .find((item) => item.props('title') === '查看详情')
      expect(tooltip.props('placement')).toBe('top')
      await view.trigger('mouseenter')
      await new Promise((resolve) => setTimeout(resolve, 150))
      await flushPromises()
      expect(document.body.querySelector('.ant-tooltip-inner')?.textContent).toBe('查看详情')
      const visibleButtons = wrapper.findAll('tbody button').filter((button) => button.isVisible())
      expect(visibleButtons).toHaveLength(3) // 主机数量、禁用状态开关与查看详情。
      expect(wrapper.get('tbody .ant-switch').attributes('disabled')).toBeDefined()
      expect(wrapper.findComponent({ name: 'ATable' }).props('scroll')).toEqual({ x: 1140 })
      expect(wrapper.vm.columns.find((column) => column.key === 'action')).toMatchObject({ fixed: 'right', width: 220 })
      await view.trigger('click')
      await flushPromises()
      const dialog = document.body.querySelector('.ant-modal-content')
      expect(dialog.textContent).toContain('查看 Inventory 详情 - Production Inventory')
      expect(dialog.textContent).toContain('生产主机只读查看')
      expect(dialog.textContent).toContain('已选主机可用')
      expect(dialog.textContent).toContain('当前范围：1台主机')
      expect(dialog.textContent).toContain('prod-01(10.0.0.1)')
      expect(dialog.querySelector('.ant-modal-footer')).toBeNull()
      expect(dialog.querySelector('.ant-switch').disabled).toBe(true)
      expect(dialog.querySelector('.ant-switch').getAttribute('aria-checked')).toBe('true')
      expect(dialog.querySelectorAll('input.ant-input')).toHaveLength(1)
      expect(dialog.querySelector('input.ant-input').placeholder).toBe('搜索分组/主机/IP')
      expect(dialog.querySelector('form')).toBeNull()
      expect(wrapper.vm.modalVisible).toBe(false)
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

  it('查看空范围仍显示状态、健康信息及备注占位，不复用编辑表单', async () => {
    const wrapper = mountPage()
    try {
      await flushPromises()
      wrapper.vm.openScopeViewer({
        id: 9, name: 'Empty Inventory', enabled: false, selected_host_ids: [],
        resolved_host_count: 0, health_status: { status: 'empty', label: '空范围', message: '未选择主机' },
      })
      await nextTick()
      expect(wrapper.vm.scopeViewInventory).toMatchObject({ name: 'Empty Inventory', enabled: false })
      expect(wrapper.vm.scopeViewTitle).toBe('查看 Inventory 详情 - Empty Inventory')
      expect(wrapper.vm.scopeViewSummary).toBe('当前范围：0台主机')
      expect(wrapper.vm.scopeViewTreeData).toEqual([])
      expect(wrapper.vm.modalVisible).toBe(false)
    } finally {
      wrapper.unmount()
    }
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
