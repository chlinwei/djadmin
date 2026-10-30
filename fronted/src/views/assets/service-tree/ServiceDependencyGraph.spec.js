import { flushPromises, mount } from '@vue/test-utils'
import Antd, { message, Modal, Spin } from 'ant-design-vue'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
vi.mock('./dependencyGraph', async (original) => {
  const actual = await original()
  return { ...actual, layoutDependencyNodes: vi.fn(), dependencySvg: vi.fn(actual.dependencySvg) }
})
vi.mock('./dependencyGraphExport', async (original) => ({
  ...await original(), dependencyPng: vi.fn(), copyDependencyPng: vi.fn(),
}))
vi.mock('@vue-flow/core', () => ({
  VueFlow: {
    name: 'VueFlow',
    props: ['modelValue'],
    emits: ['init', 'nodes-initialized', 'update:modelValue', 'node-click', 'edge-click'],
    template: '<div class="flow-stub" />',
  },
  BaseEdge: { template: '<div />' },
  EdgeLabelRenderer: { template: '<div><slot /></div>' },
  Handle: { template: '<div />' },
  Position: { Left: 'left', Right: 'right' },
}))
vi.mock('@vue-flow/background', () => ({ Background: { template: '<div />' } }))
import { VueFlow } from '@vue-flow/core'
import { dependencySvg, layoutDependencyNodes } from './dependencyGraph'
import { copyDependencyPng, dependencyPng } from './dependencyGraphExport'
import Graph from './ServiceDependencyGraph.vue'

