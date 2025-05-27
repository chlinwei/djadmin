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
    return requestUtil.get("sys/roles/list/");
}
