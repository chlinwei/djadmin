import { watch } from 'vue'
import { isNavigationFailure, NavigationFailureType, useRoute, useRouter } from 'vue-router'
import { message } from 'ant-design-vue'

const scopeQueryFields = {
  projectId: 'project_id', projectName: 'project_name',
  businessSystemId: 'business_system_id', businessSystemName: 'business_system_name',
  environment: 'environment_id', environmentName: 'environment_name',
  applicationServiceId: 'application_service_id', serviceName: 'service_name',
  deploymentId: 'deployment_id', nodeTitle: 'node_title',
}

export function useServiceTreeLocation(scope, panel, { panels, label }) {
  const route = useRoute()
  const router = useRouter()
  const pagePath = route.path
  let pendingLocation = ''

  function selectScope(value) {
    scope.value = value
    void syncLocation()
  }

  async function syncLocation() {
    if (route.path !== pagePath) return
    const query = { ...route.query }
    for (const key of Object.values(scopeQueryFields)) delete query[key]
    delete query.node_type
    delete query.panel
    if (scope.value.nodeType !== 'all') {
      query.node_type = scope.value.nodeType
      for (const [field, key] of Object.entries(scopeQueryFields)) {
        const value = scope.value[field]
        if (value !== undefined && value !== null && value !== '') query[key] = String(value)
      }
    }
    if (panel.value !== panels[0]) query.panel = panel.value
    try {
      // Node navigation shares one workspace; replace avoids filling browser history on every selection.
      const location = { path: pagePath, query, hash: route.hash }
      pendingLocation = router.resolve(location).fullPath
      const failure = await router.replace(location)
      if (failure && !isNavigationFailure(failure, NavigationFailureType.duplicated | NavigationFailureType.cancelled)) {
        pendingLocation = ''
        message.error(`${label}位置未写入地址栏，请重试`)
      }
    } catch (error) {
      pendingLocation = ''
      message.error(`保存${label}位置失败：${error.message}`)
    }
  }

  function applyLocation() {
    const query = route.query
    const id = (key) => {
      const value = Number(query[key])
      return Number.isSafeInteger(value) && value > 0 ? value : undefined
    }
    const serviceId = id('application_service_id')
    const deploymentId = id('deployment_id')
    const nodeType = query.node_type || (serviceId ? (deploymentId ? 'deployment' : 'service') : 'all')
    const titles = {
      all: '全部业务', project: query.project_name || `项目 ${id('project_id')}`,
      businessSystem: query.business_system_name || `业务系统 ${id('business_system_id')}`,
      environment: query.environment_name || (id('environment_id') ? `环境 ${id('environment_id')}` : '未配置环境'),
      service: query.service_name || `服务 ${serviceId}`, deployment: `部署 ${deploymentId}`,
    }
    const valid = {
      all: true, project: !!id('project_id'), businessSystem: !!id('business_system_id'),
      environment: !!id('business_system_id'), service: !!serviceId, deployment: !!serviceId && !!deploymentId,
    }
    if (!Object.hasOwn(valid, nodeType) || !valid[nodeType]) {
      message.error(`${label}定位参数无效，请重新选择节点`)
      scope.value = { nodeType: 'all', nodeTitle: '全部业务' }
    } else {
      scope.value = nodeType === 'all' ? { nodeType: 'all', nodeTitle: '全部业务' } : {
        nodeType, nodeTitle: String(query.node_title || titles[nodeType]),
        projectId: id('project_id'), projectName: String(query.project_name || ''),
        businessSystemId: id('business_system_id'), businessSystemName: String(query.business_system_name || ''),
        environment: id('environment_id') ?? null, environmentName: String(query.environment_name || ''),
        applicationServiceId: serviceId, serviceName: String(query.service_name || ''),
        ...(nodeType === 'deployment' ? { deploymentId } : {}),
      }
    }
    panel.value = panels.includes(query.panel) ? query.panel : panels[0]
  }

  // Cached pages share the route object. Ignore other pages and own writes, which already hold temporary filters.
  watch(() => [route.path, route.query], () => {
    if (route.path === pagePath && route.fullPath !== pendingLocation) applyLocation()
  }, { immediate: true })

  return { selectScope, syncLocation }
}
