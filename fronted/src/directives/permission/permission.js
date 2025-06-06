import { getPerms } from "@/api/menu";
import store from "@/store";

// 权限检查函数
export function checkPermission(permissionCode) {
    var  perms  = store.state.perms
    return perms.includes(permissionCode)
}

