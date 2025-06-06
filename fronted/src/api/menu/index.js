import requestUtil from '@/util/request'
import store from '@/store'
// 获取权限树
export const getMenuTree = () => {
    return requestUtil.get("sys/menus/getMenuTree/");
}

// 根据角色id获取其权限列表
export const getMenuListByRoleId = (role_id) => {
    return requestUtil.get("sys/menus/getMenuListByRoleId/",{"role_id":role_id})
}

// 分配角色相关的权限
export const grantMenu =  (role_id,menuIds) =>{
    return requestUtil.post("sys/menus/grantMenu/",{role_id:role_id,menuIds:menuIds})
}
export const saveOrCreateMenu = (menu) => {
    if(menu.id != -1) {
        // 保存
        // return requestUtil.post("sys/menus/CreateMenuView/",menu)
        return requestUtil.patch("sys/menus/" + menu.id + "/",menu)
    } else {
        // 新增
        return requestUtil.post("sys/menus/",menu)
    }
    
}

// 获取一个菜单menu
export const getMenuById = (id) => {
    return requestUtil.get("sys/menus/" + id + "/")
}

// 删除一个menu
export const deleteMenuById =(id) => {
    return requestUtil.del("/sys/menus/deleteMenuById/",{"id":id})
}
// 根据menuList生成pers
//获取权限标识
function extractPerms(menuTree) {
  const perms = [];
  function traverse(nodes) {
    nodes.forEach(node => {
      if (node.perms && node.perms.trim()) {
        perms.push(node.perms);
      }
      if (node.children) {
        traverse(node.children);
      }
    });
  }

  traverse(menuTree);
  return perms;
}
function setPerms(perms) {
    //将权限存储在vuex中
    store.commit("add_perms",perms)
    perms = JSON.stringify(perms);
    // 将权限存储在localstorage中
    localStorage.setItem("perms", perms);
}
function removePerms() {
     localStorage.removeItem("perms")
}
export function getPerms() {
    let perms = localStorage.getItem("perms");
    return JSON.parse(perms);
}



//保存权限菜单
export function saveMenuList(menuList) {
    setPerms(extractPerms(menuList))
    menuList = JSON.stringify(menuList);
    localStorage.setItem("menuList", menuList);
    
}

//获取权限菜单
export function getMenuList() {
    let menuList = localStorage.getItem("menuList");
    return JSON.parse(menuList);
}
//删除权限菜单
export function removeMenuList() {
    localStorage.removeItem("menuList")
    removePerms()
}








