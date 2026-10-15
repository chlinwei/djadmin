import { describe, expect, it } from 'vitest'
import { buildServiceTreeBreadcrumbs } from './navigation'

describe('service tree breadcrumb navigation', () => {
  const deploymentScope = {
    nodeType: 'deployment', nodeTitle: 'mysql-01', deploymentId: 101,
    projectId: 1, projectName: 'Platform',
    businessSystemId: 2, businessSystemName: 'Database',
    environment: 3, environmentName: 'Production',
    applicationServiceId: 42, serviceName: 'MySQL',
  }

  it('returns from an instance to its exact logical service without the instance ID', () => {
    const items = buildServiceTreeBreadcrumbs(deploymentScope)
    expect(items.map((item) => item.title)).toEqual([
      '全部业务', 'Platform', 'Database', 'Production', 'MySQL', 'mysql-01',
    ])
    const service = items[4].scope
    expect(service).toEqual({
      nodeType: 'service', nodeTitle: 'MySQL',
      projectId: 1, projectName: 'Platform',
      businessSystemId: 2, businessSystemName: 'Database',
      environment: 3, environmentName: 'Production',
      applicationServiceId: 42, serviceName: 'MySQL',
    })
    expect(items[5].scope).toBeNull()
    expect(buildServiceTreeBreadcrumbs(service).at(-1).scope).toBeNull()
    expect(items[2].scope).not.toHaveProperty('environment')
    expect(items[3].scope).not.toHaveProperty('applicationServiceId')
  })

  it('keeps ancestors with equal names and supports null environment scopes', () => {
    const items = buildServiceTreeBreadcrumbs({
      ...deploymentScope, environment: null, environmentName: 'MySQL',
    })
    expect(items.filter((item) => item.title === 'MySQL')).toHaveLength(2)
    expect(items[3].scope.environment).toBeNull()
  })

  it('does not navigate to missing IDs or make the root clickable on the root page', () => {
    const items = buildServiceTreeBreadcrumbs({
      ...deploymentScope, applicationServiceId: undefined,
    })
    expect(items[4].scope).toBeNull()
    expect(items[0].scope).toEqual({ nodeType: 'all', nodeTitle: '全部业务' })
    expect(buildServiceTreeBreadcrumbs({ nodeType: 'all', nodeTitle: '全部业务' }))
      .toEqual([{ title: '全部业务', scope: null }])
  })
})
