let enginePromise
let nextDiagramId = 0

function loadEngine() {
  if (!enginePromise) {
    enginePromise = import('mermaid').then(({ default: mermaid }) => {
      // Documentation is not executable content: disable HTML labels, links and diagram-specific config.
      mermaid.initialize({
        startOnLoad: false,
        securityLevel: 'strict',
        suppressErrorRendering: true,
        theme: 'default',
        // Mermaid's global default overrides the flowchart setting; SVG-only sanitizing removes HTML labels.
        htmlLabels: false,
        flowchart: { htmlLabels: false },
      })
      return mermaid
    }).catch((error) => {
      enginePromise = undefined
      throw error
    })
  }
  return enginePromise
}

export async function renderDocumentDiagrams(html) {
  const container = document.createElement('div')
  container.innerHTML = html
  const blocks = container.querySelectorAll('pre > code.language-mermaid')
  if (!blocks.length) return html
  const [mermaid, { default: DOMPurify }] = await Promise.all([loadEngine(), import('dompurify')])
  for (const block of blocks) {
    const source = block.textContent
    if (/%%\s*\{/.test(source) || /^\s*---/m.test(source) || /^\s*click\s/m.test(source)) {
      throw new Error('流程图不允许配置指令、frontmatter 或点击脚本')
    }
    const { svg } = await mermaid.render(`document-flow-${++nextDiagramId}`, source)
    const diagram = document.createElement('figure')
    diagram.className = 'document-diagram'
    diagram.setAttribute('aria-label', '模块流程图')
    diagram.innerHTML = DOMPurify.sanitize(svg, { USE_PROFILES: { svg: true, svgFilters: true } })
    block.parentElement.replaceWith(diagram)
  }
  return container.innerHTML
}
