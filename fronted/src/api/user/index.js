import requestUtil from '@/util/request'
import Cookies from 'js-cookie';
import { encrypt, decrypt } from "@/util/jsencrypt";

// 登录
export const doLogin = (data) => {
    return requestUtil.post("auth/login",data);
}

// 存储个人信息
export function saveCurrentUser(currentUser) {
    currentUser = JSON.stringify(currentUser);
    sessionStorage.setItem("currentUser",currentUser);
}

// 删除个人信息
export function removeCurrentUser(currentUser) {
    sessionStorage.removeItem("currentUser");
}
    

    

//存储token
export function saveToken(token) {
    sessionStorage.setItem("token",token);
}


//删除token
export function removeToken() {
    sessionStorage.removeItem("token");
}

//保存权限菜单
export function saveMenuList(menuList){
    menuList = JSON.stringify(menuList);
    sessionStorage.setItem("menuList",menuList);
}

//获取权限菜单
export function getMenuList(){
    let menuList = sessionStorage.getItem("menuList");
    return JSON.parse(menuList);
}
//删除权限菜单
export function removeMenuList(){
    sessionStorage.removeItem("menuList")
}

// 存储账号密码
export function setRemeberMe(user) {
    user.password = encrypt(user.password);
    user = JSON.stringify(user);
    Cookies.set("user",user, { expires: 30 });
}
// 获取账号密码
export function getRemeberMeInfo() {
    let user = Cookies.get("user");
    if(user) {
        user = JSON.parse(user);
        user.password = decrypt(user.password);
        return user;
    }
}
// 清除账号密码
export function clearRemeberMe() {
    Cookies.remove("user");
}
