import { flushPromises, mount } from '@vue/test-utils'
import Antd from 'ant-design-vue'
import { afterEach, describe, expect, it, vi } from 'vitest'

// 回归（2026-09-19 现场）：在「定时任务」页点历史任务的「立即执行」，只回一句
// `任务提交失败: 任务 handler 尚未迁移到 Go`——不知道是谁的问题、要不要等、有没有替代。
//
// 这一页现在必须做到三件事（都由后端的 supported/support_note 驱动，前端不自己维护编码清单）：
//   ① 没有 Go 实现的任务，「立即执行」**置灰**（不再让人点一下才看到报错）；
//   ② 列表里标注「未迁移」并给出说明（tooltip 里是后端那句话）；
//   ③ 顶部一条警告，说清"定时调度会跳过它们、手动执行也会失败"。
vi.mock('@/api/sys/scheduler', () => ({
  getTaskList: vi.fn(() => Promise.resolve({ data: { data: {
    results: [
      {
        id: 5, name: '登录日志清理', code: 'cleanup_login_audit_logs', enabled: true, is_running: false,
        effective_cron_expression: '0 0 * * *', last_status: '成功', supported: true, support_note: '',
      },
      {
        id: 10, name: '历史告警对账', code: 'reconcile_prometheus_alert_history', enabled: true, is_running: false,
        effective_cron_expression: '*/5 * * * *', last_status: '成功',
        supported: false,
        support_note: '定时任务「历史告警对账」(reconcile_prometheus_alert_history) 的实现尚未迁移到 Go（Django 后端已移出）：定时调度会跳过它，手动执行也只会失败。当前已实现的任务：操作日志清理、登录日志清理。',
      },
    ],
    count: 2, totalPages: 1,
  } } })),
  getTaskLogList: vi.fn(() => Promise.resolve({ data: { data: { results: [], count: 0, totalPages: 0 } } })),
  enableTask: vi.fn(),
  disableTask: vi.fn(),
  updateTask: vi.fn(),
  runTaskNow: vi.fn(() => Promise.resolve({ data: { code: 200, data: { status: 'submitted' } } })),
  getTaskStatus: vi.fn(() => Promise.resolve({ data: { data: { is_running: false, last_status: '成功' } } })),
}))

vi.mock('@/api/sys/sysconfig', () => ({
  getConfigByKey: vi.fn(() => Promise.resolve({ data: { value: '3' } })),
  CONFIG_KEYS: { SCHEDULER_ENABLED: 'sys.scheduler.enabled', SCHEDULER_POLL_INTERVAL: 'sys.scheduler.poll_interval' },
}))
vi.mock('@/api/menu', () => ({ getMenuTree: vi.fn(() => Promise.resolve({ data: { data: [] } })) }))
vi.mock('@/util/keepAliveRefresh', () => ({ useKeepAliveRefreshLifecycle: vi.fn() }))

// antd 的 Table 会调 `getComputedStyle(el, pseudoElt)` 量滚动条宽度，jsdom 对"带伪元素"这个重载
// 会打一条 not-implemented 报错（不影响断言，只是把输出刷脏）。这里收窄成只传元素。
const originalGetComputedStyle = window.getComputedStyle.bind(window)
window.getComputedStyle = (element) => originalGetComputedStyle(element)

import SchedulerPage from './index.vue'
import { runTaskNow } from '@/api/sys/scheduler'

async function mountPage() {
  const wrapper = mount(SchedulerPage, {
    attachTo: document.body,
    global: {
      plugins: [Antd],
      stubs: {
        FontAwesomeIcon: true,
        // 页面里跳菜单用，本用例不关心路由。
        'router-link': true,
      },
    },
  })
  await flushPromises()
  return wrapper
}

function rowByName(wrapper, name) {
  return wrapper.findAll('tbody tr').find((row) => row.text().includes(name))
}

describe('定时任务：未迁移任务的处理', () => {
  afterEach(() => {
    document.body.innerHTML = ''
    vi.clearAllMocks()
  })

  it('marks tasks without a Go handler and disables 「立即执行」', async () => {
    const wrapper = await mountPage()

    const legacyRow = rowByName(wrapper, '历史告警对账')
    expect(legacyRow.text()).toContain('未迁移')
    // 置灰：按钮存在但点不动（不能让人点了才发现失败）。
    const legacyRunButton = legacyRow.findAll('button').find((button) => button.text().includes('立即执行'))
    expect(legacyRunButton.attributes('disabled')).toBeDefined()

    // 已实现的任务照旧可点。
    const supportedRow = rowByName(wrapper, '登录日志清理')
    expect(supportedRow.text()).not.toContain('未迁移')
    const supportedRunButton = supportedRow.findAll('button').find((button) => button.text().includes('立即执行'))
    expect(supportedRunButton.attributes('disabled')).toBeUndefined()
    wrapper.unmount()
  })

  it('explains why on the page, not only after a failed click', async () => {
    const wrapper = await mountPage()

    expect(wrapper.vm.unsupportedTaskCount).toBe(1)
    // 顶部警告说清"定时调度会跳过它们、手动执行也会失败"。
    expect(document.body.textContent).toContain('定时调度会跳过它们、手动执行也会失败')
    // 行内标注「未迁移」，说明文案来自后端的 support_note（含"已实现的任务有哪些"这个替代方案），
    // 挂在标签的 tooltip 上——tooltip 走 portal，jsdom 里断言数据字段而不是它的 DOM。
    const legacyRow = rowByName(wrapper, '历史告警对账')
    expect(legacyRow.find('.unsupported-tag').exists()).toBe(true)
    const legacyTask = wrapper.vm.tasks.find((task) => task.id === 10)
    expect(legacyTask.support_note).toContain('当前已实现的任务')
    wrapper.unmount()
  })

  it('does not submit a run for a task without a handler', async () => {
    const wrapper = await mountPage()
    const legacyRow = rowByName(wrapper, '历史告警对账')
    const legacyRunButton = legacyRow.findAll('button').find((button) => button.text().includes('立即执行'))

    await legacyRunButton.trigger('click')
    await flushPromises()

    // 按钮置灰后再点也不该发请求：请求发出去只会拿到 400。
    expect(runTaskNow).not.toHaveBeenCalled()
    wrapper.unmount()
  })
})
