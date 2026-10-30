import { flushPromises, mount } from '@vue/test-utils'
import Antd, { Table, Tooltip, message } from 'ant-design-vue'
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

vi.mock('@/util/deleteConfirm', async () => ({
  ...await vi.importActual('@/util/deleteConfirm'),
  openDeleteConfirm: vi.fn(),
}))
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
  it('表格铺满可用宽度、描述自适应，长文本提示、排序及固定操作列保留', async () => {
    const name = 'production-service-health-check-template-'.repeat(6)
    const description = '服务健康检查的详细说明。'.repeat(30)
    automationApi.getPlaybookList.mockResolvedValueOnce({
      data: { data: { results: [{ id: 8, name, category: 'general', content_format: 'playbook', description }], count: 1 } },
    })
    const wrapper = await mountPage()
    try {
      const table = wrapper.findComponent(Table)
      expect(table.props('tableLayout')).toBe('fixed')
      expect(table.props('scroll')).toEqual({ x: 1200 })
      expect(table.element.style.width).toBe('100%')
      expect(table.element.style.maxWidth).toBe('')
      expect(table.props('columns')[0]).toMatchObject({
        key: 'name', width: 280, ellipsis: { showTitle: false }, sorter: true,
      })
      expect(table.props('columns').find((column) => column.key === 'description')).toMatchObject({
        ellipsis: { showTitle: false },
      })
      expect(table.props('columns').find((column) => column.key === 'description')).not.toHaveProperty('width')
      expect(table.props('columns').at(-1)).toMatchObject({ key: 'action', fixed: 'right', width: 180 })
      expect(wrapper.get('colgroup col').element.style.width).toBe('280px')
      expect(wrapper.findAll('colgroup col')[3].element.style.width).toBe('')
      const [label, descriptionLabel] = wrapper.findAll('.template-cell-text')
      expect(label.text()).toBe(name)
      expect(descriptionLabel.text()).toBe(description)
      expect(label.element.closest('td').classList.contains('ant-table-cell-ellipsis')).toBe(true)
      const tooltip = wrapper.findAllComponents(Tooltip).find((item) => item.props('title') === name)
      expect(tooltip.props('placement')).toBe('top')
      await label.trigger('mouseenter')
      await new Promise((resolve) => setTimeout(resolve, 200))
      expect([...document.querySelectorAll('.ant-tooltip-inner')].some((item) => item.textContent === name)).toBe(true)
      const descriptionTooltip = wrapper.findAllComponents(Tooltip).find((item) => item.props('title') === description)
      expect(descriptionTooltip.props('placement')).toBe('top')
      await label.trigger('mouseleave')
      await descriptionLabel.trigger('mouseenter')
      await new Promise((resolve) => setTimeout(resolve, 200))
      expect([...document.querySelectorAll('.ant-tooltip-inner')].some((item) => item.textContent === description)).toBe(true)
      table.vm.$emit('change', { current: 1, pageSize: 10 }, {}, { field: 'name', order: 'ascend' })
      await flushPromises()
      expect(automationApi.getPlaybookList).toHaveBeenLastCalledWith(expect.objectContaining({ ordering: 'name' }))
    } finally {
      wrapper.unmount()
    }
  })

  it.each([
    { format: 'shell', headers: {}, filename: 'health-check.sh' },
    { format: 'playbook', headers: {}, filename: 'health-check.yml' },
    {
      format: 'shell',
      headers: { 'content-disposition': `attachment; filename*=UTF-8''${encodeURIComponent('服务器脚本.sh')}` },
      filename: '服务器脚本.sh',
    },
    {
      format: 'playbook',
      headers: { 'Content-Disposition': 'attachment; filename="server-template.yml"' },
      filename: 'server-template.yml',
    },
  ])('下载 $format 模板使用 $filename，保留服务端原始内容', async ({ format, headers, filename }) => {
    automationApi.getPlaybookList.mockResolvedValueOnce({
      data: { data: { results: [{ id: 9, name: 'health-check', category: 'general', content_format: format }], count: 1 } },
    })
    const blob = new Blob(['original template content'])
    automationApi.downloadPlaybookFile.mockResolvedValueOnce({ data: blob, headers })
    const createObjectURL = vi.fn(() => 'blob:template-download')
    const revokeObjectURL = vi.fn()
    vi.stubGlobal('URL', class extends URL {
      static createObjectURL = createObjectURL
      static revokeObjectURL = revokeObjectURL
    })
    const downloads = []
    const click = vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(function () {
      downloads.push({ filename: this.download, href: this.href })
    })
    const wrapper = await mountPage()
    try {
      const download = wrapper.findAllComponents(Tooltip).find((item) => item.props('title') === '下载')
      await download.get('button').trigger('click')
      await flushPromises()
      expect(automationApi.downloadPlaybookFile).toHaveBeenLastCalledWith(9)
      expect(createObjectURL).toHaveBeenCalledOnce()
      expect(createObjectURL).toHaveBeenCalledWith(blob)
      expect(downloads).toEqual([{ filename, href: 'blob:template-download' }])
      expect(revokeObjectURL).toHaveBeenCalledOnce()
      expect(revokeObjectURL).toHaveBeenCalledWith('blob:template-download')
    } finally {
      wrapper.unmount()
      click.mockRestore()
      vi.unstubAllGlobals()
    }
  })

  it.each([true, false])('删除逐项结果 ok=%s 时正确提示', async (ok) => {
    const success = vi.spyOn(message, 'success').mockImplementation(() => {})
    const error = vi.spyOn(message, 'error').mockImplementation(() => {})
    automationApi.batchDeletePlaybooks.mockResolvedValueOnce({
      data: { data: { results: [{ id: 7, ok, message: '模板被任务引用' }] } },
    })
    const wrapper = await mountPage()
    const loads = automationApi.getPlaybookList.mock.calls.length
    try {
      const deletion = wrapper.vm.onDeleteTemplate({ id: 7 })
      if (ok) {
        await deletion
        expect(success).toHaveBeenCalledWith('模板删除成功')
        expect(automationApi.getPlaybookList.mock.calls.length).toBe(loads + 1)
      } else {
        await expect(deletion).rejects.toThrow('模板被任务引用')
        expect(error).toHaveBeenCalledWith(expect.stringContaining('模板被任务引用'))
        expect(success).not.toHaveBeenCalled()
        expect(automationApi.getPlaybookList.mock.calls.length).toBe(loads)
      }
    } finally {
      wrapper.unmount()
      success.mockRestore()
      error.mockRestore()
    }
  })

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
