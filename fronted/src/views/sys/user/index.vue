<template>
    <Dialog :open="open" @update:open="(value)=>{open=value}" :user_id="user_id" :title="title"/>
    <a-row class="search" :gutter="16">
      <a-col :span="7">
      <a-input-search
        v-model:value="SearchText"
        placeholder="用户名/邮箱/手机号"
        enter-button
        @search="onSearch"
      />
  
      </a-col>
    </a-row>
    <a-table :columns="columns" :data-source="users" :pagination="pagination" :loading="loading"  @change="handleTableChange">
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
            <a-switch :checked="record.status === 1"  @change="(checked) => onChangeStatus(checked, record.id)"  />
            
          </span>
        </template>
        <template v-else-if="column.key === 'action'">
              <a-row :gutter="6" class="action_row">
                  <a-col>
                      <a-button type="primary" id="assignRole">分配角色</a-button>
                  </a-col>
                  <a-col class="resetPwd">
                      <a-button >重置密码</a-button>
                  </a-col>
                  <a-col>
                      <a-button type="primary" @click="onSaveorChanageUser(record.id)"><FontAwesomeIcon :icon="faEdit" /></a-button>
                  </a-col>
                  <a-col>
                      <a-button class="delBtn" danger type="primary"><FontAwesomeIcon :icon="faTrash" /></a-button>
                      
                  </a-col>
              </a-row>
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
  import { usePagination } from 'vue-request';
  import { FontAwesomeIcon } from '@fortawesome/vue-fontawesome'
  import { faEdit } from '@fortawesome/free-solid-svg-icons'
  import { faTrash } from '@fortawesome/free-solid-svg-icons'
  import { computed } from 'vue';
  import { changeUserStatus } from '@/api/user/index.js';
  import { message } from 'ant-design-vue';
  import Dialog from '@/views/sys/user/components/Dialog.vue';

const SearchText = ref('')
  const columns = [
    { title: '用户名', dataIndex: 'username',key: 'username' ,sorter: (a, b) => a.username.localeCompare(b.username),sortDirections: ['ascend', 'descend']},
    { title: '角色', dataIndex: 'roles',key: 'roles' },
    { title: '邮箱', dataIndex: 'email',key: 'email' },
    { title: '手机号', dataIndex: 'phonenumber',key: 'phonenumber' },
    { title: '状态', dataIndex: 'status',key: 'status' },
    { title: '创建时间', dataIndex: 'create_time',key: 'create_time' },
    { title: '备注', dataIndex: 'remark',key: 'remark' },
    { title: '操作', key: 'action' }
  ]
  const editUser = (user) => {
    // 编辑逻辑
  }


  const open=ref(false)


const onChangeStatus = (e1,e2) => {
    var status = e1 === true ? 1 : 0
    var user_id = e2
    changeUserStatus(user_id,status).then((result) => {
        message.success("执行成功")
        run({ page: current, size: pageSize.value })
    })
  }
const total = ref(1)
const queryData = params => {
  return getUserList(params).then(res => {
    total.value = res.data.data.count
    return res.data.data.results
  });
};

  const {
    data: users,
    run,
    loading,
    current,
    pageSize,
} = usePagination(queryData,
{
    formatResult:(res) => {
        console.log("fdsfdaf")
        return "hello"
    } ,
    pagination: { currentKey: 'page', pageSizeKey: 'size' }
})



const pagination = computed(() => ({
    total: total.value,
    current: current.value,
    pageSize: pageSize.value,
}))

const handleTableChange = (page,filters, sorter) => {
    
    var sorter_str = ""
    if(sorter.field) {
        // 不为空，说明有排序
        if(sorter.order == "descend") {
            sorter_str = sorter_str + '-' + sorter.field
        }else {
            sorter_str = sorter_str  + sorter.field
        }
        run({
        size: page.pageSize,
        page: page?.current,
        order: sorter_str,
        ...filters,})
    } else {
        run({
        size: page.pageSize,
        page: page?.current,
        ...filters,})
    }



};

const onSearch = (keyword) => {
    run({ page: current, size: pageSize.value, keyword })
}

const title = ref("")
const onSaveorChanageUser = (user_id) => {
    open.value = true;
    title.value = "用户修改"
    console.log("编辑user_id:" + user_id);
}
console.log("=============count===============")

  </script>

  <style scoped>
  .search {

  }
  #assignRole {

  }
  .actionRow {
    vertical-align: middle;
  }
  .actionRow> a-col {
    height: 100%;
  }
  .resetPwd >:where(.css-dev-only-do-not-override-1p3hq3p).ant-btn-default {
      background-color: orange;
      color: white;
  }
  
</style>
 