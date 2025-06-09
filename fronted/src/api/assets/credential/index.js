import requestUtil from '@/util/request'
// 获取列表
export function getCredentailList(params) {
    return requestUtil.get("assets/credentials/",params)
}

// 保存或新增
export function SaveOrCreateCredential(credential) {
        if(credential.id == -1) {
            // 新增
            return requestUtil.post("assets/credentials/",credential)
        } else {
            // 保存
            return requestUtil.patch("assets/credentials/" + credential.id + "/" ,credential)
        }
}
// 获取详细
export function getCredentailById(id) {
    return requestUtil.get("assets/credentials/" + id + "/")
}

// 删除
export function batchDeleteCredential(ids) {
    return requestUtil.del("assets/credentials/batch-delete/",{"ids":ids})
}

// 批量导入
export function batchUpload(file) {
    var header = {
      header: { 'Content-Type': 'multipart/form-data' }
    }
    return requestUtil.fileUpload("assets/credentials/batch-create/",file,header)
}