const record = (id) => ({ id, name: `服务 ${id}`, code: String(id), business_system_name: '业务', runtime_summary: { total: 0, running: 0, stopped: 0, error: 0, unknown: 0 } })
const layoutKey = '[1,3,null,null]'
const storageKey = `service-dependency-layout:v1:${layoutKey}`
let wrapper
let restoreFullscreen = () => {}
function fullscreenApi() {
  const descriptors = [
    [document, 'fullscreenElement', Object.getOwnPropertyDescriptor(document, 'fullscreenElement')],
    [document, 'exitFullscreen', Object.getOwnPropertyDescriptor(document, 'exitFullscreen')],
    [document.documentElement, 'requestFullscreen', Object.getOwnPropertyDescriptor(document.documentElement, 'requestFullscreen')],
  ]
  let element = null
  const change = (target) => { element = target; document.dispatchEvent(new Event('fullscreenchange')) }
  const enter = vi.fn(async () => change(document.documentElement))
  const exit = vi.fn(async () => change(null))
  Object.defineProperty(document, 'fullscreenElement', { configurable: true, get: () => element })
  Object.defineProperty(document, 'exitFullscreen', { configurable: true, value: exit })
  Object.defineProperty(document.documentElement, 'requestFullscreen', { configurable: true, value: enter })
  restoreFullscreen = () => {
    for (const [target, key, descriptor] of descriptors) {
      if (descriptor) Object.defineProperty(target, key, descriptor)
      else Reflect.deleteProperty(target, key)
    }
  }
  return { enter, exit, change }
}
const flowNodes = (flow) => flow.props('modelValue').filter((element) => !('source' in element))
beforeEach(() => {
  vi.clearAllMocks()
  vi.stubGlobal('isSecureContext', true)
  localStorage.clear()
  dependencyPng.mockReset().mockResolvedValue(new Blob(['png-data'], { type: 'image/png' }))
  copyDependencyPng.mockReset().mockResolvedValue(undefined)
  layoutDependencyNodes.mockImplementation(async (nodes) => nodes.map((node, i) => ({ id: String(node.id), position: { x: i * 500, y: 30 } })))
})
afterEach(() => {
  wrapper?.unmount()
  restoreFullscreen()
  restoreFullscreen = () => {}
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
  localStorage.clear()
})
async function create(props = {}) {
  wrapper = mount(Graph, { props: { nodes: [record(1), record(2)], edges: [], layoutKey, ...props }, global: { plugins: [Antd], stubs: { FontAwesomeIcon: true } } })
  await flushPromises()
  return wrapper.findComponent(VueFlow)
}
async function clickText(text) {
  await wrapper.findAll('button').find((button) => button.text().replace(/\s+/g, '') === text).trigger('click')
  await flushPromises()
}
const layoutStatus = () => wrapper.get('.graph-controls [role="status"]')
const saveButton = () => wrapper.findAll('button').find((button) => button.text().includes('保存布局'))
const restoreButton = () => wrapper.findAll('button').find((button) => button.text() === '还原布局')
describe('dependency graph interactions', () => {
  it('restores initial coordinates without saving or changing scope and can be used repeatedly', async () => {
    const flow = await create()
    const initial = flowNodes(flow).map((node) => ({ ...node.position }))
    expect(restoreButton().attributes('disabled')).toBeDefined()
    const write = vi.spyOn(Storage.prototype, 'setItem')
    for (let i = 0; i < 2; i++) {
      flowNodes(flow)[0].position.x += 120
      await flushPromises()
      expect(restoreButton().attributes('disabled')).toBeUndefined()
      await clickText('还原布局')
      expect(flowNodes(flow).map((node) => node.position)).toEqual(initial)
      expect(layoutStatus().text()).toBe('尚未保存')
      expect(restoreButton().attributes('disabled')).toBeDefined()
    }
    expect(write).not.toHaveBeenCalled()
    expect(wrapper.props('layoutKey')).toBe(layoutKey)
    expect(layoutDependencyNodes).toHaveBeenCalledTimes(1)
  })
  it('restores saved positions after relayout without overwriting storage', async () => {
    const flow = await create()
    flowNodes(flow)[0].position.y = 240
    await flushPromises()
    await clickText('保存布局')
    const saved = localStorage.getItem(storageKey)
    await clickText('重新布局')
    expect(layoutStatus().text()).toBe('有未保存的修改')
    await clickText('还原布局')
    expect(flowNodes(flow)[0].position).toEqual({ x: 0, y: 240 })
    expect(layoutStatus().text()).toBe('已保存')
    expect(localStorage.getItem(storageKey)).toBe(saved)
    expect(saveButton().attributes('disabled')).toBeDefined()
  })
  it('restores new unsaved nodes to their initial placement alongside saved nodes', async () => {
    const flow = await create()
    await clickText('保存布局')
    await wrapper.setProps({ nodes: [record(1), record(2), record(3)] })
    await flushPromises()
    const initial = flowNodes(flow).map((node) => ({ ...node.position }))
    flowNodes(flow)[2].position.y = 400
    await flushPromises()
    await clickText('还原布局')
    expect(flowNodes(flow).map((node) => node.position)).toEqual(initial)
    expect(JSON.parse(localStorage.getItem(storageKey)).positions[3]).toBeUndefined()
  })
  it('keeps the line legend without displaying a bottom help paragraph', async () => {
    await create()
    expect(wrapper.find('.graph-help').exists()).toBe(false)
    expect(wrapper.get('.graph-legend').text()).toContain('实线：强依赖')
    expect(wrapper.text()).not.toContain('A → B 表示 A 依赖 B')
  })
  it('saves dragged positions only on click and restores them after remounting', async () => {
    let flow = await create()
    expect(layoutStatus().text()).toBe('尚未保存')
    expect(saveButton().attributes('disabled')).toBeUndefined()
    const dragged = flowNodes(flow).map((node) => ({ ...node, position: { x: node.position.x + 123, y: 240 } }))
    flow.vm.$emit('update:modelValue', dragged)
    await flushPromises()
    expect(layoutStatus().text()).toBe('有未保存的修改')
    expect(layoutStatus().classes()).toContain('ant-tag-orange')
    expect(localStorage.getItem(storageKey)).toBeNull()
    const success = vi.spyOn(message, 'success').mockImplementation(() => {})
    await clickText('保存布局')
    expect(JSON.parse(localStorage.getItem(storageKey))).toEqual({
      version: 1, positions: { 1: { x: 123, y: 240 }, 2: { x: 623, y: 240 } }, labelOffsets: {},
    })
    expect(success).toHaveBeenCalledWith('布局已保存到当前浏览器')
    expect(layoutStatus().text()).toBe('已保存')
    expect(layoutStatus().classes()).toContain('ant-tag-green')
    expect(saveButton().attributes('disabled')).toBeDefined()
    const saved = localStorage.getItem(storageKey)
    await clickText('重新布局')
    expect(flowNodes(flow)[0].position).toEqual({ x: 0, y: 30 })
    expect(localStorage.getItem(storageKey)).toBe(saved)
    expect(layoutStatus().text()).toBe('有未保存的修改')
    expect(saveButton().attributes('disabled')).toBeUndefined()
    wrapper.unmount()
    flow = await create()
    expect(flowNodes(flow).map((node) => node.position)).toEqual(dragged.map((node) => node.position))
    expect(layoutStatus().text()).toBe('已保存')
    expect(saveButton().attributes('disabled')).toBeDefined()
  })
  it('detects in-place coordinate changes and clears the marker when dragged back, without counting viewport or state refreshes', async () => {
    let flow = await create()
    await clickText('保存布局')
    wrapper.unmount()
    flow = await create()
    const node = flowNodes(flow)[0]
    const original = { ...node.position }
    node.position.x += 40
    await flushPromises()
    expect(layoutStatus().text()).toBe('有未保存的修改')
    expect(saveButton().attributes('disabled')).toBeUndefined()
    await wrapper.setProps({ nodes: [{ ...record(1), name: '状态已刷新' }, record(2)] })
    await flushPromises()
    expect(layoutStatus().text()).toBe('有未保存的修改')
    flowNodes(flow)[0].position.x = original.x
    await flushPromises()
    expect(layoutStatus().text()).toBe('已保存')
    expect(saveButton().attributes('disabled')).toBeDefined()
    const fitBounds = vi.fn().mockResolvedValue(true)
    flow.vm.$emit('init', { fitBounds })
    await clickText('适应画布')
    expect(layoutStatus().text()).toBe('已保存')
    await wrapper.setProps({ nodes: [record(1), record(2)] })
    await flushPromises()
    expect(layoutStatus().text()).toBe('已保存')
  })
  it('isolates layouts by scope even when both scopes contain the same service IDs', async () => {
    const flow = await create()
    flow.vm.$emit('update:modelValue', flowNodes(flow).map((node) => ({ ...node, position: { x: node.position.x, y: 350 } })))
    await flushPromises()
    await clickText('保存布局')
    await wrapper.setProps({ layoutKey: '[2,5,null,null]' })
    await flushPromises()
    expect(flowNodes(flow)[0].position.y).toBe(30)
    await clickText('保存布局')
    await wrapper.setProps({ layoutKey })
    await flushPromises()
    expect(flowNodes(flow)[0].position.y).toBe(350)
    expect(JSON.parse(localStorage.getItem('service-dependency-layout:v1:[2,5,null,null]')).positions[1].y).toBe(30)
  })
  it('merges filtered saves without erasing hidden coordinates and places new services outside saved cards', async () => {
    const flow = await create()
    await clickText('保存布局')
    await wrapper.setProps({ nodes: [record(1)] })
    await flushPromises()
    flow.vm.$emit('update:modelValue', flowNodes(flow).map((node) => ({ ...node, position: { x: 50, y: 400 } })))
    await flushPromises()
    await clickText('保存布局')
    expect(JSON.parse(localStorage.getItem(storageKey)).positions[2]).toEqual({ x: 500, y: 30 })
    await wrapper.setProps({ nodes: [record(1), record(2), record(3)] })
    await flushPromises()
    expect(flowNodes(flow).map((node) => node.position)).toEqual([
      { x: 50, y: 400 }, { x: 500, y: 30 }, { x: 930, y: 30 },
    ])
    expect(layoutStatus().text()).toBe('有未保存的修改')
    await wrapper.setProps({ nodes: [record(2)] })
    await flushPromises()
    expect(flowNodes(flow).map((node) => node.id)).toEqual(['2'])
    expect(flowNodes(flow)[0].position).toEqual({ x: 500, y: 30 })
  })
  it('reports write failures without claiming success or discarding dragged positions', async () => {
    const flow = await create()
    flowNodes(flow)[0].position.y = 90
    await flushPromises()
    const success = vi.spyOn(message, 'success').mockImplementation(() => {})
    const write = vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => { throw new Error('存储空间不足') })
    await clickText('保存布局')
    expect(wrapper.text()).toContain('保存布局失败：存储空间不足')
    expect(success).not.toHaveBeenCalled()
    expect(flowNodes(flow)[0].position).toEqual({ x: 0, y: 90 })
    expect(layoutStatus().text()).toBe('有未保存的修改')
    expect(saveButton().attributes('disabled')).toBeUndefined()
    expect(localStorage.getItem(storageKey)).toBeNull()
    write.mockRestore()
    await clickText('保存布局')
    expect(wrapper.text()).not.toContain('保存布局失败')
    expect(success).toHaveBeenCalledOnce()
    expect(layoutStatus().text()).toBe('已保存')
  })
  it.each(['{', '{"version":1,"positions":{"1":{"x":"bad","y":0}}}', '{"version":2,"positions":{}}', '{"version":1,"positions":{},"labelOffsets":{"1":{"x":"bad","y":0}}}', '{"version":1,"positions":{},"labelOffsets":[]}'])('reports invalid saved data and allows saving a valid replacement: %s', async (raw) => {
    localStorage.setItem(storageKey, raw)
    const flow = await create()
    expect(wrapper.text()).toContain('读取布局失败')
    expect(flowNodes(flow)[0].position).toEqual({ x: 0, y: 30 })
    await clickText('保存布局')
    expect(wrapper.text()).not.toContain('读取布局失败')
    expect(JSON.parse(localStorage.getItem(storageKey)).version).toBe(1)
  })
  it('reports unavailable browser storage and disables save without a scope', async () => {
    vi.spyOn(Storage.prototype, 'getItem').mockImplementation(() => { throw new Error('浏览器禁止存储') })
    await create()
    expect(wrapper.text()).toContain('读取布局失败：浏览器禁止存储')
    await wrapper.setProps({ layoutKey: '' })
    await flushPromises()
    const save = wrapper.findAll('button').find((button) => button.text().includes('保存布局'))
    expect(save.attributes('disabled')).toBeDefined()
  })
  it('preserves dragged coordinates on snapshot refresh and relayouts only when requested or nodes change', async () => {
    const flow = await create()
    const dragged = flowNodes(flow).map((node) => ({ ...node, position: { x: node.position.x + 123, y: 240 } }))
    flow.vm.$emit('update:modelValue', dragged)
    await flushPromises()
    await wrapper.setProps({ nodes: [record(1), { ...record(2), name: '状态已刷新' }] })
    await flushPromises()
    expect(layoutDependencyNodes).toHaveBeenCalledTimes(1)
    expect(flowNodes(flow)[0].position).toEqual({ x: 123, y: 240 })
    expect(flowNodes(flow)[1].data.record.name).toBe('状态已刷新')
    await clickText('重新布局')
    expect(layoutDependencyNodes).toHaveBeenCalledTimes(2)
    expect(flowNodes(flow)[0].position).toEqual({ x: 0, y: 30 })
    await wrapper.setProps({ nodes: [record(1)] })
    await flushPromises()
    expect(layoutDependencyNodes).toHaveBeenCalledTimes(3)
  })
  it('fits complete graph bounds, forwards selection and exports current coordinates', async () => {
    const flow = await create()
    const fitBounds = vi.fn().mockResolvedValue(true)
    flow.vm.$emit('init', { fitBounds })
    await clickText('适应画布')
    expect(fitBounds).toHaveBeenCalledWith({ x: 0, y: 0, width: 750, height: 144 }, { padding: .2, duration: 0 })
    flow.vm.$emit('node-click', { node: flowNodes(flow)[0] })
    expect(wrapper.emitted('select-node')[0][0].id).toBe(1)
    const createObjectURL = vi.fn(() => 'blob:architecture')
    const revokeObjectURL = vi.fn()
    vi.stubGlobal('URL', { createObjectURL, revokeObjectURL })
    vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => {})
    await clickText('导出SVG')
    expect(dependencySvg).toHaveBeenCalledWith(flowNodes(flow), [], '服务依赖架构', {})
    expect(createObjectURL).toHaveBeenCalledOnce()
    await new Promise((resolve) => setTimeout(resolve, 1050))
    expect(revokeObjectURL).toHaveBeenCalledWith('blob:architecture')
  })
  it('surfaces layout failures instead of leaving a stale or success-shaped graph', async () => {
    layoutDependencyNodes.mockRejectedValueOnce(new Error('布局引擎不可用'))
    await create()
    expect(wrapper.text()).toContain('自动布局失败：布局引擎不可用')
    expect(wrapper.findComponent(VueFlow).exists()).toBe(false)
    await clickText('重新布局')
    expect(wrapper.findComponent(VueFlow).exists()).toBe(true)
  })
  it('exports PNG from the current dragged graph rather than the original or visible viewport', async () => {
    const flow = await create()
    flowNodes(flow)[0].position.x = 321
    await flushPromises()
    const createObjectURL = vi.fn(() => 'blob:png')
    vi.stubGlobal('URL', { createObjectURL, revokeObjectURL: vi.fn() })
    let filename
    vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(function () { filename = this.download })
    await clickText('导出PNG')
    expect(dependencySvg).toHaveBeenLastCalledWith(flowNodes(flow), [], '服务依赖架构', {})
    expect(dependencyPng).toHaveBeenCalledWith(expect.stringContaining('x="321"'))
    expect(createObjectURL.mock.calls[0][0].type).toBe('image/png')
    expect(filename).toBe('service-architecture.png')
    await new Promise((resolve) => setTimeout(resolve, 1050))
  })
  it('shows copy success only after the PNG clipboard write completes and blocks duplicate image actions', async () => {
    await create()
    let finishCopy
    copyDependencyPng.mockReturnValueOnce(new Promise((resolve) => { finishCopy = resolve }))
    const success = vi.spyOn(message, 'success').mockImplementation(() => {})
    await clickText('复制图片')
    expect(copyDependencyPng).toHaveBeenCalledWith(expect.stringContaining('<svg'))
    expect(success).not.toHaveBeenCalled()
    for (const text of ['导出SVG', '导出PNG', '复制图片']) {
      const button = wrapper.findAll('button').find((item) => item.text().replace(/\s+/g, '') === text)
      expect(button.attributes('disabled')).toBeDefined()
    }
    finishCopy()
    await flushPromises()
    expect(success).toHaveBeenCalledWith('图片已复制，可粘贴到微信或 Word')
  })
  it('surfaces PNG or clipboard errors and re-enables actions without false success', async () => {
    await create()
    const error = vi.spyOn(message, 'error').mockImplementation(() => {})
    const success = vi.spyOn(message, 'success').mockImplementation(() => {})
    dependencyPng.mockRejectedValueOnce(new Error('生成失败'))
    await clickText('导出PNG')
    expect(error).toHaveBeenCalledWith('导出 PNG 失败：生成失败')
    copyDependencyPng.mockRejectedValueOnce(new Error('需要 HTTPS'))
    await clickText('复制图片')
    expect(error).toHaveBeenCalledWith('复制图片失败：需要 HTTPS')
    expect(success).not.toHaveBeenCalled()
    expect(wrapper.findAll('button').find((item) => item.text() === '复制图片').attributes('disabled')).toBeUndefined()
  })
  it('previews the current PNG on HTTP for native right-click copying without claiming clipboard success', async () => {
    vi.stubGlobal('isSecureContext', false)
    const createObjectURL = vi.fn(() => 'blob:copy-preview')
    const revokeObjectURL = vi.fn()
    vi.stubGlobal('URL', { createObjectURL, revokeObjectURL })
    const success = vi.spyOn(message, 'success').mockImplementation(() => {})
    const flow = await create()
    flowNodes(flow)[0].position.x = 456
    await clickText('复制图片')
    expect(dependencyPng).toHaveBeenCalledWith(expect.stringContaining('x="456"'))
    expect(createObjectURL).toHaveBeenCalledWith(await dependencyPng.mock.results[0].value)
    expect(copyDependencyPng).not.toHaveBeenCalled()
    expect(success).not.toHaveBeenCalled()
    const modal = wrapper.findComponent(Modal)
    expect(modal.props('open')).toBe(true)
    const image = document.querySelector('.copy-preview img')
    expect(image.getAttribute('src')).toBe('blob:copy-preview')
    expect(image.closest('.ant-modal').textContent).toContain('不要选择「复制图片地址」')
    const contextmenu = new MouseEvent('contextmenu', { bubbles: true, cancelable: true })
    image.dispatchEvent(contextmenu)
    expect(contextmenu.defaultPrevented).toBe(false)
    modal.vm.$emit('cancel')
    await flushPromises()
    expect(modal.props('open')).toBe(false)
    expect(revokeObjectURL).toHaveBeenCalledWith('blob:copy-preview')
  })
  it('releases HTTP preview URLs when replacing the image and unmounting', async () => {
    vi.stubGlobal('isSecureContext', false)
    const revokeObjectURL = vi.fn()
    vi.stubGlobal('URL', { createObjectURL: vi.fn().mockReturnValueOnce('blob:first').mockReturnValueOnce('blob:second'), revokeObjectURL })
    await create()
    await clickText('复制图片')
    await clickText('复制图片')
    expect(revokeObjectURL).toHaveBeenCalledWith('blob:first')
    expect(document.querySelector('.copy-preview img').getAttribute('src')).toBe('blob:second')
    wrapper.unmount()
    wrapper = null
    expect(revokeObjectURL).toHaveBeenCalledWith('blob:second')
  })
  it('reports HTTP PNG conversion and preview loading failures explicitly', async () => {
    vi.stubGlobal('isSecureContext', false)
    const revokeObjectURL = vi.fn()
    vi.stubGlobal('URL', { createObjectURL: vi.fn(() => 'blob:broken'), revokeObjectURL })
    const error = vi.spyOn(message, 'error').mockImplementation(() => {})
    await create()
    dependencyPng.mockRejectedValueOnce(new Error('生成失败'))
    await clickText('复制图片')
    expect(error).toHaveBeenCalledWith('复制图片失败：生成失败')
    expect(wrapper.findComponent(Modal).props('open')).toBe(false)
    await clickText('复制图片')
    document.querySelector('.copy-preview img').dispatchEvent(new Event('error'))
    await flushPromises()
    expect(error).toHaveBeenCalledWith('图片预览加载失败，请重试或使用导出 PNG')
    expect(wrapper.findComponent(Modal).props('open')).toBe(false)
    expect(revokeObjectURL).toHaveBeenCalledWith('blob:broken')
  })
  it('does not open a late HTTP preview after unmounting', async () => {
    vi.stubGlobal('isSecureContext', false)
    const createObjectURL = vi.fn()
    vi.stubGlobal('URL', { createObjectURL, revokeObjectURL: vi.fn() })
    let finish
    dependencyPng.mockReturnValueOnce(new Promise((resolve) => { finish = resolve }))
    await create()
    await clickText('复制图片')
    wrapper.unmount()
    wrapper = null
    finish(new Blob(['png'], { type: 'image/png' }))
    await flushPromises()
    expect(createObjectURL).not.toHaveBeenCalled()
  })
  it('does not await interruptible viewport animations when init and node initialization both request fit', async () => {
    const flow = await create()
    const fitBounds = vi.fn((bounds, options) => options.duration ? new Promise(() => {}) : Promise.resolve(true))
    flow.vm.$emit('init', { fitBounds })
    flow.vm.$emit('nodes-initialized')
    await clickText('重新布局')
    expect(fitBounds).toHaveBeenCalled()
    expect(fitBounds.mock.calls.every(([, options]) => options.duration === 0)).toBe(true)
    expect(wrapper.findAll('button').find((button) => button.text().includes('导出')).attributes('disabled')).toBeUndefined()
    expect(wrapper.findComponent(Spin).props('spinning')).toBe(false)
  })
  it('enters and exits fullscreen without remounting or relayout, and refits the resized canvas', async () => {
    const api = fullscreenApi()
    const flow = await create()
    const fitBounds = vi.fn().mockResolvedValue(true)
    flow.vm.$emit('init', { fitBounds })
    await clickText('全屏显示')
    expect(api.enter).toHaveBeenCalledOnce()
    expect(document.querySelector('.dependency-graph--fullscreen')).not.toBeNull()
    const graph = document.querySelector('.dependency-graph--fullscreen')
    const canvasParent = graph.querySelector('.graph-canvas').parentElement.parentElement
    expect(canvasParent.classList.contains('ant-spin-nested-loading')).toBe(true)
    expect(canvasParent.classList.contains('graph-spin')).toBe(true)
    expect(wrapper.findComponent(Spin).props('wrapperClassName')).toBe('graph-spin')
    const exitButton = document.querySelector('button[aria-pressed="true"]')
    expect(exitButton.textContent).toContain('退出全屏')
    expect(wrapper.findComponent(VueFlow).vm).toBe(flow.vm)
    await new Promise((resolve) => setTimeout(resolve, 75))
    expect(fitBounds).toHaveBeenCalled()
    fitBounds.mockClear()
    exitButton.click()
    await flushPromises()
    await new Promise((resolve) => setTimeout(resolve, 75))
    expect(api.exit).toHaveBeenCalledOnce()
    expect(document.querySelector('.dependency-graph--fullscreen')).toBeNull()
    expect(wrapper.text()).toContain('全屏显示')
    expect(fitBounds).toHaveBeenCalled()
    expect(layoutDependencyNodes).toHaveBeenCalledTimes(1)
  })
  it('synchronizes the button and viewport when the browser exits fullscreen through Esc', async () => {
    const api = fullscreenApi()
    await create()
    await clickText('全屏显示')
    api.change(null)
    await flushPromises()
    expect(document.querySelector('.dependency-graph--fullscreen')).toBeNull()
    expect(wrapper.find('button[aria-pressed="false"]').exists()).toBe(true)
    expect(api.exit).not.toHaveBeenCalled()
  })
  it('shows fullscreen API rejection instead of claiming fullscreen succeeded', async () => {
    const api = fullscreenApi()
    api.enter.mockRejectedValueOnce(new Error('用户拒绝'))
    const error = vi.spyOn(message, 'error').mockImplementation(() => {})
    await create()
    await clickText('全屏显示')
    expect(error).toHaveBeenCalledWith('进入全屏失败：用户拒绝')
    expect(document.querySelector('.dependency-graph--fullscreen')).toBeNull()
    expect(wrapper.text()).toContain('全屏显示')
  })
  it('exits its own fullscreen on unmount and removes its event listener', async () => {
    const api = fullscreenApi()
    const remove = vi.spyOn(document, 'removeEventListener')
    await create()
    await clickText('全屏显示')
    wrapper.unmount()
    wrapper = null
    await flushPromises()
    expect(api.exit).toHaveBeenCalledOnce()
    expect(remove).toHaveBeenCalledWith('fullscreenchange', expect.any(Function))
    expect(document.querySelector('.dependency-graph--fullscreen')).toBeNull()
  })
  it('does not close fullscreen owned by another page', async () => {
    const api = fullscreenApi()
    const error = vi.spyOn(message, 'error').mockImplementation(() => {})
    await create()
    api.change(document.documentElement)
    await clickText('全屏显示')
    expect(api.enter).not.toHaveBeenCalled()
    expect(error).toHaveBeenCalledWith('进入全屏失败：请先退出当前全屏，再打开依赖图全屏')
    wrapper.unmount()
    wrapper = null
    expect(api.exit).not.toHaveBeenCalled()
  })
  it('reports an unsupported Fullscreen API without changing graph mode', async () => {
    fullscreenApi()
    Object.defineProperty(document.documentElement, 'requestFullscreen', { configurable: true, value: undefined })
    const error = vi.spyOn(message, 'error').mockImplementation(() => {})
    await create()
    await clickText('全屏显示')
    expect(error).toHaveBeenCalledWith('进入全屏失败：当前浏览器不支持全屏显示')
    expect(document.querySelector('.dependency-graph--fullscreen')).toBeNull()
  })
  it('exits a fullscreen request that completes after the graph has unmounted', async () => {
    const api = fullscreenApi()
    let finish
    api.enter.mockImplementationOnce(() => new Promise((resolve) => {
      finish = () => { api.change(document.documentElement); resolve() }
    }))
    await create()
    await clickText('全屏显示')
    wrapper.unmount()
    wrapper = null
    finish()
    await flushPromises()
    expect(api.exit).toHaveBeenCalledOnce()
    expect(document.fullscreenElement).toBeNull()
  })
})
