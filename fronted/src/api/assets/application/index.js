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

