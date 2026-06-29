<template>
  <a-space>
    <a-button type="primary" @click="handleLogin">测试登录</a-button>
    <a-button danger @click="handleUserList">测试获取用户列表信息</a-button>
  </a-space>
</template>

<script setup>
import requestUtil from '@/util/request'

const handleLogin = async () => {
  const result = await requestUtil.get('user/test')
  const data = result.data
  if (data.code == 200) {
    const token = data.token
    console.log('登录成功：token=' + token)
    window.sessionStorage.setItem('token', token)
  } else {
    console.log('登录出错！')
  }
}

const handleUserList = async () => {
  const result = await requestUtil.get('user/users')
  const data = result.data
  if (data.code == 200) {
    const userList = data.data
    console.log('用户列表信息：userList=' + userList)
  } else {
    console.log('出错！')
  }
}
</script>

<style>
</style>
  