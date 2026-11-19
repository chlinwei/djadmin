import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { Button, Collapse, CollapsePanel, Table, Tooltip } from 'ant-design-vue'
import Issues from './index.vue'
import IssueDetail from './detail.vue'
import { getSecurityIssue, listSecurityIssueGroups, listSecurityIssueHistory, listSecurityIssues } from '@/api/securityIssues'
import { checkPermission } from '@/directives/permission/permission'
import store from '@/store'

const push = vi.fn()
vi.mock('vue-router', () => ({
  useRouter: () => ({ push: (...args) => push(...args) }),
  useRoute: () => ({ params: { id: '42' } }),
}))
vi.mock('@/api/securityIssues', () => ({
  listSecurityIssueGroups: vi.fn(), listSecurityIssues: vi.fn(),
  getSecurityIssue: vi.fn(), listSecurityIssueHistory: vi.fn(),
}))
vi.mock('@/directives/permission/permission', () => ({ checkPermission: vi.fn(() => true) }))

const slot = { template: '<div><slot /></div>' }
const select = {
  props: ['value', 'options', 'placeholder', 'getPopupContainer'],
  emits: ['update:value', 'change'],
  template: '<div />',
}
const descriptionItem = {
  props: ['label'],
  template: '<div :data-label="label"><slot /></div>',
}
const table = {
  props: ['dataSource', 'columns', 'pagination', 'scroll'],
  template: `<div><template v-if="dataSource.length"><slot v-for="column in columns.filter(c => ['action', 'environment'].includes(c.key))"
    :key="column.key" name="bodyCell" :column="column" :record="dataSource[0]" /></template></div>`,
}
const stubs = {
  'a-card': slot, 'a-space': slot, 'a-alert': true, 'a-button': true,
  'a-radio-group': true, 'a-radio-button': true, 'a-select': select,
  'a-input-search': true, 'a-table': table, 'a-tag': slot, 'a-typography-paragraph': slot,
  'a-tooltip': true, FontAwesomeIcon: true, 'a-spin': slot,
  'a-descriptions': slot, 'a-descriptions-item': descriptionItem, 'router-link': true,
  'a-collapse': true, 'a-collapse-panel': true,
}
const group = { rule_key: 'a'.repeat(64), title: 'SSH', severity: 'high', host_count: 21 }
const issue = { id: 42, title: 'SSH', host_name: 'app-01', status: 'open', last_result: 'error', environment_id: 2, environment_name: '生产' }
const environments = [{ id: null, name: '' }, { id: 1, name: '测试' }, { id: 2, name: '生产' }]
const mountIssues = (options = {}) => mount(Issues, { global: { stubs }, ...options })

beforeEach(() => {
  vi.clearAllMocks()
  checkPermission.mockReturnValue(true)
  listSecurityIssueGroups.mockResolvedValue({ results: [group], count: 21, environments })
  listSecurityIssues.mockResolvedValue({ results: [issue], count: 21, environments })
  getSecurityIssue.mockResolvedValue(issue)
  listSecurityIssueHistory.mockResolvedValue({ results: [{ id: 9, scan_id: 7, result: 'error' }], count: 15 })
})

