<template>
    <Dialog :open="open" @update:open="(value)=>{open=value}" :user_id="user_id" :title="title" @initUserList="HandleInitUserList"/>
    
    
     <a-row class="tools" :gutter="16">
      <a-col :span="7">
      <a-input-search
        class="tool-item"
        v-model:value="SearchText"
        placeholder="用户名/邮箱/手机号"
        enter-button
        size="large"
        @search="onSearch"
      />
      
      </a-col>
      <a-col class="AddUserBtn tool-item">
        <a-button size="large" @click="HandleAddUser" >新增</a-button>
      </a-col>
          <a-col class="BatchDelUserBtn tool-item" v-show="batchDelUserBtnVisiable">
            <a-popconfirm
    placement="top"
    title="您确定要删除么？"
    ok-text="确认"
    cancel-text="取消"
    @confirm="confirm"
    @cancel="cancel"
    :overlayStyle="{ width: '300px', minHeight: '200px' }"  
  >
         <a-button size="large" type="primary" :loading="state.loading" danger>
            <FontAwesomeIcon :icon="faTrash" />批量删除</a-button>
  </a-popconfirm>
        <!-- <a-button size="large" type="primary" :loading="state.loading" danger @click="HandleBatchDelUser" >
            <FontAwesomeIcon :icon="faTrash" />批量删除</a-button> -->
      </a-col>
    </a-row>
    <!-- 注意需要rowKey -->
    <a-table :row-selection="{ selectedRowKeys: state.selectedRowKeys, onChange: onSelectChange }"  rowKey="id" :columns="columns" :data-source="users" :pagination="pagination" :loading="loading"  @change="handleTableChange">
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
  import { computed,reactive } from 'vue';
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
var lastSearchKeyword = null
const onChangeStatus = (e1,e2) => {
    var status = e1 === true ? 1 : 0
    var user_id = e2
    changeUserStatus(user_id,status).then((result) => {
        message.success("执行成功")
        run({ page: current, size: pageSize.value,keyworkd: lastSearchKeyword })
    })
  }
const total = ref(1)

const queryData = params => {
  return getUserList(params).then(res => {
    if(res.data.code == 200) {
        lastSearchKeyword = res.config.params.keyword
        total.value = res.data.data.count
        return res.data.data.results

    }

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
    // 这里疑问？？formatResult没什么用
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
        keyword: lastSearchKeyword,
        order: sorter_str,
        ...filters,})
    } else {
        run({
        size: page.pageSize,
        page: page?.current,
        keyword: lastSearchKeyword,
        ...filters,})
    }
};

const onSearch = (keyword) => {
    run({size: pageSize.value, keyword })
}

const title = ref("")
const user_id = ref(-1)
const open = ref(false)
const onSaveorChanageUser = (id) => {
    open.value = true;
    title.value = "用户修改"
    user_id.value = id
}
const HandleInitUserList = (res) => {
    run({page: current,size: pageSize.value,keyword:lastSearchKeyword})
}
// 新增用户
const HandleAddUser = () => {
    open.value = true;
    title.value = "用户添加"
    user_id.value = -1
}
// 删除用户
const batchDelUserBtnVisiable = ref(false)
const state = reactive({
  selectedRowKeys: [],
  // Check here to configure the default column
  loading: false,
});

const onSelectChange = selectedRowKeys => {
    if(selectedRowKeys.length >= 1) {
        batchDelUserBtnVisiable.value = true
    }else {
        batchDelUserBtnVisiable.value = false
        
    }
    state.selectedRowKeys = selectedRowKeys;
};


import {batchDeleteUser} from '@/api/user/index.js';
// 批量删除用户
const HandleBatchDelUser = ()=>{
    state.loading = true;
    batchDeleteUser(state.selectedRowKeys).then((res)=>{
        if(res.data.code == 200) {
            HandleInitUserList();
            state.loading = false;
            message.success("删除成功");
        }else {
            message.error("删除失败 ");
        }
    })
}
const confirm = ()=> {
    HandleBatchDelUser()
}
const cancel = ()=> {
}
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

  .AddUserBtn >:where(.css-dev-only-do-not-override-1p3hq3p).ant-btn-default {
      background-color: green;
      color: white;
  }
.tools { 
    margin-bottom: 20px;
    height: 50px;;
}
.tool-item {
    height: 200px !important;
}
.BatchDelUserBtn  >:where(.css-dev-only-do-not-override-1p3hq3p).ant-btn-default {
      background-color: orange;
      color: white;
}
</style>
 