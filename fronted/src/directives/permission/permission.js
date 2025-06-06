import store from "@/store";

// 权限检查函数
export function checkPermission(permissionCode) {
    var  perms  = store.state.perms
    var role_codes = store.state.role_codes
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

 

