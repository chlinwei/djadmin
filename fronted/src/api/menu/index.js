import requestUtil from '@/util/request'
// 获取权限树
export const getMenuTree = () => {
    return requestUtil.get("sys/menus/menuTree/");
}

// 根据角色id获取其权限列表
export const getMenuListByRoleId = (role_id) => {
    return requestUtil.get("sys/menus/GetMenuListByRoleId/",{"role_id":role_id})
}

// 分配角色相关的权限
export const grantMenu =  (role_id,menuIds) =>{
    return requestUtil.post("sys/menus/GrantMenu/",{role_id:role_id,menuIds:menuIds})
}