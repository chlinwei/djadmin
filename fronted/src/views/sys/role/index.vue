<template>
    <Dialog :open="open" @update:open="(value) => { open = value }" :role_id="role_id" :title="title"
        @initList="initList" />

    <MenuAssign :open="menuAssignVisible" @update:menuAssignVisible="(value) => { menuAssignVisible = value }"
        :role_id="role_id" :title="menu_assign_title" @initList="initList" />

    <a-row class="tools" :gutter="16">
        <a-col :span="7">
            <a-input-search class="tool-item" v-model:value="SearchText" placeholder="角色名/权限字符/备注" enter-button size="large"
                @search="onSearch" />

        </a-col>
        <a-col class="AddBtn tool-item" v-permission.remove="'system:roles:create'">
            <a-button size="large" @click="HandleAdd">
                <FontAwesomeIcon :icon="['fas','fa-plus-circle']">
                </FontAwesomeIcon>
                <span>&nbsp;新增</span>
            </a-button>
        </a-col>
        <a-col class="BatchDelUserBtn tool-item" v-if="state.selectedRowKeys.length >= 1" v-permission.remove="'system:roles:delete'">
            <a-popconfirm placement="top" title="您确定要删除么？" ok-text="确认" cancel-text="取消" @confirm="confirm" @cancel="cancel"
                :overlayStyle="{ width: '300px', minHeight: '200px' }">
                <a-button size="large" type="primary" :loading="state.loading" danger>
                    <FontAwesomeIcon :icon="['fas','trash']" />批量删除
                </a-button>
            </a-popconfirm>
        </a-col>
        <a-col>
            <div class="selectedItems" v-if="state.selectedRowKeys.length >= 1">
                <span style="height: 100%;">已选择{{ state.selectedRowKeys.length }}项</span>
            </div>
        </a-col>
    </a-row>
    <a-row>
        <a-col :span="24">
            <a-table :row-selection="{ selectedRowKeys: state.selectedRowKeys, onChange: onSelectChange }" rowKey="id"
                :columns="columns" :data-source="users" :pagination="pagination" :loading="loading"
                @change="handleTableChange">
                <template #bodyCell="{ column, record }">
                    <template v-if="column.key === 'action'">
                        <div :key="record.id">
                            <a-row :gutter="6" class="action_row">
                                <a-col  v-permission.remove="'system:roles:update'">
                                    <a-button type="primary" id="assignRole"
                                        @click="handleMenuAssign(record.id, record.name)">分配权限</a-button>
                                </a-col>
                                <a-col v-if="record.name != 'admin'"  v-permission.remove="'system:roles:update'">
                                    <a-button type="primary" @click="onSaveorChanageRole(record.id)">
                                        <FontAwesomeIcon :icon="['fa','edit']" />
                                    </a-button>
                                </a-col>
                                <a-col v-permission.remove="'system:roles:delete'">
                                    <a-popconfirm placement="bottom" title="您确定要删除么？" ok-text="确认" cancel-text="取消"
                                        @confirm="delconfirm(record.id)" @cancel="cancel"
                                        :overlayStyle="{ width: '200px', minHeight: '150px' }">
                                        <a-button class="delBtn" :loading="rowLoadingStates[record.id]" danger
                                            type="primary">
                                            <FontAwesomeIcon :icon="['fas','trash']" />
                                        </a-button>
                                    </a-popconfirm>
                                </a-col>
                            </a-row>
                        </div>
                    </template>
                </template>
            </a-table>
        </a-col>
        <!-- 注意需要rowKey -->

    </a-row>
</template>
  
<script setup>

// 缓存
defineOptions({
    name: 'role'
})
import { ref } from 'vue'
import { getRoleList } from '@/api/role/index.js';
import { usePagination } from 'vue-request';
import { computed, reactive } from 'vue';
import { message } from 'ant-design-vue';
import Dialog from '@/views/sys/role/components/Dialog.vue';
import MenuAssign from '@/views/sys/role/components/MenuAssign.vue';
import {checkPermission} from '@/directives/permission/permission'

const menu_assign_title = ref("")

// 取消删除
const cancel = () => {

}

const roleassign_title = ref("权限分配")

const SearchText = ref('')
const columns = [
    { title: '角色名', dataIndex: 'name', fixed: true, width: 100, key: 'name', sorter: true, sortDirections: ['ascend', 'descend'] },
    { title: '权限字符', dataIndex: 'code', key: 'code', width: 150 },
    { title: '创建时间', dataIndex: 'create_time', key: 'create_time', width: 80,sorter: true},
    { title: '备注', dataIndex: 'remark', key: 'remark', width: 200 },
]
if(checkPermission(['system:roles:update','system:roles:delete'])) {
    columns.push({ title: '操作', key: 'action', fixed: 'right', width: 330 })
}

var lastSearchKeyword = null
const total = ref(1)

const queryData = params => {
    return getRoleList(params).then(res => {
        console.log(res)
        if (res.data.code == 200) {
            lastSearchKeyword = res.config.params.search
            total.value = res.data.data.count
            return res.data.data.results
        } else {
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
    showTotal: (total) => `共有${total}条数据`,
    pageSizeOptions: ['10', '20', '30'],
    showQuickJumper: true,
}))
const handleTableChange = (page, filters, sorter) => {

    var sorter_str = ""
    if (sorter.order) {
        // 不为空，说明有排序
        if (sorter.order == "descend") {
            sorter_str = sorter_str + '-' + sorter.field
        } else {
            sorter_str = sorter_str + sorter.field
        }
        run({
            size: page.pageSize,
            page: page?.current,
            search: lastSearchKeyword,
            ordering: sorter_str,
            ...filters,
        })
    } else {
        run({
            size: page.pageSize,
            page: page?.current,
            search: lastSearchKeyword,
            ...filters,
        })
    }
};

const onSearch = (keyword) => {
    run({ size: pageSize.value, search:keyword })
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

    }).finally(() => {
        state.loading = false;
    })
}

const rowLoadingStates = reactive({
});
const setRowLoading = (id, isLoading) => {
    rowLoadingStates[id] = isLoading;
};
const delconfirm = (id) => {
    setRowLoading(id, true)
    batchDeleteRole([id]).then((res) => {
        if (res.data.code == 200) {
            initList();
            message.success("删除成功");
        } else {
            message.error("删除失败: " + res.data.msg);
        }

    }).finally(() => {
        setRowLoading(id, false)
    })
}
const rowLoadingStates2 = reactive({
});
const setRowLoading2 = (id, isLoading) => {
    rowLoadingStates2[id] = isLoading;
};

// 角色分配
const menuAssignVisible = ref(false)

const handleMenuAssign = (id, name) => {
    menuAssignVisible.value = true
    role_id.value = id
    console.log(role_id.value)
    menu_assign_title.value = "权限分配-" + name

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

.AddBtn>:where(.css-dev-only-do-not-override-1p3hq3p).ant-btn-default {
    background-color: green;
    color: white;
}

.tools {
    margin-bottom: 20px;
    height: 50px;
}

.tool-item {
    height: 200px !important;
}

.BatchDelUserBtn>:where(.css-dev-only-do-not-override-1p3hq3p).ant-btn-default {
    background-color: orange;
    color: white;
}

.selectedItems span {
    line-height: 40px;
    /* 匹配按钮高度 */
    vertical-align: middle;
    font-size: 20px;
}
</style>