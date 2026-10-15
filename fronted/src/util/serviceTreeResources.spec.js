import { describe, expect, it } from 'vitest'
import { buildServiceTreeResources } from './serviceTreeResources'

const data = {
  projects: [
    { id: 1, name: 'A', business_systems: [10] },
    { id: 2, name: 'B', business_systems: [20] },
  ],
  businessSystems: [
    { id: 10, name: 'Orders', project: 1 },
    { id: 20, name: 'Payments', project: 2 },
  ],
  services: [
    { id: 100, name: 'API', business_system: 10, environment: 1, environment_name: 'Production' },
    { id: 101, name: 'DB', business_system: 10, environment: 1, environment_name: 'Production' },
    { id: 102, name: 'API-test', business_system: 10, environment: 2, environment_name: 'Test' },
    { id: 200, name: 'Pay', business_system: 20, environment: 1, environment_name: 'Production' },
  ],
  deployments: [
    { host: 1, application_service_ids: [100] },
    { host_id: 1, application_service_ids: [100, 101, 200] },
    { host: 2, application_service_ids: [102] },
    { host: 999, application_service_ids: [100] },
  ],
  hosts: [
    { id: 1, hardware: { cpu_cores: 8, memory_gb: 16 } },
    { id: 2, cpu_cores: 4, memory_gb: 8 },
  ],
}

describe('service tree resource capacity', () => {
  it('groups all by project, deduplicating hosts within each group', () => {
    const result = buildServiceTreeResources({ ...data, scope: { nodeType: 'all' } })
    expect(result.dimension).toBe('project')
    expect(result.cpuRows).toEqual([{ name: 'A', value: 12 }, { name: 'B', value: 8 }])
    expect(result.totalCpu).toBe(20)
    expect(result.totalMemory).toBe(40)
  })

  it('limits projects to their business systems', () => {
    const result = buildServiceTreeResources({ ...data, scope: { nodeType: 'project', projectId: 1 } })
    expect(result.dimension).toBe('businessSystem')
    expect(result.cpuRows).toEqual([{ name: 'Orders', value: 12 }])
  })

  it('groups business systems by environment', () => {
    const result = buildServiceTreeResources({ ...data, scope: { nodeType: 'businessSystem', businessSystemId: 10 } })
    expect(result.dimension).toBe('environment')
    expect(result.cpuRows).toEqual([{ name: 'Production', value: 8 }, { name: 'Test', value: 4 }])
  })

  it('groups an environment by service, allowing shared host capacity across groups', () => {
    const result = buildServiceTreeResources({
      ...data, scope: { nodeType: 'environment', businessSystemId: 10, environment: 1 },
    })
    expect(result.dimension).toBe('service')
    expect(result.cpuRows).toEqual([{ name: 'API', value: 8 }, { name: 'DB', value: 8 }])
    expect(result.memoryRows).toEqual([{ name: 'API', value: 16 }, { name: 'DB', value: 16 }])
  })

  it('respects root filters and clears empty scope data', () => {
    const filtered = buildServiceTreeResources({
      ...data, scope: { nodeType: 'all', projectIds: [1], environmentIds: [2] },
    })
    expect(filtered.cpuRows).toEqual([{ name: 'A', value: 4 }])
    const empty = buildServiceTreeResources({ ...data, scope: { nodeType: 'project', projectId: 999 } })
    expect(empty.cpuRows).toEqual([])
    expect(empty.totalCpu).toBe(0)
    expect(empty.totalMemory).toBe(0)
  })
})
