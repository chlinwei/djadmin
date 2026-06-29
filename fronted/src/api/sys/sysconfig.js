import requestUtil from '@/util/request'

const prefix = 'sys/configs/'

export function getConfigList(params) {
  return requestUtil.get(prefix, params)
}

export function updateConfig(id, value) {
  return requestUtil.patch(prefix + `${id}/`, { value })
}

export function getConfigByKey(key) {
  return requestUtil.get(`sys/configs/by-key/${key}/`)
}

// 常用参数 key 常量，避免硬编码
export const CONFIG_KEYS = {
  TASK_POLL_INTERVAL:  'sys.scheduler.task_poll_interval',
  TASK_POLL_MAX_COUNT: 'sys.scheduler.task_poll_max_count',
  SSH_CONNECT_TIMEOUT: 'sys.scheduler.ssh_connect_timeout',
  SYSTEM_TITLE:        'sys.system_title',
}
