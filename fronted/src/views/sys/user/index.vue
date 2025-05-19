<template>
    <a-table :columns="columns" :dataSource="users" rowKey="id">
      <template #action="{ record }">
        <a-button @click="editUser(record)">编辑</a-button>
      </template>
    </a-table>
  </template>
  
  <script setup>
  import { ref, onMounted } from 'vue'
  import axios from 'axios'
  import { getUserList } from '@/api/user/index.js';
  
  const users = ref([])
  const columns = [
    { title: '用户名', dataIndex: 'username' },
    { title: '邮箱', dataIndex: 'email' },
    { title: '操作', slots: { customRender: 'action' } }
  ]
  
  let userList =  getUserList().then((result)=>{
    if(result.data.code) {
        users.value =   result.data.data.results
    }
  })
  const editUser = (user) => {
    // 编辑逻辑
  }
  </script>
  