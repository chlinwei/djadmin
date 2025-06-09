<template>
    <Dialog :open="itemAssignVisible" @update:open="(value) => { itemAssignVisible = value }" :item_id="item_id"
        :title="item_assign_title" @initList="initList" />
    <a-row class="tools" :gutter="16">
        <a-col :span="7">
            <a-input-search class="tool-item" v-model:value="SearchText" placeholder="全局搜索" enter-button size="large"
                @search="onSearch" />
        </a-col>
        <a-col class="AddBtn tool-item" v-permission="'assets:credentials:create'">
            <a-button size="large" @click="HandleAdd">
                <FontAwesomeIcon :icon="['fas', 'fa-plus-circle']" />
                <span>&nbsp新增</span>
            </a-button>
        </a-col>
        <a-col class="BatchAddBtn tool-item" v-permission="'assets:credentials:create'">
            <a-upload accept=".csv" :before-upload="beforeUpload" :custom-request="HandleBatchAdd" @change="uploadChange" :fileList=fileList>
                <a-button size="large" :loading="uploading">
                    <upload-outlined></upload-outlined>
                    <span>&nbsp{{uploading ? '上传中': '批量导入'}}</span>
                </a-button>
            </a-upload>

        </a-col>
        <a-col class="BatchDelBtn tool-item" v-if="state.selectedRowKeys.length >= 1" 
            v-permission="'assets:credentials:delete'">
            <a-popconfirm placement="top" title="您确定要删除么？" ok-text="确认" cancel-text="取消" @confirm="confirm"
                @cancel="cancel" :overlayStyle="{ width: '300px', minHeight: '200px' }">
                <a-button size="large" type="primary" :loading="state.loading" danger>
                    <FontAwesomeIcon :icon="['fas', 'trash']" />批量删除
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
                :columns="columns" :data-source="datasources" :pagination="pagination" :loading="loading"
                onSelectChange="onSelectChange" @change="handleTableChange">
                <template #bodyCell="{ column, record }">
                    <template v-if="column.key === 'action'">
                        <div :key="record.id">
                            <a-row :gutter="6" class="action_row">
                                <a-col v-permission="'assets:credentials:update'">
                                    <a-button type="primary" @click="onSaveOrCreate(record.id, record.name)">
                                        <FontAwesomeIcon :icon="['fa', 'edit']" />
                                    </a-button>
                                </a-col>
                                <a-col v-permission="'assets:credentials:delete'">
                                    <a-popconfirm placement="bottom" title="您确定要删除么？" ok-text="确认" cancel-text="取消"
                                        @confirm="delconfirm(record.id)" @cancel="cancel"
                                        :overlayStyle="{ width: '200px', minHeight: '150px' }">
                                        <a-button class="delBtn" :loading="rowLoadingStates[record.id]" danger
                                            type="primary">
                                            <FontAwesomeIcon :icon="['fas', 'trash']" />
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
import { usePagination } from 'vue-request';
import { computed } from 'vue'
import { message } from 'ant-design-vue';
const SearchText = ref('')
const lastSearchKeyword = ref('')
const rowLoadingStates = reactive({
});

const setRowLoading = (id, isLoading) => {
    rowLoadingStates[id] = isLoading;
};
const state = reactive({
    selectedRowKeys: [],
    // Check here to configure the default column
    loading: false,
});
import { getCredentailList } from '@/api/assets/credential';

const columns = [
    {
        title: '名称',
        dataIndex: 'name',
        key: 'name',
        sorter: true,
    },
    {
        title: '创建时间',
        dataIndex: 'create_time',
        key: 'create_time',
        sorter: true,
    },

    {
        title: '备注',
        dataIndex: 'remark',
        key: 'remark',
    },
    {
        title: '操作',
        key: 'action'
    }
];

const total = ref(0)

const queryData = params => {
    return getCredentailList(params).then(res => {
        if (res.data.code == 200) {
            lastSearchKeyword.value = res.config.params.search
            console.log("hahaha")
            console.log(res.data.data)
            total.value = res.data.data.count
            return res.data.data.results
        } else {
            message.error("获取Credential列表失败")
            return []
        }

    });
};



