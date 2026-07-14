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

export function collectHostInfo(id) {
    return requestUtil.post(prefix + `${id}/collect-info/`)
}

export function batchCollectHostInfo(ids) {
    return requestUtil.post(prefix + 'batch-collect-info/', { ids })
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

export function getHostWebSshFiles(id, path) {
    return requestUtil.get(prefix + `${id}/files/list/`, { path })
}

export function uploadHostWebSshFile(id, formData, options = {}) {
    const transferBaseUrl = String(requestUtil.getTransferServerUrl?.() || '').replace(/\/$/, '')
    const uploadPath = `${prefix}${id}/files/upload/chunk/`
    const uploadUrl = transferBaseUrl ? `${transferBaseUrl}/${uploadPath}` : uploadPath
    return requestUtil.fileUpload(uploadUrl, formData, options)
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