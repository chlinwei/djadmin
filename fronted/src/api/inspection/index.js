import requestUtil from '@/util/request'

const groupPrefix = 'sys/inspection/groups/'
const taskPrefix = 'sys/inspection/tasks/'
const executionPrefix = 'sys/inspection/executions/'

export function getInspectionGroups(params) {
  return requestUtil.get(groupPrefix, params)
}

export function getInspectionGroup(id) {
  return requestUtil.get(`${groupPrefix}${id}/`)
}

export function saveInspectionGroup(data) {
  return data.id
    ? requestUtil.patch(`${groupPrefix}${data.id}/`, data)
    : requestUtil.post(groupPrefix, data)
}

export function batchDeleteInspectionGroups(ids) {
  return requestUtil.post(`${groupPrefix}batch-delete/`, { ids })
}

export function getInspectionTasks(params) {
  return requestUtil.get(taskPrefix, params)
}

export function saveInspectionTask(data) {
  return data.id
    ? requestUtil.patch(`${taskPrefix}${data.id}/`, data)
    : requestUtil.post(taskPrefix, data)
}

export function batchDeleteInspectionTasks(ids) {
  return requestUtil.post(`${taskPrefix}batch-delete/`, { ids })
}

export function runInspectionTask(id) {
  return requestUtil.post(`${taskPrefix}${id}/run/`, {})
}

export function getInspectionHostScopeTree(params) {
  return requestUtil.get(`${taskPrefix}host-scope-tree/`, params)
}

export function getInspectionExecutions(params) {
  return requestUtil.get(executionPrefix, params)
}

export function getInspectionExecution(id) {
  return requestUtil.get(`${executionPrefix}${id}/`)
}

export function cancelInspectionExecution(id) {
  return requestUtil.post(`${executionPrefix}${id}/cancel/`, {})
}