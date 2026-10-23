import { describe, expect, it } from 'vitest'
import mermaid from 'mermaid'
import { DOCUMENT_HOME, documentLoaders, renderDocument } from '../documentCenter'
import { renderDocumentDiagrams } from '../documentDiagrams'

describe('模块手册真实流程图', () => {
  it('首页与所有模块流程图均符合 Mermaid 语法', async () => {
    mermaid.initialize({ startOnLoad: false, securityLevel: 'strict' })
    const paths = Object.keys(documentLoaders).filter((path) => path === DOCUMENT_HOME || path.startsWith('docs/architecture/modules/'))
    let count = 0
    for (const path of paths) {
      const source = await documentLoaders[path]()
      const diagrams = [...source.matchAll(/```mermaid\n([\s\S]*?)```/g)]
      expect(diagrams.length, path).toBeGreaterThan(0)
      for (const [, diagram] of diagrams) {
        expect(await mermaid.parse(diagram), path).toBeTruthy()
        count++
      }
    }
    expect(paths).toHaveLength(12)
    expect(count).toBeGreaterThanOrEqual(26)
  }, 30000)

  it('真实 Mermaid 渲染并清洗后保留中文节点及连线文字，不使用 foreignObject', async () => {
    // JSDOM cannot measure SVG text; only stub layout, not Mermaid or sanitizing.
    const descriptor = Object.getOwnPropertyDescriptor(SVGElement.prototype, 'getBBox')
    const lengthDescriptor = Object.getOwnPropertyDescriptor(SVGElement.prototype, 'getComputedTextLength')
    Object.defineProperty(SVGElement.prototype, 'getBBox', {
      configurable: true,
      value() { return { x: 0, y: 0, width: 120, height: 24 } },
    })
    Object.defineProperty(SVGElement.prototype, 'getComputedTextLength', {
      configurable: true,
      value() { return (this.textContent || '').length * 8 },
    })
    try {
      const { html } = renderDocument('```mermaid\nflowchart LR\n A["登录认证"] -->|通过| B["加载菜单与权限"]\n```', DOCUMENT_HOME)
      const output = await renderDocumentDiagrams(html)
      const container = document.createElement('div')
      container.innerHTML = output
      expect(container.querySelector('svg')).not.toBeNull()
      expect(container.querySelector('foreignObject')).toBeNull()
      const labels = [...container.querySelectorAll('svg text')].map((node) => node.textContent)
      expect(labels).toContain('登录认证')
      expect(labels).toContain('加载菜单与权限')
      expect(labels).toContain('通过')
    } finally {
      if (descriptor) Object.defineProperty(SVGElement.prototype, 'getBBox', descriptor)
      else delete SVGElement.prototype.getBBox
      if (lengthDescriptor) Object.defineProperty(SVGElement.prototype, 'getComputedTextLength', lengthDescriptor)
      else delete SVGElement.prototype.getComputedTextLength
    }
  }, 30000)
})
