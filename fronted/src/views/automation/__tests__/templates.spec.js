import { flushPromises, mount } from '@vue/test-utils'
import Antd from 'ant-design-vue'
import { createMemoryHistory, createRouter } from 'vue-router'
import { describe, expect, it, vi } from 'vitest'

vi.mock('@/api/sys/automation', () => ({
  getPlaybookList: vi.fn(() => Promise.resolve({ data: { data: { results: [], count: 0 } } })),
  getShellcheckStatus: vi.fn(() => Promise.resolve({ data: { data: { ready: true, source: 'uploaded', version: '0.9.0' } } })),
  uploadShellcheckBinary: vi.fn(),
  deleteShellcheckBinary: vi.fn(),
  createPlaybook: vi.fn(),
  updatePlaybook: vi.fn(),
  batchDeletePlaybooks: vi.fn(),
  uploadPlaybookFile: vi.fn(),
  downloadPlaybookFile: vi.fn(),
  validatePlaybookContent: vi.fn(() => Promise.resolve({ data: { data: { valid: true, warnings: [] } } })),
}))

vi.mock('@/util/deleteConfirm', () => ({ openDeleteConfirm: vi.fn() }))
vi.mock('@/store', () => ({ default: { state: { user: { timezone: 'Asia/Shanghai' } } } }))
vi.mock('@/directives/permission/permission', () => ({ checkPermission: () => true }))

import * as automationApi from '@/api/sys/automation'
import LogTemplates from '../templates/index.vue'

async function mountPage() {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [{ path: '/sys/automation/templates', component: LogTemplates }],
  })
  router.push('/sys/automation/templates')
  await router.isReady()
  const wrapper = mount(LogTemplates, {
    attachTo: document.body,
    global: {
      plugins: [Antd, router],
      stubs: { FontAwesomeIcon: true },
      directives: { permission: () => {} },
    },
  })
  await flushPromises()
  return wrapper
}

describe('自动化模板页：Shell 类型与 ShellCheck', () => {
  it('挂载时拉取 ShellCheck 状态并展示就绪标签', async () => {
    automationApi.getShellcheckStatus.mockResolvedValue({ data: { data: { ready: true, source: 'uploaded', version: '0.9.0' } } })
    const wrapper = await mountPage()
    expect(automationApi.getShellcheckStatus).toHaveBeenCalled()
    expect(wrapper.text()).toContain('ShellCheck 已就绪')
    wrapper.unmount()
  })

  it('切到 Shell 类型自动填示例，校验请求带 content_format，并提示 warnings', async () => {
    automationApi.getShellcheckStatus.mockResolvedValue({ data: { data: { ready: true, source: 'uploaded', version: '0.9.0' } } })
    automationApi.validatePlaybookContent.mockResolvedValueOnce({
      data: { data: { valid: true, warnings: [{ line: 2, level: 'warning', code: 2086, message: 'Double quote to prevent globbing' }] } },
    })
    const wrapper = await mountPage()
    // 打开新增模板弹窗
    await wrapper.find('.right-tools button').trigger('click')
    await flushPromises()

    // 切到 Shell 类型（segmented 第二项；弹窗 teleport 到 body，用 document 查询）
    document.querySelectorAll('.ant-segmented-item')[1].click()
    await flushPromises()
    expect(String(document.querySelector('.template-editor-textarea').value)).toContain('MemAvailable')

    // 检查语法 → 请求带 content_format=shell
    const checkButton = [...document.querySelectorAll('button')].find((button) => button.textContent.includes('检查语法'))
    checkButton.click()
    await flushPromises()
    expect(automationApi.validatePlaybookContent).toHaveBeenCalledWith(expect.objectContaining({ content_format: 'shell' }))
    wrapper.unmount()
  })

  it('ShellCheck 未就绪时点校验只弹提示，不发校验请求', async () => {
    automationApi.getShellcheckStatus.mockResolvedValue({ data: { data: { ready: false } } })
    const wrapper = await mountPage()
    await wrapper.find('.right-tools button').trigger('click')
    await flushPromises()
    document.querySelectorAll('.ant-segmented-item')[1].click()
    await flushPromises()

    automationApi.validatePlaybookContent.mockClear()
    const checkButton = [...document.querySelectorAll('button')].find((button) => button.textContent.includes('检查语法'))
    checkButton.click()
    await flushPromises()
    expect(automationApi.validatePlaybookContent).not.toHaveBeenCalled()
    wrapper.unmount()
  })
})
