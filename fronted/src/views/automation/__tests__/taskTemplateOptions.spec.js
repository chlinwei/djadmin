import { flushPromises, mount } from '@vue/test-utils'
import { createMemoryHistory, createRouter } from 'vue-router'
import { describe, expect, it, vi } from 'vitest'

// 任务表单的模板下拉必须只列「通用」模板：software_package / agent 是系统托管模板，
// 由监控软件仓库与 Agent 安装流程各自派发，不应在自动化任务里手动选择执行。
vi.mock('@/api/sys/automation', () => ({
  getPlaybookList: vi.fn(() => Promise.resolve({ data: { data: { results: [
    { id: 4, name: 'uptime', category: 'general', content_format: 'playbook' },
    { id: 25, name: 'mem-check', category: 'general', content_format: 'shell' },
  ], count: 2 } } })),
  getInventoryList: vi.fn(() => Promise.resolve({ data: { data: { results: [], count: 0 } } })),
  getTaskList: vi.fn(() => Promise.resolve({ data: { data: { results: [], count: 0 } } })),
  createTask: vi.fn(),
  updateTask: vi.fn(),
  batchDeleteTasks: vi.fn(),
  precheckTaskRun: vi.fn(),
  runTaskNow: vi.fn(),
  precheckInventoryLimit: vi.fn(),
}))
vi.mock('@/util/deleteConfirm', () => ({ openDeleteConfirm: vi.fn() }))
vi.mock('ant-design-vue', () => ({ message: { success: vi.fn(), error: vi.fn(), warning: vi.fn() } }))
vi.mock('@/store', () => ({ default: { state: { user: { timezone: 'Asia/Shanghai' } } } }))
vi.mock('@/directives/permission/permission', () => ({ checkPermission: () => true }))

import * as automationApi from '@/api/sys/automation'
import { message } from 'ant-design-vue'
import { openDeleteConfirm } from '@/util/deleteConfirm'
import AutomationTaskPage from '../automationtask/index.vue'

async function mountPage() {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [{ path: '/sys/automation', component: AutomationTaskPage }],
  })

  router.push('/sys/automation')
  await router.isReady()
  const wrapper = mount(AutomationTaskPage, {
    global: {
      plugins: [router],
      stubs: {
        TaskListCard: true,
        TaskFormModal: true,
        RunNowModal: true,
        ExecutionScopePreviewModal: true,
        FontAwesomeIcon: true,
      },
      directives: { permission: () => {} },
    },
  })
  await flushPromises()
  return wrapper
}

describe('自动化任务：模板下拉只取通用模板', () => {
  it('加载模板列表时带 category=general，排除系统托管模板', async () => {
    const wrapper = await mountPage()
    expect(automationApi.getPlaybookList).toHaveBeenCalledWith(
      expect.objectContaining({ category: 'general' }),
    )
    // Playbook 与 Shell 两种通用模板都应可用。
    const formModal = wrapper.findComponent({ name: 'TaskFormModal' })
    if (formModal.exists()) {
      expect(formModal.props('taskTemplateOptions')).toEqual([
        { value: 4, label: 'uptime' },
        { value: 25, label: 'mem-check' },
      ])
    }
    wrapper.unmount()
  })
})

describe('自动化任务：删除结果', () => {
  it.each([
    ['成功', { results: [{ id: 34, ok: true, message: '' }] }, null],
    ['运行中', { results: [{ id: 34, ok: false, message: '作业仍在运行' }] }, '作业仍在运行'],
    ['响应缺失', {}, '删除结果异常'],
    ['结果不匹配', { results: [{ id: 35, ok: true }] }, '删除结果异常'],
  ])('%s时仅按当前任务结果提示', async (_label, data, errorText) => {
    vi.clearAllMocks()
    automationApi.batchDeleteTasks.mockResolvedValueOnce({ data: { data } })
    const wrapper = await mountPage()
    const initialLoads = automationApi.getTaskList.mock.calls.length
    wrapper.findComponent({ name: 'TaskListCard' }).vm.$emit('delete', { id: 34, name: 'diagnostic' })
    const options = openDeleteConfirm.mock.calls.at(-1)[0]
    expect(options.summary).toContain('保留执行历史')
    if (errorText) {
      await expect(options.onConfirm()).rejects.toThrow(errorText)
      expect(message.error).toHaveBeenCalledWith(expect.stringContaining(errorText))
      expect(message.success).not.toHaveBeenCalled()
      expect(automationApi.getTaskList.mock.calls.length).toBe(initialLoads)
    } else {
      await options.onConfirm()
      expect(message.success).toHaveBeenCalledWith('任务已删除')
      expect(automationApi.getTaskList.mock.calls.length).toBe(initialLoads + 1)
    }
    expect(automationApi.batchDeleteTasks).toHaveBeenCalledWith([34])
    wrapper.unmount()
  })
})
