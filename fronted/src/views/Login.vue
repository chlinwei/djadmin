<template>
<div class="login">
    <el-row>
        <el-col class="form-wrapper">
            <el-form :model="form" label-width="auto">
        <h3 class="title">python222 Django后台管理系统</h3>
        <el-form-item label="账号">
            <el-input v-model="form.username" />
        </el-form-item>
        <el-form-item label="密码">
            <el-input v-model="form.password" />
        </el-form-item>
    </el-form>
        </el-col>

    </el-row>


</div>
</template>
<script setup>
import { reactive } from 'vue'
const form = reactive({
    username: 'hello',
    password: ''

})

</script>
<style lang="scss" scoped>

.login {
    display: flex;
    justify-content: center;
    align-items: center;
    background-image: url("../assets/images/login-background.jpg");
    background-size: cover;
    height:100%;

}
.title {
    margin: 0px auto 30px auto;
    text-align: center;
    color: #707070;

}
.form-wrapper {
    height:400px;
    // margin: 0px 50px;
}
.login-form {
  border-radius: 6px;
  background: #ffffff;
  padding: 25px 25px 5px 25px;
  .el-input {
    height: 40px;
    input {
      display: inline-block;
      height: 140px;
    }
  }
}
</style>