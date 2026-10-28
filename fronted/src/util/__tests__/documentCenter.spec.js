import { describe, expect, it } from 'vitest'
import mermaid from 'mermaid'
import {
  buildDocumentTree, directoryMarkdown, DOCUMENT_HOME, documentLoaders,
  normalizeDocumentPath, renderDocument, resolveDocumentLink,
} from '../documentCenter'
import { documentCatalog, documentAncestorKeys, flattenDocumentCatalog } from '../documentCatalog'

describe('文档中心目录与链接', () => {
  it('只打包项目文档，不包含指令、配置或源码', () => {
    const paths = Object.keys(documentLoaders)
    expect(paths).toContain(DOCUMENT_HOME)
    expect(paths).toContain('autoadmin/README.md')
    expect(paths).toContain('docs/architecture/modules/ACCESS.md')
    expect(paths).toContain('dj_agent/TASK_RESULT_SCHEMA_V1.md')
    expect(paths.some((path) => /docs\/(plans|archive)\/|DEVELOPMENT_PLAN|K8S_INTEGRATION|WORKFLOW_FRONTEND_GUIDE/.test(path))).toBe(false)
    expect(paths.every((path) => path.endsWith('.md'))).toBe(true)
    expect(paths.some((path) => /AGENTS|SKILL|\.github|\.env|backend\//.test(path))).toBe(false)
    expect(paths).not.toContain('docs/ops/FLUENT_BIT_UNINSTALL.md')
    expect(paths).not.toContain('docs/plans/LOG_COLLECTOR_MIGRATION.md')
  })

  it('主目录遵循菜单，多级章节可定位；技术参考与主阅读区分开', () => {
    const paths = Object.keys(documentLoaders)
    const nodes = buildDocumentTree(paths)
    const keys = flattenDocumentCatalog(nodes).map((node) => node.key)
    expect(new Set(keys.map((key) => key.split('#')[0]))).toEqual(new Set(paths))
    expect(new Set(keys).size).toBe(keys.length)
    expect(nodes.map((node) => node.title)).toContain('资产管理')
    expect(nodes.at(-1).title).toBe('开发参考')
    const filtered = buildDocumentTree(paths, '日志采集')
    expect(flattenDocumentCatalog(filtered).some((node) => node.key.endsWith('LOGGING.md#collectors'))).toBe(true)
    expect(documentAncestorKeys('docs/architecture/modules/LOGGING.md#collectors')).toEqual(['monitor', 'logging'])
    expect(documentAncestorKeys('docs/architecture/modules/LOGGING.md#fingerprints')).toEqual(['monitor', 'logging', 'log-center'])
    const fingerprints = flattenDocumentCatalog(buildDocumentTree(paths, '指纹'))
    expect(fingerprints).toContainEqual({ title: '配置指纹与待下发排查', key: 'docs/architecture/modules/LOGGING.md#fingerprints' })
    expect(documentAncestorKeys('docs/architecture/modules/ASSETS.md#service-deletion')).toEqual(['assets'])
    expect(flattenDocumentCatalog(buildDocumentTree(paths, '删除'))).toContainEqual({
      title: '逻辑服务删除全流程', key: 'docs/architecture/modules/ASSETS.md#service-deletion',
    })
    expect(buildDocumentTree(paths, 'not-a-document')).toEqual([])
  })

  it('解析相对文档、跨模块、目录与章节链接', () => {
    expect(resolveDocumentLink(DOCUMENT_HOME, 'architecture/DJ_AGENT_ARCHITECTURE.md#agent-install-update'))
      .toEqual({ type: 'document', path: 'docs/architecture/DJ_AGENT_ARCHITECTURE.md', anchor: 'agent-install-update' })
    expect(resolveDocumentLink(DOCUMENT_HOME, '../fronted/README.md').path).toBe('fronted/README.md')
    expect(resolveDocumentLink(DOCUMENT_HOME, 'plans/')).toEqual({ type: 'unavailable' })
    expect(resolveDocumentLink(DOCUMENT_HOME, '../dj_agent/').path).toBe('dj_agent/README.md')
    expect(resolveDocumentLink(DOCUMENT_HOME, '#%E7%BB%B4%E6%8A%A4%E8%A7%84%E5%88%99').anchor).toBe('维护规则')
    expect(directoryMarkdown('docs/architecture/modules')).toContain('/docs/architecture/modules/ACCESS.md')
  })

  it('拒绝目录越界、未知文件、危险协议和畸形编码', () => {
    expect(normalizeDocumentPath('../../etc/passwd')).toBeNull()
    for (const href of ['../../etc/passwd', '../AGENTS.md', '../autoadmin/config.env', 'javascript:alert(1)', 'data:text/html,x', '//evil.test/x', '%zz']) {
      expect(resolveDocumentLink(DOCUMENT_HOME, href)).toEqual({ type: 'unavailable' })
    }
    expect(resolveDocumentLink(DOCUMENT_HOME, 'https://example.com')).toEqual({ type: 'external', href: 'https://example.com' })
    expect(() => directoryMarkdown('nonexistent')).toThrow('文档目录不存在')
  })
})

describe('Markdown 阅读与安全边界', () => {
  it('逻辑服务删除集中章节包含可解析的主流程、重试与重建图，日志手册链接到同一入口', async () => {
    const path = 'docs/architecture/modules/ASSETS.md'
    const source = await documentLoaders[path]()
    const section = source.split('<a id="service-deletion"></a>')[1].split('## 排障重点')[0]
    const diagrams = [...section.matchAll(/```mermaid\n([\s\S]*?)```/g)]
    expect(diagrams).toHaveLength(3)
    mermaid.initialize({ startOnLoad: false, securityLevel: 'strict' })
    for (const [, diagram] of diagrams) {
      await expect(mermaid.parse(diagram)).resolves.toMatchObject({ diagramType: 'flowchart-v2' })
    }
    const logging = await documentLoaders['docs/architecture/modules/LOGGING.md']()
    expect(logging).toContain('[逻辑服务删除全流程](ASSETS.md#service-deletion)')
    expect(resolveDocumentLink('docs/architecture/modules/LOGGING.md', 'ASSETS.md#service-deletion'))
      .toEqual({ type: 'document', path, anchor: 'service-deletion' })
  })

  it('渲染表格、代码、重复标题与合并文档显式锚点', () => {
    const result = renderDocument('# 文档\n\n## 章节 `code`\n\n## 章节 `code`\n\n<a id="inventory"></a>\n\n| a | b |\n|---|---|\n| 1 | 2 |\n\n```go\nfmt.Println("ok")\n```\n', DOCUMENT_HOME)
    expect(result.html).toContain('<table>')
    expect(result.html).toContain('language-go')
    expect(result.html).toContain('id="章节-code"')
    expect(result.html).toContain('id="章节-code-1"')
    expect(result.html).toContain('<a id="inventory"></a>')
    expect(renderDocument('## 章节 `code`', DOCUMENT_HOME).html).not.toContain('章节-code-2')
  })

  it('内部链接交给阅读器，外链隔离，源码链接明确标记不可用', () => {
    const { html } = renderDocument('[首页](README.md) [源码](../fronted/src/main.js) [外链](https://example.com)', DOCUMENT_HOME)
    expect(html).toContain('data-document="docs/README.md"')
    expect(html).toContain('data-unavailable="../fronted/src/main.js"')
    expect(html).toContain('rel="noopener noreferrer"')
  })

  it('不执行原始 HTML、脚本或图片请求，代码块里的锚点不生效', () => {
    const { html } = renderDocument('<script>alert(1)</script>\n\n<img src=x onerror=alert(1)>\n\n![test](https://evil.test/beacon)\n\n[bad](javascript:alert(1))\n\n```\n<a id="hidden"></a>\n```', DOCUMENT_HOME)
    expect(html).not.toContain('<script>')
    expect(html).not.toContain('<img')
    expect(html).not.toContain('href="javascript:')
    expect(html).not.toContain('<a id="hidden">')
    expect(html).toContain('&lt;script&gt;')
  })

  it('真实文档首页与所有架构文档可以加载并渲染', async () => {
    const paths = Object.keys(documentLoaders).filter((path) => path === DOCUMENT_HOME || path.startsWith('docs/architecture/'))
    for (const path of paths) {
      const source = await documentLoaders[path]()
      const { html, headings } = renderDocument(source, path)
      expect(html.length).toBeGreaterThan(100)
      expect(headings.some((heading) => heading.level === 1)).toBe(true)
    }
  })

  it('菜单中的每个模块与章节存在；每份模块手册都有重点和流程图', async () => {
    for (const entry of flattenDocumentCatalog(documentCatalog)) {
      const [path, anchor] = entry.key.split('#')
      expect(documentLoaders[path], entry.key).toBeTypeOf('function')
      const source = await documentLoaders[path]()
      const { html } = renderDocument(source, path)
      if (anchor) expect(html, entry.key).toContain(`id="${anchor}"`)
      expect(source, path).toContain('```mermaid')
      expect(source, path).toContain('> **')
      const content = document.createElement('div')
      content.innerHTML = html
      expect([...content.querySelectorAll('a[data-unavailable]')].map((link) => link.dataset.unavailable), path).toEqual([])
    }
  })
})
