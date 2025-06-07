import requestUtil from '@/util/request'

// 获取用户角色列表
export const getCurrentUserRoleList = () => {
    return requestUtil.get("sys/roles/getCurrentUserRoleList/");
}





// 获取角色列表
export const getRoleList = (params) => {
    return requestUtil.get("sys/roles/",params);
}

// 保存角色根据角色id
export const savOrCreateRole= (role) => {
    if(role.id!=-1){
        //保存
        return requestUtil.patch("sys/roles/" + role.id + "/",role);
    }else {
        // 新增
        return requestUtil.post("sys/roles/",role);
    }
    
}

// 根据角色id查询角色
export const getRoleById = (id) => {
    if(id){
         return requestUtil.get("sys/roles/" + id + "/");
    } 
}

// 批量删除角色
export function batchDeleteRole(ids) {
    if(ids.length <=0) {
        message.error("角色id数组必须大于1")
        return;
    }else {
        return requestUtil.del("sys/roles/batch-delete/",{role_ids:ids})
    }
}

// 保存一个用户的角色列表到Localstorage
export function saveRoleCodes(role_codes) {
    var data = JSON.stringify(role_codes)
    localStorage.setItem("role_codes", data);
}

// 获取角色列表从Localstorage
export function getRoleCodes() {
    var data = localStorage.getItem("role_codes");
    return JSON.parse(data) 
}