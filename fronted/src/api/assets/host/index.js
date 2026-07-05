import requestUtil from '@/util/request'

const prefix = 'assets/hosts/'
const getTransferBaseUrl = () => String(requestUtil.getTransferServerUrl() || '').replace(/\/$/, '')
const buildTransferUrl = (path) => `${getTransferBaseUrl()}${path}`

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

export function downloadHostWebSshFile(id, path) {
    return requestUtil.download(prefix + `${id}/files/download/`, { path }, 0)
}

export function getHostWebSshDownloadTicket(id, payload) {
    return requestUtil.post(prefix + `${id}/files/download-ticket/`, payload)
}

export function getHostWebSshUploadTicket(id, payload) {
    return requestUtil.post(prefix + `${id}/files/upload-ticket/`, payload)
}

export function uploadHostWebSshFile(id, formData, options = {}) {
    return requestUtil.fileUpload(prefix + `${id}/files/upload/`, formData, options)
}

export function uploadHostWebSshFileChunk(id, formData, options = {}) {
    return requestUtil.fileUpload(prefix + `${id}/files/upload/chunk/`, formData, options)
}

export function uploadHostWebSshFileChunkByTicket(formData, options = {}) {
    return requestUtil.fileUpload(buildTransferUrl('/transfer/upload/chunk/'), formData, options)
}

export function getHostWebSshFileUploadStatus(id, params) {
    return requestUtil.get(prefix + `${id}/files/upload/status/`, params)
}

export function getHostWebSshFileUploadStatusByTicket(params) {
    return requestUtil.get(buildTransferUrl('/transfer/upload/status/'), params)
}

export function cancelHostWebSshFileUpload(id, payload) {
    return requestUtil.post(prefix + `${id}/files/upload/cancel/`, payload)
}

export function cancelHostWebSshFileUploadByTicket(payload) {
    return requestUtil.post(buildTransferUrl('/transfer/upload/cancel/'), payload)
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