import dayjs from 'dayjs'
import { formatTimeWithTimezone } from '@/util/timezone'
import { toUtcQueryISOStringByUserTimezone } from '@/util/timezoneRange'

export const assetKinds = { project: '项目', business: '业务系统', environment: '服务树环境', service: '逻辑服务', host: '主机' }
const operatorLabels = { '=': '等于', contains: '包含' }
export const operators = Object.entries(operatorLabels).map(([value, label]) => ({ value, label }))
let nextScopeKey = 0
export const newScope = (kind = 'tree') => ({ key: ++nextScopeKey, kind, values: [] })
export function initForm(row = {}, timezone, prefill = {}) {
  const scopes = new Map()
  function addScopeValue(kind, index, value) {
    const key = `${index}:${kind}`
    if (!scopes.has(key)) scopes.set(key, newScope(kind))
    scopes.get(key).values.push(value)
  }
  const labels = []
  for (const m of row.matchers || []) {
    if (m.type === 'label') labels.push({ ...m })
    else if (m.type === 'host') addScopeValue('host', m.scope_row || 0, m.id)
    else addScopeValue('tree', m.scope_row || 0, `${m.node_type}:${m.id}${m.node_type === 'environment' ? `:${m.business_id}` : ''}`)
  }
  if (prefill.alertname) labels.push({ type: 'label', label: 'alertname', operator: '=', value: prefill.alertname })
  if (Number(prefill.host_id) > 0) addScopeValue('host', 0, Number(prefill.host_id))
  const local = (value) => dayjs(formatTimeWithTimezone(value, timezone))
  return { id: row.id, title: row.title || '', reason: row.reason || '', lockedStart: row.status === 'active' ? row.starts_at : null,
    start: row.starts_at ? local(row.starts_at) : null, end: row.ends_at ? local(row.ends_at) : null, scopes: [...scopes.values()], labels }
}

export function buildMatchers(form) {
  const result = form.scopes.flatMap(({ kind, values }, index) => {
    if (!values.length) throw new Error(`请为第 ${index + 1} 行选择范围，或移除该行`)
    const row = index ? { scope_row: index } : {}
    return values.map(value => {
      if (kind === 'host') return { type: 'host', id: value, ...row }
      const [node_type, id, business] = value.split(':')
      return { type: 'tree', node_type, id: Number(id), ...(node_type === 'environment' ? { business_id: Number(business) } : {}), ...row }
    })
  })
  for (const m of form.labels) {
    const label = m.label.trim()
    if (!label || !m.value.trim()) throw new Error('请填写完整的 Label 名称和值')
    if (!Object.hasOwn(operatorLabels, m.operator)) throw new Error('标签条件仅支持等于或包含')
    result.push({ type: 'label', label, operator: m.operator, value: m.value })
  }
  if (!result.length) {
    throw new Error('请至少选择一个资产或添加 Label 条件，不能静音全部告警')
  }
  return result
}

export function buildWindow(form, timezone) {
  if (!form.title.trim() || !form.reason.trim()) throw new Error('请填写窗口名称和维护原因')
  if (!form.start || !form.end) throw new Error('请指定维护的开始和结束时间')
  // 已生效窗口保留数据库的原始精度，不从只显示到秒的控件重新构造开始时间。
  const starts_at = form.lockedStart || toUtcQueryISOStringByUserTimezone(form.start, timezone)
  const ends_at = toUtcQueryISOStringByUserTimezone(form.end, timezone)
  if (Date.parse(ends_at) <= Math.max(Date.parse(starts_at), Date.now())) throw new Error('结束时间必须晚于开始时间和当前时间')
  return { id: form.id, title: form.title.trim(), reason: form.reason.trim(), starts_at, ends_at, matchers: buildMatchers(form) }
}

export function scopeSummary(matchers) {
  return (matchers || []).map(m => m.type === 'label' ? `${m.label} ${operatorLabels[m.operator]} ${m.value}` :
    `${m.business_id ? `业务 #${m.business_id} / ` : ''}${assetKinds[m.type === 'host' ? 'host' : m.node_type]} #${m.id}`).join('；')
}
