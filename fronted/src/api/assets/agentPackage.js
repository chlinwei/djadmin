import requestUtil from '@/util/request'

const prefix = 'api/agent/packages/'

// dj-agent 二进制包：单包语义，服务端仅保留一个当前包
export function listAgentPackages(params) {
    return requestUtil.get(prefix, params)
}

// 上传 dj-agent 二进制包（覆盖已有包）：multipart（file），成功返回新记录
export function uploadAgentPackage(file) {
    const formData = new FormData()
    formData.append('file', file)
    return requestUtil.fileUpload(prefix + 'upload/', formData)
}

// 下载当前包（blob，走统一鉴权）
export function downloadAgentPackage() {
    return requestUtil.download(prefix + 'download/')
}

// 删除当前包（删除约定：唯一批删接口，单删传 ids: [id]）
export function batchDeleteAgentPackages(ids) {
    return requestUtil.post(prefix + 'batch-delete/', { ids })
}
