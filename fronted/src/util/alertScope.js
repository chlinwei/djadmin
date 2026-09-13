/**
 * 告警媒介绑定的「服务树订阅范围」公共逻辑，供个人中心绑定表单/列表与通知链路视图复用。
 * scope 的唯一真相是 [{ type, id }] 数组（type: service|environment|business|project）；
 * null / 空数组 = 全局订阅（不限制归属）。
 */

export const SCOPE_TYPE_LABELS = {
  service: '服务',
  environment: '环境',
  business: '业务',
  project: '项目',
}

export function isGlobalScope(scope) {
  return !Array.isArray(scope) || scope.length === 0
}

/** scope → a-tree-select 的 value（'type:id' 字符串），忽略无法解析的项。 */
export function scopeToSelectValues(scope) {
  if (isGlobalScope(scope)) return []
  return scope
    .map((item) => {
      const id = Number(item?.id)
      const type = String(item?.type || '')
      if (!SCOPE_TYPE_LABELS[type] || !Number.isInteger(id) || id <= 0) return null
      return `${type}:${id}`
    })
    .filter(Boolean)
}

/** a-tree-select 的 value → scope，保持勾选顺序。 */
export function selectValuesToScope(values) {
  return (Array.isArray(values) ? values : [])
    .map((value) => {
      const match = String(value || '').match(/^(service|environment|business|project):(\d+)$/)
      if (!match) return null
      return { type: match[1], id: Number(match[2]) }
    })
    .filter(Boolean)
}

/** 单个 scope 项的展示名：环境优先用已知名，否则补类型前缀；missing 节点标「已删除」。 */
export function formatScopeItem(item, nameFallbacks = {}) {
  const type = String(item?.type || '')
  const id = Number(item?.id)
  const typeLabel = SCOPE_TYPE_LABELS[type] || type
  const name = String(item?.name || nameFallbacks[`${type}:${id}`] || '').trim()
  const suffix = item?.missing ? '（已删除）' : ''
  if (name) return `${typeLabel}：${name}${suffix}`
  return `${typeLabel} #${Number.isInteger(id) ? id : '?'}${suffix}`
}

/**
 * 构建订阅范围 tree-select 的树数据。
 * 层级：项目 → 业务系统 → 环境 → 服务，全部节点可直接选中（multiple 模式，不做父子联动勾选，
 * 因为订阅语义是"命中任一所选节点"，选父节点本身就是一条 scope，无需展开成叶子）。
 * 数据来源与 assets/service-tree 页面一致（projects / business-systems / application-services）。
 */
export function buildAlertScopeTreeData({ projects = [], systems = [], services = [], environmentNames = new Map() }) {
  const servicesBySystem = new Map()
  for (const service of Array.isArray(services) ? services : []) {
    const businessId = service?.business_system
    if (businessId === undefined || businessId === null || businessId === '') continue
    if (!servicesBySystem.has(businessId)) servicesBySystem.set(businessId, [])
    servicesBySystem.get(businessId).push(service)
  }

  const serviceNode = (service) => ({
    value: `service:${service.id}`,
    title: String(service.name || `服务#${service.id}`),
    key: `service:${service.id}`,
    isLeaf: true,
  })

  const environmentNodesFor = (businessId) => {
    const byEnv = new Map()
    const order = []
    for (const service of servicesBySystem.get(businessId) || []) {
      const envId = service.environment ?? 'unassigned'
      if (!byEnv.has(envId)) {
        byEnv.set(envId, {
          name: service.environment_name || environmentNames.get(String(envId)) || (envId === 'unassigned' ? '未指定环境' : `环境#${envId}`),
          services: [],
        })
        order.push(envId)
      }
      byEnv.get(envId).services.push(service)
    }
    return order.map((envId) => {
      const group = byEnv.get(envId)
      const selectable = envId !== 'unassigned'
      return {
        key: selectable ? `environment:${envId}` : `environment:unassigned:${businessId}`,
        value: selectable ? `environment:${envId}` : undefined,
        selectable,
        checkable: selectable,
        title: group.name,
        children: group.services.map(serviceNode),
      }
    })
  }

  const businessNode = (system) => {
    const businessId = system.id
    const selectable = businessId !== 'unassigned' && businessId !== undefined && businessId !== null && businessId !== ''
    return {
      key: selectable ? `business:${businessId}` : `business:unassigned:${system.name || ''}`,
      value: selectable ? `business:${businessId}` : undefined,
      selectable,
      checkable: selectable,
      title: String(system.name || `业务#${businessId}`),
      children: environmentNodesFor(businessId),
    }
  }

  const systemsByProject = new Map()
  const systemOrder = []
  for (const system of Array.isArray(systems) ? systems : []) {
    const projectId = system?.project ?? 'unassigned'
    if (!systemsByProject.has(projectId)) {
      systemsByProject.set(projectId, [])
      systemOrder.push(projectId)
    }
    systemsByProject.get(projectId).push(businessNode(system))
  }

  const projectRecords = Array.isArray(projects) ? projects : []
  const knownProjectIds = new Set(projectRecords.map((project) => String(project?.id)))
  const projectNodes = projectRecords.map((project) => ({
    key: `project:${project.id}`,
    value: `project:${project.id}`,
    title: String(project.name || `项目#${project.id}`),
    children: systemsByProject.get(String(project.id)) || systemsByProject.get(project.id) || [],
  }))
  // 未归属项目的业务系统（以及不在项目列表里的孤儿 projectId）挂在"未分配项目"下，本身不可选。
  const unassignedChildren = [
    ...(systemsByProject.get('unassigned') || []),
    ...systemOrder
      .filter((projectId) => projectId !== 'unassigned' && !knownProjectIds.has(String(projectId)))
      .flatMap((projectId) => systemsByProject.get(projectId) || []),
  ]
  if (unassignedChildren.length) {
    projectNodes.push({
      key: 'project:unassigned',
      selectable: false,
      checkable: false,
      title: '未分配项目',
      children: unassignedChildren,
    })
  }

  return projectNodes
}
