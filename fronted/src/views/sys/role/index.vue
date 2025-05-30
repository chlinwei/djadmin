<template>
    <Dialog :open="open" @update:open="(value) => { open = value }" :role_id="role_id" :title="title"
      @initList="initList" />
  
    <MenuAssign :open="menuAssignVisible" @update:menuAssignVisible="(value) => { menuAssignVisible = value}" :role_id="role_id" :title="menu_assign_title"
      @initList="initList" />
  
    <a-row class="tools" :gutter="16">
      <a-col :span="7">
        <a-input-search class="tool-item" v-model:value="SearchText" placeholder="用户名/邮箱/手机号" enter-button size="large"
          @search="onSearch" />
  
      </a-col>
      <a-col class="AddUserBtn tool-item">
        <a-button size="large" @click="HandleAdd">新增</a-button>
      </a-col>
      <a-col class="BatchDelUserBtn tool-item" v-if="state.selectedRowKeys.length>=1">
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
    <a-table  :row-selection="{ selectedRowKeys: state.selectedRowKeys, onChange: onSelectChange }" rowKey="id"
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
        <template v-else-if="column.key === 'action'">
          <div :key="record.id">
          <a-row :gutter="6" class="action_row">
            <a-col>
              <a-button type="primary" id="assignRole" @click="handleMenuAssign(record.id,record.name)">分配权限</a-button>
            </a-col>
            <a-col v-if="record.name !='admin'">
              <a-button type="primary" @click="onSaveorChanageRole(record.id)">
                <FontAwesomeIcon :icon="faEdit" />
              </a-button>
            </a-col>
            <a-col v-if="record.username !='admin'">
              
              <a-popconfirm placement="bottom" title="您确定要删除么？" ok-text="确认" cancel-text="取消" @confirm="delconfirm(record.id)"
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
  import { getRoleList } from '@/api/role/index.js';
  import { usePagination } from 'vue-request';
  import { FontAwesomeIcon } from '@fortawesome/vue-fontawesome'
  import { faEdit } from '@fortawesome/free-solid-svg-icons'
  import { faTrash } from '@fortawesome/free-solid-svg-icons'
  import { computed, reactive } from 'vue';
  import { message } from 'ant-design-vue';
  import Dialog from '@/views/sys/role/components/Dialog.vue';
  import MenuAssign from '@/views/sys/role/components/MenuAssign.vue';
  
  const menu_assign_title = ref("")

  // 取消删除
  const cancel = ()=>{
    
  }
  
  const roleassign_title = ref("权限分配")
  
  const SearchText = ref('')
  const columns = [
    { title: '角色名', dataIndex: 'name',fixed:true,width:100, key: 'name', sorter: (a, b) => a.name.localeCompare(b.name), sortDirections: ['ascend', 'descend'] },
    { title: '权限字符', dataIndex: 'code', key: 'code' ,width:150},
    { title: '创建时间', dataIndex: 'create_time', key: 'create_time',width:80 },
    { title: '备注', dataIndex: 'remark', key: 'remark',width:200 },
    { title: '操作', key: 'action',fixed: 'right',width: 330 }
  ]
  var lastSearchKeyword = null
  const total = ref(1)
  
  const queryData = params => {
    return getRoleList(params).then(res => {
      if (res.data.code == 200) {
        lastSearchKeyword = res.config.params.keyword
        total.value = res.data.data.count
        return res.data.data.results
      }else {
        message.error("获取角色列表失败")
        return []
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
  const role_id = ref(-1)
  const open = ref(false)
  const onSaveorChanageRole = (id) => {
    open.value = true;
    title.value = "角色修改"
    role_id.value = id
  }
  const initList = (res) => {
    run({ page: current, size: pageSize.value, keyword: lastSearchKeyword })
  }
  // 新增角色
  const HandleAdd = () => {
    open.value = true;
    title.value = "角色添加"
    role_id.value = -1
  }
  // 删除角色
  const state = reactive({
    selectedRowKeys: [],
    // Check here to configure the default column
    loading: false,
  });
  
  const onSelectChange = selectedRowKeys => {
    state.selectedRowKeys = selectedRowKeys;
  };
  
  
  import { batchDeleteRole } from '@/api/role/index.js';
  
  // 批量删除权限
  const confirm = () => {
      state.loading = true;
    batchDeleteRole(state.selectedRowKeys).then((res) => {
      if (res.data.code == 200) {
        initList();
        message.success("删除角色成功");
        state.selectedRowKeys = []
      } else {
        message.error("删除角色失败:" + res.data.msg);
      }
      
    }).finally(()=>{
        state.loading = false;
    })
  }
  
  const rowLoadingStates = reactive({
  });
  const setRowLoading = (id, isLoading) => {
    rowLoadingStates[id] = isLoading;
  };
  const delconfirm = (id) => {
        setRowLoading(id,true)
    batchDeleteRole([id]).then((res) => {
      if (res.data.code == 200) {
        initList();
        message.success("删除成功");
      } else {
        message.error("删除失败: "+ res.data.msg);
      }
      
    }).finally(()=>{
        setRowLoading(user_id,false)
    })
  }
  const rowLoadingStates2 = reactive({
  });
  const setRowLoading2 = (id, isLoading) => {
    rowLoadingStates2[id] = isLoading;
  };

  // 角色分配
  const menuAssignVisible=ref(false)
  const user_id2=ref(-1)
  
  const handleMenuAssign = (id,name)=> {
    menuAssignVisible.value = true
    role_id.value = id
    console.log(role_id.value)
    menu_assign_title.value = "权限分配-" + name + "id:" + id
    
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