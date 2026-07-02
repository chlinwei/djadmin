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

export function runPlaybook(id, params) {
  return requestUtil.post(prefix + `playbooks/${id}/run/`, params)
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

export function getTargetList(params) {
  return requestUtil.get(prefix + 'targets/', params)
}
