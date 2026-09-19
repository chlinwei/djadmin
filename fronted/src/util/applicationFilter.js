// 「日志处理规则」页左侧应用列表的筛选（2026-09-19 加）。
//
// 抽成纯函数是为了能直接测：规则页挂载要 mock 一整套接口，而这段逻辑本身只跟"名称/编码/关键词"
// 有关（`fronted/src/views/monitor/log-parsers/index.vue` 的 visibleApplicationGroups）。
//
// 两条约定：
//  1. **「全部规则」任何时候都保留**——它是"回到全量"的入口；筛没了会让人无路可退，
//     只能清空输入框或刷新页面；
//  2. 匹配**名称或编码**（列表显示名称，但应用编码才是大家平时嘴上说的那个词，
//     两处口径与应用下拉保持一致）。

/** 按关键词过滤左侧应用分组；关键词为空（或全空白）时原样返回。 */
export function filterApplicationGroups(groups, keyword) {
  const text = String(keyword ?? '').trim().toLowerCase()
  if (!text) return groups
  return groups.filter((item) => (
    item.key === 'all' || `${item.label || ''} ${item.code || ''}`.toLowerCase().includes(text)
  ))
}

/** 筛选是否"一条应用都没命中"：此时界面要给出提示，而不是只剩一行「全部规则」。 */
export function isApplicationFilterMissed(groups, keyword) {
  if (!String(keyword ?? '').trim()) return false
  return !groups.some((item) => item.key !== 'all')
}
