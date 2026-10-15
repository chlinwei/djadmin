export function buildDeploymentWebSSHRoute(deployment, host) {
  return {
    path: '/assets/hosts/webssh',
    query: {
      host_id: String(deployment.host_id ?? deployment.host),
      // 部署实例名不是 Agent 主机身份，终端只使用主机的实例名。
      instance_name: host?.instance_name || '',
      ip: host?.ip || deployment.host_ip || '',
    },
  }
}

export function buildServiceTreeBreadcrumbs(scope) {
  const items = [{ title: '全部业务', scope: { nodeType: 'all', nodeTitle: '全部业务' } }]
  const levels = [
    ['project', 'projectName', ['projectId', 'projectName']],
    ['businessSystem', 'businessSystemName', ['businessSystemId', 'businessSystemName']],
    ['environment', 'environmentName', ['environment', 'environmentName']],
    ['service', 'serviceName', ['applicationServiceId', 'serviceName']],
    ['deployment', null, ['deploymentId']],
  ]
  const context = {}
  for (const [nodeType, nameField, fields] of levels) {
    for (const field of fields) {
      if (Object.hasOwn(scope, field)) context[field] = scope[field]
    }
    const current = scope.nodeType === nodeType
    const title = current ? scope.nodeTitle || scope[nameField] : scope[nameField]
    if (title) {
      const idField = fields[0]
      const navigable = nodeType === 'environment'
        ? Boolean(context.businessSystemId) && Object.hasOwn(context, 'environment')
        : Boolean(context[idField])
      items.push({
        title,
        scope: navigable ? { ...context, nodeType, nodeTitle: title } : null,
      })
    }
    if (current) break
  }
  return items.map((item) => ({
    ...item,
    scope: item.scope?.nodeType === scope.nodeType ? null : item.scope,
  }))
}
