export function buildHostServiceTree(host, deployments, services, systems) {
  const serviceMap = new Map(services.map((item) => [String(item.id), item]))
  const systemMap = new Map(systems.map((item) => [String(item.id), item]))
  const tree = []
  const unlinked = []
  const anomalies = []
  const serviceIds = new Set()
  const hostDeployments = deployments.filter((item) => String(item.host_id ?? item.host) === String(host.id))
  function branch(children, key, title) {
    let node = children.find((item) => item.key === key)
    if (!node) {
      node = { key, title, children: [] }
      children.push(node)
    }
    return node
  }
  for (const deployment of hostDeployments) {
    const ids = deployment.application_service_ids || []
    if (!ids.length) unlinked.push(deployment)
    for (const id of ids) {
      const service = serviceMap.get(String(id))
      if (!service) {
        anomalies.push({ key: `${deployment.id}:${id}`, title: `${deployment.instance_name || deployment.id}：无法读取关联服务 ${id}` })
        continue
      }
      if (!host.environment || String(service.environment) !== String(host.environment)) {
        anomalies.push({ key: `${deployment.id}:${id}`, title: `${service.name} / ${deployment.instance_name || deployment.id}：主机与服务环境不一致或主机未设置环境` })
        continue
      }
      const system = systemMap.get(String(service.business_system))
      if (!system) {
        anomalies.push({ key: `${deployment.id}:${id}`, title: `${service.name}：无法读取所属业务系统` })
        continue
      }
      serviceIds.add(String(id))
      const project = branch(tree, `project:${system.project ?? 'none'}`, system.project_name || '未分配项目')
      const business = branch(project.children, `${project.key}:system:${system.id}`, system.name)
      const environment = branch(business.children, `${business.key}:environment:${host.environment}`, host.environment_name || `环境 ${host.environment}`)
      const node = branch(environment.children, `${environment.key}:service:${id}`, service.name)
      const query = {
        application_service_id: String(id), service_name: service.name,
        business_system_id: String(system.id), business_system_name: system.name,
        project_id: String(system.project || ''), project_name: system.project_name || '',
        environment_id: String(host.environment), environment_name: host.environment_name || '',
      }
      node.route = { path: '/assets/service-tree', query }
      node.children.push({
        key: `${node.key}:deployment:${deployment.id}`,
        title: deployment.instance_name || `部署 ${deployment.id}`,
        route: { path: '/assets/service-tree', query: { ...query, deployment_id: String(deployment.id) } },
      })
    }
  }
  return { tree, unlinked, anomalies, serviceCount: serviceIds.size, deploymentCount: hostDeployments.length }
}
