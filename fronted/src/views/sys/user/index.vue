<template>
  <Dialog :open="open" @update:open="(value) => { open = value }" :user_id="user_id" :title="title"
    @initUserList="HandleInitUserList" />

  <RoleAssign :open2="open2" @update:open2="(value) => { open2 = value }" :user_id2="user_id2" :title="roleassign_title"
    @initUserList="HandleInitUserList" />

  <a-row class="tools" :gutter="16">
    <a-col :span="7">
      <a-input-search class="tool-item" v-model:value="SearchText" placeholder="用户名/邮箱/手机号" enter-button size="large"
        @search="onSearch" />

    </a-col>
    <a-col class="AddUserBtn tool-item">
      <a-button size="large" @click="HandleAddUser">新增</a-button>
    </a-col>
    <a-col class="BatchDelUserBtn tool-item" v-show="batchDelUserBtnVisiable">
      <a-popconfirm placement="top" title="您确定要删除么？" ok-text="确认" cancel-text="取消" @confirm="confirm" @cancel="cancel"
        :overlayStyle="{ width: '300px', minHeight: '200px' }">
        <a-button size="large" type="primary" :loading="state.loading" danger>
          <FontAwesomeIcon :icon="faTrash" />批量删除
        </a-button>
      </a-popconfirm>
    </a-col>
    <a-col>
          <div class="selectedItems" v-if="state.selectedRowKeys.length>=1" >
            <span style="height: 100%;">已选择{{state.selectedRowKeys.length}}项</span>
          </div>

    </a-col>
  </a-row>
  <!-- 注意需要rowKey -->
  <a-table :scroll="{ x: 2000 }" :row-selection="{ selectedRowKeys: state.selectedRowKeys, onChange: onSelectChange }" rowKey="id"
    :columns="columns" :data-source="users" :pagination="pagination" :loading="loading" @change="handleTableChange">
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
          <a-tag color="orange" v-for="role in record.roles" :key="role.id">
            {{ role.name }}
          </a-tag>
        </span>
      </template>
      <template v-else-if="column.key === 'status'">
        <span>
          <a-switch :checked="record.status === 1" @change="(checked) => onChangeStatus(checked, record.id)" />

        </span>
      </template>
      <template v-else-if="column.key === 'action'">
        <div :key="record.id">
        <a-row :gutter="6" class="action_row">
          <a-col>
            <a-button type="primary" id="assignRole" @click="handleRoleAssign(record.id,record.username)">分配角色</a-button>
          </a-col>
          <a-col class="resetPwd" v-if="record.username !='admin'">
              <a-popconfirm placement="bottom" title="您确定要重置密码？" ok-text="确认" cancel-text="取消" @confirm="resetPwdconfirm(record.id)"
              @cancel="cancel" :overlayStyle="{ width: '200px', minHeight: '150px' }">
            <a-button  :loading="rowLoadingStates2[record.id]">
              重置密码
            </a-button>
            </a-popconfirm>
          </a-col>
          <a-col v-if="record.username !='admin'">
            <a-button type="primary" @click="onSaveorChanageUser(record.id)">
              <FontAwesomeIcon :icon="faEdit" />
            </a-button>
          </a-col>
          <a-col v-if="record.username !='admin'">
            
            <a-popconfirm placement="bottom" title="您确定要删除么？" ok-text="确认" cancel-text="取消" @confirm="delUserconfirm(record.id)"
              @cancel="cancel" :overlayStyle="{ width: '200px', minHeight: '150px' }">
            <a-button class="delBtn" :loading="rowLoadingStates[record.id]" danger type="primary">
              <FontAwesomeIcon :icon="faTrash" />
            </a-button>
            </a-popconfirm>
            


          </a-col>
        </a-row>
        </div>
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
import { computed, reactive } from 'vue';
import { changeUserStatus } from '@/api/user/index.js';
import { message } from 'ant-design-vue';
import Dialog from '@/views/sys/user/components/Dialog.vue';
import RoleAssign from '@/views/sys/user/components/RoleAssign.vue';


const roleassign_title = ref("角色分配")

