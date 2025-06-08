import requestUtil from '@/util/request'
// 获取列表
export function getCredentailList(params) {
    return requestUtil.get("assets/credential/",params)
}

// 保存或新增
export function SaveOrCreateCredential(credential) {
        if(credential.id == -1) {
            // 新增
            return requestUtil.post("assets/credential/",credential)
        } else {
            // 保存
            return requestUtil.patch("assets/credential/" + credential.id + "/" ,credential)
        }
}
// 获取详细
export function getCredentailById(id) {
    return requestUtil.get("assets/credential/" + id + "/")
}

