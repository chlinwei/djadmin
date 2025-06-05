import { getPerms } from "@/api/menu";
import { getCurrentUser } from "@/api/user";

// 权限检查函数
export function checkPermission(permissionCode) {
    const perms  = getPerms();
 // 从Vuex获取权限列表
    return perms.includes(permissionCode)
}

