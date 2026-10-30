import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { copyDependencyPng, dependencyPng, downloadDependencyImage } from './dependencyGraphExport'

const svg = (width = 800, height = 350) => `<svg xmlns="http://www.w3.org/2000/svg" width="${width}" height="${height}"><text>服务依赖</text></svg>`
let drawImage
let toBlob
let imageFails
let write
let clipboardItems
let canvases
beforeEach(() => {
  imageFails = false
  canvases = []
  clipboardItems = []
  drawImage = vi.fn()
  vi.spyOn(HTMLCanvasElement.prototype, 'getContext').mockReturnValue({ drawImage })
  toBlob = vi.spyOn(HTMLCanvasElement.prototype, 'toBlob').mockImplementation(function (callback, type) {
    canvases.push({ width: this.width, height: this.height, type, canvas: this })
    callback(new Blob(['png-data'], { type: 'image/png' }))
  })
  vi.stubGlobal('URL', { createObjectURL: vi.fn(() => 'blob:dependency-image'), revokeObjectURL: vi.fn() })
  vi.stubGlobal('Image', class {
    set src(value) {
      this.source = value
      queueMicrotask(() => imageFails ? this.onerror?.() : this.onload?.())
    }
  })
  vi.stubGlobal('isSecureContext', true)
  vi.stubGlobal('ClipboardItem', class {
    static supports(type) { return type === 'image/png' }
    constructor(items) {
      this.items = items
      clipboardItems.push(this)
    }
  })
  write = vi.fn(async (items) => { await items[0].items['image/png'] })
  vi.stubGlobal('navigator', { clipboard: { write } })
})
afterEach(() => {
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
  vi.useRealTimers()
})

