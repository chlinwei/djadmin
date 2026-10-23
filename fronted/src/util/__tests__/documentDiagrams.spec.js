import { afterEach, describe, expect, it, vi } from 'vitest'
import mermaid from 'mermaid'
import { renderDocumentDiagrams } from '../documentDiagrams'
import { renderDocument } from '../documentCenter'

vi.mock('mermaid', () => ({
  default: {
    initialize: vi.fn(),
    render: vi.fn(async (id) => ({ svg: `<svg xmlns="http://www.w3.org/2000/svg" id="${id}"><script>alert(1)</script><text onclick="alert(1)">流程</text></svg>` })),
  },
}))

afterEach(() => vi.clearAllMocks())

describe('流程图渲染边界', () => {
  it('仅 Mermaid 代码块渲染为清洗后的 SVG，普通代码保留', async () => {
    const { html } = renderDocument('```mermaid\nflowchart LR\nA-->B\n```\n\n```go\nfmt.Println(1)\n```', 'docs/README.md')
    const output = await renderDocumentDiagrams(html)
    expect(output).toContain('<figure class="document-diagram"')
    expect(output).toContain('<svg')
    expect(output).not.toContain('<script')
    expect(output).not.toContain('onclick')
    expect(output).toContain('language-go')
    expect(output).not.toContain('language-mermaid')
    expect(output).toContain('流程</text>')
    expect(mermaid.initialize).toHaveBeenCalledWith(expect.objectContaining({
      htmlLabels: false,
      flowchart: { htmlLabels: false },
      securityLevel: 'strict',
    }))
  })

  it('为各图生成唯一 ID，不复用可能冲突的 SVG 标识', async () => {
    const { html } = renderDocument('```mermaid\nflowchart LR\nA-->B\n```\n\n```mermaid\nflowchart TD\nC-->D\n```', 'docs/README.md')
    await renderDocumentDiagrams(html)
    expect(mermaid.render).toHaveBeenCalledTimes(2)
    expect(mermaid.render.mock.calls[0][0]).not.toBe(mermaid.render.mock.calls[1][0])
  })

  it('拒绝图表配置与点击脚本；语法错误交由页面显式展示', async () => {
    for (const source of ['%%{init: {"securityLevel":"loose"}}%%\nflowchart LR\nA-->B', '---\nconfig:\n---\nflowchart LR\nA-->B', 'flowchart LR\nA-->B\nclick A "javascript:alert(1)"']) {
      const { html } = renderDocument(`\`\`\`mermaid\n${source}\n\`\`\``, 'docs/README.md')
      await expect(renderDocumentDiagrams(html)).rejects.toThrow('不允许')
    }
    mermaid.render.mockRejectedValueOnce(new Error('parse error'))
    const { html } = renderDocument('```mermaid\nbad syntax\n```', 'docs/README.md')
    await expect(renderDocumentDiagrams(html)).rejects.toThrow('parse error')
  })

  it('没有图时不改变正文', async () => {
    expect(await renderDocumentDiagrams('<p>正文</p>')).toBe('<p>正文</p>')
    expect(mermaid.render).not.toHaveBeenCalled()
  })
})
