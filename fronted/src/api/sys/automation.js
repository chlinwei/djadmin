import requestUtil from '@/util/request'

const prefix = 'sys/automation/'

export function getPlaybookList(params) {
  return requestUtil.get(prefix + 'playbooks/', params)
}

export function createPlaybook(params) {
  return requestUtil.post(prefix + 'playbooks/', params)
}

export function updatePlaybook(id, params) {
  return requestUtil.patch(prefix + `playbooks/${id}/`, params)
}

export function batchDeletePlaybooks(ids) {
  return requestUtil.post(prefix + 'playbooks/batch-delete/', { ids })
}

export function uploadPlaybookFile(id, formData) {
  return requestUtil.fileUpload(prefix + `playbooks/${id}/upload/`, formData)
}

export function downloadPlaybookFile(id) {
  return requestUtil.download(prefix + `playbooks/${id}/download/`)
}

export function validatePlaybookContent(params) {
  return requestUtil.post(prefix + 'playbooks/validate/', params)
}

// ShellCheck 组件（Shell 类模板校验依赖）：上传二进制 / 状态 / 删除。
export function getShellcheckStatus() {
  return requestUtil.get(prefix + 'shellcheck/binary/')
}

export function uploadShellcheckBinary(formData) {
  return requestUtil.fileUpload(prefix + 'shellcheck/binary/', formData)
}

export function deleteShellcheckBinary() {
  return requestUtil.del(prefix + 'shellcheck/binary/')
}

export function getTaskList(params) {
  return requestUtil.get(prefix + 'tasks/', params)
}

export function createTask(params) {
  return requestUtil.post(prefix + 'tasks/', params)
}

export function updateTask(id, params) {
  return requestUtil.patch(prefix + `tasks/${id}/`, params)
}

export function batchDeleteTasks(ids) {
  return requestUtil.post(prefix + 'tasks/batch-delete/', { ids })
}

export function runTaskNow(id, params = {}) {
  return requestUtil.post(prefix + `tasks/${id}/run_now/`, params)
}

export function precheckTaskRun(id, params = {}) {
  return requestUtil.post(prefix + `tasks/${id}/precheck/`, params)
}

export function getInventoryList(params) {
  return requestUtil.get(prefix + 'inventories/', params)
}

export function createInventory(params) {
  return requestUtil.post(prefix + 'inventories/', params)
}

export function updateInventory(id, params) {
  return requestUtil.patch(prefix + `inventories/${id}/`, params)
}

export function batchDeleteInventories(ids) {
  return requestUtil.post(prefix + 'inventories/batch-delete/', { ids })
}

export function precheckInventoryLimit(id, params = {}) {
  return requestUtil.post(prefix + `inventories/${id}/precheck-limit/`, params)
}

export function getAutomationHostOptions(params) {
  return requestUtil.get(prefix + 'playbooks/host-options/', params)
}

export function getAutomationGroupTree(params) {
  return requestUtil.get(prefix + 'playbooks/group-tree/', params)
}

export function getJobList(params) {
  return requestUtil.get(prefix + 'jobs/', params)
}

export function getJobDetail(id) {
  return requestUtil.get(prefix + `jobs/${id}/`)
}

export function cancelJob(id) {
  return requestUtil.post(prefix + `jobs/${id}/cancel/`)
}

export function getJobLog(id) {
  return requestUtil.get(prefix + `jobs/${id}/log/`)
}


