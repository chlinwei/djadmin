import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { Collapse, Table } from 'ant-design-vue'
import ScanDetail from './scanDetail.vue'
import requestUtil from '@/util/request'

vi.mock('vue-router', () => ({
  useRoute: () => ({ params: { id: '5' } }),
  useRouter: () => ({ push: vi.fn(), back: vi.fn() }),
}))
vi.mock('@/util/request', () => ({ default: { get: vi.fn() } }))

const slotStub = { template: '<div><slot /></div>' }
const descriptionStub = {
  props: ['label'],
  template: '<div :data-label="label"><slot /></div>',
}
const selectStub = {
  props: ['children', 'value', 'options', 'placeholder'],
  emits: ['update:value'],
  template: '<div />',
}
const stubs = {
  'a-card': slotStub, 'a-spin': slotStub, 'a-descriptions': slotStub,
  'a-descriptions-item': descriptionStub, 'a-space': slotStub, 'a-button': true,
  'a-popconfirm': true, 'a-alert': true, 'a-progress': true, 'a-tooltip': true,
  'a-popover': true, 'a-table': true, 'a-tag': slotStub, 'a-input-search': true,
  'a-select': selectStub,
  'a-collapse': true, 'a-collapse-panel': true,
  'a-radio-group': true, 'a-radio-button': true, FontAwesomeIcon: true,
}