const {
    data: datasources,
    run,
    loading,
    current,
    pageSize,
} = usePagination(queryData,
    {
        // 这里疑问？？formatResult没什么用
        formatResult: (res) => {
            return res.data.results
        },
        pagination: {currentKey: 'page', pageSizeKey: 'size' ,}
    })



const pagination = computed(() => ({
    total: total.value,
    current: current.value,
    pageSize: pageSize.value,
    showTotal: (total) => `共有${total}条数据`,
    pageSizeOptions: ['10', '20', '30'],
    showQuickJumper: true,
}))

const initList = () => {
    run({ page: current, size: pageSize.value, keyword: lastSearchKeyword.value })
}

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
            search: lastSearchKeyword.value,
            ordering: sorter_str,
            ...filters,
        })
    } else {
        run({
            size: page.pageSize,
            page: page?.current,
            search: lastSearchKeyword.value,
            ...filters,
        })
    }
};

const onSearch = (keyword) => {
    run({ size: pageSize.value, search: keyword })
}

const onSelectChange = selectedRowKeys => {
    state.selectedRowKeys = selectedRowKeys;
};
const cancel = () => {

}
// 删除
import { batchDeleteCredential } from '@/api/assets/credential/index'
const delconfirm = (id) => {
    var ids = [id]
    setRowLoading(id, true)
    batchDeleteCredential(ids).then((res) => {
        if (res.data.code == 200) {
            message.success("删除Credential成功")
        } else {
            message.error("删除Credential失败")
        }

    }).finally(() => {
        setRowLoading(id, false)
    })
}
// 批量删除

// 批量删除权限
const confirm = () => {
    state.loading = true;
    state.selectedRowKeys.forEach((e) => {
        setRowLoading(e, true)
    })
    batchDeleteCredential(state.selectedRowKeys).then((res) => {
        if (res.data.code == 200) {
            initList();
            message.success("删除Credential成功");
            state.selectedRowKeys = []
        } else {
            message.error("删除Credential失败:" + res.data.msg);
        }

    }).finally(() => {
        state.loading = false;
        state.selectedRowKeys.forEach((e) => {
            setRowLoading(e, false)
        })
    })
}

// 批量导入
import { UploadOutlined } from '@ant-design/icons-vue'
import { batchUpload } from '@/api/assets/credential/index'
const uploading = ref(false)
// 已上传的文件
const fileList = ref([])
const beforeUpload = (file) => {
  const isCSV = file.name.endsWith('.csv')
  if (!isCSV) message.error('仅支持CSV格式')
  return isCSV
}
const HandleBatchAdd = (file) => {
    uploading.value = true
    batchUpload(file).then((res)=>{
        // 刷新
        initList()
    }).finally((res)=>{
        uploading.value = false
    })
}
const uploadChange = (info)=>{
    console.log(info.file.status)
}

// Dialog
import Dialog from '@/views/assets/credential/components/Dialog.vue'
const itemAssignVisible = ref(false)
const item_assign_title = ref("错误界面")
const item_id = ref(-1)
const onSaveOrCreate = (id, name) => {
    itemAssignVisible.value = true
    item_id.value = id
    item_assign_title.value = "编辑" + "-" + name
}
// 新增
const HandleAdd = () => {
    item_id.value = -1
    itemAssignVisible.value = true
    item_assign_title.value = "新增"
}


</script>
<style scoped>
.AddBtn>:where(.css-dev-only-do-not-override-1p3hq3p).ant-btn-default {
    background-color: green;
    color: white;
}

.BatchDelBtn>:where(.css-dev-only-do-not-override-1p3hq3p).ant-btn-default {
    background-color: orange;
    color: white;
}
.tools {
    margin-bottom: 20px;
    height: 50px;
}

.selectedItems span {
    line-height: 40px;
    /* 匹配按钮高度 */
    vertical-align: middle;
    font-size: 20px;
}
</style>