import { describe, expect, it } from 'vitest'
import { buildHostServiceTree } from './hostServiceTree'

const host = { id: 1, environment: 3, environment_name: '生产' }
const systems = [{ id: 2, project: 5, project_name: '项目', name: '业务' }]
const services = [
  { id: 7, business_system: 2, environment: 3, name: '服务 A' },
  { id: 8, business_system: 2, environment: 3, name: '服务 B' },
  { id: 9, business_system: 2, environment: 4, name: '测试服务' },
]

describe('host service tree ownership', () => {
  it('merges ancestors and retains all services on only the selected host', () => {
    const result = buildHostServiceTree(host, [
      { id: 10, host: 1, application_service_ids: [7, 8] },
      { id: 11, host: 1, application_service_ids: [7] },
      { id: 12, host: 99, application_service_ids: [7] },
    ], services, systems)
    expect(result.serviceCount).toBe(2)
    expect(result.deploymentCount).toBe(2)
    expect(result.tree).toHaveLength(1)
    const nodes = result.tree[0].children[0].children[0].children
    expect(nodes).toHaveLength(2)
    expect(nodes[0].children).toHaveLength(2)
    expect(nodes[0].children[0].route.query).toMatchObject({
      deployment_id: '10', application_service_id: '7', environment_id: '3', project_id: '5',
    })
  })

  it('separates unlinked deployments and invalid associations without hiding them', () => {
    const result = buildHostServiceTree(host, [
      { id: 10, host_id: 1, application_service_ids: [9, 999] },
      { id: 11, host: 1, application_service_ids: [] },
    ], services, systems)
    expect(result.tree).toEqual([])
    expect(result.unlinked.map((item) => item.id)).toEqual([11])
    expect(result.anomalies).toHaveLength(2)
    expect(result.deploymentCount).toBe(2)
  })

  it('does not infer an environment for a host without one', () => {
    const result = buildHostServiceTree({ id: 1 }, [
      { id: 10, host: 1, application_service_ids: [7] },
    ], services, systems)
    expect(result.tree).toEqual([])
    expect(result.anomalies).toHaveLength(1)
    expect(buildHostServiceTree(host, [], services, systems).deploymentCount).toBe(0)
  })
})
