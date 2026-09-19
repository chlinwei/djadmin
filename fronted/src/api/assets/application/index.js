import requestUtil from '@/util/request'
// 获取列表
var prefix="assets/applications/"
export function getApplicationList(params) {
    return requestUtil.get(prefix,params)
}

// 保存或新增
export function SaveOrCreateApplication(obj) {
        if(obj.id == -1) {
            // 新增
            return requestUtil.post(prefix,obj)
        } else {
            // 保存
            return requestUtil.patch(prefix + obj.id + "/" ,obj)
        }
}
// 获取详细
export function getApplicationById(id) {
    return requestUtil.get(prefix + id + "/")
}

// 删除
export function batchDeleteApplication(ids) {
    return requestUtil.post(prefix +"batch-delete/",{"ids":ids})
}

const versionPrefix = 'assets/application-versions/'
const businessSystemPrefix = 'assets/business-systems/'
const projectPrefix = 'assets/projects/'
const businessEnvironmentPrefix = 'assets/business-environments/'
const clusterProfilePrefix = 'assets/cluster-profiles/'
const applicationServicePrefix = 'assets/application-services/'
const templatePrefix = 'assets/application-deployment-templates/'
const deploymentPrefix = 'assets/application-deployments/'

export function getBusinessSystemList(params) {
    return requestUtil.get(businessSystemPrefix, params)
}

export function getBusinessSystem(id) {
    return requestUtil.get(`${businessSystemPrefix}${id}/`)
}

export function saveBusinessSystem(obj) {
    if (obj.id) return requestUtil.patch(`${businessSystemPrefix}${obj.id}/`, obj)
    return requestUtil.post(businessSystemPrefix, obj)
}

export function batchDeleteBusinessSystems(ids) {
    return requestUtil.post(`${businessSystemPrefix}batch-delete/`, { ids })
}

export function getProjectList(params) {
    return requestUtil.get(projectPrefix, params)
}

export function getProject(id) {
    return requestUtil.get(`${projectPrefix}${id}/`)
}

export function saveProject(obj) {
    if (obj.id) return requestUtil.patch(`${projectPrefix}${obj.id}/`, obj)
    return requestUtil.post(projectPrefix, obj)
}

export function batchDeleteProjects(ids) {
    return requestUtil.post(`${projectPrefix}batch-delete/`, { ids })
}

export function getBusinessEnvironmentList(params) {
    return requestUtil.get(businessEnvironmentPrefix, params)
}

export function getBusinessEnvironment(id) {
    return requestUtil.get(`${businessEnvironmentPrefix}${id}/`)
}

export function saveBusinessEnvironment(obj) {
    if (obj.id) return requestUtil.patch(`${businessEnvironmentPrefix}${obj.id}/`, obj)
    return requestUtil.post(businessEnvironmentPrefix, obj)
}

export function batchDeleteBusinessEnvironments(ids) {
    return requestUtil.post(`${businessEnvironmentPrefix}batch-delete/`, { ids })
}

export function getClusterProfileList(params) {
    return requestUtil.get(clusterProfilePrefix, params)
}

export function getClusterProfile(id) {
    return requestUtil.get(`${clusterProfilePrefix}${id}/`)
}

export function saveClusterProfile(obj) {
    if (obj.id) return requestUtil.patch(`${clusterProfilePrefix}${obj.id}/`, obj)
    return requestUtil.post(clusterProfilePrefix, obj)
}

export function batchDeleteClusterProfiles(ids) {
    return requestUtil.post(`${clusterProfilePrefix}batch-delete/`, { ids })
}

export function getApplicationServiceList(params) {
    return requestUtil.get(applicationServicePrefix, params)
}

export function getApplicationService(id) {
    return requestUtil.get(`${applicationServicePrefix}${id}/`)
}

export function getApplicationServiceLogConfig(id) {
    return requestUtil.get(`${applicationServicePrefix}${id}/log-config/`)
}

// 服务级日志采集总开关（log_collection_enabled）。关闭后该服务下所有日志都不采集，
// 逐条日志的开关随之不生效（配置意图仍保留，重新打开即恢复）。
export function setApplicationServiceLogCollection(id, enabled) {
    return requestUtil.post(`${applicationServicePrefix}${id}/log-collection/`, { enabled })
}

// 按行保存一条（服务 × 日志定义）的日志覆盖值：采集开关与保留档位。
// payload = { log_definition_id, collection_enabled, retention_tier }
// **两列都是这一行覆盖值的权威值**：后端按"提交即该行覆盖值"写入，少传一列等于把它清成
// "不覆盖"（采集默认采、档位继承服务默认），所以调用方要么传全，要么用页面里的 saveOverride 合并。
export function saveApplicationServiceLogSetting(id, payload) {
    return requestUtil.post(`${applicationServicePrefix}${id}/log-config/settings/`, payload)
}

// 日志格式认证：对一条（逻辑服务 × 日志定义）抽样校验一次格式。
// payload = { log_definition_id, source: 'instance' | 'sample_log' | 'waiver', deployment_id }
// （source=instance 时必填 deployment_id）。漏字段取不到样例时后端报错，不会静默判"通过"。
export function verifyApplicationServiceLogFormat(id, payload, timeout = 120000) {
    return requestUtil.post(`${applicationServicePrefix}${id}/log-config/verify/`, payload, timeout)
}

export function saveApplicationService(obj) {
    if (obj.id) return requestUtil.patch(`${applicationServicePrefix}${obj.id}/`, obj)
    return requestUtil.post(applicationServicePrefix, obj)
}

export function batchDeleteApplicationServices(ids) {
    return requestUtil.post(`${applicationServicePrefix}batch-delete/`, { ids })
}

export function refreshApplicationServiceRuntimeStatus(id, timeout = 120000) {
    return requestUtil.post(`${applicationServicePrefix}${id}/refresh-runtime-status/`, {}, timeout)
}

export function getApplicationVersionList(params) {
    return requestUtil.get(versionPrefix, params)
}

export function saveApplicationVersion(obj) {
    if (obj.id) return requestUtil.patch(`${versionPrefix}${obj.id}/`, obj)
    return requestUtil.post(versionPrefix, obj)
}

export function batchDeleteApplicationVersions(ids) {
    return requestUtil.post(`${versionPrefix}batch-delete/`, { ids })
}

export function getApplicationDeploymentTemplateList(params) {
    return requestUtil.get(templatePrefix, params)
}

export function getApplicationDeploymentTemplate(id) {
    return requestUtil.get(`${templatePrefix}${id}/`)
}

export function saveApplicationDeploymentTemplate(obj) {
    if (obj.id) return requestUtil.patch(`${templatePrefix}${obj.id}/`, obj)
    return requestUtil.post(templatePrefix, obj)
}

export function batchDeleteApplicationDeploymentTemplates(ids) {
    return requestUtil.post(`${templatePrefix}batch-delete/`, { ids })
}

export function getApplicationDeploymentList(params) {
    return requestUtil.get(deploymentPrefix, params)
}

export function getApplicationDeployment(id) {
    return requestUtil.get(`${deploymentPrefix}${id}/`)
}

export function saveApplicationDeployment(obj) {
    if (obj.id) return requestUtil.patch(`${deploymentPrefix}${obj.id}/`, obj)
    return requestUtil.post(deploymentPrefix, obj)
}

export function batchDeleteApplicationDeployments(ids) {
    return requestUtil.post(`${deploymentPrefix}batch-delete/`, { ids })
}

export function controlApplicationDeployment(id, action, options = {}) {
    return requestUtil.post(`${deploymentPrefix}${id}/control/`, { action }, null, options)
}

