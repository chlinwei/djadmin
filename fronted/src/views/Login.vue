<template>
    <div class="login">
        <a-form
      :model="loginForm"
      name="basic"
      :label-col="{ span: 4 }"
      :wrapper-col="{ span: 20 }"
      autocomplete="off"
      @finish="onFinish"
      @finishFailed="onFinishFailed"
      class="login-form"
    >
    <a-row>
    <a-col :span="24">
        <h3 class="title">djadmin 管理平台</h3>
    </a-col>
  </a-row>
    
      <a-form-item
        label="账号"
        name="username"
        :rules="[{ required: true, message: '请输入账号!' }]">
        
        <a-input v-model:value="loginForm.username">
            <template #prefix> <SvgIcon name="user" class="username-icon"></SvgIcon></template>
            </a-input>
      </a-form-item>
  
      <a-form-item
        label="密码"
        name="password"
        :rules="[{ required: true, message: '请输入密码!' }]"
      >
        <a-input-password v-model:value="loginForm.password">
            <template #prefix> <SvgIcon name="password" class="username-icon"></SvgIcon></template>
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
  import  Cookies from 'js-cookie';

 import  qs from 'qs'
  const loginForm = reactive({
    username: '',
    password: '',
    remember: false
  });

  
  const onFinish = values => {
    console.log(qs.stringify(loginForm.value));
    let result = requestUtil.post("auth/login" , qs.stringify(values));
    if(result.token) {
        //登录成功
        if( loginForm.remember === true) {
            Cookies.set("username",loginForm.username,{expires:30});
        Cookies.set("password",encrypt(loginForm.password,{expires:30}));
        Cookies.set("remember",loginForm.remember,{expires:30});
        }

    } 
    
    // windows.sessionStorage.setItem("token",data.token)
  };
  const onFinishFailed = errorInfo => {
    console.log('Failed:', errorInfo);
  };

  if(loginForm.remember) {
    Cookies.set("username",loginForm.username,{expires:30});
    Cookies.set("password",encrypt(loginForm.password,{expires:30}));
    Cookies.set("remember",loginForm.remember,{expires:30});
  }else {
    Cookies.remove("username");
    Cookies.remove("password");
    Cookies.remove("remember");
  }

  function getCookie() {
    var username = Cookies.get("username");
    var password = Cookies.get("password");
    console.log(username)
    // var remember = Cookies.get("remember");
    // loginForm= {
    //     username: username === undefined ? loginForm.username : username,
    //     password: password === undefined ? loginForm.password : decrypt(password),
    //     remember: remember === undefined ? false : Boolean(remember)
    // }
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
    height:400px;
    margin-top: 50px;
    
  }
  .login-form > a-form-item {
    height:100px;
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
