<template>
    <div class="login">
        <el-form :model="form" class="login-form">
            <h3 class="title">python222 Django后台管理系统</h3>
            <el-form-item label="账号">
                <el-input v-model="form.username" type="text" placeholder="账号" />
            </el-form-item>
            <el-form-item label="密码">
                <el-input v-model="form.password" type="password" placeholder="密码" />
            </el-form-item>
            <el-checkbox style="margin:0px 0px 25px 0px;">记住密码</el-checkbox>
            <el-form-item style="width:100%;">
                <el-button size="large" type="primary" style="width:100%;">
                    <span>登 录</span>
                </el-button>
            </el-form-item>
        </el-form>
        <!-- 底部 -->
        <!-- <div class="el-login-footer">
            <span>Copyright © 2013-2025 <a href="http://www.python222.com" target="_blank">python222.com</a> 版权所有.</span>
        </div> -->
    </div>
</template>
<script setup>
import { reactive } from 'vue'
const form = reactive({
    username: '',
    password: ''

})

</script>
<style lang="scss" scoped>
.login {
    display: flex;
    justify-content: center;
    align-items: center;
    height: 100%;
    background-image: url("../assets/images/login-background.jpg");
    background-size: cover;
    background-repeat: no-repeat;
}
.title {
    margin: 0px auto 30px auto;
    text-align: center;
    color: #707070;
}
.login-form {
    border-radius: 6px;
    background: #ffffff;
    width: 400px;
    padding: 25px 25px 5px 25px;
    .el-input {
        height: 40px;
    }
}
</style>