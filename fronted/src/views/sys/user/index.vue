<template>
  <a-table :columns="columns" :data-source="users">
    <template #headerCell="{ column }">
      <template v-if="column.key === 'name'">
        <span>
          Name
        </span>
      </template>
    </template>

    <template #bodyCell="{ column, record }">
      <template v-if="column.key === 'name'">
        <a>
          {{ record.name }}
        </a>
      </template>
      <template v-else-if="column.key === 'roles'">
        <span>
          <a-tag
            v-for="role in record.roles"
            :key="role.id"
          >
            {{ role.name }}
          </a-tag>
        </span>
      </template>
      <template v-else-if="column.key === 'action'">
        <span>
          <a>分配角色</a>
          <a>重置密码</a>
          <a>修改用户</a>
          <a>删除用户</a>
        </span>
      </template>
    </template>
  </a-table>
</template>
  
  <script setup>

// 缓存
  defineOptions({
    name: 'user'
})
  import { ref } from 'vue'
  import { getUserList } from '@/api/user/index.js';
  
  const users = ref([])
  const columns = [
    { title: 'id', dataIndex: 'id',key: 'id' },
    { title: '用户名', dataIndex: 'username',key: 'username' },
    { title: '角色', dataIndex: 'roles',key: 'roles' },
    { title: '邮箱', dataIndex: 'email',key: 'email' },
    { title: '手机号', dataIndex: 'phonenumber',key: 'phonenumber' },
    { title: '状态', dataIndex: 'status',key: 'status' },
    { title: '创建时间', dataIndex: 'create_time',key: 'create_time' },
    { title: '备注', dataIndex: 'remark',key: 'remark' },
    { title: '操作', key: 'action' }
  ]
  
getUserList().then((result)=>{
    if(result.data.code) {
        users.value =   result.data.data.results
        console.log(users.value)
    }
  })
  const editUser = (user) => {
    // 编辑逻辑
  }
  </script>
 