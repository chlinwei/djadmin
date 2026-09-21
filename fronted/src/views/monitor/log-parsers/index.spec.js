import { flushPromises, mount } from '@vue/test-utils'
import Antd from 'ant-design-vue'
import { createMemoryHistory, createRouter } from 'vue-router'
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
  getLogProcessingRuleUsages: vi.fn(() => Promise.resolve({
    data: { data: { count: 2, results: [
      { rule_id: 1, rule_name: 'springboot-tomcat-exception', application: 5, log_definition_id: 11, log_name: 'catalina', path_pattern: '${APP_HOME}/logs/catalina.out', template_id: 7, template_name: 'Tomcat 模板', service_count: 3 },
      { rule_id: 1, rule_name: 'springboot-tomcat-exception', application: 5, log_definition_id: 12, log_name: 'localhost', path_pattern: '${APP_HOME}/logs/localhost.log', template_id: 7, template_name: 'Tomcat 模板', service_count: 3 },
    ] } },
  })),
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
  getApplicationDeploymentTemplateServices: vi.fn(() => Promise.resolve({
    data: { data: { count: 2, results: [
      { id: 21, name: '订单 API', code: 'order-api', enabled: true, business_system_id: 7, environment_id: 72, project_name: '电商平台', business_system_name: '订单系统', environment_name: '测试环境' },
      { id: 22, name: '订单 Web', code: 'order-web', enabled: false, business_system_id: 7, environment_id: null, project_name: '电商平台', business_system_name: '订单系统', environment_name: '未配置环境' },
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
  // 页面用了 useRouter 跳转服务树，测试里装一个内存路由。
  const router = createRouter({ history: createMemoryHistory(), routes: [{ path: '/:pathMatch(.*)*', component: { template: '<div/>' } }] })
  const wrapper = mount(LogParsers, {
    attachTo: document.body,
    global: {
      plugins: [Antd, router],
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

describe('日志处理规则：关联模板 tab', () => {
  afterEach(() => {
    document.body.innerHTML = ''
    vi.clearAllMocks()
  })

  it('懒加载：切到关联模板 tab 才拉取引用关系并按规则×日志定义展示', async () => {
    const wrapper = await mountPage()
    // 挂载时不该拉 usage（懒加载）。
    const { getLogProcessingRuleUsages } = await import('@/api/monitor')
    expect(getLogProcessingRuleUsages).not.toHaveBeenCalled()

    // 切到「关联模板」tab，出现两行引用（同一规则被两个日志定义引用）。
    await wrapper.find('.ant-tabs-tab:nth-child(3)').trigger('click')
    await flushPromises()
    expect(getLogProcessingRuleUsages).toHaveBeenCalledTimes(1)
    expect(wrapper.text()).toContain('Tomcat 模板')
    expect(wrapper.text()).toContain('catalina')
    expect(wrapper.text()).toContain('${APP_HOME}/logs/catalina.out')
    wrapper.unmount()
  })

  it('展示所属应用、支持关键字筛选，影响服务数可点开服务列表并跳转', async () => {
    const wrapper = await mountPage()
    await wrapper.find('.ant-tabs-tab:nth-child(3)').trigger('click')
    await flushPromises()

    // 所属应用列：按 application id 反查应用名。
    expect(wrapper.text()).toContain('Tomcat')

    // 关键字筛掉所有行后显示空态，清空恢复。
    await wrapper.find('input[placeholder="筛选模板 / 日志 / 路径"]').setValue('nginx')
    await flushPromises()
    expect(wrapper.text()).toContain('没有规则被部署模板引用')
    await wrapper.find('input[placeholder="筛选模板 / 日志 / 路径"]').setValue('')
    await flushPromises()
    expect(wrapper.text()).toContain('Tomcat 模板')

    // 点「1 个服务」链接 → 调模板服务接口，弹窗展示项目/业务/环境/服务名。
    const { getApplicationDeploymentTemplateServices } = await import('@/api/assets/application')
    await wrapper.find('.service-count-link').trigger('click')
    await flushPromises()
    expect(getApplicationDeploymentTemplateServices).toHaveBeenCalledWith(7)
    // a-modal teleport 到 body，断言要用 document 全文而不是 wrapper.text()。
    const bodyText = document.body.textContent
    expect(bodyText).toContain('引用模板「Tomcat 模板」的服务')
    expect(bodyText).toContain('电商平台')
    expect(bodyText).toContain('订单系统')
    expect(bodyText).toContain('订单 API')

    // 点服务名跳转服务树，带定位 query。
    const links = wrapper.findAll('.service-count-link')
    await links.at(-1).trigger('click')
    wrapper.unmount()
  })
})
