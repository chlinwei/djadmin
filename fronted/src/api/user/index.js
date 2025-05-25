import requestUtil from '@/util/request'
import Cookies from 'js-cookie';
import { encrypt, decrypt } from "@/util/jsencrypt";
import { message } from 'ant-design-vue';
import router from '@/router';
// 登录
export const doLogin = (data) => {
    return requestUtil.post("sys/login", data);
}

// 存储个人信息
export function saveCurrentUser(currentUser) {
    currentUser = JSON.stringify(currentUser);
    localStorage.setItem("currentUser", currentUser);
}

// 删除个人信息
export function removeCurrentUser() {
    localStorage.removeItem("currentUser");
}

// 获取个人信息
export function getCurrentUser() {
    let currentUser = localStorage.getItem("currentUser");
    return JSON.parse(currentUser);
}

//存储token
export function saveToken(token) {
    localStorage.setItem("token", token);
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
export function saveMenuList(menuList) {
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
}

// 存储账号密码
export function setRemeberMe(user) {
    user.password = encrypt(user.password);
    user = JSON.stringify(user);
    Cookies.set("user", user, { expires: 30 });
}
// 获取账号密码
export function getRemeberMeInfo() {
    let user = Cookies.get("user");
    if (user) {
        user = JSON.parse(user);
        user.password = decrypt(user.password);
        return user;
    }
}
// 清除账号密码
export function clearRemeberMe() {
    Cookies.remove("user");
}


//更新基本资料
export function updateUserInfo(user, callback) {
    requestUtil.post("sys/updateUserInfo", user).then(result => {
        saveCurrentUser(result.data.data.user);
        callback(result);
    })
}
//获取用户列表
export function getUserList(params = {page:1,size:3,keyword}) {
   return requestUtil.get("sys/userList",params)
}


// 修改用户密码
export function updateUserPassword(password_pair) {
    requestUtil.post("sys/updateUserPassword", password_pair).then(result => {

        if (result.data.code == 200) {
            message.success("密码更新成功，请重新登陆...");
            // 删除个人信息
            removeCurrentUser();
            // 删除token
            removeToken();
            router.push("/login");
        } else if (result.data.code == 1001) {
            message.error("旧密码错误，请重新输入旧密码...");
        } else {
            message.error("服务器错误...");
        }

    })
}
// 修改用户状态
export function changeUserStatus(user_id,status) {
    return requestUtil.post("sys/user/changeStatus",{user_id:user_id,status:status})
}

// 根据用户id获取用户信息
export function getUserById(user_id) {
    return requestUtil.get("sys/users/" + user_id + '/');
}

// 保存用户信息
export function saveUserInfo(userInfo) {
    return requestUtil.patch("sys/users/" + userInfo.id + "/",userInfo)
}
// 新增用户
export function addUser(userInfo) {
    return requestUtil.post("sys/users/" ,userInfo)
}

// 检查用户是否存在
export function checkUserName(username) {
    return requestUtil.get("sys/users/CheckUsername",{"username": username});
}