import request from '@/util/request'

async function query(path, params, paginated = false) {
  const response = await request.get(`sys/security/issues/${path}`, params, null, { suppressBusinessErrorMessage: true })
  const envelope = response?.data
  if (envelope?.code !== 200 || !envelope.data) {
    throw new Error(envelope?.msg || '安全问题查询失败')
  }
  const data = envelope.data
  if (paginated && (!Array.isArray(data.results) || !Number.isInteger(data.count) || data.count < 0)) {
    throw new Error('安全问题分页响应格式无效')
  }
  return data
}

export const listSecurityIssues = (params) => query('', params, true)
export const listSecurityIssueGroups = (params) => query('groups/', params, true)
export const getSecurityIssue = (id) => query(`${id}/`)
export const listSecurityIssueHistory = (id, params) => query(`${id}/history/`, params, true)
