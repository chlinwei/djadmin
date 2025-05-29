import requestUtil from '@/util/request'

// 获取用户角色列表
export const getCurrentUserRoleList = () => {
    return requestUtil.get("sys/roles/getCurrentUserRoleList/");
}



// 获取用户角色列表根据用户id
export const getUserRoleListByUserId = (user_id) => {
    return requestUtil.get("sys/roles/getUserRoleList/",{"user_id":user_id});
}

// 获取角色列表
export const getRoleList = () => {
    return requestUtil.get("sys/roles/");
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