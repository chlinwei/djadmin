import requestUtil from '@/util/request'

const prefix = 'api/agent/packages/'

// dj-agent 二进制包列表；激活包有且仅有一个（is_active 唯一）
export function listAgentPackages(params) {
    return requestUtil.get(prefix, params)
}

// 上传 dj-agent 二进制包：multipart（file + version），成功返回新记录
export function uploadAgentPackage({ version, file }) {
    const formData = new FormData()
    formData.append('version', String(version || ''))
    formData.append('file', file)
    return requestUtil.fileUpload(prefix + 'upload/', formData)
}

// 设为激活包（唯一激活，后端负责取消旧激活）
export function activateAgentPackage(id) {
    return requestUtil.post(prefix + id + '/activate/', {})
}

// 删除约定：唯一批删接口，单删传 ids: [id]
export function batchDeleteAgentPackages(ids) {
    return requestUtil.post(prefix + 'batch-delete/', { ids })
}
