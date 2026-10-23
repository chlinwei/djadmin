import MarkdownIt from 'markdown-it'
import { documentCatalog, DOCUMENT_HOME } from './documentCatalog'

export { DOCUMENT_HOME } from './documentCatalog'

// Only published project documentation is bundled; never glob the entire repository.
const sources = import.meta.glob([
  '../../../docs/README.md',
  '../../../docs/overview/*.md',
  '../../../docs/architecture/**/*.md',
  '!../../../docs/architecture/K8S_INTEGRATION_ARCHITECTURE.md',
  '../../../docs/ops/*.md',
  '../../../README.md',
  '../../../autoadmin/README.md',
  '../../../autoadmin/db/**/README.md',
  '../../../autoadmin/deploy/README.md',
  '../../../fronted/README.md',
  '../../../dj_agent/README.md',
  '../../../dj_agent/deploy/README.md',
  '../../../dj_agent/TASK_RESULT_SCHEMA_V1.md',
], { query: '?raw', import: 'default' })

export const documentLoaders = Object.fromEntries(
  Object.entries(sources).map(([path, load]) => [normalizeDocumentPath(`fronted/src/util/${path}`), load]),
)

export function normalizeDocumentPath(path) {
  const parts = []
  for (const part of path.split('/')) {
    if (!part || part === '.') continue
    if (part === '..') {
      if (!parts.length) return null
      parts.pop()
    } else {
      parts.push(part)
    }
  }
  return parts.join('/')
}

export function resolveDocumentLink(currentPath, href, loaders = documentLoaders) {
  if (/^(https?:|mailto:)/i.test(href)) return { type: 'external', href }
  if (/^[a-z][\w+.-]*:/i.test(href) || href.startsWith('//')) return { type: 'unavailable' }
  let decoded
  try {
    decoded = decodeURIComponent(href)
  } catch {
    return { type: 'unavailable' }
  }
  const [rawPath, ...fragment] = decoded.split('#')
  const anchor = fragment.join('#')
  const base = currentPath.slice(0, currentPath.lastIndexOf('/') + 1)
  const path = normalizeDocumentPath(rawPath ? (rawPath.startsWith('/') ? rawPath : base + rawPath) : currentPath)
  if (path === null) return { type: 'unavailable' }
  if (Object.hasOwn(loaders, path)) return { type: 'document', path, anchor }
  if (Object.hasOwn(loaders, `${path}/README.md`)) return { type: 'document', path: `${path}/README.md`, anchor }
  if (Object.keys(loaders).some((entry) => entry.startsWith(`${path}/`))) {
    return { type: 'directory', path, anchor }
  }
  return { type: 'unavailable' }
}

const documentTitles = {
  'README.md': '项目说明',
  'docs/README.md': '文档中心首页',
  'docs/overview/PROJECT_CONTEXT.md': '项目概览',
  'docs/architecture/CONVENTIONS.md': '开发与设计约定',
  'docs/architecture/SQL_DESIGN.md': 'SQL 设计',
  'docs/architecture/ASSET_CATALOG.md': '资产与服务树',
  'docs/architecture/DJ_AGENT_ARCHITECTURE.md': 'Agent 架构与安装更新',
  'docs/architecture/AUTOMATION_JOB_EXECUTION.md': '自动化作业与 Inventory',
  'docs/architecture/INSPECTION_ARCHITECTURE.md': '巡检架构',
  'docs/architecture/BASELINE_ARCHITECTURE.md': '基线架构',
  'docs/architecture/MONITOR_PROMETHEUS_PROXY.md': '监控与 Exporter',
  'docs/architecture/ALERT_HISTORY_ARCHITECTURE.md': '告警与通知',
  'docs/architecture/LOG_COLLECTION_ARCHITECTURE.md': '日志采集与存储',
  'docs/architecture/SCHEDULER_ARCHITECTURE.md': '持久化定时清理',
  'docs/architecture/SYSTEM_MANAGEMENT_AUDIT.md': '系统管理与审计',
  'docs/architecture/K8S_INTEGRATION_ARCHITECTURE.md': 'Kubernetes 集成',
  'docs/architecture/FRONTEND_TABS_CACHE.md': '前端标签页与缓存',
  'docs/ops/ALERT_MEDIA_SETUP_GUIDE.md': '告警媒介配置',
  'autoadmin/README.md': '后端开发与运行',
  'fronted/README.md': '前端开发说明',
  'dj_agent/README.md': 'Agent 开发与运行',
  'dj_agent/TASK_RESULT_SCHEMA_V1.md': 'Agent 任务结果协议',
}

