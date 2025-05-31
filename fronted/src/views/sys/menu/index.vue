<template>
 <Dialog :open="itemAssignVisible" @update:itemAssignVisible="(value) => { itemAssignVisible = value }"
        :menu_id="menu_id" :title="menu_assign_title" @initList="initList" />
    <a-row class="tools" :gutter="16">
        <a-col class="AddBtn tool-item">
            <a-button size="large" @click="HandleAdd">
                <FontAwesomeIcon :icon="faPlusCircle" />
                <span>&nbsp新增</span>
            </a-button>
        </a-col>
    </a-row>
    <a-row>

        <a-col :offset="12">
                <a-spin :spinning="loading" style="margin-top:30px">
             </a-spin>
            </a-col>
        <a-col :span="24">
            <a-table v-if="treeData.length" 
            :columns="columns" :defaultExpandAllRows="true" :data-source="treeData"
            :row-selection="rowSelection">
               
                <template #bodyCell="{ column, record }">
                    <template v-if="column.key === 'action'">
                        <div :key="record.id">
                            <a-row :gutter="6" class="action_row">
                                <a-col>
                                    <a-button type="primary" @click="onSaveorChanageMenu(record.id)">
                                        <FontAwesomeIcon :icon="faEdit" />
                                    </a-button>
                                </a-col>
                                <a-col>
                                    <a-popconfirm placement="bottom" title="您确定要删除么？" ok-text="确认" cancel-text="取消"
                                        @confirm="delconfirm(record.id)" @cancel="cancel"
                                        :overlayStyle="{ width: '200px', minHeight: '150px' }">
                                        <a-button class="delBtn" :loading="rowLoadingStates[record.id]" danger
                                            type="primary">
                                            <FontAwesomeIcon :icon="faTrash" />
                                        </a-button>
                                    </a-popconfirm>
                                </a-col>
                            </a-row>
                        </div>
                    </template>
                </template>
            </a-table>
        </a-col>
    </a-row>
</template>
<script setup>
import { ref, reactive } from 'vue';
import { FontAwesomeIcon } from '@fortawesome/vue-fontawesome'
import { faEdit } from '@fortawesome/free-solid-svg-icons'
import { faTrash, faPlusCircle } from '@fortawesome/free-solid-svg-icons'
import Dialog from '@/views/sys/menu/components/Dialog.vue';
const itemAssignVisible = ref(false)
// 缓存
defineOptions({
    name: 'menu'
})
const columns = [
    {
        title: '菜单名称',
        dataIndex: 'name',
        key: 'name',
    },
    {
        title: '图标',
        dataIndex: 'icon',
        key: 'icon',
    },
    {
        title: '排序',
        dataIndex: 'order_num',
        key: 'order_num',
    },
    {
        title: '权限标识',
        dataIndex: 'perms',
        key: 'perms',
    },
    {
        title: '组件路径',
        dataIndex: 'path',
        key: 'path',
    },
    {
        title: '菜单类型',
        dataIndex: 'menu_type',
        key: 'menu_type',
    },
    {
        title: '创建时间',
        dataIndex: 'create_time',
        key: 'create_time',
    },
    {
        title: '操作',
        key: 'action',
    },

];
const rowLoadingStates = reactive({
});
const treeData = ref([])
import { getMenuTree,saveOrCreateMenu } from '@/api/menu';

const parseTreeData = (data) => {
    return data.map(item => ({
        name: item.name,
        key: item.id,
        icon: item.icon,
        order_num: item.order_num,
        perms: item.perms,
        path: item.path,
        menu_type: item.menu_type,
        create_time: item.create_time,
        // icon: <a-icon type={item.icon} />,  // 注意：这里使用了Ant Design的图标组件，需要根据实际情况调整
        children: item.children ? parseTreeData(item.children) : null,
    }));
};
const loading = ref(false)
const initList = () => {
    loading.value = true
    getMenuTree().then((res) => {
        var data = parseTreeData(res.data.data)
        treeData.value = data
        loading.value = false
    }).finally(() => {
        loading.value = false;
    })
}

initList()
const rowSelection = ref({
    checkStrictly: false,
    onChange: (selectedRowKeys, selectedRows) => {
        console.log(`selectedRowKeys: ${selectedRowKeys}`, 'selectedRows: ', selectedRows);
    },
    onSelect: (record, selected, selectedRows) => {
        console.log(record, selected, selectedRows);
    },
    onSelectAll: (selected, selectedRows, changeRows) => {
        console.log(selected, selectedRows, changeRows);
    },
});

const onSaveorChanageMenu = (res) => {
    console.log(res)
}
const cancel = () => {
}

// 新增
const HandleAdd = () => {
    itemAssignVisible.value = true
    menu_assign_title.value = "菜单添加"
}
</script>
<style scoped>
.AddBtn>:where(.css-dev-only-do-not-override-1p3hq3p).ant-btn-default {
    background-color: green;
    color: white;
}
.tools {
    margin-bottom: 20px;
}
</style>