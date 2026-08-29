import requestUtil from '@/util/request'

const groupPrefix = 'sys/inspection/groups/'
const taskPrefix = 'sys/inspection/tasks/'
const executionPrefix = 'sys/inspection/executions/'

export function getInspectionGroups(params) {
  return requestUtil.get(groupPrefix, params)
}

export function saveInspectionGroup(data) {
  return data.id
    ? requestUtil.patch(`${groupPrefix}${data.id}/`, data)
    : requestUtil.post(groupPrefix, data)
}

export function deleteInspectionGroup(id) {
  return requestUtil.del(`${groupPrefix}${id}/`)
}

export function getInspectionTasks(params) {
  return requestUtil.get(taskPrefix, params)
}

export function saveInspectionTask(data) {
  return data.id
    ? requestUtil.patch(`${taskPrefix}${data.id}/`, data)
    : requestUtil.post(taskPrefix, data)
}

export function deleteInspectionTask(id) {
  return requestUtil.del(`${taskPrefix}${id}/`)
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