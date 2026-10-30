import { beforeEach, describe, expect, it, vi } from 'vitest'
vi.mock('@/util/request', () => ({ default: { get: vi.fn(), post: vi.fn(), put: vi.fn() } }))
import request from '@/util/request'
import * as api from './serviceDependencies'

beforeEach(() => vi.clearAllMocks())
describe('service dependency API contracts', () => {
  it('uses the real read routes and forwards scope filters', () => {
    const params = { project: 2, environment: 3, service: 4 }
    api.getServiceDependencyTopology(params)
    api.getServiceDependencyTypes({ enabled: true })
    expect(request.get.mock.calls).toEqual([
      ['assets/service-dependencies/topology/', params],
      ['assets/service-dependency-types/', { enabled: true }],
    ])
  })
  it('creates with POST, fully updates with PUT and deletes only through batch endpoints', () => {
    const edge = { source_service_id: 1, target_service_id: 2, reverse: { strength: 'weak' } }
    const type = { code: 'http', name: '服务调用', enabled: true }
    api.saveServiceDependency(null, edge)
    api.saveServiceDependency(9, edge)
    api.saveServiceDependencyType(null, type)
    api.saveServiceDependencyType(4, type)
    api.deleteServiceDependencies([9, 10])
    api.deleteServiceDependencyTypes([4])
    expect(request.post.mock.calls).toEqual([
      ['assets/service-dependencies/', edge],
      ['assets/service-dependency-types/', type],
      ['assets/service-dependencies/batch-delete/', { ids: [9, 10] }],
      ['assets/service-dependency-types/batch-delete/', { ids: [4] }],
    ])
    expect(request.put.mock.calls).toEqual([
      ['assets/service-dependencies/9/', edge],
      ['assets/service-dependency-types/4/', type],
    ])
  })
})
