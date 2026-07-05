import requestUtil from '@/util/request'

const prefix = 'sys/scheduler/'

export function getTaskList(params) {
  return requestUtil.get(prefix + 'tasks/', params)
}

export function getTaskLogList(params) {
  return requestUtil.get(prefix + 'task-logs/', params)
}

export function enableTask(id) {
  return requestUtil.post(prefix + `tasks/${id}/enable/`)
}

export function disableTask(id) {
  return requestUtil.post(prefix + `tasks/${id}/disable/`)
}

export function updateTask(id, params) {
  return requestUtil.patch(prefix + `tasks/${id}/`, params)
}

export function createTask(params) {
  return requestUtil.post(prefix + 'tasks/', params)
}

export function runTaskNow(id) {
  // Return immediately, task runs in background
  return requestUtil.post(prefix + `tasks/${id}/run_now/`, {})
}

export function getTaskStatus(id) {
  // Check task execution status
  return requestUtil.get(prefix + `tasks/${id}/status/`)
}
