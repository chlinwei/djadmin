import { formatTimeWithTimezone } from '@/util/timezone'
import store from '@/store'

// 日志格式认证的展示口径（架构文档 §4.8）。
//
// 单独抽出来是因为**两个页面都要展示同一条 (服务×日志定义) 的认证状态**：逻辑服务编辑弹窗与
// 日志中心页。文案分叉过一次就没人再信这个状态了——比如一边说"需重新认证"、另一边说"已认证"。
export const FORMAT_STATE_LABEL = { verified: '已验证', needs_recheck: '需重新验证', unverified: '未验证' }
export const FORMAT_STATE_COLOR = { verified: 'green', needs_recheck: 'orange', unverified: 'default' }

function verifiedAtText(record) {
  if (!record.format_verified_at) return '从未认证'
  const timezone = store.state.user?.timezone || 'Asia/Shanghai'
  return `上次认证：${formatTimeWithTimezone(record.format_verified_at, timezone)}`
}

export function formatStateTooltip(record) {
  const verifiedAt = verifiedAtText(record)
  if (record.format_state === 'verified') {
    return `${verifiedAt}（依据：${record.format_verified_source || '未知'}）。配置指纹未变，不需要再检查。`
  }
  if (record.format_state === 'needs_recheck') {
    return `${verifiedAt}，但格式指纹已变化（模板日志定义/解析规则/服务宏/应用版本有改动）→ 需要重新抽样验证。`
  }
  return '尚未验证这条日志的格式能否被模板上的解析规则解析出必备字段（log_level / log_message / error_fingerprint）。'
    + '开启采集前需要抽样验证一次；验证通过后不再重复检查，只有格式指纹变化时才要求重新验证。'
}

// 认证入口是否可用：未保存的服务没有 service_id，没挂解析规则的日志不会被采集（也就无从认证格式）。
export function canVerifyLogFormat(record, serviceId) {
  return Boolean(serviceId) && Boolean(record?.template_processing_rule_id)
}

export function formatActionTooltip(record, serviceId) {
  if (!serviceId) return '先保存逻辑服务，再发起格式认证。'
  if (!record?.template_processing_rule_id) return '这条日志还没有关联解析规则（不会采集），无法认证格式。先到部署模板里给它挂规则。'
  if (record.format_state === 'needs_recheck') return '配置指纹已变化，需要重新抽样认证一次。'
  if (record.format_state === 'verified') return '已认证。改了实例级运行时变量（不进指纹）之后需要人工重跑一次。'
  return '取该实例最近若干行真实日志（或用规则样例）跑一遍解析规则，确认必备字段能解析出来。'
}