describe('dependency graph PNG and clipboard export', () => {
  it('rasterizes the full SVG at two-times resolution and releases its URL and canvas', async () => {
    const blob = await dependencyPng(svg(800.2, 350.2))
    expect(blob.type).toBe('image/png')
    expect(blob.size).toBeGreaterThan(0)
    expect(canvases[0]).toMatchObject({ width: 1601, height: 701, type: 'image/png' })
    expect(drawImage).toHaveBeenCalledWith(expect.anything(), 0, 0, 1601, 701)
    expect(URL.createObjectURL.mock.calls[0][0].type).toBe('image/svg+xml;charset=utf-8')
    expect(URL.revokeObjectURL).toHaveBeenCalledWith('blob:dependency-image')
    expect(canvases[0].canvas.width).toBe(0)
    expect(canvases[0].canvas.height).toBe(0)
  })
  it.each([
    ['broken SVG', '<svg'],
    ['zero width', svg(0)],
    ['non-finite width', svg('Infinity')],
    ['non-SVG root', '<image width="10" height="10"/>'],
    ['side limit', svg(4097, 100)],
    ['pixel limit', svg(2048, 2048.5)],
  ])('rejects invalid or oversized images before allocating raster resources: %s', async (_, source) => {
    await expect(dependencyPng(source)).rejects.toThrow()
    expect(drawImage).not.toHaveBeenCalled()
    expect(URL.createObjectURL).not.toHaveBeenCalled()
  })
  it('accepts the exact side and total-pixel thresholds', async () => {
    await dependencyPng(svg(4096, 100))
    await dependencyPng(svg(2048, 2048))
    expect(canvases[0]).toMatchObject({ width: 8192, height: 200 })
    expect(canvases[1]).toMatchObject({ width: 4096, height: 4096 })
  })
  it('reports an unavailable canvas context', async () => {
    HTMLCanvasElement.prototype.getContext.mockReturnValue(null)
    await expect(dependencyPng(svg())).rejects.toThrow('无法创建图片画布')
    expect(URL.createObjectURL).not.toHaveBeenCalled()
  })
  it('reports image loading failure and releases the temporary URL', async () => {
    imageFails = true
    await expect(dependencyPng(svg())).rejects.toThrow('图像加载失败')
    expect(URL.revokeObjectURL).toHaveBeenCalledOnce()
    expect(toBlob).not.toHaveBeenCalled()
  })
  it.each([null, new Blob([], { type: 'image/png' }), new Blob(['jpeg'], { type: 'image/jpeg' })])('rejects an empty or incorrect PNG encoding result', async (blob) => {
    toBlob.mockImplementation((callback) => callback(blob))
    await expect(dependencyPng(svg())).rejects.toThrow('生成 PNG 图片失败')
    expect(URL.revokeObjectURL).toHaveBeenCalledOnce()
  })
  it('reports a thrown encoding error and still releases the URL', async () => {
    toBlob.mockImplementation(() => { throw new Error('编码失败') })
    await expect(dependencyPng(svg())).rejects.toThrow('编码失败')
    expect(URL.revokeObjectURL).toHaveBeenCalledOnce()
  })
  it('writes a PNG ClipboardItem immediately during the gesture, before conversion finishes', async () => {
    const copying = copyDependencyPng(svg())
    expect(write).toHaveBeenCalledOnce()
    expect(drawImage).not.toHaveBeenCalled()
    expect(Object.keys(clipboardItems[0].items)).toEqual(['image/png'])
    expect(clipboardItems[0].items['image/png']).toBeInstanceOf(Promise)
    await copying
    expect((await clipboardItems[0].items['image/png']).type).toBe('image/png')
  })
  it('requires a secure context rather than copying text or claiming HTTP success', async () => {
    vi.stubGlobal('isSecureContext', false)
    await expect(copyDependencyPng(svg())).rejects.toThrow('需要 HTTPS 或 localhost')
    expect(write).not.toHaveBeenCalled()
    expect(URL.createObjectURL).not.toHaveBeenCalled()
  })
  it.each(['clipboard', 'ClipboardItem', 'PNG support'])('reports missing clipboard capabilities: %s', async (capability) => {
    if (capability === 'clipboard') vi.stubGlobal('navigator', {})
    if (capability === 'ClipboardItem') vi.stubGlobal('ClipboardItem', undefined)
    if (capability === 'PNG support') vi.spyOn(ClipboardItem, 'supports').mockReturnValue(false)
    await expect(copyDependencyPng(svg())).rejects.toThrow('不支持复制 PNG')
    expect(write).not.toHaveBeenCalled()
  })
  it('reports denied clipboard permission explicitly', async () => {
    write.mockRejectedValue(new DOMException('Denied', 'NotAllowedError'))
    await expect(copyDependencyPng(svg())).rejects.toThrow('剪贴板写入被拒绝')
  })
  it('observes concurrent clipboard and conversion failures without an unhandled rejection', async () => {
    imageFails = true
    write.mockRejectedValue(new DOMException('Denied', 'NotAllowedError'))
    await expect(copyDependencyPng(svg())).rejects.toThrow(/剪贴板写入被拒绝|图像加载失败/)
    await new Promise((resolve) => setTimeout(resolve, 0))
    expect(URL.revokeObjectURL).toHaveBeenCalledOnce()
  })
  it('rejects copying when PNG conversion fails even if the clipboard API resolves', async () => {
    write.mockResolvedValue(undefined)
    imageFails = true
    await expect(copyDependencyPng(svg())).rejects.toThrow('图像加载失败')
  })
  it('downloads the PNG with its proper filename and releases the download URL', () => {
    vi.useFakeTimers()
    let download
    vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(function () { download = this.download })
    downloadDependencyImage(new Blob(['png'], { type: 'image/png' }), 'png')
    expect(download).toBe('service-architecture.png')
    expect(URL.revokeObjectURL).not.toHaveBeenCalled()
    vi.advanceTimersByTime(1000)
    expect(URL.revokeObjectURL).toHaveBeenCalledOnce()
  })
})
