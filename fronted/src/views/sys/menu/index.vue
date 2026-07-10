<template>
 <Dialog :open="itemAssignVisible" @update:open="(value) => { itemAssignVisible = value }"
        :item_id="menu_id" :title="menu_assign_title" :treeData="treeData2"
        :max_tree_depth="menuMaxTreeDepth" @initList="initList" />
    <a-row class="tools" :gutter="16">
        <a-col class="AddBtn tool-item" v-permission="'system:menus:create'">
            <a-button size="large" @click="HandleAdd">
                <FontAwesomeIcon :icon="['fas','fa-plus-circle']" />
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
            :columns="columns" :data-source="treeData"
            >
               
                <template #bodyCell="{ column, record }">
                    <template v-if="column.key === 'icon'">
                        <FontAwesomeIcon :icon="record.icon" />
                    </template>
                    <template v-else-if="column.key === 'create_time'">
                        {{ formatDateTime(record.create_time) }}
                    </template>
                    <template v-if="column.key === 'action'">
                        <div :key="record.id">
                            <a-row :gutter="6" class="action_row">
                                <a-col v-permission="'system:menus:update'">
                                    <a-tooltip title="编辑">
                                        <a-button type="primary" @click="saveItem(record.key,record.name)">
                                            <FontAwesomeIcon :icon="['fa','edit']" />
                                        </a-button>
                                    </a-tooltip>
                                </a-col>
                                <a-col v-permission="'system:menus:delete'">
                                    <a-popconfirm placement="bottom" title="您确定要删除么？" ok-text="确认" cancel-text="取消"
                                        @confirm="delconfirm(record.key)" @cancel="cancel"
                                        :overlayStyle="{ width: '200px', minHeight: '150px' }">
                                        <a-tooltip title="删除">
                                            <a-button class="delBtn" :loading="rowLoadingStates[record.key]" danger
                                                type="primary">
                                                <FontAwesomeIcon :icon="['fa','trash']" />
                                            </a-button>
                                        </a-tooltip>
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
import { ref, reactive, onMounted } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import Dialog from '@/views/sys/menu/components/Dialog.vue';
import { message } from 'ant-design-vue';
import { checkPermission } from '@/directives/permission/permission';
import { formatTimeWithTimezone } from '@/util/timezone'
import store from '@/store'
const itemAssignVisible = ref(false)
const route = useRoute()
const router = useRouter()
// 缓存
defineOptions({
    name: 'menu'
})
const menu_id = ref(-1)
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
        title: '路由路径',
        dataIndex: 'path',
        key: 'path',
    },
    {
        title: '组件路径',
        dataIndex: 'component',
        key: 'component',
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
];
if(checkPermission(['system:menus:update','system:menus:delete'])) {
    columns.push({
        title: '操作',
        key: 'action',
    })
}
const rowLoadingStates = reactive({
});
const treeData = ref([])
// 给Dialog界面的
const treeData2 = ref([])
import { getMenuTree,deleteMenuById } from '@/api/menu';
import { getConfigByKey, CONFIG_KEYS } from '@/api/sys/sysconfig.js';

const menuMaxTreeDepth = ref(5)
getConfigByKey(CONFIG_KEYS.MENU_MAX_TREE_DEPTH).then(res => {
    if (res.data?.value) menuMaxTreeDepth.value = Number(res.data.value) || 5
}).catch(() => {})



const parseTreeData = (data) => {
    return data.map(function(item) {
        return {
        name: item.name,
        key: item.id,
        icon: item.icon,
        order_num: item.order_num,
        perms: item.perms,
        path: item.path,
        component: item.component,
        menu_type: item.menu_type,
        create_time: item.create_time,
        children: item.children ? parseTreeData(item.children) : null,
    }});
};
const loading = ref(false)

const normalizeUtcTime = (value) => {
    if (!value || typeof value !== 'string') {
        return value
    }
    const text = value.trim()
    if (!text) {
        return value
    }
    if (/[zZ]$|[+-]\d{2}:\d{2}$/.test(text)) {
        return text
    }
    return `${text.replace(' ', 'T')}Z`
}

const formatDateTime = (value) => {
    if (!value) {
        return '-'
    }
    try {
        return formatTimeWithTimezone(normalizeUtcTime(value), store.state.user?.timezone || 'Asia/Shanghai', 'YYYY-MM-DD HH:mm:ss')
    } catch (error) {
        return value
    }
}


const findMenuNameById = (nodes, id) => {
    for (const node of nodes) {
        if (node.key === id) {
            return node.name || '菜单'
        }
        if (node.children && node.children.length) {
            const childName = findMenuNameById(node.children, id)
            if (childName) {
                return childName
            }
        }
    }
    return ''
}

const tryOpenFocusedMenu = () => {
    const focusMenuId = Number(route.query.focusMenuId)
    if (!focusMenuId || Number.isNaN(focusMenuId)) {
        return
    }
    const name = findMenuNameById(treeData.value, focusMenuId) || '菜单'
    itemAssignVisible.value = true
    menu_id.value = focusMenuId
    menu_assign_title.value = '编辑' + '-' + name

    const query = { ...route.query }
    delete query.focusMenuId
    router.replace({ path: route.path, query })
}

const initList = () => {
    loading.value = true
    getMenuTree().then((res) => {
        var data = parseTreeData(res.data.data)
        treeData.value = data
        treeData2.value = [{"name":"根系统目录","key":0,"menu_type":"M","children":treeData.value}]
        console.log(treeData2)
        tryOpenFocusedMenu()
        loading.value = false
    }).finally(() => {
        loading.value = false;
    })
}

initList()
onMounted(() => {
})
const menu_assign_title = ref("错误界面")
const saveItem = (id,name) => {
    itemAssignVisible.value = true
    menu_id.value = id
    menu_assign_title.value = "编辑" + "-" + name
}
const cancel = () => {
}

// 新增
const HandleAdd = () => {
    itemAssignVisible.value = true
    menu_assign_title.value = "菜单添加"
    menu_id.value = -1

}
// 删除
const setRowLoading = (id, isLoading) => {
    rowLoadingStates[id] = isLoading;
};
const delconfirm = (id) => {
    setRowLoading(id, true)
    deleteMenuById(id).then((res) => {
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
</script>
<style scoped>
.tools {
    margin-bottom: 20px;
}
</style>