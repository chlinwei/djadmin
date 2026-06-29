import requestUtil from '@/util/request'

const prefix = 'assets/host-groups/'

export function getHostGroupTree(params) {
    return requestUtil.get(prefix + 'tree/', params)
}

export function getHostGroupList(params) {
    return requestUtil.get(prefix, params)
}

export function getHostGroupById(id) {
    return requestUtil.get(prefix + id + '/')
}

export function saveOrCreateHostGroup(group) {
    if (group.id !== -1) {
        return requestUtil.patch(prefix + group.id + '/', group)
    }
    return requestUtil.post(prefix, group)
}

export function deleteHostGroupById(id) {
    return requestUtil.del(prefix + id + '/', { id })
}

export function batchDeleteHostGroup(ids) {
    return requestUtil.del(prefix + 'batch-delete/', { ids })
}