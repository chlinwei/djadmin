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

export function deletePlaybook(id) {
  return requestUtil.del(prefix + `playbooks/${id}/`)
}

export function uploadPlaybookFile(id, formData) {
  return requestUtil.fileUpload(prefix + `playbooks/${id}/upload/`, formData)
}

export function downloadPlaybookFile(id) {
  return requestUtil.download(prefix + `playbooks/${id}/download/`)
}

export function runPlaybook(id, params) {
  return requestUtil.post(prefix + `playbooks/${id}/run/`, params)
}

export function validatePlaybookContent(params) {
  return requestUtil.post(prefix + 'playbooks/validate/', params)
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

export function deleteTask(id) {
  return requestUtil.del(prefix + `tasks/${id}/`)
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

export function deleteInventory(id) {
  return requestUtil.del(prefix + `inventories/${id}/`)
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

export function getTargetList(params) {
  return requestUtil.get(prefix + 'targets/', params)
}

export function getWorkflowList(params) {
  return requestUtil.get(prefix + 'workflows/', params)
}

export function getWorkflowDetail(id) {
  return requestUtil.get(prefix + `workflows/${id}/`)
}

export function createWorkflow(params) {
  return requestUtil.post(prefix + 'workflows/', params)
}

export function updateWorkflow(id, params) {
  return requestUtil.patch(prefix + `workflows/${id}/`, params)
}

export function deleteWorkflow(id) {
  return requestUtil.del(prefix + `workflows/${id}/`)
}

export function launchWorkflow(id, params = {}) {
  return requestUtil.post(prefix + `workflows/${id}/launch/`, params)
}

export function getWorkflowRunList(params) {
  return requestUtil.get(prefix + 'workflow-runs/', params)
}

export function getWorkflowRunDetail(id) {
  return requestUtil.get(prefix + `workflow-runs/${id}/`)
}

export function cancelWorkflowRun(id) {
  return requestUtil.post(prefix + `workflow-runs/${id}/cancel/`)
}
