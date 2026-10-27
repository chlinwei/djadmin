import { formatTimeWithTimezone } from '@/util/timezone'
import store from '@/store'

// Lists and verification reports share one vocabulary; waivers never count as real instance passes.
export const FORMAT_STATE_LABEL = {
  pending: '待认证', running: '认证中', verified: '已验证', failed: '不通过', waiting: '等待样本', waived: '已豁免',
}
export const FORMAT_STATE_COLOR = {
  pending: 'orange', running: 'blue', verified: 'green', failed: 'red', waiting: 'default', waived: 'gold',
}

export function formatStateTooltip(record) {
  const coverage = `真实实例通过 ${record.format_passed || 0}/${record.format_total || 0}，豁免 ${record.format_waived || 0}。`
  const details = (record.format_instances || []).map((instance) => {
    const at = instance.updated_at
      ? formatTimeWithTimezone(instance.updated_at, store.state.user?.timezone || 'Asia/Shanghai') : ''
    const exhausted = instance.status === 'waiting' && instance.attempts >= 4 ? '；自动重试已用尽' : ''
    return `${instance.name}：${FORMAT_STATE_LABEL[instance.status] || '待认证'}${exhausted}${instance.reason ? `；${instance.reason}` : ''}${at ? `（${at}）` : ''}`
  })
  return [coverage, ...details].join('\n')
}

// 认证入口是否可用：未保存的服务没有 service_id，没挂解析规则的日志不会被采集（也就无从认证格式）。
export function canVerifyLogFormat(record, serviceId) {
  return Boolean(serviceId) && Boolean(record?.template_processing_rule_id)
}

export function formatActionTooltip(record, serviceId) {
  if (!serviceId) return '先保存逻辑服务，再发起格式认证。'
  if (!record?.template_processing_rule_id) return '这条日志还没有关联解析规则（不会采集），无法认证格式。先到部署模板里给它挂规则。'
  return '重新认证'
}
