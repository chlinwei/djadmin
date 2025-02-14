import requestUtil from '@/util/request'

// 获取用户角色列表
export const getCurrentUserRoleList = () => {
    return requestUtil.get("role/getCurrentUserRoleList");
}