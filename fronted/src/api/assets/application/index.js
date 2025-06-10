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
    return requestUtil.del(prefix +"batch-delete/",{"ids":ids})
}

