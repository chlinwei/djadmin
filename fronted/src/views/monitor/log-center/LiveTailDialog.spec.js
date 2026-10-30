import { flushPromises, mount } from '@vue/test-utils'
import { defineComponent, h, KeepAlive, ref } from 'vue'
import { afterEach, describe, expect, it, vi } from 'vitest'
import Antd, { message, Modal, Tooltip } from 'ant-design-vue'
import LiveTailDialog from './LiveTailDialog.vue'
import { copyTextWithFallback } from '@/util/clipboard'
import { Blob as NodeBlob, Buffer } from 'node:buffer'
import store from '@/store'

vi.mock('@/api/user', () => ({ getToken: () => 'secret-jwt' }))
vi.mock('@/util/request', () => ({ getWebSocketBaseUrl: () => 'ws://localhost:9000' }))
vi.mock('@/util/clipboard', () => ({ copyTextWithFallback: vi.fn() }))
vi.mock('@/store', () => ({ default: { state: { user: { timezone: 'Asia/Shanghai' } } } }))

class FakeSocket {
  static OPEN = 1
  static sockets = []
  readyState = 1
  send = vi.fn()
  close = vi.fn()
  constructor(url) {
    this.url = url
    FakeSocket.sockets.push(this)
  }
}

const log = (id = 1) => ({ host_ip: `10.0.0.${id}`, service_id: '7', service: 'orders', instance: 'app', log_name: 'main', log_path: `/logs/${id}.log` })

function getCopyButton(wrapper) {
  return wrapper.findAll('button').find((button) => button.text().replace(/\s/g, '') === '复制')
}

function getDownloadButton(wrapper) {
  return wrapper.findAll('button').find((button) => button.text().replace(/\s/g, '') === '下载')
}

function mockDownloads() {
  const createObjectURL = vi.fn(() => 'blob:tail-download')
  const revokeObjectURL = vi.fn()
  vi.stubGlobal('URL', class extends URL {
    static createObjectURL = createObjectURL
    static revokeObjectURL = revokeObjectURL
  })
  vi.stubGlobal('Blob', NodeBlob)
  const downloads = []
  const click = vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(function () {
    downloads.push({ filename: this.download, href: this.href })
  })
  return { createObjectURL, revokeObjectURL, downloads, click }
}

