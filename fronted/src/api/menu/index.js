import requestUtil from '@/util/request'
// 获取权限树
export const getMenuTree = () => {
    return requestUtil.get("sys/menus/menuTree/");
}