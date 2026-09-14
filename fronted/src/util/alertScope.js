/**
 * 服务树公共树数据构建，供「通知策略」页面的 tree matcher（tree type 条目）选择服务树节点使用。
 * 树层级：项目 → 业务系统 → 环境 → 服务，全部节点可直接选中（订阅/路由语义是"命中任一所选节点"）。
 * 数据来源与 assets/service-tree 页面一致（projects / business-systems / application-services）。
 */

/**
 * 构建 a-tree-select 的树数据；节点 value 形如 `type:id`（type: service|environment|business|project）。
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

  // 同一环境可挂在多个业务系统下：只在首次出现的位置携带 value（可选），
  // 其余分支的同环境节点仅作层级展示（key 追加业务后缀，不可选），避免 a-tree `value` 重复。
  const valuedEnvironmentIds = new Set()

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
      const selectable = envId !== 'unassigned' && !valuedEnvironmentIds.has(String(envId))
      if (selectable) {
        valuedEnvironmentIds.add(String(envId))
      }
      const key = selectable
        ? `environment:${envId}`
        : `environment:${envId === 'unassigned' ? 'unassigned' : `${envId}:dup`}:${businessId}`
      const node = {
        key,
        selectable,
        checkable: selectable,
        title: group.name,
        children: group.services.map(serviceNode),
      }
      // 不可选节点也必须带 value 且等于 key，否则 rc-tree-select 告警 key/value 不一致。
      node.value = selectable ? `environment:${envId}` : key
      return node
    })
  }

  const businessNode = (system) => {
    const businessId = system.id
    const selectable = businessId !== 'unassigned' && businessId !== undefined && businessId !== null && businessId !== ''
    const node = {
      key: selectable ? `business:${businessId}` : `business:unassigned:${system.name || ''}`,
      selectable,
      checkable: selectable,
      title: String(system.name || `业务#${businessId}`),
      children: environmentNodesFor(businessId),
    }
    node.value = selectable ? `business:${businessId}` : node.key
    return node
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
      value: 'project:unassigned',
      selectable: false,
      checkable: false,
      title: '未分配项目',
      children: unassignedChildren,
    })
  }

  return projectNodes
}
