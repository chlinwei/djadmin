import store from "@/store";

// 权限检查函数
export function checkPermission(permissionCode) {
    // 会话状态缺失时按无授权处理，不能让权限检查异常中断组件挂载。
    const perms = Array.isArray(store.state.perms) ? store.state.perms : []
    const role_codes = Array.isArray(store.state.role_codes) ? store.state.role_codes : []
    if(role_codes.includes("admin")){
        // 是admin
        return true
    }
    if(Array.isArray(permissionCode)){
        const perms_set = new Set(perms)
        return permissionCode.some(item => perms_set.has(item))
    }else if(typeof permissionCode === 'string') {
        return perms.includes(permissionCode)
    }
    return false
    
}

 