export function documentLabel(path) {
  if (documentTitles[path]) return documentTitles[path]
  const name = path.split('/').pop()
  return name === 'README.md' ? `${path.slice(0, -9)}说明` : name.replace(/\.md$/, '').replaceAll('_', ' ')
}

export function buildDocumentTree(paths, search = '') {
  const keyword = search.trim().toLowerCase()
  const references = paths.filter((path) => path !== DOCUMENT_HOME && !path.startsWith('docs/architecture/modules/'))
  const nodes = [
    ...documentCatalog,
    {
      key: 'references', title: '开发参考', selectable: false,
      children: references.sort().map((path) => ({ key: path, title: documentLabel(path) })),
    },
  ]
  function filter(nodes, parents = '') {
    return nodes.flatMap((node) => {
      const context = `${parents} ${node.title}`
      if (node.children) {
        const children = filter(node.children, context)
        return children.length ? [{ ...node, children }] : []
      }
      return paths.includes(node.key.split('#')[0])
        && (!keyword || `${context} ${node.key}`.toLowerCase().includes(keyword)) ? [{ ...node }] : []
    })
  }
  return filter(nodes)
}

export function directoryMarkdown(path, loaders = documentLoaders) {
  const entries = Object.keys(loaders).filter((entry) => entry.startsWith(`${path}/`)).sort()
  if (!entries.length) throw new Error(`文档目录不存在：${path}`)
  return `# ${path}\n\n${entries.map((entry) => `- [${documentLabel(entry)}](/${entry})`).join('\n')}`
}

export function headingSlug(text) {
  return text.toLowerCase().replace(/[^\p{L}\p{M}\p{N}_\-\s]/gu, '').replace(/\s/g, '-')
}

const markdown = new MarkdownIt({ html: false, linkify: false })

// Merged documents use explicit anchors. Recognize only this exact safe form, not arbitrary HTML.
markdown.block.ruler.before('paragraph', 'document_anchor', (state, startLine, endLine, silent) => {
  const line = state.src.slice(state.bMarks[startLine] + state.tShift[startLine], state.eMarks[startLine])
  const match = /^<a id="([a-zA-Z0-9_-]+)"><\/a>$/.exec(line)
  if (!match) return false
  if (!silent) {
    const token = state.push('document_anchor', '', 0)
    token.attrSet('id', match[1])
    state.line = startLine + 1
  }
  return true
})
markdown.renderer.rules.document_anchor = (tokens, index) => `<a id="${tokens[index].attrGet('id')}"></a>\n`

markdown.renderer.rules.heading_open = (tokens, index, options, env, renderer) => {
  const inline = tokens[index + 1]
  const title = (inline.children || []).filter((token) => ['text', 'code_inline'].includes(token.type))
    .map((token) => token.content).join('')
  const slug = headingSlug(title)
  const count = env.slugs.get(slug) || 0
  env.slugs.set(slug, count + 1)
  const id = count ? `${slug}-${count}` : slug
  tokens[index].attrSet('id', id)
  env.headings.push({ id, title, level: Number(tokens[index].tag.slice(1)) })
  return renderer.renderToken(tokens, index, options)
}

markdown.renderer.rules.link_open = (tokens, index, options, env, renderer) => {
  const token = tokens[index]
  const href = token.attrGet('href') || ''
  const resolved = resolveDocumentLink(env.path, href, env.loaders)
  if (resolved.type === 'external') {
    token.attrSet('target', '_blank')
    token.attrSet('rel', 'noopener noreferrer')
  } else {
    token.attrSet('href', '#')
    if (resolved.type === 'document' || resolved.type === 'directory') {
      token.attrSet('data-document', resolved.path)
      token.attrSet('data-anchor', resolved.anchor)
    } else {
      token.attrSet('data-unavailable', href)
      token.attrSet('title', '此链接不是已发布的项目文档')
    }
  }
  return renderer.renderToken(tokens, index, options)
}
// Images must not initiate requests to arbitrary servers from the reader.
markdown.renderer.rules.image = (tokens, index) => markdown.utils.escapeHtml(`[图片：${tokens[index].content}]`)

export function renderDocument(source, path, loaders = documentLoaders) {
  const env = { path, loaders, headings: [], slugs: new Map() }
  return { html: markdown.render(source, env), headings: env.headings }
}
