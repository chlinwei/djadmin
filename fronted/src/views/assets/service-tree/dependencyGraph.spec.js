import { describe, expect, it } from 'vitest'
import { dependencyData, dependencyEdgeGeometry, dependencyGraphBounds, dependencyPage, dependencySvg, edgeLabelHeight, edgeLabelWidth, graphEdges, layoutDependencyNodes, nodeHeight, nodeWidth, positionedDependencyEdges, resolveDependencyScope, runtimePresentation, selectDependencyTopology } from './dependencyGraph'

const summary = { total: 4, running: 1, stopped: 1, error: 1, unknown: 1 }
const nodes = [1, 2, 3, 4].map((id) => ({ id, name: `服务 ${id}`, code: `service-${id}`, business_system_id: id === 1 ? 10 : 20, business_system_name: '业务', runtime_summary: summary }))
const edge = (id, source, target, strength = 'unknown') => ({ id, source_service_id: source, target_service_id: target, source_name: `服务 ${source}`, target_name: `服务 ${target}`, interaction_type_id: 1, function_scope: `功能 ${id}`, strength, protocol: '' })
const edges = [edge(1, 1, 2, 'strong'), edge(2, 2, 1, 'weak'), edge(3, 1, 2), edge(4, 2, 3)]

describe('service topology modeling and layout', () => {
  it('resolves tree nodes missing project IDs without guessing the environment', () => {
    const business = [{ id: 10, project: 2 }]
    expect(resolveDependencyScope({ nodeType: 'service', businessSystemId: 10, environment: 3, applicationServiceId: 1 }, business)).toEqual({ project: 2, environment: 3, business_system: 10, service: 1 })
    expect(resolveDependencyScope({ nodeType: 'businessSystem', businessSystemId: 10 }, business).environment).toBeUndefined()
    expect(resolveDependencyScope({ nodeType: 'all', projectIds: [2, 3], environmentIds: [4, 5] }, business).project).toBeUndefined()
  })
  it('retains isolated nodes and direct boundary neighbors, without recursively broadening a focused graph', () => {
    expect(selectDependencyTopology({ nodes, edges }, {}).nodes).toHaveLength(4)
    const focused = selectDependencyTopology({ nodes, edges }, { service: 1 })
    expect(focused.edges.map((item) => item.id)).toEqual([1, 2, 3])
    expect(focused.nodes.map((item) => [item.id, item.external_to_selection])).toEqual([[1, false], [2, true]])
    expect(selectDependencyTopology({ nodes, edges }, { business_system: 10 }).nodes.map((item) => item.id)).toEqual([1, 2])
    expect(selectDependencyTopology({ nodes, edges }, {}, { strength: 'weak' }).edges.map((item) => item.id)).toEqual([2])
    expect(selectDependencyTopology({ nodes, edges }, {}, { search: '功能 4' }).nodes.map((item) => item.id)).toEqual([2, 3])
  })
  it('runs real ELK layout for reciprocal, parallel, cyclic and isolated services with nonoverlapping finite positions', async () => {
    const layout = await layoutDependencyNodes(nodes, [...edges, edge(5, 3, 1)])
    expect(layout.map((item) => Number(item.id)).sort()).toEqual([1, 2, 3, 4])
    for (const a of layout) {
      expect(Number.isFinite(a.position.x) && Number.isFinite(a.position.y)).toBe(true)
      for (const b of layout) {
        if (a.id === b.id) continue
        const separated = Math.abs(a.position.x - b.position.x) >= nodeWidth || Math.abs(a.position.y - b.position.y) >= nodeHeight
        expect(separated).toBe(true)
      }
    }
    expect(await layoutDependencyNodes([], [])).toEqual([])
    expect(await layoutDependencyNodes([nodes[0]], [])).toHaveLength(1)
  })
  it('keeps directed edges independent and offsets parallel labels', () => {
    const visual = graphEdges(edges)
    expect(visual.map((item) => item.id)).toEqual(['1', '2', '3', '4'])
    expect(new Set(visual.slice(0, 3).map((item) => item.data.offset)).size).toBe(3)
    expect(visual[0].style.strokeDasharray).toBeUndefined()
    expect(visual[1].style.strokeDasharray).toBe('7 5')
    expect(visual[2].style.strokeDasharray).toBe('2 5')
    expect(visual[1].source).toBe('2')
    expect(visual[1].target).toBe('1')
  })
  it('routes backward edges outside cards with distinct lanes and includes them in fit/export bounds', () => {
    const input = { sourceX: 750, sourceY: 57, targetX: 0, targetY: 57, minY: 0 }
    const a = dependencyEdgeGeometry({ ...input, lane: 0 })
    const b = dependencyEdgeGeometry({ ...input, lane: 1 })
    expect(a.y).toBeLessThan(0)
    expect(a.path).not.toBe(b.path)
    expect(a.path).toContain(`L-40,${a.y}`)
    const positioned = nodes.slice(0, 2).map((record, i) => ({ id: String(record.id), position: { x: i * 500, y: 0 }, data: { record } }))
    const bounds = dependencyGraphBounds(positioned, edges.slice(0, 3))
    expect(bounds.x).toBeLessThan(0)
    expect(bounds.y).toBeLessThan(a.y)
    expect(bounds.x + bounds.width).toBeGreaterThan(750)
  })
  it('separates labels on crossing, parallel and return edges from every label and card, including after dragging', () => {
    const positioned = Array.from({ length: 7 }, (_, i) => ({
      id: String(i + 1), position: { x: i % 2 * 500, y: Math.floor(i / 2) * 180 },
      data: { record: { ...nodes[0], id: i + 1 } },
    }))
    const relations = [
      edge(1, 1, 4), edge(2, 3, 2), edge(3, 1, 2), edge(4, 3, 4),
      edge(5, 5, 4), edge(6, 3, 6), edge(7, 1, 4), edge(8, 4, 1), edge(9, 6, 3),
    ]
    const separated = (a, b) => a.x + a.width <= b.x || b.x + b.width <= a.x
      || a.y + a.height <= b.y || b.y + b.height <= a.y
    const verify = () => {
      const placed = positionedDependencyEdges(positioned, relations)
      const bounds = dependencyGraphBounds(positioned, relations)
      for (const item of placed) {
        expect(item.label.width).toBe(edgeLabelWidth)
        expect(item.label.height).toBe(edgeLabelHeight)
        for (const other of placed) {
          if (other.id !== item.id) expect(separated(item.label, other.label)).toBe(true)
        }
        for (const node of positioned) expect(separated(item.label, { ...node.position, width: nodeWidth, height: nodeHeight })).toBe(true)
        expect(item.label.x).toBeGreaterThanOrEqual(bounds.x)
        expect(item.label.y).toBeGreaterThanOrEqual(bounds.y)
        expect(item.label.x + item.label.width).toBeLessThanOrEqual(bounds.x + bounds.width)
        expect(item.label.y + item.label.height).toBeLessThanOrEqual(bounds.y + bounds.height)
      }
      const reordered = new Map(positionedDependencyEdges(positioned, [...relations].reverse()).map((item) => [item.id, item.label]))
      for (const item of placed) expect(item.label).toEqual(reordered.get(item.id))
      return placed
    }
    const original = verify()
    expect(original.some((item) => item.labelGuide)).toBe(true)
    expect(original[0].label).not.toEqual(original[1].label)
    positioned[3].position = { x: 300, y: 80 }
    const dragged = verify()
    expect(dragged[0].label).not.toEqual(original[0].label)
    const svg = new DOMParser().parseFromString(dependencySvg(positioned, relations, '交叉依赖'), 'image/svg+xml')
    const exported = svg.querySelectorAll('g.edge-label rect')
    expect(exported).toHaveLength(relations.length)
    exported.forEach((rect, i) => {
      for (const key of ['x', 'y', 'width', 'height']) expect(Number(rect.getAttribute(key))).toBe(dragged[i].label[key])
    })
  })
  it('exports safe standalone SVG with current dragged positions, arrows and state counts, retaining the title but no footer', () => {
    const positioned = nodes.slice(0, 2).map((record, i) => ({ id: String(record.id), position: { x: -40 + i * 500, y: 30 }, data: { record: { ...record, name: '<script>alert("x")</script> & 服务' } } }))
    const svg = dependencySvg(positioned, edges.slice(0, 3), '业务 <A>')
    const document = new DOMParser().parseFromString(svg, 'image/svg+xml')
    expect(document.querySelector('parsererror')).toBeNull()
    expect(document.querySelector('script')).toBeNull()
    expect(document.querySelectorAll('path[marker-end]')).toHaveLength(3)
    expect(svg).toContain('x="-40"')
    expect(svg).toContain('&lt;script&gt;')
    expect(svg).toContain('检查异常')
    expect(svg).toContain('运行 1 / 停止 1 / 异常 1 / 未知 1')
    expect(Array.from(document.documentElement.children).filter((element) => element.tagName === 'text').map((element) => element.textContent)).toEqual(['业务 <A>'])
    expect(svg).not.toContain('A → B：A 依赖 B')
    expect(svg).not.toContain('不代表实时业务健康')
    expect(() => dependencySvg(positioned.slice(0, 1), edges.slice(0, 1), '')).toThrow('无法定位端点')
  })
  it('routes forward, parallel and return lines through manually dragged labels and exports the same paths and bounds', () => {
    const positioned = nodes.slice(0, 2).map((record, i) => ({ id: String(record.id), position: { x: i * 500, y: 0 }, data: { record } }))
    const relations = edges.slice(0, 3)
    const original = positionedDependencyEdges(positioned, relations)
    const offsets = { 1: { x: -600, y: -400 }, 2: { x: 400, y: 200 } }
    const placed = positionedDependencyEdges(positioned, relations, offsets)
    for (let i = 0; i < 2; i++) {
      expect(placed[i].label.x + edgeLabelWidth / 2).toBe(original[i].geometry.x + offsets[i + 1].x)
      expect(placed[i].label.y + edgeLabelHeight / 2).toBe(original[i].geometry.y + offsets[i + 1].y)
      expect(placed[i].geometry.path).not.toBe(original[i].geometry.path)
      expect(placed[i].geometry.path).toContain(`${placed[i].geometry.x},${placed[i].geometry.y}`)
      expect(placed[i].labelGuide).toBe('')
    }
    const bounds = dependencyGraphBounds(positioned, relations, offsets)
    const svg = new DOMParser().parseFromString(dependencySvg(positioned, relations, '手动走线', offsets), 'image/svg+xml')
    svg.querySelectorAll('path[marker-end]').forEach((path, i) => expect(path.getAttribute('d')).toBe(placed[i].geometry.path))
    svg.querySelectorAll('g.edge-label rect').forEach((rect, i) => {
      expect(Number(rect.getAttribute('x'))).toBe(placed[i].label.x)
      expect(Number(rect.getAttribute('y'))).toBe(placed[i].label.y)
      expect(placed[i].label.x).toBeGreaterThanOrEqual(bounds.x)
      expect(placed[i].label.y).toBeGreaterThanOrEqual(bounds.y)
      expect(placed[i].label.x + edgeLabelWidth).toBeLessThanOrEqual(bounds.x + bounds.width)
      expect(placed[i].label.y + edgeLabelHeight).toBeLessThanOrEqual(bounds.y + bounds.height)
    })
    positioned[1].position.x += 200
    const moved = positionedDependencyEdges(positioned, relations, offsets)
    expect(moved[0].label.x - placed[0].label.x).toBe(100)
    expect(moved[0].geometry.path).toContain('700,57')
  })
  it('reserves manual label positions before automatically avoiding collisions regardless of relation order', () => {
    const positioned = nodes.slice(0, 2).map((record, i) => ({ id: String(record.id), position: { x: i * 500, y: 0 }, data: { record } }))
    const relations = [edge(1, 1, 2), edge(2, 1, 2)]
    const original = positionedDependencyEdges(positioned, relations)
    const offsets = { 2: { x: 0, y: original[0].geometry.y - original[1].geometry.y } }
    const placed = positionedDependencyEdges(positioned, relations, offsets)
    expect(placed[1].geometry.y).toBe(original[0].geometry.y)
    expect(placed[0].label.y + edgeLabelHeight + 6 <= placed[1].label.y
      || placed[1].label.y + edgeLabelHeight + 6 <= placed[0].label.y).toBe(true)
    const reversed = new Map(positionedDependencyEdges(positioned, [...relations].reverse(), offsets).map((item) => [item.id, item]))
    for (const item of placed) {
      expect(item.geometry).toEqual(reversed.get(item.id).geometry)
      expect(item.label).toEqual(reversed.get(item.id).label)
    }
  })
  it('never labels a failed or missing check as stopped or healthy', () => {
    expect(runtimePresentation(summary).label).toBe('存在检查异常')
    expect(runtimePresentation({ total: 1, unknown: 1 }).label).toBe('存在未知状态')
    expect(runtimePresentation({ total: 1, stopped: 1 }).label).toBe('全部实例停止')
    expect(runtimePresentation({ total: 0 }).label).toBe('暂无启用实例')
  })
  it('rejects malformed or business-error payloads instead of displaying a successful empty graph', () => {
    expect(() => dependencyData({ data: { code: 500, msg: '不可用', data: {} } })).toThrow('不可用')
    expect(() => dependencyPage({ data: { code: 200, data: {} } })).toThrow('分页')
    expect(() => dependencyPage({ data: { code: 200, data: { results: [], totalPages: 0 } } })).not.toThrow()
  })
})