describe('安全问题列表', () => {
  it('环境筛选发往服务端，聚合钻取及分页保留条件，支持未设置和清除', async () => {
    const wrapper = mountIssues()
    await flushPromises()
    const environmentSelect = wrapper.findAllComponents(select).find((component) => component.props('placeholder') === '全部环境')
    expect(environmentSelect.props('options')).toEqual([
      { value: 'unassigned', label: '未设置环境' }, { value: 1, label: '测试' }, { value: 2, label: '生产' },
    ])
    expect(environmentSelect.props('getPopupContainer')).toBeTypeOf('function')
    wrapper.vm.pagination.current = 3
    environmentSelect.vm.$emit('update:value', 2)
    environmentSelect.vm.$emit('change', 2)
    await flushPromises()
    expect(listSecurityIssueGroups).toHaveBeenLastCalledWith(expect.objectContaining({ environment_id: 2, page: 1 }))
    await wrapper.vm.showHosts(group)
    await flushPromises()
    expect(listSecurityIssues).toHaveBeenLastCalledWith(expect.objectContaining({ environment_id: 2, rule_key: group.rule_key, page: 1 }))
    expect(wrapper.findComponent(table).props('columns').some((column) => column.key === 'environment')).toBe(true)
    expect(wrapper.findComponent(table).text()).toContain('生产')
    await wrapper.vm.changePage({ current: 2, pageSize: 10 })
    expect(listSecurityIssues).toHaveBeenLastCalledWith(expect.objectContaining({ environment_id: 2, page: 2 }))
    environmentSelect.vm.$emit('update:value', 'unassigned')
    environmentSelect.vm.$emit('change', 'unassigned')
    await flushPromises()
    expect(listSecurityIssues).toHaveBeenLastCalledWith(expect.objectContaining({ environment_id: 'unassigned', page: 1 }))
    wrapper.vm.view = 'groups'
    await wrapper.vm.changeView()
    expect(listSecurityIssueGroups).toHaveBeenLastCalledWith(expect.objectContaining({ environment_id: 'unassigned', rule_key: '', page: 1 }))
    environmentSelect.vm.$emit('update:value', undefined)
    environmentSelect.vm.$emit('change', undefined)
    await flushPromises()
    expect(listSecurityIssueGroups).toHaveBeenLastCalledWith(expect.objectContaining({ environment_id: undefined, page: 1 }))
    wrapper.unmount()
  })

  it('聚合钻取保留筛选，分页在服务端完成，切换视角清除规则范围', async () => {
    const wrapper = mountIssues()
    await flushPromises()
    expect(listSecurityIssueGroups).toHaveBeenLastCalledWith(expect.objectContaining({ page: 1, page_size: 10, status: 'open' }))
    wrapper.vm.severity = 'high'
    await wrapper.vm.showHosts(group)
    expect(listSecurityIssues).toHaveBeenLastCalledWith(expect.objectContaining({ rule_key: group.rule_key, severity: 'high', page: 1 }))
    expect(wrapper.findComponent(table).props('columns').at(-1)).toMatchObject({ key: 'action', fixed: 'right', width: 110 })
    expect(wrapper.findComponent(table).props('scroll').x).toBeGreaterThan(0)
    await wrapper.vm.changePage({ current: 2, pageSize: 10 })
    expect(listSecurityIssues).toHaveBeenLastCalledWith(expect.objectContaining({ page: 2 }))
    await wrapper.vm.changePage({ current: 3, pageSize: 20 })
    expect(listSecurityIssues).toHaveBeenLastCalledWith(expect.objectContaining({ page: 1, page_size: 20 }))
    wrapper.vm.view = 'groups'
    await wrapper.vm.changeView()
    expect(wrapper.vm.ruleKey).toBe('')
    expect(listSecurityIssueGroups).toHaveBeenLastCalledWith(expect.objectContaining({ rule_key: '' }))
    wrapper.unmount()
  })

  it('迟到响应不能覆盖新筛选，失败显示错误而不是保留旧结果', async () => {
    let resolveOld
    listSecurityIssueGroups.mockReturnValueOnce(new Promise((resolve) => { resolveOld = resolve }))
    const wrapper = mountIssues()
    wrapper.vm.search = 'new'
    await wrapper.vm.searchIssues()
    resolveOld({ results: [{ rule_key: 'old', title: 'old' }], count: 1 })
    await flushPromises()
    expect(wrapper.vm.rows).toEqual([group])
    listSecurityIssueGroups.mockRejectedValueOnce(new Error('查询失败'))
    await wrapper.vm.load()
    expect(wrapper.vm.rows).toEqual([])
    expect(wrapper.vm.error).toBe('查询失败')
    expect(wrapper.vm.pagination.total).toBe(0)
    listSecurityIssueGroups.mockResolvedValueOnce({ results: [], count: 0 })
    await wrapper.vm.load()
    expect(wrapper.vm.error).toBe('安全问题环境筛选数据无效')
    wrapper.unmount()
  })

  it('没有独立查看权限时不发请求', async () => {
    checkPermission.mockReturnValue(false)
    const wrapper = mountIssues()
    await flushPromises()
    expect(listSecurityIssueGroups).not.toHaveBeenCalled()
    expect(wrapper.find('a-alert-stub').attributes('message')).toContain('没有')
    wrapper.unmount()
  })

  it('操作提示可见、位置为顶部，按钮仍可点击进入详情', async () => {
    const wrapper = mount(Issues, {
      attachTo: document.body,
      global: { stubs: { ...stubs, 'a-tooltip': false, 'a-button': false }, components: { ATooltip: Tooltip, AButton: Button } },
    })
    await flushPromises()
    await wrapper.vm.showHosts(group)
    await flushPromises()
    const tooltip = wrapper.findComponent(Tooltip)
    expect(tooltip.props()).toMatchObject({ title: '历史记录', placement: 'top' })
    await tooltip.find('button').trigger('mouseenter')
    await new Promise((resolve) => setTimeout(resolve, 300))
    await flushPromises()
    const overlay = document.querySelector('.ant-tooltip:not(.ant-tooltip-hidden)')
    expect(overlay?.textContent).toContain('历史记录')
    await tooltip.find('button').trigger('click')
    expect(push).toHaveBeenCalledWith('/sys/security/issues/42')
    wrapper.unmount()
  })
})