const SearchText = ref('')
const columns = [
  { title: '用户名', dataIndex: 'username',fixed:true,width:100, key: 'username', sorter: (a, b) => a.username.localeCompare(b.username), sortDirections: ['ascend', 'descend'] },
  { title: '角色', dataIndex: 'roles', key: 'roles' ,width:150},
  { title: '邮箱', dataIndex: 'email', key: 'email',width:100 },
  { title: '手机号', dataIndex: 'phonenumber', key: 'phonenumber',width:100 },
  { title: '状态', dataIndex: 'status', key: 'status',width:80 },
  { title: '创建时间', dataIndex: 'create_time', key: 'create_time',width:80 },
  { title: '备注', dataIndex: 'remark', key: 'remark',width:200 },
  { title: '操作', key: 'action',fixed: 'right',width: 330 }
]
var lastSearchKeyword = null
const onChangeStatus = (e1, e2) => {
  var status = e1 === true ? 1 : 0
  var user_id = e2
  changeUserStatus(user_id, status).then((result) => {
    
    if(result.data.code==200) {
      run({ page: current, size: pageSize.value, keyworkd: lastSearchKeyword })
      message.success("状态修改成功")
    }else {
      message.success("状态修改失败")
    }
    
  })
}
const total = ref(1)

const queryData = params => {
  return getUserList(params).then(res => {
    if (res.data.code == 200) {
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
    formatResult: (res) => {
      console.log("fdsfdaf")
      return "hello"
    },
    pagination: { currentKey: 'page', pageSizeKey: 'size' }
  })



const pagination = computed(() => ({
  total: total.value,
  current: current.value,
  pageSize: pageSize.value,
}))

const handleTableChange = (page, filters, sorter) => {

  var sorter_str = ""
  if (sorter.field) {
    // 不为空，说明有排序
    if (sorter.order == "descend") {
      sorter_str = sorter_str + '-' + sorter.field
    } else {
      sorter_str = sorter_str + sorter.field
    }
    run({
      size: page.pageSize,
      page: page?.current,
      keyword: lastSearchKeyword,
      order: sorter_str,
      ...filters,
    })
  } else {
    run({
      size: page.pageSize,
      page: page?.current,
      keyword: lastSearchKeyword,
      ...filters,
    })
  }
};

const onSearch = (keyword) => {
  run({ size: pageSize.value, keyword })
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
  run({ page: current, size: pageSize.value, keyword: lastSearchKeyword })
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
  if (selectedRowKeys.length >= 1) {
    batchDelUserBtnVisiable.value = true
  } else {
    batchDelUserBtnVisiable.value = false

  }
  state.selectedRowKeys = selectedRowKeys;
};


import { batchDeleteUser } from '@/api/user/index.js';

// 批量删除用户
const confirm = () => {
    state.loading = true;
  batchDeleteUser(state.selectedRowKeys).then((res) => {
    if (res.data.code == 200) {
      HandleInitUserList();
      message.success("删除成功");
    } else {
      message.error("删除失败:" + res.data.msg);
    }
    state.loading = false;
  })
}

const rowLoadingStates = reactive({
});
const setRowLoading = (id, isLoading) => {
  rowLoadingStates[id] = isLoading;
};
const delUserconfirm = (user_id) => {
      setRowLoading(user_id,true)
  batchDeleteUser([user_id]).then((res) => {
    if (res.data.code == 200) {
      HandleInitUserList();
      message.success("删除成功");
    } else {
      message.error("删除失败: "+ res.data.msg);
    }
    setRowLoading(user_id,false)
  })
}
const rowLoadingStates2 = reactive({
});
const setRowLoading2 = (id, isLoading) => {
  rowLoadingStates2[id] = isLoading;
};
// 重置密码
import {resetPwd} from '@/api/user/index.js';
const resetPwdconfirm = (id)=>{
        setRowLoading2(id,true)
  resetPwd(id).then((res) => {
    if (res.data.code == 200) {
      HandleInitUserList();
      message.success("重置密码成功");
    } else {
      message.error("重置密码失败:" + res.data.msg);
    }
    setRowLoading2(id,false)
  })
}
const cancel = () => {
}

// 角色分配
const open2=ref(false)
const user_id2=ref(-1)

const handleRoleAssign = (id,username)=> {
  open2.value = true
  user_id2.value = id
  roleassign_title.value = "角色分配-" + username
  
}
</script>

<style scoped>
.search {}

#assignRole {}

.actionRow {
  vertical-align: middle;
}

.actionRow>a-col {
  height: 100%;
}

.resetPwd>:where(.css-dev-only-do-not-override-1p3hq3p).ant-btn-default {
  background-color: orange;
  color: white;
}

.AddUserBtn>:where(.css-dev-only-do-not-override-1p3hq3p).ant-btn-default {
  background-color: green;
  color: white;
}

.tools {
  margin-bottom: 20px;
  height: 50px;
  ;
}

.tool-item {
  height: 200px !important;
}

.BatchDelUserBtn>:where(.css-dev-only-do-not-override-1p3hq3p).ant-btn-default {
  background-color: orange;
  color: white;
}

.selectedItems span {
  line-height: 40px;  /* 匹配按钮高度 */
  vertical-align: middle;
  font-size:20px;
}
</style>