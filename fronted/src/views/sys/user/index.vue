
  
  <script setup>

// 缓存
  defineOptions({
    name: 'user'
})
  import { ref } from 'vue'
  import { getUserList } from '@/api/user/index.js';
  import { FontAwesomeIcon } from '@fortawesome/vue-fontawesome'
  import { faEdit } from '@fortawesome/free-solid-svg-icons'
  import { faTrash } from '@fortawesome/free-solid-svg-icons'
  const users = ref([])
  const columns = [
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

  const changeStatus = (e) => {
    
  }
  </script>
<template>
  <a-row class="search" :gutter="16">
    <a-col :span="7">
    <a-input-search
      v-model:value="value"
      placeholder="全局搜索"
      enter-button
      @search="onSearch"
    />

    </a-col>
  </a-row>
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
          <a-tag color="orange"
            v-for="role in record.roles"
            :key="role.id"
          >
            {{ role.name }}
          </a-tag>
        </span>
      </template>
            <template v-else-if="column.key === 'status'">
        <span>
          <a-switch :checked="record.status === 1"  @change="changeStatus" />
          
        </span>
      </template>
      <template v-else-if="column.key === 'action'">
        <span>
          <a-button type="primary" id="assignRole">分配角色</a-button>
          <div id="resetPwd" style="display: inline;">
              <a-button >重置密码</a-button>   
          </div>
          <FontAwesomeIcon :icon="faEdit" />
          <FontAwesomeIcon :icon="faTrash" />
      
        </span>
      </template>
    </template>
  </a-table>
</template>
  <style scoped>
  .search {
    margin-bottom: 20px;
  }
  #assignRole {
    /* background-color: '#1677ff' */
  }
  /* :deep(#resetPwd  .ant-btn-default) {
    background-color: '#1677ff';
  } */
  #resetPwd >:where(.css-dev-only-do-not-override-1p3hq3p).ant-btn-default {
      background-color: orange;
      color: white;
  }
</style>
 