describe('扫描详情的项目与环境', () => {
  let computedStyleSpy
  beforeEach(() => {
    const getComputedStyle = window.getComputedStyle.bind(window)
    // jsdom 不支持 Ant Design Table 测量滚动条时传入的伪元素参数。
    computedStyleSpy = vi.spyOn(window, 'getComputedStyle').mockImplementation((element) => getComputedStyle(element))
  })
  afterEach(() => computedStyleSpy.mockRestore())

  it.each([{ details: [] }, { details: [{ property: 'assertion', title: '不完整断言', successful: true, actual: 'partial' }] }])(
    '异常结果不展示违规零项或部分成功断言', async ({ details }) => {
      requestUtil.get.mockResolvedValueOnce({ data: { data: {
        scan: { id: 5, status: 'failed' }, targets: [],
        items: [{ status: 'error', message: '检查未返回有效断言，无法验证通过',
          actual: { violation_count: 0, violations: [], details } }],
      } } })
      const actualTable = {
        props: ['rowKey', 'dataSource'],
        template: `<div><template v-if="rowKey === 'idx'"><slot v-for="row in dataSource"
          name="bodyCell" :column="{ key: 'message' }" :record="row" /></template></div>`,
      }
      const wrapper = mount(ScanDetail, { global: { stubs: { ...stubs, 'a-table': actualTable, 'a-popover': slotStub } } })
      await flushPromises()
      expect(wrapper.text()).toContain('检查未返回有效断言，无法验证通过')
      expect(wrapper.text()).not.toContain('违规 0 项')
      expect(wrapper.text()).not.toContain('✓ 实际')
      wrapper.unmount()
    },
  )

  it('执行异常不计为不符合或通过，可单独筛选', async () => {
    requestUtil.get.mockResolvedValueOnce({ data: { data: {
      scan: { id: 5, status: 'failed' }, targets: [],
      items: [{ status: 'pass' }, { status: 'fail' }, { status: 'error', message: '采集失败' }],
    } } })
    const wrapper = mount(ScanDetail, { global: { stubs } })
    await flushPromises()
    expect(wrapper.vm.failedCount).toBe(1)
    expect(wrapper.vm.passedCount).toBe(1)
    expect(wrapper.vm.errorCount).toBe(1)
    wrapper.vm.statusFilter = 'error'
    await flushPromises()
    expect(wrapper.vm.filteredItems).toHaveLength(1)
    expect(wrapper.vm.filteredItems[0].message).toBe('采集失败')
    wrapper.unmount()
  })
  it('通过筛选隐藏消息和修复建议，其他状态恢复列且保留数据', async () => {
    requestUtil.get.mockResolvedValueOnce({ data: { data: {
      scan: { id: 5, status: 'failed' }, targets: [],
      items: [
        { host_id: 10, status: 'pass', message: '', remediation: '' },
        { host_id: 11, status: 'fail', message: '配置不符合', remediation: '修改配置' },
      ],
    } } })
    const tableStub = { props: ['rowKey', 'columns', 'scroll'], template: '<div />' }
    const wrapper = mount(ScanDetail, { global: { stubs: { ...stubs, 'a-table': tableStub } } })
    await flushPromises()
    const table = wrapper.findAllComponents(tableStub).find((component) => component.props('rowKey') === 'idx')
    const allKeys = table.props('columns').map((column) => column.key)
    expect(allKeys).toContain('message')
    expect(allKeys).toContain('remediation')
    wrapper.vm.statusFilter = 'pass'
    await flushPromises()
    expect(table.props('columns').map((column) => column.key)).toEqual(
      allKeys.filter((key) => key !== 'message' && key !== 'remediation'),
    )
    const expectScrollableColumns = () => {
      const columns = table.props('columns')
      expect(columns.every((column) => column.width > 0)).toBe(true)
      expect(table.props('scroll').x).toBeGreaterThanOrEqual(columns.reduce((sum, column) => sum + column.width, 0))
    }
    expectScrollableColumns()
    expect(wrapper.vm.filteredItems).toHaveLength(1)
    expect(wrapper.vm.filteredItems[0].status).toBe('pass')
    for (const status of ['fail', 'all']) {
      wrapper.vm.statusFilter = status
      await flushPromises()
      expect(table.props('columns').map((column) => column.key)).toEqual(allKeys)
      expectScrollableColumns()
    }
    expect(wrapper.vm.items[1].message).toBe('配置不符合')
    expect(wrapper.vm.items[1].remediation).toBe('修改配置')
    wrapper.unmount()
  })

  it('列表只展示摘要，展开后对照完整断言并可打开原始证据', async () => {
    const expected = { policy: 'package baseline\n# original policy' }
    const actual = {
      details: [
        { property: 'assertion', title: '账号有效期', expected: '≤ 90', actual: { accounts: [
          { account: 'root', max_days: '99999' }, { account: 'alice', max_days: '99999' },
        ] }, successful: false },
        { property: 'assertion', title: '默认有效期', expected: '≤ 90', actual: '90', successful: true },
      ],
      diagnostic: '完整原始证据标记',
    }
    const remediation = '修改配置并逐一核对现有账号。\n复查所有账号。'
    requestUtil.get.mockResolvedValueOnce({ data: { data: {
      scan: { id: 5, status: 'failed' }, targets: [],
      items: [{ item_name: '口令有效期', status: 'fail', actual, expected, remediation, message: '账号配置不符合' }],
    } } })
    const wrapper = mount(ScanDetail, { global: {
      components: { ATable: Table, ACollapse: Collapse, ACollapsePanel: Collapse.Panel },
      stubs: { ...stubs, 'a-table': false, 'a-collapse': false, 'a-collapse-panel': false },
    } })
    await flushPromises()
    const itemTable = wrapper.findAllComponents(Table).find((table) => table.props('rowKey') === 'idx')
    expect(itemTable.props('tableLayout')).toBe('fixed')
    expect(itemTable.props('columns').map((column) => column.key)).not.toContain('actual')
    expect(wrapper.text()).toContain('共 2 项检查，1 项不符合：账号有效期')
    expect(wrapper.text()).not.toContain('99999')
    expect(wrapper.text()).not.toContain('完整原始证据标记')

    await wrapper.find('.ant-table-row-expand-icon').trigger('click')
    await flushPromises()
    const detail = wrapper.find('.result-detail')
    expect(detail.text()).toContain('账号配置不符合')
    expect(detail.text()).toContain('复查所有账号。')
    expect(detail.findAll('.evidence-value').map((node) => node.text())).toContain(JSON.stringify(actual.details[0].actual, null, 2))
    expect(detail.text()).not.toContain('[object Object]')
    expect(detail.find('.evidence-grid').exists()).toBe(false)

    await detail.find('.ant-collapse-header').trigger('click')
    await flushPromises()
    const evidence = detail.findAll('.evidence-grid pre')
    expect(JSON.parse(evidence[0].text())).toEqual(expected)
    expect(JSON.parse(evidence[1].text())).toEqual(actual)
    wrapper.unmount()
  })

  it('展开执行异常不展示部分断言结论，原始数据仍完整保留', async () => {
    const actual = { details: [
      { property: 'assertion', title: '不能采信的部分断言', successful: true, actual: 'partial' },
    ], violation_count: 0 }
    requestUtil.get.mockResolvedValueOnce({ data: { data: {
      scan: { id: 5, status: 'failed' }, targets: [],
      items: [{ status: 'error', message: '采集失败', actual }],
    } } })
    const wrapper = mount(ScanDetail, { global: {
      components: { ATable: Table, ACollapse: Collapse, ACollapsePanel: Collapse.Panel },
      stubs: { ...stubs, 'a-table': false, 'a-collapse': false, 'a-collapse-panel': false },
    } })
    await flushPromises()
    await wrapper.find('.ant-table-row-expand-icon').trigger('click')
    await flushPromises()
    const detail = wrapper.find('.result-detail')
    expect(detail.text()).toContain('采集失败')
    expect(detail.findComponent(Table).exists()).toBe(false)
    expect(detail.text()).not.toContain('不能采信的部分断言')
    expect(detail.find('a-alert-stub').attributes('message')).toContain('无法得出合规结论')
    await detail.find('.ant-collapse-header').trigger('click')
    await flushPromises()
    expect(JSON.parse(detail.findAll('.evidence-grid pre')[1].text())).toEqual(actual)
    wrapper.unmount()
  })

  it.each([
    ['独立主机范围', { mount_type: 'host', environments: [{ id: 3, name: '生产' }] }, '不限定项目', '生产'],
    ['项目环境范围', { mount_type: 'environment', project_id: 2, project_name: '核心项目', environment_id: 3, environment_name: '生产', environments: [{ id: 4, name: '灾备' }] }, '核心项目', '灾备'],
    ['项目范围', { mount_type: 'project', project_name: '核心项目', environments: [{ id: 1, name: 'test' }, { id: 2, name: 'prod' }] }, '核心项目', 'test、prod'],
    ['环境名称不可用', { mount_type: 'environment', project_id: 2, environments: [{ id: 3, name: '' }] }, '项目 #2（已删除或不可用）', '环境 #3（已删除或不可用）'],
    ['主机未设置环境', { mount_type: 'environment', environment_id: 3, environment_name: '扫描时的环境' }, '-', '未设置环境'],
  ])('%s', async (_label, scope, project, environment) => {
    requestUtil.get.mockResolvedValueOnce({
      data: { data: { scan: { id: 5, baseline: '基线', status: 'success', ...scope }, targets: [], items: [] } },
    })
    const wrapper = mount(ScanDetail, {
      global: {
        stubs,
      },
    })
    await flushPromises()
    expect(requestUtil.get).toHaveBeenLastCalledWith('sys/security/scans/5/')
    expect(wrapper.find('[data-label="项目"]').text()).toBe(project)
    expect(wrapper.find('[data-label="主机环境"]').text()).toBe(environment)
    wrapper.unmount()
  })

  it('两处环境筛选同步过滤两张表，支持未设置和清除，切换重置分页和无效主机条件', async () => {
    const test = { id: 1, name: 'test' }
    const prod = { id: 2, name: 'prod' }
    const targets = [
      { host_id: 10, host_name: 'prod-only', host_ip: '10.0.0.1', environments: [prod] },
      { host_id: 11, host_name: 'test-only', host_ip: '10.0.0.2', environments: [test] },
      { host_id: 12, host_name: 'unknown', host_ip: '10.0.0.3', environments: [] },
    ]
    requestUtil.get.mockResolvedValueOnce({ data: { data: {
      scan: { id: 5, mount_type: 'project', environments: [test, prod], environment_source: 'current_assets' },
      targets, items: targets.map((target, i) => ({ ...target, status: i === 0 ? 'pass' : 'fail' })),
    } } })
    const wrapper = mount(ScanDetail, { global: { stubs } })
    await flushPromises()
    expect(wrapper.find('[data-label="主机环境"]').text()).toBe('test、prod')
    expect(wrapper.find('a-alert-stub').attributes('message')).toContain('非扫描时快照')
    expect(wrapper.vm.environmentOptions).toEqual([
      { value: 1, label: 'test' }, { value: 2, label: 'prod' }, { value: 'unknown', label: '未设置环境' },
    ])
    const environmentSelectors = wrapper.findAllComponents(selectStub)
      .filter((select) => select.props('placeholder') === '全部环境')
    expect(environmentSelectors).toHaveLength(2)
    environmentSelectors[1].vm.$emit('update:value', 1)
    await flushPromises()
    expect(environmentSelectors.map((select) => select.props('value'))).toEqual([1, 1])
    expect(wrapper.vm.filteredTargets.map((row) => row.host_id)).toEqual([11])
    expect(wrapper.vm.filteredItems).toHaveLength(1)
    expect(wrapper.vm.failedCount).toBe(1)
    wrapper.vm.hostFilter = 11
    wrapper.vm.targetPagination.current = 2
    wrapper.vm.itemPagination.current = 2
    environmentSelectors[0].vm.$emit('update:value', 2)
    await flushPromises()
    expect(environmentSelectors.map((select) => select.props('value'))).toEqual([2, 2])
    expect(wrapper.vm.hostFilter).toBeUndefined()
    expect(wrapper.vm.filteredTargets.map((row) => row.host_id)).toEqual([10])
    expect(wrapper.vm.filteredItems.map((row) => row.host_id)).toEqual([10])
    expect(wrapper.vm.targetPagination.current).toBe(1)
    expect(wrapper.vm.itemPagination.current).toBe(1)
    expect(wrapper.vm.targetPagination.total).toBe(1)
    environmentSelectors[1].vm.$emit('update:value', 'unknown')
    await flushPromises()
    expect(wrapper.vm.filteredTargets.map((row) => row.host_id)).toEqual([12])
    expect(wrapper.vm.filteredItems.map((row) => row.host_id)).toEqual([12])
    environmentSelectors[0].vm.$emit('update:value', undefined)
    await flushPromises()
    expect(environmentSelectors.map((select) => select.props('value'))).toEqual([undefined, undefined])
    expect(wrapper.vm.filteredTargets).toHaveLength(3)
    expect(wrapper.vm.filteredItems).toHaveLength(3)
    wrapper.unmount()
  })

  it('刷新按新的主机环境显示并清除失效环境筛选，不改变历史检查结果', async () => {
    const report = (environment) => ({ data: { data: {
      scan: { id: 5, mount_type: 'host', environments: [environment], environment_source: 'current_assets' },
      targets: [{ host_id: 10, host_name: 'host', environments: [environment] }],
      items: [{ host_id: 10, item_name: '检查项', status: 'fail', message: '历史检查消息', environments: [environment] }],
    } } })
    requestUtil.get.mockResolvedValueOnce(report({ id: 1, name: '测试' }))
      .mockResolvedValueOnce(report({ id: 2, name: '生产' }))
    const wrapper = mount(ScanDetail, { global: { stubs } })
    await flushPromises()
    wrapper.vm.environmentFilter = 1
    await flushPromises()
    expect(wrapper.find('[data-label="主机环境"]').text()).toBe('测试')
    await wrapper.vm.loadDetail()
    await flushPromises()
    expect(wrapper.find('[data-label="主机环境"]').text()).toBe('生产')
    expect(wrapper.vm.environmentFilter).toBeUndefined()
    expect(wrapper.vm.filteredTargets[0].environments).toEqual([{ id: 2, name: '生产' }])
    expect(wrapper.vm.filteredItems[0]).toMatchObject({
      status: 'fail', message: '历史检查消息', environments: [{ id: 2, name: '生产' }],
    })
    wrapper.unmount()
  })
})
