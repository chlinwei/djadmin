import { flushPromises, mount } from '@vue/test-utils'
import Antd, { message } from 'ant-design-vue'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import OperationLog from './index.vue'
import { getAuditOperationLogs } from '@/api/sys/audit.js'

vi.mock('@/store', () => ({
  default: { state: { user: { timezone: 'Asia/Shanghai' }, perms: [], role_codes: ['readonly'] } },
}))
vi.mock('@/api/sys/audit.js', () => ({ getAuditOperationLogs: vi.fn() }))

const record = {
  id: 1, username: 'view', method: 'POST', path: '/monitor/pipeline-simulate/',
  route_name: '/monitor/pipeline-simulate/', status_code: 200, duration_ms: 0,
  created_at: '2026-11-02T08:00:00Z', message: 'success',
  request_data: JSON.stringify({ body: { sample: '<adb:message>A&B</adb:message>' } }),
  response_data: JSON.stringify({ code: 200, msg: 'success', data: { count: 5 } }),
}
const originalClipboard = Object.getOwnPropertyDescriptor(navigator, 'clipboard')
const originalExecCommand = Object.getOwnPropertyDescriptor(document, 'execCommand')
let wrapper
let writeText

beforeEach(() => {
  vi.clearAllMocks()
  writeText = vi.fn().mockResolvedValue(undefined)
  Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { writeText } })
  vi.spyOn(message, 'success').mockImplementation(() => {})
  vi.spyOn(message, 'warning').mockImplementation(() => {})
  vi.spyOn(message, 'error').mockImplementation(() => {})
})

afterEach(() => {
  wrapper?.unmount()
  document.body.innerHTML = ''
  vi.restoreAllMocks()
  if (originalClipboard) Object.defineProperty(navigator, 'clipboard', originalClipboard)
  else delete navigator.clipboard
  if (originalExecCommand) Object.defineProperty(document, 'execCommand', originalExecCommand)
  else delete document.execCommand
})

async function openDetails(overrides = {}) {
  getAuditOperationLogs.mockResolvedValue({
    data: { data: { results: [{ ...record, ...overrides }], count: 1, pageNumber: 1 } },
  })
  wrapper = mount(OperationLog, {
    attachTo: document.body,
    global: { plugins: [Antd], stubs: { FontAwesomeIcon: true } },
  })
  await flushPromises()
  await wrapper.get('tr[data-row-key="1"] button').trigger('click')
  await flushPromises()
}

function detailCards() {
  return [...document.querySelectorAll('.ant-modal-body .ant-card')]
}

describe('操作日志详情复制', () => {
  it('两侧标题右侧提供统一小号默认复制按钮，复制格式化后的展示文本', async () => {
    await openDetails()
    const cards = detailCards()
    expect(cards).toHaveLength(2)
    for (const [index, field] of ['request_data', 'response_data'].entries()) {
      const button = cards[index].querySelector('.ant-card-head .ant-card-extra button')
      expect(button.textContent.replace(/\s/g, '')).toBe('复制')
      expect(button.classList.contains('ant-btn-sm')).toBe(true)
      expect(button.classList.contains('ant-btn-default')).toBe(true)
      expect(cards[index].querySelector('pre').textContent).toBe(JSON.stringify(JSON.parse(record[field]), null, 2))
      button.click()
      await flushPromises()
      expect(writeText).toHaveBeenLastCalledWith(cards[index].querySelector('pre').textContent)
    }
    expect(writeText).toHaveBeenCalledTimes(2)
    expect(message.success).toHaveBeenCalledTimes(2)
    expect(message.success).toHaveBeenLastCalledWith('已复制')
    const table = wrapper.findComponent({ name: 'ATable' })
    expect(table.props('columns').at(-1)).toMatchObject({ fixed: 'right', width: 110 })
    expect(table.props('scroll')).toEqual({ x: 1300 })
  })

  it('复制16 KiB截断文本时保留全部已保存内容和标记，不把XML作为HTML渲染', async () => {
    const prefix = '{"text":"<b>literal</b>'
    const notice = '\n[truncated]'
    const truncated = prefix + 'x'.repeat(16384 - prefix.length - notice.length) + notice
    await openDetails({ request_data: truncated, response_data: truncated })
    for (const card of detailCards()) {
      expect(card.querySelector('pre').textContent).toBe(truncated)
      expect(card.querySelector('pre b')).toBeNull()
      card.querySelector('.ant-card-extra button').click()
      await flushPromises()
      expect(writeText).toHaveBeenLastCalledWith(truncated)
    }
    expect(writeText).toHaveBeenCalledTimes(2)
  })

  it('空内容只提示，不把暂无内容占位文本复制到剪贴板', async () => {
    await openDetails({ request_data: '', response_data: null })
    for (const card of detailCards()) {
      expect(card.querySelector('pre').textContent).toBe('暂无内容')
      card.querySelector('.ant-card-extra button').click()
      await flushPromises()
    }
    expect(writeText).not.toHaveBeenCalled()
    expect(message.warning).toHaveBeenCalledTimes(2)
    expect(message.warning).toHaveBeenLastCalledWith('没有可复制的内容')
    expect(message.success).not.toHaveBeenCalled()
  })

  it('HTTP页面没有Clipboard API时复用公共工具复制，并清理临时文本框', async () => {
    Object.defineProperty(navigator, 'clipboard', { configurable: true, value: undefined })
    const copied = []
    const execCommand = vi.fn(() => {
      copied.push(document.querySelector('textarea[readonly]').value)
      return true
    })
    Object.defineProperty(document, 'execCommand', { configurable: true, value: execCommand })
    await openDetails()
    for (const card of detailCards()) {
      card.querySelector('.ant-card-extra button').click()
      await flushPromises()
      expect(copied.at(-1)).toBe(card.querySelector('pre').textContent)
      expect(document.querySelector('textarea[readonly]')).toBeNull()
    }
    expect(execCommand).toHaveBeenCalledTimes(2)
    expect(execCommand).toHaveBeenLastCalledWith('copy')
    expect(message.success).toHaveBeenCalledTimes(2)
  })

  it('剪贴板及公共降级路径均失败时明确报错，不误报复制成功', async () => {
    writeText.mockRejectedValue(new Error('clipboard denied'))
    Object.defineProperty(document, 'execCommand', { configurable: true, value: vi.fn(() => false) })
    await openDetails()
    for (const card of detailCards()) {
      card.querySelector('.ant-card-extra button').click()
      await flushPromises()
    }
    expect(message.error).toHaveBeenCalledTimes(2)
    expect(message.error).toHaveBeenLastCalledWith('复制失败，请检查浏览器权限')
    expect(message.success).not.toHaveBeenCalled()
    expect(document.querySelector('textarea[readonly]')).toBeNull()
  })
})