describe('LiveTailDialog', () => {
  afterEach(() => {
    vi.useRealTimers()
    vi.restoreAllMocks()
    vi.unstubAllGlobals()
    copyTextWithFallback.mockReset()
    FakeSocket.sockets = []
    store.state.user.timezone = 'Asia/Shanghai'
  })

  it.each([
    ['Asia/Shanghai', '20261030_144318'],
    ['UTC', '20261030_064318'],
  ])('downloads all retained active-tab text using timezone %s without stopping the reader', async (timezone, timestamp) => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-10-30T06:43:18Z'))
    store.state.user.timezone = timezone
    vi.stubGlobal('WebSocket', FakeSocket)
    const { createObjectURL, revokeObjectURL, downloads } = mockDownloads()
    const wrapper = mount(LiveTailDialog, {
      global: { plugins: [Antd], stubs: { AModal: { template: '<div><slot /></div>' } } },
    })
    try {
      const record = { ...log(), log_name: 'jmsgw.log' }
      const content = `${Array.from({ length: 100 }, (_, index) => `行 ${index}`).join('\n')}\n最后一行\n`
      wrapper.vm.start(record, '订单/网关:服务')
      FakeSocket.sockets[0].onmessage({
        data: JSON.stringify({ type: 'data', data: Buffer.from(content).toString('base64') }),
      })
      wrapper.vm.start(log(2), '其他服务')
      FakeSocket.sockets[1].onmessage({ data: JSON.stringify({ type: 'data', data: btoa('other tab\n') }) })
      wrapper.vm.start(record)
      await vi.advanceTimersByTimeAsync(200)
      expect(wrapper.findAll('.tail-line').length).toBeLessThanOrEqual(36)
      const download = getDownloadButton(wrapper)
      expect(download.isVisible()).toBe(true)
      expect(download.element.previousElementSibling).toBe(getCopyButton(wrapper).element)
      const tooltip = wrapper.findAllComponents(Tooltip).find((item) => item.props('title') === '下载日志')
      expect(tooltip.props('placement')).toBe('top')
      await download.trigger('mouseenter')
      await vi.advanceTimersByTimeAsync(200)
      expect([...document.body.querySelectorAll('.ant-tooltip-inner')].some((node) => node.textContent === '下载日志')).toBe(true)
      await download.trigger('click')
      expect(downloads).toEqual([{ filename: `10.0.0.1-订单_网关_服务-jmsgw-${timestamp}.log`, href: 'blob:tail-download' }])
      const blob = createObjectURL.mock.calls[0][0]
      expect(blob.type).toBe('text/plain;charset=utf-8')
      expect(await blob.text()).toBe(content)
      expect(revokeObjectURL).toHaveBeenCalledWith('blob:tail-download')
      expect(document.querySelector('a[download]')).toBeNull()
      expect(FakeSocket.sockets[0].close).not.toHaveBeenCalled()
      expect(FakeSocket.sockets).toHaveLength(2)
      await wrapper.findAll('button').find((button) => button.text() === '停止跟踪').trigger('click')
      await download.trigger('click')
      expect(downloads).toHaveLength(2)
      expect(await createObjectURL.mock.calls[1][0].text()).toBe(content)
    } finally {
      wrapper.unmount()
    }
  })

  it('downloads only retained rows after eviction and reports empty or cleared content', async () => {
    vi.stubGlobal('WebSocket', FakeSocket)
    const { createObjectURL } = mockDownloads()
    const warning = vi.spyOn(message, 'warning').mockImplementation(() => {})
    const wrapper = mount(LiveTailDialog, {
      global: { plugins: [Antd], stubs: { AModal: { template: '<div><slot /></div>' } } },
    })
    try {
      wrapper.vm.start(log())
      await flushPromises()
      const download = getDownloadButton(wrapper)
      await download.trigger('click')
      expect(createObjectURL).not.toHaveBeenCalled()
      const lines = Array.from({ length: 5100 }, (_, index) => `L${index}`)
      FakeSocket.sockets[0].onmessage({ data: JSON.stringify({ type: 'data', data: btoa(`${lines.join('\n')}\n`) }) })
      await download.trigger('click')
      expect(await createObjectURL.mock.calls[0][0].text()).toBe(`${lines.slice(101).join('\n')}\n`)
      await wrapper.findAll('button').find((button) => button.text() === '清空显示').trigger('click')
      await download.trigger('click')
      expect(createObjectURL).toHaveBeenCalledOnce()
      expect(warning).toHaveBeenCalledTimes(2)
      expect(warning).toHaveBeenLastCalledWith('没有可下载的内容')
    } finally {
      wrapper.unmount()
    }
  })

  it('reports download errors and releases browser resources without stopping the reader', async () => {
    vi.stubGlobal('WebSocket', FakeSocket)
    const { revokeObjectURL, click } = mockDownloads()
    click.mockImplementation(() => { throw new Error('download blocked') })
    const error = vi.spyOn(message, 'error').mockImplementation(() => {})
    const wrapper = mount(LiveTailDialog, {
      global: { plugins: [Antd], stubs: { AModal: { template: '<div><slot /></div>' } } },
    })
    try {
      wrapper.vm.start(log())
      FakeSocket.sockets[0].onmessage({ data: JSON.stringify({ type: 'data', data: btoa('output\n') }) })
      await flushPromises()
      await getDownloadButton(wrapper).trigger('click')
      expect(error).toHaveBeenCalledWith('下载失败：download blocked')
      expect(revokeObjectURL).toHaveBeenCalledWith('blob:tail-download')
      expect(document.querySelector('a[download]')).toBeNull()
      expect(FakeSocket.sockets[0].close).not.toHaveBeenCalled()
    } finally {
      wrapper.unmount()
    }
  })

  it('reports missing service names instead of downloading a misidentified file', async () => {
    vi.stubGlobal('WebSocket', FakeSocket)
    const { createObjectURL } = mockDownloads()
    const error = vi.spyOn(message, 'error').mockImplementation(() => {})
    const wrapper = mount(LiveTailDialog, {
      global: { plugins: [Antd], stubs: { AModal: { template: '<div><slot /></div>' } } },
    })
    try {
      const record = log()
      delete record.service
      wrapper.vm.start(record)
      FakeSocket.sockets[0].onmessage({ data: JSON.stringify({ type: 'data', data: btoa('output\n') }) })
      await flushPromises()
      await getDownloadButton(wrapper).trigger('click')
      expect(error).toHaveBeenCalledWith('缺少服务名称，无法生成下载文件名，请重新查询后再试')
      expect(createObjectURL).not.toHaveBeenCalled()
      expect(FakeSocket.sockets[0].close).not.toHaveBeenCalled()
    } finally {
      wrapper.unmount()
    }
  })

  it('reports the actual missing field without attempting a connection', () => {
    vi.stubGlobal('WebSocket', FakeSocket)
    const warning = vi.spyOn(message, 'warning').mockImplementation(() => {})
    const wrapper = mount(LiveTailDialog, {
      global: { plugins: [Antd], stubs: { AModal: { template: '<div><slot /></div>' } } },
    })
    const record = log()
    delete record.host_ip
    wrapper.vm.start(record)
    expect(warning).toHaveBeenCalledWith(expect.stringContaining('缺少 host_ip'))
    expect(warning.mock.calls[0][0]).not.toContain('共享')
    expect(FakeSocket.sockets).toHaveLength(0)
    wrapper.unmount()
  })

  it('authenticates in first frame, caps tabs, deduplicates and cleans all sockets', async () => {
    vi.stubGlobal('WebSocket', FakeSocket)
    const wrapper = mount(LiveTailDialog, {
      global: { plugins: [Antd], stubs: { AModal: { template: '<div><slot /></div>' } } },
    })
    for (let id = 1; id <= 6; id++) wrapper.vm.start(log(id))
    expect(FakeSocket.sockets).toHaveLength(5)
    wrapper.vm.start(log(1))
    expect(FakeSocket.sockets).toHaveLength(5)
    const socket = FakeSocket.sockets[0]
    expect(socket.url).not.toContain('secret-jwt')
    socket.onopen()
    const auth = JSON.parse(socket.send.mock.calls[0][0])
    expect(auth).toMatchObject({ token: 'secret-jwt', source: { host_ip: '10.0.0.1', instance: 'app' } })
    expect(auth.source).not.toHaveProperty('host_id')
    wrapper.vm.closeAll()
    await flushPromises()
    for (const ws of FakeSocket.sockets) expect(ws.close).toHaveBeenCalledOnce()
    wrapper.unmount()
  })

  it('renders only visible plain text, stops without reconnecting and preserves output', async () => {
    vi.useFakeTimers()
    vi.stubGlobal('WebSocket', FakeSocket)
    const wrapper = mount(LiveTailDialog, {
      global: { plugins: [Antd], stubs: { AModal: { template: '<div><slot /></div>' } } },
    })
    wrapper.vm.start(log())
    const socket = FakeSocket.sockets[0]
    socket.onmessage({ data: JSON.stringify({ type: 'data', data: btoa('<script>alert(1)</script>\n' + 'line\n'.repeat(1000)) }) })
    await vi.advanceTimersByTimeAsync(200)
    await flushPromises()
    expect(wrapper.findAll('.tail-line').length).toBeLessThanOrEqual(36)
    expect(wrapper.find('script').exists()).toBe(false)
    const stopButton = wrapper.findAll('button').find((button) => button.text().includes('停止跟踪'))
    expect(stopButton.isVisible()).toBe(true)
    expect(wrapper.findComponent(Tooltip).props('placement')).toBe('top')
    await stopButton.trigger('mouseenter')
    await vi.advanceTimersByTimeAsync(200)
    expect(document.body.querySelector('.ant-tooltip-inner')?.textContent).toBe('取消')
    await stopButton.trigger('click')
    expect(socket.close).toHaveBeenCalledOnce()
    expect(wrapper.text()).toContain('已停止')
    expect(wrapper.findAll('button').some((button) => button.text() === '继续跟踪')).toBe(true)
    expect(FakeSocket.sockets).toHaveLength(1)
    wrapper.unmount()
  })

  it('continues in the same tab, reauthenticates and ignores callbacks from the old connection', async () => {
    vi.useFakeTimers()
    vi.stubGlobal('WebSocket', FakeSocket)
    const wrapper = mount(LiveTailDialog, {
      global: { plugins: [Antd], stubs: { AModal: { template: '<div><slot /></div>' } } },
    })
    try {
      wrapper.vm.start(log())
      const previous = FakeSocket.sockets[0]
      previous.onopen()
      previous.onmessage({ data: JSON.stringify({ type: 'data', data: btoa('previous output\n') }) })
      await vi.advanceTimersByTimeAsync(200)
      await wrapper.findAll('button').find((button) => button.text() === '停止跟踪').trigger('click')

      const resumeButton = wrapper.findAll('button').find((button) => button.text() === '继续跟踪')
      expect(resumeButton.isVisible()).toBe(true)
      expect(resumeButton.attributes('disabled')).toBeUndefined()
      const tooltip = wrapper.findAllComponents(Tooltip).find((component) => component.props('title') === '运行')
      expect(tooltip.props('placement')).toBe('top')
      await resumeButton.trigger('mouseenter')
      await vi.advanceTimersByTimeAsync(200)
      expect([...document.body.querySelectorAll('.ant-tooltip-inner')].some((node) => node.textContent === '运行')).toBe(true)
      await resumeButton.trigger('click')
      // Even a queued second click must not open another reader.
      await resumeButton.trigger('click')
      expect(FakeSocket.sockets).toHaveLength(2)
      const current = FakeSocket.sockets[1]
      current.onopen()
      expect(current.send).toHaveBeenCalledWith(previous.send.mock.calls[0][0])
      previous.onmessage({ data: JSON.stringify({ type: 'data', data: btoa('stale output\n') }) })
      previous.onerror()
      previous.onclose()
      await flushPromises()
      expect(wrapper.text()).toContain('连接中')
      current.onmessage({ data: JSON.stringify({ type: 'data', data: btoa('current output\n') }) })
      await vi.advanceTimersByTimeAsync(200)
      await flushPromises()

      const output = wrapper.find('.tail-viewport').text()
      expect(output).toContain('previous output')
      expect(output).toContain('继续跟踪')
      expect(output).toContain('缺口')
      expect(output).toContain('重复')
      expect(output).toContain('current output')
      expect(output).not.toContain('stale output')
      expect(wrapper.text()).toContain('跟踪中')
      expect(wrapper.findAll('.ant-tabs-tab')).toHaveLength(1)
      wrapper.vm.closeAll()
      expect(previous.close).toHaveBeenCalledOnce()
      expect(current.close).toHaveBeenCalledOnce()
    } finally {
      wrapper.unmount()
    }
  })

  it('copies the active file cache beyond visible rows, both while tracking and stopped', async () => {
    vi.useFakeTimers()
    vi.stubGlobal('WebSocket', FakeSocket)
    copyTextWithFallback.mockResolvedValue(true)
    const success = vi.spyOn(message, 'success').mockImplementation(() => {})
    const wrapper = mount(LiveTailDialog, {
      global: { plugins: [Antd], stubs: { AModal: { template: '<div><slot /></div>' } } },
    })
    try {
      const content = `日志 <script>literal text</script>\n${'line\n'.repeat(100)}last cached row\n`
      wrapper.vm.start(log(1))
      const first = FakeSocket.sockets[0]
      first.onmessage({ data: JSON.stringify({
        type: 'data', data: btoa(String.fromCharCode(...new TextEncoder().encode(content))),
      }) })
      wrapper.vm.start(log(2))
      FakeSocket.sockets[1].onmessage({ data: JSON.stringify({ type: 'data', data: btoa('other file\n') }) })
      wrapper.vm.start(log(1))
      await vi.advanceTimersByTimeAsync(200)
      expect(wrapper.findAll('.tail-line').length).toBeLessThanOrEqual(36)
      expect(wrapper.find('.tail-viewport').text()).not.toContain('last cached row')
      const copyButton = getCopyButton(wrapper)
      expect(copyButton.isVisible()).toBe(true)
      expect(copyButton.element.closest('.tail-output-head')).toBe(wrapper.find('.tail-output-head').element)
      expect(copyButton.element.closest('.ant-space')).toBeNull()
      expect(wrapper.find('.tail-sidebar .tail-path').text()).toBe('/logs/1.log')
      expect(wrapper.find('.tail-sidebar .ant-alert').exists()).toBe(true)
      expect(wrapper.find('.tail-sidebar').text()).toContain('停止跟踪')
      expect(wrapper.find('.tail-sidebar').text()).toContain('清空显示')
      expect(wrapper.find('.tail-output').element.contains(copyButton.element)).toBe(true)
      expect(wrapper.find('.tail-output .tail-viewport').exists()).toBe(true)
      expect(wrapper.find('.tail-output').text()).not.toContain('清空显示')
      await copyButton.trigger('click')
      await flushPromises()
      expect(copyTextWithFallback).toHaveBeenLastCalledWith(content)
      expect(success).toHaveBeenCalledWith('已复制')
      expect(first.close).not.toHaveBeenCalled()
      await wrapper.findAll('button').find((button) => button.text() === '停止跟踪').trigger('click')
      await copyButton.trigger('click')
      await flushPromises()
      expect(copyTextWithFallback).toHaveBeenCalledTimes(2)
      expect(copyTextWithFallback).toHaveBeenLastCalledWith(content)
      expect(first.close).toHaveBeenCalledOnce()
      expect(FakeSocket.sockets).toHaveLength(2)
    } finally {
      wrapper.unmount()
    }
  })

  it('copies only retained content after older rows are evicted', async () => {
    vi.useFakeTimers()
    vi.stubGlobal('WebSocket', FakeSocket)
    copyTextWithFallback.mockResolvedValue(true)
    vi.spyOn(message, 'success').mockImplementation(() => {})
    const wrapper = mount(LiveTailDialog, {
      global: { plugins: [Antd], stubs: { AModal: { template: '<div><slot /></div>' } } },
    })
    try {
      const lines = Array.from({ length: 5100 }, (_, index) => `L${index}`)
      wrapper.vm.start(log())
      FakeSocket.sockets[0].onmessage({ data: JSON.stringify({ type: 'data', data: btoa(`${lines.join('\n')}\n`) }) })
      await vi.advanceTimersByTimeAsync(200)
      await getCopyButton(wrapper).trigger('click')
      await flushPromises()
      expect(copyTextWithFallback).toHaveBeenCalledWith(`${lines.slice(101).join('\n')}\n`)
      expect(wrapper.text()).toContain('页面缓存已淘汰较早内容')
    } finally {
      wrapper.unmount()
    }
  })

  it('warns without touching the clipboard when output is empty or cleared', async () => {
    vi.stubGlobal('WebSocket', FakeSocket)
    const warning = vi.spyOn(message, 'warning').mockImplementation(() => {})
    const wrapper = mount(LiveTailDialog, {
      global: { plugins: [Antd], stubs: { AModal: { template: '<div><slot /></div>' } } },
    })
    try {
      wrapper.vm.start(log())
      await flushPromises()
      const copyButton = getCopyButton(wrapper)
      await copyButton.trigger('click')
      FakeSocket.sockets[0].onmessage({ data: JSON.stringify({ type: 'data', data: btoa('discarded output\n') }) })
      await wrapper.findAll('button').find((button) => button.text() === '清空显示').trigger('click')
      await copyButton.trigger('click')
      expect(warning).toHaveBeenCalledTimes(2)
      expect(warning).toHaveBeenLastCalledWith('没有可复制的内容')
      expect(copyTextWithFallback).not.toHaveBeenCalled()
    } finally {
      wrapper.unmount()
    }
  })

  it('reports a clipboard failure without stopping the reader or claiming success', async () => {
    vi.stubGlobal('WebSocket', FakeSocket)
    copyTextWithFallback.mockResolvedValue(false)
    const error = vi.spyOn(message, 'error').mockImplementation(() => {})
    const success = vi.spyOn(message, 'success').mockImplementation(() => {})
    const wrapper = mount(LiveTailDialog, {
      global: { plugins: [Antd], stubs: { AModal: { template: '<div><slot /></div>' } } },
    })
    try {
      wrapper.vm.start(log())
      FakeSocket.sockets[0].onmessage({ data: JSON.stringify({ type: 'data', data: btoa('output\n') }) })
      await flushPromises()
      await getCopyButton(wrapper).trigger('click')
      await flushPromises()
      expect(error).toHaveBeenCalledWith('复制失败，请检查浏览器权限')
      expect(success).not.toHaveBeenCalled()
      expect(FakeSocket.sockets[0].close).not.toHaveBeenCalled()
    } finally {
      wrapper.unmount()
    }
  })

  it('allows manual continuation after a disconnect but never automatically reconnects', async () => {
    vi.useFakeTimers()
    vi.stubGlobal('WebSocket', FakeSocket)
    const wrapper = mount(LiveTailDialog, {
      global: { plugins: [Antd], stubs: { AModal: { template: '<div><slot /></div>' } } },
    })
    try {
      wrapper.vm.start(log())
      const socket = FakeSocket.sockets[0]
      socket.onerror()
      socket.onclose()
      await vi.advanceTimersByTimeAsync(20000)
      expect(FakeSocket.sockets).toHaveLength(1)
      expect(wrapper.text()).toContain('异常结束')
      await wrapper.findAll('button').find((button) => button.text() === '继续跟踪').trigger('click')
      expect(FakeSocket.sockets).toHaveLength(2)
      expect(wrapper.text()).toContain('连接中')
    } finally {
      wrapper.unmount()
    }
  })

  it('continues a stopped file at the five-tab limit without adding another tab', async () => {
    vi.stubGlobal('WebSocket', FakeSocket)
    const info = vi.spyOn(message, 'info').mockImplementation(() => {})
    const wrapper = mount(LiveTailDialog, {
      global: { plugins: [Antd], stubs: { AModal: { template: '<div><slot /></div>' } } },
    })
    try {
      for (let id = 1; id <= 5; id++) wrapper.vm.start(log(id))
      await flushPromises()
      await wrapper.findAll('button').find((button) => button.text() === '停止跟踪').trigger('click')
      wrapper.vm.start(log(5))
      await flushPromises()
      expect(info).toHaveBeenCalledWith(expect.stringContaining('继续跟踪'))
      expect(FakeSocket.sockets).toHaveLength(5)
      await wrapper.findAll('button').find((button) => button.text() === '继续跟踪').trigger('click')
      expect(FakeSocket.sockets).toHaveLength(6)
      expect(wrapper.findAll('.ant-tabs-tab')).toHaveLength(5)
    } finally {
      wrapper.unmount()
    }
  })

  it('retains the stopped state and output when creating a continuation connection fails', async () => {
    vi.useFakeTimers()
    vi.stubGlobal('WebSocket', FakeSocket)
    const error = vi.spyOn(message, 'error').mockImplementation(() => {})
    const wrapper = mount(LiveTailDialog, {
      global: { plugins: [Antd], stubs: { AModal: { template: '<div><slot /></div>' } } },
    })
    try {
      wrapper.vm.start(log())
      FakeSocket.sockets[0].onmessage({ data: JSON.stringify({ type: 'data', data: btoa('retained output\n') }) })
      await vi.advanceTimersByTimeAsync(200)
      await wrapper.findAll('button').find((button) => button.text() === '停止跟踪').trigger('click')
      vi.stubGlobal('WebSocket', class {
        constructor() { throw new Error('connection blocked') }
      })
      await wrapper.findAll('button').find((button) => button.text() === '继续跟踪').trigger('click')
      expect(error).toHaveBeenCalledWith(expect.stringContaining('无法建立实时日志连接'))
      expect(wrapper.text()).toContain('已停止')
      expect(wrapper.find('.tail-viewport').text()).toContain('retained output')
      expect(FakeSocket.sockets).toHaveLength(1)
      vi.stubGlobal('WebSocket', FakeSocket)
      await wrapper.findAll('button').find((button) => button.text() === '继续跟踪').trigger('click')
      expect(FakeSocket.sockets).toHaveLength(2)
    } finally {
      wrapper.unmount()
    }
  })

  it.each(['escape', 'close button', 'mask'])('stops every reader when the real modal closes via %s', async (action) => {
    vi.stubGlobal('WebSocket', FakeSocket)
    const underlyingEscape = vi.fn()
    document.addEventListener('keydown', underlyingEscape)
    const backgroundButton = document.createElement('button')
    document.body.appendChild(backgroundButton)
    const wrapper = mount(LiveTailDialog, {
      attachTo: document.body,
      global: { plugins: [Antd] },
    })
    try {
      wrapper.vm.start(log(1))
      wrapper.vm.start(log(2))
      await flushPromises()
      expect(wrapper.findComponent(Modal).props('open')).toBe(true)
      if (action === 'escape') {
        // The details drawer can keep or regain focus behind the tail modal.
        backgroundButton.focus()
        backgroundButton.dispatchEvent(new KeyboardEvent('keydown', {
          key: 'Escape', keyCode: 27, bubbles: true, cancelable: true,
        }))
      } else {
        document.querySelector(action === 'close button' ? '.ant-modal-close' : '.ant-modal-wrap').click()
      }
      await flushPromises()

      expect(wrapper.findComponent(Modal).props('open')).toBe(false)
      if (action === 'escape') expect(underlyingEscape).not.toHaveBeenCalled()
      for (const socket of FakeSocket.sockets) {
        expect(socket.send).toHaveBeenCalledWith(JSON.stringify({ type: 'stop' }))
        expect(socket.close).toHaveBeenCalledOnce()
        socket.onmessage({ data: JSON.stringify({ type: 'data', data: btoa('late output\n') }) })
        socket.onclose()
      }
      await flushPromises()
      expect(wrapper.findComponent(Modal).props('open')).toBe(false)
      expect(document.body.textContent).not.toContain('late output')
      underlyingEscape.mockClear()
      backgroundButton.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }))
      expect(underlyingEscape).toHaveBeenCalledOnce()
    } finally {
      wrapper.unmount()
      backgroundButton.remove()
      document.removeEventListener('keydown', underlyingEscape)
    }
  })

  it('removes its Escape listener when an open dialog is unmounted', async () => {
    vi.stubGlobal('WebSocket', FakeSocket)
    const wrapper = mount(LiveTailDialog, {
      global: { plugins: [Antd], stubs: { AModal: { template: '<div><slot /></div>' } } },
    })
    wrapper.vm.start(log())
    await flushPromises()
    wrapper.unmount()
    const otherPageEscape = vi.fn()
    document.addEventListener('keydown', otherPageEscape)
    try {
      document.body.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }))
      expect(otherPageEscape).toHaveBeenCalledOnce()
      expect(FakeSocket.sockets[0].close).toHaveBeenCalledOnce()
    } finally {
      document.removeEventListener('keydown', otherPageEscape)
    }
  })

  it('closes readers on KeepAlive page departure and does not reopen them on return', async () => {
    vi.stubGlobal('WebSocket', FakeSocket)
    const viewing = ref(true)
    const OtherPage = defineComponent({ render: () => h('div', 'Other page') })
    const Page = defineComponent({
      setup: () => () => h(KeepAlive, null, {
        default: () => viewing.value ? h(LiveTailDialog) : h(OtherPage),
      }),
    })
    const wrapper = mount(Page, {
      attachTo: document.body,
      global: { plugins: [Antd] },
    })
    try {
      const dialog = wrapper.findComponent(LiveTailDialog)
      dialog.vm.start(log(1))
      dialog.vm.start(log(2))
      await flushPromises()
      viewing.value = false
      await flushPromises()
      for (const socket of FakeSocket.sockets) {
        expect(socket.send).toHaveBeenCalledWith(JSON.stringify({ type: 'stop' }))
        expect(socket.close).toHaveBeenCalledOnce()
      }
      viewing.value = true
      await flushPromises()
      expect(FakeSocket.sockets).toHaveLength(2)
      expect(wrapper.findComponent(Modal).props('open')).toBe(false)
      wrapper.findComponent(LiveTailDialog).vm.start(log(1))
      await flushPromises()
      expect(FakeSocket.sockets).toHaveLength(3)
      expect(wrapper.findComponent(Modal).props('open')).toBe(true)
      expect(document.body.textContent).not.toContain('继续跟踪：')
    } finally {
      wrapper.unmount()
    }
  })
})
