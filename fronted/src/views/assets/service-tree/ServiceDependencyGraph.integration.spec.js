import { flushPromises, mount } from '@vue/test-utils'
import Antd, { message, Spin } from 'ant-design-vue'
import { unref } from 'vue'
import { VueFlow } from '@vue-flow/core'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

vi.mock('./dependencyGraph', async (original) => {
  const actual = await original()
  return { ...actual, layoutDependencyNodes: vi.fn() }
})
import { layoutDependencyNodes, positionedDependencyEdges } from './dependencyGraph'
vi.mock('./dependencyGraphExport', () => ({
  dependencyPng: vi.fn(async () => new Blob(['png'], { type: 'image/png' })),
  copyDependencyPng: vi.fn(async () => {}),
  downloadDependencyImage: vi.fn(),
}))
import { copyDependencyPng, dependencyPng } from './dependencyGraphExport'
import Graph from './ServiceDependencyGraph.vue'

const record = (id) => ({
  id, name: `服务 ${id}`, code: String(id), enabled: true, business_system_name: '业务',
  runtime_summary: { total: 0, running: 0, stopped: 0, error: 0, unknown: 0 },
})
const relation = (id, source, target) => ({
  id, source_service_id: source, target_service_id: target, strength: 'strong', function_scope: `功能 ${id}`,
})
const positions = (ids) => ids.map((id, i) => ({ id: String(id), position: { x: 30 + i * 500, y: 30 } }))
let wrapper
let warn
beforeEach(() => {
  vi.clearAllMocks()
  localStorage.clear()
  vi.stubGlobal('isSecureContext', true)
  layoutDependencyNodes.mockImplementation(async (nodes) => positions(nodes.map((node) => node.id)))
  warn = vi.spyOn(console, 'warn').mockImplementation(() => {})
  vi.stubGlobal('DOMMatrixReadOnly', class { m22 = 1 })
  vi.stubGlobal('ResizeObserver', class {
    constructor(callback) { this.callback = callback }
    observe(target) { this.callback([{ target, contentRect: target.getBoundingClientRect() }]) }
    unobserve() {}
    disconnect() {}
  })
  for (const property of ['offsetWidth', 'clientWidth']) {
    vi.spyOn(HTMLElement.prototype, property, 'get').mockImplementation(function () {
      return this.classList.contains('vue-flow') ? 1000 : 250
    })
  }
  for (const property of ['offsetHeight', 'clientHeight']) {
    vi.spyOn(HTMLElement.prototype, property, 'get').mockImplementation(function () {
      return this.classList.contains('vue-flow') ? 540 : 114
    })
  }
  vi.spyOn(HTMLElement.prototype, 'getBoundingClientRect').mockImplementation(function () {
    return { x: 0, y: 0, top: 0, left: 0, width: this.offsetWidth, height: this.offsetHeight, right: this.offsetWidth, bottom: this.offsetHeight, toJSON: () => ({}) }
  })
})
afterEach(() => {
  wrapper?.unmount()
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
  localStorage.clear()
})
async function create(props = {}) {
  wrapper = mount(Graph, { props: { nodes: [record(24), record(23)], edges: [relation(1, 24, 23)], ...props }, global: { plugins: [Antd], stubs: { FontAwesomeIcon: true } } })
  await flushPromises()
  return wrapper.findComponent(VueFlow)
}
const missingEndpointWarnings = () => warn.mock.calls.filter((args) => args.some((arg) => String(arg).includes('source or target is missing')))
const storedNodes = (flow) => unref(flow.vm.$.exposed.getNodes)
const storedEdges = (flow) => unref(flow.vm.$.exposed.getEdges)
const status = () => wrapper.get('.graph-controls [role="status"]').text()
const labelFor = (id) => wrapper.findAll('.edge-label').find((label) => label.text().startsWith(`功能 ${id} `))
async function pointer(target, type, x, y, pointerId = 1) {
  const event = new MouseEvent(type, { bubbles: true, cancelable: true, clientX: x, clientY: y, button: 0 })
  Object.defineProperty(event, 'pointerId', { value: pointerId })
  target.dispatchEvent(event)
  await flushPromises()
}
async function startDrag(label) {
  label.element.setPointerCapture = vi.fn()
  label.element.hasPointerCapture = vi.fn(() => true)
  label.element.releasePointerCapture = vi.fn()
  await pointer(label.element, 'pointerdown', 100, 100)
}
async function drag(label, dx, dy) {
  await startDrag(label)
  await pointer(window, 'pointermove', 100 + dx, 100 + dy)
  await pointer(window, 'pointerup', 100 + dx, 100 + dy)
}
async function clickControl(text) {
  await wrapper.findAll('.graph-controls button').find((button) => button.text().replace(/\s+/g, '') === text).trigger('click')
  await flushPromises()
}

