import { describe, expect, it } from 'vitest'
import { buildDeploymentWebSSHRoute, buildServiceTreeBreadcrumbs } from './navigation'

describe('deployment WebSSH navigation', () => {
  it('opens the associated host rather than treating the deployment name as an agent identity', () => {
    expect(buildDeploymentWebSSHRoute(
      { id: 101, host: 17, instance_name: 'mysql-01', host_ip: '10.0.0.17' },
      { id: 17, instance_name: 'host-17', ip: '10.0.0.17' },
    )).toEqual({
      path: '/assets/hosts/webssh',
      query: { host_id: '17', instance_name: 'host-17', ip: '10.0.0.17' },
    })
  })

  it('can use the host ID without a loaded host snapshot', () => {
    expect(buildDeploymentWebSSHRoute({
      host_id: 18, instance_name: 'mysql-02', host_ip: '10.0.0.18',
    }).query).toEqual({ host_id: '18', instance_name: '', ip: '10.0.0.18' })
  })
})

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
