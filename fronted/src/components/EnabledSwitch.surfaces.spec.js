import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import fg from 'fast-glob'
import { parse } from '@vue/compiler-sfc'
import { baseParse, NodeTypes } from '@vue/compiler-dom'
import { describe, expect, it } from 'vitest'

function elements(node, result = []) {
  if (node.type === NodeTypes.ELEMENT) result.push(node)
  for (const child of node.children || []) elements(child, result)
  return result
}
const files = fg.sync('src/views/**/*.vue')
const surfaces = files.map(file => {
  const source = readFileSync(resolve(file), 'utf8')
  const template = parse(source).descriptor.template?.content
  return { file, source, nodes: template ? elements(baseParse(template)) : [] }
})
describe('enabled status conventions across frontend views', () => {
  it('uses the shared control for ordinary enabled fields instead of bare switches, badges or tags', () => {
    const exceptions = new Set([
      'src/views/assets/service-tree/ServiceDependencyGraph.vue',
      'src/views/monitor/alerts/index.vue',
    ])
    const violations = []
    for (const { file, nodes } of surfaces) {
      for (const node of nodes) {
        if (!['a-switch', 'a-checkbox', 'a-badge', 'a-tag'].includes(node.tag) || exceptions.has(file)) continue
        if (node.props.some(prop => /\b(?:record|entity|form|editForm|taskForm|baselineForm|bindingForm|createForm|filterForm|scopeViewInventory)\.enabled\b|\bmemberEnabled\b/.test(prop.exp?.content || ''))) {
          violations.push(`${file}:${node.loc.start.line} ${node.tag}`)
        }
      }
    }
    expect(violations).toEqual([])
  })
  it('requires explicit permissions for every remote control and limits self-service to own bindings', () => {
    let remoteCount = 0
    for (const { file, source, nodes } of surfaces) {
      for (const node of nodes.filter(node => node.tag === 'EnabledSwitch')) {
        expect(source, file).toContain("import EnabledSwitch from '@/components/EnabledSwitch.vue'")
        const endpoint = node.props.find(prop => prop.arg?.content === 'endpoint')
        if (!endpoint) continue
        remoteCount++
        const selfService = node.props.find(prop => prop.name === 'self-service')
        if (selfService) {
          expect(file).toBe('src/views/userCenter/index.vue')
          expect(endpoint.exp.content).toContain('sys/usercenter/alertMediaBindings/')
        } else {
          expect(node.props.some(prop => prop.name === 'permission' || prop.arg?.content === 'permission'), file).toBe(true)
        }
        expect(node.props.some(prop => prop.name === 'size' && prop.value?.content === 'small'), file).toBe(false)
      }
    }
    expect(remoteCount).toBeGreaterThanOrEqual(13)
  })
  it.each([
    ['src/views/assets/projects/index.vue', 'assets/projects/', 'assets:applications:update'],
    ['src/views/assets/environments/index.vue', 'assets/business-environments/', 'assets:applications:update'],
    ['src/views/assets/application/components/TemplateManagerDialog.vue', 'assets/application-deployment-templates/', 'assets:applications:update'],
    ['src/views/assets/application/components/VersionDialog.vue', 'assets/application-versions/', 'assets:applications:update'],
    ['src/views/assets/service-tree/ServiceDependencyTypes.vue', 'assets/service-dependency-types/', 'assets:applications:update'],
    ['src/views/monitor/index.vue', 'monitor/packages/', 'monitor:update'],
    ['src/views/monitor/log-storage/index.vue', 'monitor/elasticsearch-clusters/', 'monitor:update'],
    ['src/views/monitor/log-retention/index.vue', 'monitor/log-retention-tiers/', 'monitor:update'],
    ['src/views/monitor/log-parsers/index.vue', 'monitor/log-collection-filter-rules/', 'monitor:update'],
  ])('wires %s to its state-only PATCH and action permission', (file, endpoint, permission) => {
    const { nodes } = surfaces.find(surface => surface.file === file)
    const node = nodes.find(node => node.tag === 'EnabledSwitch' && node.props.some(prop => prop.arg?.content === 'endpoint' && prop.exp.content.includes(endpoint)))
    expect(node).toBeDefined()
    expect(node.props.find(prop => prop.name === 'permission')?.value?.content).toBe(permission)
  })
  it('keeps API tokens and notification diagnostics read-only, and protects built-in profiles', () => {
    for (const file of ['src/views/sys/apiToken/index.vue', 'src/views/monitor/alerts/UserNotificationChain.vue']) {
      const { nodes } = surfaces.find(surface => surface.file === file)
      const switches = nodes.filter(node => node.tag === 'EnabledSwitch')
      expect(switches.length).toBeGreaterThan(0)
      expect(switches.every(node => node.props.some(prop => prop.name === 'read-only'))).toBe(true)
    }
    const { nodes } = surfaces.find(surface => surface.file === 'src/views/assets/application/components/ApplicationWorkspace.vue')
    const profile = nodes.find(node => node.tag === 'EnabledSwitch' && node.props.some(prop => prop.exp?.content.includes('assets/cluster-profiles/')))
    expect(profile.props.some(prop => prop.arg?.content === 'read-only' && prop.exp.content === "record.profile_type !== 'custom'")).toBe(true)
  })
})
