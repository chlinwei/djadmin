<template>
    <div class="login">
        <a-form :model="loginForm" name="basic" :label-col="{ span: 4 }" :wrapper-col="{ span: 20 }" autocomplete="off"
            @finish="onFinish" @finishFailed="onFinishFailed" class="login-form">
            <a-row>
                <a-col :span="24">
                    <h3 class="title">djadmin 管理平台</h3>
                </a-col>
            </a-row>

            <a-form-item label="账号" name="username" :rules="[{ required: true, message: '请输入账号!' }]">

                <a-input v-model:value="loginForm.username">
                    <template #prefix>
                        <SvgIcon name="user" class="username-icon"></SvgIcon>
                    </template>
                </a-input>
            </a-form-item>

            <a-form-item label="密码" name="password" :rules="[{ required: true, message: '请输入密码!' }]">
                <a-input-password v-model:value="loginForm.password">
                    <template #prefix>
                        <SvgIcon name="password" class="username-icon"></SvgIcon>
                    </template>
                </a-input-password>
            </a-form-item>

            <a-form-item name="remember" :wrapper-col="{ offset: 0, span: 16 }">
                <a-checkbox v-model:checked="loginForm.remember">记住密码</a-checkbox>
            </a-form-item>

            <a-form-item :wrapper-col="{ offset: 0, span: 24 }" class="login-btn">
                <a-button type="primary" html-type="submit" style="width: 100%;"><span>登录</span></a-button>
            </a-form-item>
        </a-form>
    </div>
</template>
<script setup>
import { reactive } from 'vue';
import router from '@/router'
import {addDynamicRoutes} from '@/router/index.js';
import { message } from 'ant-design-vue';
import {doLogin,saveCurrentUser,saveToken,setRemeberMe,clearRemeberMe,getRemeberMeInfo} from '@/api/user/index.js'
import {saveMenuList} from '@/api/menu/index.js'
import {saveRoleCodes} from '@/api/role/index.js'
import qs from 'qs'
const loginForm = reactive({
    username: '',
    password: '',
    remember: false
});

const onFinish = values => {
    doLogin(qs.stringify(values)).then(result => {
    let data = result.data
    if (data.code == 200) {
        saveToken(data.data.token);
        saveCurrentUser(data.data.currentUser);
        saveMenuList(data.data.menuList);
        saveRoleCodes(data.data.role_codes);
        //登录成功
        if (loginForm.remember == true) {
            // 30天过期
            const user = {
                username: loginForm.username,
                password: loginForm.password,
                remember: true
            }
            setRemeberMe(user);
        } else {
            clearRemeberMe()
        }
        //登录成功后，加载动态路由
        addDynamicRoutes();
        //跳转主页
        router.replace("/index")
    }else {
        message.error("用户或者密码错误，请重新登陆...")
        clearRemeberMe()
    }
    })
    
};

const onFinishFailed = errorInfo => {
    console.log('Failed:', errorInfo);
};

function setRemeberMeInfo() {
    let user = getRemeberMeInfo();
    if(user) {
        loginForm.username = user.username;
        loginForm.password = user.password;
        loginForm.remember = Boolean(user.remember);
    }
}
setRemeberMeInfo()
</script>

<style scoped>
.login {
    display: flex;
    justify-content: center;
    height: 100%;
    background-image: url("../assets/images/login-background.jpg");
    background-repeat: no-repeat;
    background-size: cover;
}

.login-form {
    padding: 25px 25px 5px 25px;
    background-color: white;
    height: 400px;
    margin-top: 50px;

}

.login-form>a-form-item {
    height: 100px;
}

.title {
    font-size: 20px;
    margin-bottom: 30px;
    text-align: center;

}

.login-btn {
    width: 100%;
    display: inline-block;
}
</style>
