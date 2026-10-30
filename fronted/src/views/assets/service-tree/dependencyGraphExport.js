export async function dependencyPng(svg) {
  const parsed = new DOMParser().parseFromString(svg, 'image/svg+xml')
  const root = parsed.documentElement
  const width = Math.ceil(Number(root.getAttribute('width')) * 2)
  const height = Math.ceil(Number(root.getAttribute('height')) * 2)
  if (parsed.querySelector('parsererror') || root.localName !== 'svg'
    || !Number.isFinite(width) || !Number.isFinite(height) || width <= 0 || height <= 0) {
    throw new Error('图像尺寸或 SVG 格式无效')
  }
  // Bound raster memory explicitly; large topologies can still be exported losslessly as SVG.
  if (width > 8192 || height > 8192 || width * height > 16777216) {
    throw new Error('图片尺寸过大，请缩小图范围或导出 SVG')
  }
  const canvas = document.createElement('canvas')
  canvas.width = width
  canvas.height = height
  let url
  try {
    const context = canvas.getContext('2d')
    if (!context) throw new Error('当前浏览器无法创建图片画布')
    const image = new Image()
    url = URL.createObjectURL(new Blob([svg], { type: 'image/svg+xml;charset=utf-8' }))
    await new Promise((resolve, reject) => {
      image.onload = resolve
      image.onerror = () => reject(new Error('架构图图像加载失败'))
      image.src = url
    })
    context.drawImage(image, 0, 0, width, height)
    return await new Promise((resolve, reject) => {
      canvas.toBlob((blob) => {
        if (blob?.type === 'image/png' && blob.size > 0) resolve(blob)
        else reject(new Error('生成 PNG 图片失败'))
      }, 'image/png')
    })
  } finally {
    if (url) URL.revokeObjectURL(url)
    canvas.width = 0
    canvas.height = 0
  }
}

async function writeClipboardPng(png) {
  try {
    // Call write during the click gesture; Safari requires deferred image data inside ClipboardItem.
    await navigator.clipboard.write([new ClipboardItem({ 'image/png': png })])
  } catch (error) {
    if (error.name === 'NotAllowedError') throw new Error('剪贴板写入被拒绝，请允许剪贴板权限后重试，或导出 PNG')
    throw error
  }
}

export async function copyDependencyPng(svg) {
  if (!window.isSecureContext) throw new Error('复制图片需要 HTTPS 或 localhost；当前地址无法直接复制，请使用导出 PNG')
  if (typeof navigator.clipboard?.write !== 'function' || typeof ClipboardItem !== 'function'
    || (typeof ClipboardItem.supports === 'function' && !ClipboardItem.supports('image/png'))) {
    throw new Error('当前浏览器不支持复制 PNG 图片，请使用导出 PNG')
  }
  const png = dependencyPng(svg)
  // Observe both promises so permission rejection or conversion failure cannot become an unhandled rejection.
  await Promise.all([png, writeClipboardPng(png)])
}

export function downloadDependencyImage(blob, extension) {
  const url = URL.createObjectURL(blob)
  try {
    const anchor = document.createElement('a')
    anchor.href = url
    anchor.download = `service-architecture.${extension}`
    anchor.click()
  } finally {
    setTimeout(() => URL.revokeObjectURL(url), 1000)
  }
}
