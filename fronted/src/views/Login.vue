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
import requestUtil from '@/util/request';
import { encrypt, decrypt } from "@/util/jsencrypt";
import Cookies from 'js-cookie';
import router from '@/router'

import qs from 'qs'
const loginForm = reactive({
    username: '',
    password: '',
    remember: false
});



function clear_cookie() {
    Cookies.remove("username");
    Cookies.remove("password");
    Cookies.remove("remember");

}
const onFinish = values => {
    requestUtil.post("auth/login", qs.stringify(values)).then(result => {
    let data = result.data
    if (data.code == 200) {
         //token存在session storage
        sessionStorage.setItem("token",data.data.token)
        //currentUser存到session storage中
        sessionStorage.setItem("currentUser",JSON.stringify(data.data.currentUser))
        //menuList,存到session storage
        sessionStorage.setItem("menuList",JSON.stringify(data.data.menuList))
        
        //登录成功
        if (loginForm.remember == true) {
            // 30天过期
            Cookies.set("username", loginForm.username, { expires: 30 });
            Cookies.set("password", encrypt(loginForm.password, { expires: 30 }));
            Cookies.set("remember", true, { expires: 30 });
        } else {
            //没有remember就清楚cookie
            clear_cookie()
        }
        //跳转主页
        router.replace("/")
    }else {
        clear_cookie()
    }
    })
    
};

const onFinishFailed = errorInfo => {
    console.log('Failed:', errorInfo);
};



function getCookie() {
    const username = Cookies.get("username");
    const password = Cookies.get("password");
    const remember = Cookies.get("remember");
    if (username) {
        loginForm.username = username
    }
    if(password) {
        loginForm.password = decrypt(password)
    }
    if(remember) {
        loginForm.remember = Boolean(remember)
        Cookies.set("username", loginForm.username, { expires: 30 });
        Cookies.set("password", encrypt(loginForm.password, { expires: 30 }));
        Cookies.set("remember", loginForm.remember, { expires: 30 });
    }
}
getCookie()

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
