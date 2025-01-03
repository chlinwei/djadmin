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
    localStorage.setItem("currentUser",currentUser);
}

// 删除个人信息
export function removeCurrentUser(currentUser) {
    localStorage.removeItem("currentUser");
}

// 获取个人信息
export function getCurrentUser() {
    let currentUser = localStorage.getItem("currentUser");
    return JSON.parse(currentUser);
}
    
//存储token
export function saveToken(token) {
    localStorage.setItem("token",token);
}
//获取token
export function getToken() {
    return localStorage.getItem("token");
}


//删除token
export function removeToken() {
    localStorage.removeItem("token");
}

//保存权限菜单
export function saveMenuList(menuList){
    menuList = JSON.stringify(menuList);
    localStorage.setItem("menuList",menuList);
}

//获取权限菜单
export function getMenuList(){
    let menuList = localStorage.getItem("menuList");
    return JSON.parse(menuList);
}
//删除权限菜单
export function removeMenuList(){
    localStorage.removeItem("menuList")
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
