import { flushPromises, mount } from '@vue/test-utils'
import Antd from 'ant-design-vue'
import { afterEach, describe, expect, it, vi } from 'vitest'

// 「问题」列（2026-09-20 现场）：当前告警有这一栏、历史告警没有——同一个告警在两处显示不一样。
// 后端已把 summary 口径统一（annotation 的 summary → description），这里钉住**两个 tab 的表头
// 都真的有「问题」列**，并且历史行的单元格读的是后端给的 summary。
vi.mock('@/api/monitor', () => ({
  getPrometheusAlerts: vi.fn(() => Promise.resolve({ data: { data: {
    status: 'success', count: 1, firing_count: 1, resolved_count: 0,
    results: [{
      name: 'HighErrorRate', severity: 'critical', state: 'firing', instance: 'node-a',
      labels: { alertname: 'HighErrorRate', instance: 'node-a' },
      summary: '错误率超过 5%', active_at: '2026-09-20T10:00:00Z', value: '7.5',
      rule_group: 'log', rule_details: null, history_id: 3, notification_count: 0, notification_status: 'none',
    }],
  } } })),
  getAlertHistories: vi.fn(() => Promise.resolve({ data: { data: {
    count: 1, pageNumber: 1, results: [{
      id: 3, alertname: 'HighErrorRate', severity: 'critical', instance: 'node-a',
      labels: { alertname: 'HighErrorRate' }, annotations: { summary: '错误率超过 5%' },
      // 后端统一算出来的「问题」文案：与当前告警那一栏同一个函数。
      summary: '错误率超过 5%',
      state: 'resolved', started_at: '2026-09-20T10:00:00Z', resolved_at: '2026-09-20T11:00:00Z',
      rule_group: 'log', rule_details: null, notification_count: 0, notification_status: 'none',
    }],
  } } })),
  getAlertNotificationChain: vi.fn(),
}))
vi.mock('@/util/deleteConfirm', () => ({ openDeleteConfirm: vi.fn(() => Promise.resolve(false)) }))

// antd Table 会调带伪元素的 getComputedStyle，jsdom 不认识（噪声）。
const originalGetComputedStyle = window.getComputedStyle.bind(window)
window.getComputedStyle = (element) => originalGetComputedStyle(element)

import Alerts from '../index.vue'

async function mountPage() {
  const wrapper = mount(Alerts, {
    attachTo: document.body,
    global: { plugins: [Antd], stubs: { FontAwesomeIcon: true } },
  })
  await flushPromises()
  return wrapper
}

function headersOf(wrapper, tableIndex) {
  return wrapper.findAll('table')[tableIndex].find('thead').text()
}

describe('告警页：当前告警与历史告警的「问题」列', () => {
  afterEach(() => {
    document.body.innerHTML = ''
    vi.clearAllMocks()
  })

  it('shows the 问题 column in both tabs, with the same text source', async () => {
    const wrapper = await mountPage()

    // 两张表都要有这一列（历史那张以前没有）。
    const headers = wrapper.findAll('table').map((table) => table.find('thead').text())
    expect(headers.length).toBeGreaterThanOrEqual(2)
    expect(headers[0]).toContain('问题')
    expect(headers[1]).toContain('问题')

    // 两个 tab 渲染的都是后端给的 summary（同一口径），不是各自拼的。
    expect(document.body.textContent).toContain('错误率超过 5%')
    wrapper.unmount()
  })

  it('falls back to a dash when an old history row has no annotations', async () => {
    const { getAlertHistories } = await import('@/api/monitor')
    getAlertHistories.mockResolvedValueOnce({ data: { data: {
      count: 1, pageNumber: 1, results: [{
        id: 4, alertname: 'NoAnnotation', severity: 'warning', instance: 'node-b',
        labels: {}, annotations: {}, summary: '',
        state: 'resolved', started_at: '2026-09-20T10:00:00Z', resolved_at: '2026-09-20T10:30:00Z',
        rule_group: 'log', rule_details: null, notification_count: 0, notification_status: 'none',
      }],
    } } })

    const wrapper = await mountPage()

    // 没有注释的历史告警：显示"-"，而不是把告警名塞进「问题」列（名字在展开行里能看到）。
    const historyRows = wrapper.findAll('table')[1].findAll('tbody tr')
    const historyText = historyRows.map((row) => row.text()).join('|')
    expect(historyText).toContain('-')
    expect(historyText).not.toContain('NoAnnotation')
    wrapper.unmount()
  })
})