describe('安全问题详情', () => {
  let computedStyleSpy
  afterEach(() => computedStyleSpy?.mockRestore())

  function mountWithEvidenceTable() {
    const getComputedStyle = window.getComputedStyle.bind(window)
    computedStyleSpy = vi.spyOn(window, 'getComputedStyle').mockImplementation((element) => getComputedStyle(element))
    const wrapper = mount(IssueDetail, {
      global: {
        components: { ATable: Table, ACollapse: Collapse, ACollapsePanel: CollapsePanel },
        stubs: { ...stubs, 'a-table': false, 'a-collapse': false, 'a-collapse-panel': false },
      },
    })
    wrapper.vm.pagination.showSizeChanger = false
    return wrapper
  }

  it('历史列表摘要展开后只对照失败项，并保留修复建议和原始证据', async () => {
    const failed = { property: 'assertion', title: 'net.ipv6.conf.default.accept_redirects', successful: false, actual: 2, expected: 0 }
    const passed = { property: 'assertion', title: 'net.ipv6.conf.all.accept_redirects', successful: true, actual: 0, expected: 0 }
    const rawMessage = 'OPA 策略违规：默认值应为 0'
    listSecurityIssueHistory.mockResolvedValueOnce({
      count: 1,
      results: [{ id: 9, scan_id: 7, result: 'fail', actual_value: { details: [failed, passed] },
        expected_value: { redirect: 0 }, message: rawMessage, remediation: '检查重定向配置' }],
    })
    const wrapper = mountWithEvidenceTable()
    await flushPromises()
    expect(wrapper.text()).toContain('扫描发起时间')
    expect(wrapper.text()).toContain('1 项不符合')
    expect(wrapper.text()).not.toContain(rawMessage)
    await wrapper.get('button[aria-expanded="false"]').trigger('click')
    expect(wrapper.findAll('th').slice(-3).map((cell) => cell.text())).toEqual(['检查项', '实际值', '期望值'])
    expect(wrapper.findAll('td').slice(-3).map((cell) => cell.text())).toEqual([failed.title, '2', '0'])
    expect(wrapper.text()).not.toContain(passed.title)
    expect(wrapper.get('[data-label="修复建议"]').text()).toBe('检查重定向配置')
    expect(wrapper.get('[data-label="检查消息"]').text()).toBe(rawMessage)
    await wrapper.get('.ant-collapse-header').trigger('click')
    expect(wrapper.get('[data-label="实际证据"]').text()).toContain(passed.title)
    expect(wrapper.get('[data-label="预期证据"]').text()).toContain('"redirect": 0')
    wrapper.unmount()
  })

  it('检查异常携带部分断言时不统计为违规，展开可查看异常原因', async () => {
    const reason = '读取接口参数失败'
    listSecurityIssueHistory.mockResolvedValueOnce({
      count: 1,
      results: [{ id: 9, scan_id: 7, result: 'error', message: reason,
        actual_value: { details: [{ property: 'assertion', title: '部分观测', successful: false }] } }],
    })
    const wrapper = mountWithEvidenceTable()
    await flushPromises()
    expect(wrapper.text()).toContain('无法验证，展开查看原因')
    expect(wrapper.text()).not.toContain('项不符合')
    await wrapper.get('button[aria-expanded="false"]').trigger('click')
    expect(wrapper.get('[data-label="检查消息"]').text()).toBe(reason)
    expect(wrapper.text()).not.toContain('部分观测')
    wrapper.unmount()
  })

  it.each([
    [{ environment_id: 2, environment_name: '生产' }, '生产'],
    [{ environment_id: null, environment_name: '' }, '未设置环境'],
    [{ environment_id: 2, environment_name: '' }, '环境 #2（已删除或不可用）'],
  ])('显示主机当前环境，不依赖历史扫描快照', async (environment, expected) => {
    getSecurityIssue.mockResolvedValueOnce({ ...issue, ...environment })
    const wrapper = mount(IssueDetail, { global: { stubs } })
    await flushPromises()
    expect(wrapper.find('[data-label="主机环境（当前）"]').text()).toBe(expected)
    wrapper.unmount()
  })

  it('保留稳定策略 ID 与版本证据，规则变更关闭不冒充主机修复', async () => {
    getSecurityIssue.mockResolvedValueOnce({
      ...issue, item_id: 7, status: 'closed', last_result: 'pass', closure_reason: 'rule_changed',
      last_rule_version: 'corrected-policy-version', failure_rule_version: 'original-policy-version',
    })
    const wrapper = mount(IssueDetail, { global: { stubs } })
    await flushPromises()
    expect(wrapper.text()).toContain('基线策略 #7')
    expect(wrapper.text()).toContain('规则变更后通过')
    expect(wrapper.text()).toContain('corrected-policy-version')
    expect(wrapper.text()).toContain('original-policy-version')
    const warning = wrapper.findAll('a-alert-stub').find((alert) => alert.attributes('type') === 'warning')
    expect(warning?.attributes('message')).toContain('不能证明主机已修复')
    expect(wrapper.vm.transitionLabels.closed_rule_changed).toBe('规则变更后通过')
    wrapper.unmount()
  })

  it('分页读取历史，详情失败明确提示，不展示不完整的成功数据', async () => {
    const wrapper = mount(IssueDetail, { global: { stubs } })
    await flushPromises()
    expect(getSecurityIssue).toHaveBeenCalledWith('42')
    expect(listSecurityIssueHistory).toHaveBeenLastCalledWith('42', { page: 1, page_size: 10, scope: 'milestones' })
    expect(wrapper.text()).toContain('关键事件')
    expect(wrapper.get('router-link-stub').attributes('to')).toBe('/sys/security/issues/42/history')
    await wrapper.vm.changePage({ current: 2, pageSize: 10 })
    expect(listSecurityIssueHistory).toHaveBeenLastCalledWith('42', { page: 2, page_size: 10, scope: 'milestones' })
    listSecurityIssueHistory.mockRejectedValueOnce(new Error('历史读取失败'))
    await wrapper.vm.load()
    expect(wrapper.vm.issue).toBeNull()
    expect(wrapper.vm.history).toEqual([])
    expect(wrapper.vm.error).toBe('历史读取失败')
    wrapper.unmount()
  })

  it('独立检查记录页请求完整历史，不重复展示问题概要', async () => {
    const wrapper = mount(IssueDetail, { props: { fullHistory: true }, global: { stubs } })
    await flushPromises()
    expect(listSecurityIssueHistory).toHaveBeenLastCalledWith('42', { page: 1, page_size: 10, scope: 'all' })
    expect(wrapper.text()).toContain('按扫描发起时间列出全部检查记录')
    expect(wrapper.find('[data-label="状态"]').exists()).toBe(false)
    expect(wrapper.find('router-link-stub').exists()).toBe(false)
    wrapper.unmount()
  })

  it('按用户当前时区展示时间，修改时区后立即使用新设置', async () => {
    const wrapper = mount(IssueDetail, { global: { stubs } })
    await flushPromises()
    const previous = store.state.user
    store.state.user = { timezone: 'UTC' }
    expect(wrapper.vm.formatTime('2026-11-17T01:00:00Z')).toBe('2026-11-17 01:00:00')
    store.state.user = { timezone: 'Asia/Shanghai' }
    expect(wrapper.vm.formatTime('2026-11-17T01:00:00Z')).toBe('2026-11-17 09:00:00')
    store.state.user = previous
    wrapper.unmount()
  })
})
