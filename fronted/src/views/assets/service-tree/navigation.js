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
