import { flushPromises, mount } from '@vue/test-utils'
import Antd from 'ant-design-vue'
import { afterEach, describe, expect, it, vi } from 'vitest'

// 左侧应用筛选框（2026-09-19 加）：应用一多，逐个点太慢。
//
// 匹配规则本身在 util/applicationFilter.test.js 里逐条钉过；这里测的是**接线**——
// 输入框真的在左侧、真的筛掉了条目、筛空时真的给提示、清空后回到全部。
// 之所以要这层：规则页挂载要 mock 一整套接口，接线错了单看 util 的单测是发现不了的（页面照样白着）。
vi.mock('@/api/monitor', () => ({
  getElasticsearchClusterList: vi.fn(() => Promise.resolve({
    data: { data: { results: [{ id: 2, name: 'test', index_prefix: 'autoadmin', enabled: true, is_default: true }] } },
  })),
  getLogProcessingRules: vi.fn(() => Promise.resolve({
    data: { data: { results: [
      { id: 1, name: 'springboot-tomcat-exception', application: 5, input_format: 'text', multiline_enabled: true, pipeline_body: { processors: [] } },
      { id: 2, name: 'redis.log', application: 8, input_format: 'text', multiline_enabled: false, pipeline_body: { processors: [] } },
      { id: 3, name: 'common-error', application: null, input_format: 'text', multiline_enabled: false, pipeline_body: { processors: [] } },
    ] } },
  })),
  getLogCollectionFilterRules: vi.fn(() => Promise.resolve({ data: { data: { results: [] } } })),
  getLogRetentionTiers: vi.fn(() => Promise.resolve({ data: { data: { results: [] } } })),
  saveLogProcessingRule: vi.fn(),
  batchDeleteLogProcessingRules: vi.fn(),
  saveLogCollectionFilterRule: vi.fn(),
  batchDeleteLogCollectionFilterRules: vi.fn(),
  simulateElasticsearchPipeline: vi.fn(),
}))

vi.mock('@/api/assets/application', () => ({
  getApplicationList: vi.fn(() => Promise.resolve({
    data: { data: { results: [
      { id: 5, name: 'Tomcat', code: 'tomcat' },
      { id: 8, name: 'Redis', code: 'redis' },
      { id: 16, name: '中间件', code: 'cdm' },
    ] } },
  })),
}))

vi.mock('@/util/deleteConfirm', () => ({ openDeleteConfirm: vi.fn(() => Promise.resolve(false)) }))

// antd 的 Table 会调 `getComputedStyle(el, pseudoElt)` 量滚动条宽度，jsdom 对"带伪元素"这个重载
// 会打一条 not-implemented 报错（不影响断言，只是把输出刷脏）。这里收窄成只传元素。
const originalGetComputedStyle = window.getComputedStyle.bind(window)
window.getComputedStyle = (element) => originalGetComputedStyle(element)

import LogParsers from './index.vue'

async function mountPage() {
  const wrapper = mount(LogParsers, {
    attachTo: document.body,
    global: {
      plugins: [Antd],
      stubs: { FontAwesomeIcon: true },
    },
  })
  await flushPromises()
  return wrapper
}

function applicationLabels(wrapper) {
  return wrapper.findAll('.application-label').map((node) => node.text())
}

describe('日志处理规则：左侧应用筛选', () => {
  afterEach(() => {
    document.body.innerHTML = ''
    vi.clearAllMocks()
  })

  it('左侧有筛选框，默认列出全部应用与「通用」', async () => {
    const wrapper = await mountPage()
    expect(wrapper.find('input[placeholder="筛选应用 / 编码"]').exists()).toBe(true)
    expect(applicationLabels(wrapper)).toEqual([
      '全部规则', 'Tomcat', 'Redis', '中间件', '通用（不限应用）',
    ])
    wrapper.unmount()
  })

  it('按名称筛选后只剩命中的应用，且「全部规则」始终在', async () => {
    const wrapper = await mountPage()
    await wrapper.find('input[placeholder="筛选应用 / 编码"]').setValue('redis')
    expect(applicationLabels(wrapper)).toEqual(['全部规则', 'Redis'])
    // 规则表跟着选中项走：这里选中的仍是"全部规则"（筛选只影响左侧列表，不改选中项）。
    expect(wrapper.text()).toContain('springboot-tomcat-exception')
    wrapper.unmount()
  })

  it('按编码也能筛（中文名 + 英文编码的场景）', async () => {
    const wrapper = await mountPage()
    await wrapper.find('input[placeholder="筛选应用 / 编码"]').setValue('cdm')
    expect(applicationLabels(wrapper)).toEqual(['全部规则', '中间件'])
    wrapper.unmount()
  })

  it('筛空时给出提示，清空后恢复全部', async () => {
    const wrapper = await mountPage()
    const input = wrapper.find('input[placeholder="筛选应用 / 编码"]')
    await input.setValue('不存在的应用')
    expect(applicationLabels(wrapper)).toEqual(['全部规则'])
    expect(wrapper.text()).toContain('没有匹配的应用')

    await input.setValue('')
    expect(applicationLabels(wrapper)).toEqual([
      '全部规则', 'Tomcat', 'Redis', '中间件', '通用（不限应用）',
    ])
    expect(wrapper.text()).not.toContain('没有匹配的应用')
    wrapper.unmount()
  })
})