describe('dependency graph with the real Vue Flow engine', () => {
  it('restores nodes and label routes in place without saving or changing dependencies', async () => {
    const layoutKey = '[1,3,null,null]'
    const flow = await create({ layoutKey })
    await flow.vm.$.exposed.setViewport({ x: 0, y: 0, zoom: 1 })
    await flushPromises()
    expect(wrapper.findComponent(Spin).props('spinning')).toBe(false)
    const initialLabel = labelFor(1).element.style.transform
    await drag(labelFor(1), 60, 40)
    expect(labelFor(1).element.style.transform).not.toBe(initialLabel)
    await clickControl('还原布局')
    expect(labelFor(1).element.style.transform).toBe(initialLabel)
    expect(status()).toBe('尚未保存')

    await flow.vm.$.exposed.setViewport({ x: 0, y: 0, zoom: 1 })
    await drag(labelFor(1), 80, 50)
    await clickControl('保存布局')
    const savedLabel = labelFor(1).element.style.transform
    const savedNodes = storedNodes(flow).map((node) => ({ ...node.position }))
    const saved = localStorage.getItem(`service-dependency-layout:v1:${layoutKey}`)
    await drag(labelFor(1), 100, 60)
    flow.vm.$emit('update:modelValue', flow.props('modelValue').map((item) => 'source' in item
      ? item : { ...item, position: { x: item.position.x, y: item.position.y + 150 } }))
    await flushPromises()
    expect(status()).toBe('有未保存的修改')
    await clickControl('还原布局')

    expect(storedNodes(flow).map((node) => node.position)).toEqual(savedNodes)
    expect(labelFor(1).element.style.transform).toBe(savedLabel)
    expect(status()).toBe('已保存')
    expect(storedEdges(flow).map((edge) => [edge.source, edge.target])).toEqual([['24', '23']])
    expect(localStorage.getItem(`service-dependency-layout:v1:${layoutKey}`)).toBe(saved)
    expect(wrapper.emitted('select-edge')).toBeUndefined()
    expect(missingEndpointWarnings()).toEqual([])
  })

  it('registers both endpoints, renders the directed edge and finishes initialization without a loading deadlock', async () => {
    const flow = await create()
    expect(storedNodes(flow).map((node) => node.id)).toEqual(['24', '23'])
    expect(storedEdges(flow).map((edge) => [edge.id, edge.source, edge.target])).toEqual([['1', '24', '23']])
    expect(wrapper.find('.vue-flow__edge-path').exists()).toBe(true)
    expect(wrapper.get('.graph-canvas').element.parentElement.parentElement.classList.contains('graph-spin')).toBe(true)
    expect(wrapper.find('.edge-label').text()).toContain('功能 1')
    expect(missingEndpointWarnings()).toEqual([])
    expect(wrapper.findComponent(Spin).props('spinning')).toBe(false)
    expect(wrapper.findAll('button').find((button) => button.text().includes('导出')).attributes('disabled')).toBeUndefined()
  })
  it('keeps new edges out of the store until their asynchronously positioned endpoints are ready', async () => {
    const flow = await create()
    let finishLayout
    layoutDependencyNodes.mockReturnValueOnce(new Promise((resolve) => { finishLayout = resolve }))
    await wrapper.setProps({ nodes: [record(3), record(4)], edges: [relation(2, 3, 4)] })
    await flushPromises()
    expect(storedEdges(flow).map((edge) => edge.id)).toEqual(['1'])
    expect(missingEndpointWarnings()).toEqual([])
    finishLayout(positions([3, 4]))
    await flushPromises()
    expect(storedNodes(flow).map((node) => node.id)).toEqual(['3', '4'])
    expect(storedEdges(flow).map((edge) => [edge.id, edge.source, edge.target])).toEqual([['2', '3', '4']])
    expect(missingEndpointWarnings()).toEqual([])
    expect(wrapper.find('.vue-flow__edge-path').exists()).toBe(true)
    expect(wrapper.findComponent(Spin).props('spinning')).toBe(false)
  })
  it('renders collision-free crossing labels, keeps them clickable and updates positions after a drag', async () => {
    const flow = await create()
    layoutDependencyNodes.mockImplementation(async () => [
      { id: '1', position: { x: 0, y: 0 } }, { id: '2', position: { x: 500, y: 0 } },
      { id: '3', position: { x: 0, y: 180 } }, { id: '4', position: { x: 500, y: 180 } },
    ])
    const relations = [relation(1, 1, 4), relation(2, 3, 2)]
    await wrapper.setProps({ nodes: [1, 2, 3, 4].map(record), edges: relations })
    await flushPromises()
    const verify = () => {
      const placements = positionedDependencyEdges(storedNodes(flow), relations)
      const labels = relations.map((item) => wrapper.findAll('.edge-label').find((label) => label.text().includes(item.function_scope)))
      expect(labels).toHaveLength(2)
      for (let i = 0; i < labels.length; i++) {
        expect(labels[i].element.style.transform).toBe(`translate(${placements[i].label.x}px, ${placements[i].label.y}px)`)
        expect(labels[i].element.style.width).toBe(`${placements[i].label.width}px`)
        expect(labels[i].element.style.height).toBe(`${placements[i].label.height}px`)
      }
      expect(Math.abs(placements[0].label.y - placements[1].label.y)).toBeGreaterThanOrEqual(30)
      return labels
    }
    const labels = verify()
    expect(wrapper.get('.graph-controls [role="status"]').text()).toBe('尚未保存')
    expect(wrapper.find('path[stroke="#b9c6d8"]').exists()).toBe(true)
    await labels[1].trigger('click')
    expect(wrapper.emitted('select-edge').at(-1)[0].id).toBe(2)
    const before = labels[0].element.style.transform
    flow.vm.$emit('update:modelValue', flow.props('modelValue').map((item) => item.id === '4' && !('source' in item)
      ? { ...item, position: { x: 500, y: 100 } } : item))
    await flushPromises()
    const moved = verify()
    expect(moved[0].element.style.transform).not.toBe(before)
    expect(wrapper.get('.graph-controls [role="status"]').text()).toBe('有未保存的修改')
  })
  it.each([0.5, 2])('drags labels and their lines in flow coordinates at zoom %s without moving endpoints or opening details', async (zoom) => {
    const flow = await create()
    await flow.vm.$.exposed.setViewport({ x: 80, y: 40, zoom })
    expect(flow.vm.$.exposed.getViewport().zoom).toBe(zoom)
    const label = labelFor(1)
    const before = positionedDependencyEdges(storedNodes(flow), [relation(1, 24, 23)])[0]
    const originalNodes = storedNodes(flow).map((node) => ({ ...node.position }))
    const originalPath = wrapper.get('.vue-flow__edge-path').attributes('d')
    await drag(label, 60, 40)
    const x = before.label.x + 60 / zoom, y = before.label.y + 40 / zoom
    expect(label.element.style.transform).toBe(`translate(${x}px, ${y}px)`)
    const path = wrapper.get('.vue-flow__edge-path').attributes('d')
    expect(path).not.toBe(originalPath)
    expect(path).toContain(`${x + 62.5},${y + 12}`)
    expect(path.startsWith('M280,87 ')).toBe(true)
    expect(path.endsWith('530,87')).toBe(true)
    expect(storedNodes(flow).map((node) => ({ ...node.position }))).toEqual(originalNodes)
    expect(status()).toBe('有未保存的修改')
    expect(label.element.releasePointerCapture).toHaveBeenCalledWith(1)
    label.element.dispatchEvent(new MouseEvent('click', { bubbles: true, detail: 1 }))
    await flushPromises()
    expect(wrapper.emitted('select-edge')).toBeUndefined()
    label.element.dispatchEvent(new MouseEvent('click', { bubbles: true, detail: 0 }))
    await flushPromises()
    expect(wrapper.emitted('select-edge').at(-1)[0].id).toBe(1)
  })
  it('saves label routes, restores them per scope, preserves them on refresh and uses them in all image outputs', async () => {
    const layoutKey = '[1,3,null,null]'
    const key = `service-dependency-layout:v1:${layoutKey}`
    let flow = await create({ layoutKey })
    await flow.vm.$.exposed.setViewport({ x: 0, y: 0, zoom: 1 })
    await drag(labelFor(1), 120, -160)
    const moved = labelFor(1).element.style.transform
    const path = wrapper.get('.vue-flow__edge-path').attributes('d')
    await clickControl('保存布局')
    const offsets = JSON.parse(localStorage.getItem(key)).labelOffsets
    expect(offsets).toEqual({ 1: { x: 120, y: -160 } })
    expect(status()).toBe('已保存')
    await wrapper.setProps({ nodes: [record(24), { ...record(23), name: '状态刷新' }] })
    await flushPromises()
    expect(labelFor(1).element.style.transform).toBe(moved)
    expect(status()).toBe('已保存')
    await clickControl('导出SVG')
    await clickControl('导出PNG')
    await clickControl('复制图片')
    expect(dependencyPng).toHaveBeenLastCalledWith(expect.stringContaining(`d="${path}"`))
    expect(copyDependencyPng).toHaveBeenLastCalledWith(expect.stringContaining(`d="${path}"`))
    await wrapper.setProps({ layoutKey: '[2,3,null,null]' })
    await flushPromises()
    expect(labelFor(1).element.style.transform).not.toBe(moved)
    expect(status()).toBe('尚未保存')
    wrapper.unmount()
    flow = await create({ layoutKey })
    expect(labelFor(1).element.style.transform).toBe(moved)
    expect(wrapper.get('.vue-flow__edge-path').attributes('d')).toBe(path)
    expect(status()).toBe('已保存')
    await startDrag(labelFor(1))
    await pointer(window, 'pointermove', 200, 200)
    await pointer(window, 'pointercancel', 200, 200)
    expect(labelFor(1).element.style.transform).toBe(moved)
    expect(status()).toBe('已保存')
    await clickControl('重新布局')
    expect(labelFor(1).element.style.transform).not.toBe(moved)
    expect(status()).toBe('有未保存的修改')
    expect(JSON.parse(localStorage.getItem(key)).labelOffsets).toEqual(offsets)
    await clickControl('保存布局')
    expect(JSON.parse(localStorage.getItem(key)).labelOffsets).toEqual({})
    expect(status()).toBe('已保存')
  })
  it('keeps hidden relation routes when saving a filtered graph and ignores removed relations', async () => {
    const layoutKey = '[1,3,null,null]', key = `service-dependency-layout:v1:${layoutKey}`
    const relations = [relation(1, 24, 23), relation(2, 23, 24)]
    const flow = await create({ layoutKey, edges: relations })
    await flow.vm.$.exposed.setViewport({ x: 0, y: 0, zoom: 1 })
    await drag(labelFor(1), 40, 60)
    await clickControl('保存布局')
    const first = JSON.parse(localStorage.getItem(key)).labelOffsets[1]
    await wrapper.setProps({ edges: [relations[1]] })
    await flushPromises()
    await drag(labelFor(2), -40, -60)
    await clickControl('保存布局')
    const saved = JSON.parse(localStorage.getItem(key)).labelOffsets
    expect(saved[1]).toEqual(first)
    expect(saved[2]).toBeDefined()
    await wrapper.setProps({ edges: relations })
    await flushPromises()
    const expected = positionedDependencyEdges(storedNodes(flow), relations, saved)
    expect(labelFor(1).element.style.transform).toBe(`translate(${expected[0].label.x}px, ${expected[0].label.y}px)`)
    await wrapper.setProps({ edges: [] })
    await flushPromises()
    expect(wrapper.findAll('.edge-label')).toHaveLength(0)
    expect(status()).toBe('已保存')
  })
  it('distinguishes clicks from drags, ignores other pointers and restores cancelled or returned drags', async () => {
    const flow = await create({ layoutKey: '[1,3,null,null]' })
    await flow.vm.$.exposed.setViewport({ x: 0, y: 0, zoom: 1 })
    await clickControl('保存布局')
    const label = labelFor(1), original = label.element.style.transform
    await startDrag(label)
    await pointer(window, 'pointermove', 103, 100)
    await pointer(window, 'pointerup', 103, 100)
    expect(status()).toBe('已保存')
    label.element.dispatchEvent(new MouseEvent('click', { bubbles: true, detail: 1 }))
    await flushPromises()
    expect(wrapper.emitted('select-edge')).toHaveLength(1)
    await startDrag(label)
    await pointer(window, 'pointermove', 104, 100)
    expect(status()).toBe('有未保存的修改')
    await pointer(window, 'pointercancel', 104, 100)
    expect(status()).toBe('已保存')
    await startDrag(label)
    await pointer(window, 'pointermove', 200, 200, 2)
    await pointer(window, 'pointercancel', 200, 200, 2)
    expect(label.element.style.transform).toBe(original)
    await pointer(window, 'pointermove', 200, 200)
    expect(status()).toBe('有未保存的修改')
    await pointer(window, 'pointermove', 100, 100)
    expect(status()).toBe('已保存')
    expect(label.element.style.transform).toBe(original)
    await pointer(window, 'pointermove', 200, 200)
    await pointer(window, 'pointercancel', 200, 200)
    expect(label.element.style.transform).toBe(original)
    expect(status()).toBe('已保存')
    await startDrag(label)
    await pointer(window, 'pointermove', 200, 200)
    window.dispatchEvent(new Event('blur'))
    await flushPromises()
    expect(status()).toBe('已保存')
    await drag(label, 60, 40)
    expect(status()).toBe('有未保存的修改')
    await drag(label, -60, -40)
    expect(label.element.style.transform).toBe(original)
    expect(status()).toBe('已保存')
    await startDrag(label)
    await pointer(window, 'pointermove', 200, 200)
    await wrapper.setProps({ layoutKey: '[2,3,null,null]' })
    await pointer(window, 'pointermove', 300, 300)
    expect(labelFor(1).element.style.transform).toBe(original)
    expect(status()).toBe('尚未保存')
  })
  it('keeps unsaved label adjustments on storage failure and removes active drag listeners on unmount', async () => {
    const flow = await create({ layoutKey: '[1,3,null,null]' })
    await flow.vm.$.exposed.setViewport({ x: 0, y: 0, zoom: 1 })
    await drag(labelFor(1), 40, 60)
    const moved = labelFor(1).element.style.transform
    const write = vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => { throw new Error('存储被禁用') })
    const success = vi.spyOn(message, 'success').mockImplementation(() => {})
    await clickControl('保存布局')
    expect(wrapper.text()).toContain('保存布局失败：存储被禁用')
    expect(labelFor(1).element.style.transform).toBe(moved)
    expect(status()).toBe('有未保存的修改')
    expect(success).not.toHaveBeenCalled()
    write.mockRestore()
    await clickControl('保存布局')
    expect(status()).toBe('已保存')
    const remove = vi.spyOn(window, 'removeEventListener')
    await startDrag(labelFor(1))
    wrapper.unmount()
    wrapper = null
    for (const type of ['pointermove', 'pointerup', 'pointercancel', 'blur']) {
      expect(remove).toHaveBeenCalledWith(type, expect.any(Function))
    }
  })
})
