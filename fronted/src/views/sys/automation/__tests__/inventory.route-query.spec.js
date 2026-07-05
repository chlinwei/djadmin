import { mount, flushPromises } from '@vue/test-utils'
import { nextTick, reactive } from 'vue'
import InventoryPage from '../inventory.vue'

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
  deleteInventory: vi.fn(),
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
})
