import { beforeEach, describe, expect, it, vi } from 'vitest'
import request from '@/util/request'
import { getSecurityIssue, listSecurityIssueGroups, listSecurityIssues } from './securityIssues'

vi.mock('@/util/request', () => ({ default: { get: vi.fn() } }))
beforeEach(() => vi.clearAllMocks())

describe('安全问题 API', () => {
  it.each([
    [listSecurityIssues, '', 2], [listSecurityIssueGroups, 'groups/', 'unassigned'],
  ])('使用 GET URL 环境参数并保留服务端环境候选', async (query, path, environment) => {
    const data = { results: [], count: 0, environments: [{ id: 2, name: '生产' }] }
    request.get.mockResolvedValue({ data: { code: 200, data } })
    await expect(query({ page: 2, status: 'open', environment_id: environment })).resolves.toEqual(data)
    expect(request.get).toHaveBeenCalledWith(`sys/security/issues/${path}`, { page: 2, status: 'open', environment_id: environment }, null, { suppressBusinessErrorMessage: true })
  })
  it('业务失败及坏分页响应不能变成空列表成功', async () => {
    request.get.mockResolvedValueOnce({ data: { code: 403, msg: '无权限', data: null } })
    await expect(getSecurityIssue(1)).rejects.toThrow('无权限')
    request.get.mockResolvedValueOnce({ data: { code: 200, data: { results: [], count: '0' } } })
    await expect(listSecurityIssues({})).rejects.toThrow('分页响应格式无效')
  })
})
