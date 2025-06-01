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
export const saveOrCreateMenu = (menu) => {
    if(menu.id != -1) {
        // 保存
        // return requestUtil.post("sys/menus/CreateMenuView/",menu)
        return requestUtil.patch("sys/menus/CreateOrUpdateMenuView/" + menu.id + "/",menu)
    } else {
        // 新增
        return requestUtil.post("sys/menus/CreateOrUpdateMenuView/",menu)
    }
    
}

// 获取一个菜单menu
export const getMenuById = (id) => {
    return requestUtil.get("sys/menus/GetMenuById/",{"id":id})
}

// 删除一个menu
export const deleteMenuById =(id) => {
    return requestUtil.del("/sys/menus/DeleteMenuById/",{"id":id})
}