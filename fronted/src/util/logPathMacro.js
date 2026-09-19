/**
 * 日志路径里的宏状态（两个页面共用：日志中心的「日志配置」与逻辑服务编辑弹窗）。
 *
 * 为什么需要它：路径列展示的是**模板级的路径模式**（`path_pattern`），服务端只展开了**服务级宏**
 * （`macro_values`）。`${APP_HOME}` 这类值来自部署模板的 `app_home` 与**部署实例的
 * `runtime_variables`**——是主机/实例级的值，服务这一层拿不到，也不该猜：猜一个值显示出来，
 * 用户会以为那就是主机上的真实路径，而实际上同一日志定义在不同主机上路径本就不同。
 *
 * 所以这里的做法是：**把还没展开的宏显式标出来**，并说清去哪看真实路径，而不是偷偷展开一半。
 */
export function unexpandedMacros(path) {
  const matched = String(path || '').match(/\$\{[A-Za-z_][A-Za-z0-9_]*\}/g) || []
  return [...new Set(matched)]
}

// 真实路径在哪看：渲染是**按实例**展开的（模板 macro_definitions 的 value → 服务 macro_values
// → 实例 runtime_variables；app_home 作为 APP_HOME 默认值），全部展开后的绝对路径出现在
// 「日志采集」页的主机配置预览里，认证按实例抽样时用的也是同一套展开。
export const PATH_MACRO_HINT =
  '这是模板里的路径模式：服务级宏已展开，${APP_HOME} 这类来自部署模板 app_home / 实例变量的宏'
  + '要等到按主机渲染时才展开（同一日志定义在不同主机上路径可能不同）。'
  + '展开后的真实路径见「日志采集」页的主机配置预览；格式认证按实例抽样时用的也是那一套。'

// 「路径」列的小开关：原始 pattern ↔ 解析后（模板默认 + 服务覆盖；实例级变量仍留占位符）。
// 默认显示解析后——绝大多数时候人想看的是"这条日志实际落在哪"，但排查"宏到底哪层给的"时要看原始。
export function pathCellValue(record, showResolved) {
  return showResolved ? (record?.resolved_path || record?.path_pattern || '') : (record?.path_pattern || '')
}
