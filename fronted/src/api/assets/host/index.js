import requestUtil from '@/util/request'

const prefix = 'assets/hosts/'

export function getHostList(params) {
    return requestUtil.get(prefix, params)
}

export function getHostById(id) {
    return requestUtil.get(prefix + id + '/')
}

export function saveOrCreateHost(obj) {
    if (obj.id === -1) {
        return requestUtil.post(prefix, obj)
    }
    return requestUtil.patch(prefix + obj.id + '/', obj)
}

export function deleteHostById(id) {
    return requestUtil.del(prefix + id + '/', { id })
}

export function batchDeleteHost(ids) {
    return requestUtil.del(prefix + 'batch-delete/', { ids })
}

export function checkHostConnection(id) {
    return requestUtil.post(prefix + 'check-connection/', { id })
}

export function batchCheckHostConnection(ids) {
    return requestUtil.post(prefix + 'batch-check-connection/', { ids })
}

export function getHostWebSshSessions(id, params) {
    return requestUtil.get(prefix + `${id}/webssh-sessions/`, params)
}

export function getHostWebSshActiveCount(id) {
    return requestUtil.get(prefix + `${id}/webssh-active-count/`)
}

export function getHostWebSshActiveSessions(id) {
    return requestUtil.get(prefix + `${id}/webssh-active-sessions/`)
}

export function getHostAgentRuntimeStatus(id) {
    return requestUtil.get(prefix + `${id}/agent-runtime-status/`)
}

export function refreshHostInfo(id) {
    return requestUtil.post(prefix + `${id}/refresh-info/`, {})
}

export function batchRefreshHostInfo(ids) {
    return requestUtil.post(prefix + 'refresh-info/', { ids })
}

export function getHostWebSshFiles(id, path) {
    return requestUtil.get(prefix + `${id}/files/list/`, { path })
}

export function uploadHostWebSshFile(id, formData, options = {}) {
    const uploadPath = `${prefix}${id}/files/upload/chunk/`
    // 上传候选地址：优先主服务（9000），失败再回退到独立传输服务（9101）。
    // 与下载的多候选回退逻辑保持一致——独立 runtransfer 进程未启动时仍可通过主服务上传。
    const candidates = [
        String(requestUtil.getServerUrl?.() || '').replace(/\/$/, ''),
        String(requestUtil.getTransferServerUrl?.() || '').replace(/\/$/, ''),
    ].filter(Boolean)
    const uniqueBases = [...new Set(candidates)]
    const uploadUrls = uniqueBases.length
        ? uniqueBases.map((base) => `${base}/${uploadPath}`)
        : [uploadPath]

    // 逐个候选地址尝试；仅当"网络层失败"（服务不可达/无响应）时才回退，
    // 业务错误（后端已响应的非 2xx）不回退，直接抛出交页面处理。
    const attempt = async (index) => {
        try {
            return await requestUtil.fileUpload(uploadUrls[index], formData, options)
        } catch (error) {
            const isNetworkError = !error?.response
                && error?.name !== 'CanceledError'
                && error?.code !== 'ERR_CANCELED'
            if (isNetworkError && index < uploadUrls.length - 1) {
                return attempt(index + 1)
            }
            throw error
        }
    }
    return attempt(0)
}

export function renameHostWebSshFile(id, payload) {
    return requestUtil.post(prefix + `${id}/files/rename/`, payload)
}

export function deleteHostWebSshFile(id, payload) {
    return requestUtil.del(prefix + `${id}/files/delete/`, payload)
}

export function createHostWebSshDir(id, payload) {
    return requestUtil.post(prefix + `${id}/files/create-dir/`, payload)
}

export function createHostWebSshFile(id, payload) {
    return requestUtil.post(prefix + `${id}/files/create-file/`, payload)
}

export function getHostGroupList(params) {
    return requestUtil.get('assets/host-groups/', params)
}

export function getCredentialOptionList(params) {
    return requestUtil.get('assets/credentials/', params)
}

export function createAgentJob(payload) {
    return requestUtil.post('api/agent/jobs/create', payload)
}

export function queryAgentJobs(params) {
    return requestUtil.get('api/agent/jobs/query', params)
}

export function queryHostDynamicTasks(hostId, options = {}) {
    const params = {
        host_id: hostId,
        page: options.page ?? 1,
        size: options.size ?? 20,
    }
    if (options.action) {
        params.action = options.action
    }
    if (options.groupBy) {
        params.group_by = options.groupBy
    }
    return requestUtil.get('api/agent/jobs/query', params)
}

export function cancelAgentJob(payload) {
    return requestUtil.post('api/agent/jobs/cancel', payload)
}

export function retryAgentJob(payload) {
    return requestUtil.post('api/agent/jobs/retry', payload)
}