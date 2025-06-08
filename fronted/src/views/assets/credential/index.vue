<template>
     <Dialog :open="itemAssignVisible" @update:open="(value) => { itemAssignVisible = value }"
        :item_id="item_id" :title="item_assign_title" @initList="initList" />
    <a-row class="tools" :gutter="16">
        <a-col :span="7">
            <a-input-search class="tool-item" v-model:value="SearchText" placeholder="全局搜索" enter-button size="large"
                @search="onSearch" />
        </a-col>
        <a-col class="AddBtn tool-item">
            <a-button size="large" @click="HandleAdd">
                <FontAwesomeIcon :icon="['fas', 'fa-plus-circle']" />
                <span>&nbsp新增</span>
            </a-button>
        </a-col>
    </a-row>
    <a-row>
        <a-col :span="24">
            <a-table :row-selection="{ selectedRowKeys: state.selectedRowKeys, onChange: onSelectChange }" rowKey="id"
                :columns="columns" :data-source="datasources" :pagination="pagination" :loading="loading" onSelectChange="onSelectChange"
                @change="handleTableChange">
                <template #bodyCell="{ column, record }">
                    <template v-if="column.key === 'action'">
                        <div :key="record.id">
                            <a-row :gutter="6" class="action_row">
                                <a-col>
                                    <a-button type="primary" @click="onSaveOrCreate(record.id,record.name)">
                                        <FontAwesomeIcon :icon="['fa','edit']" />
                                    </a-button>
                                </a-col>
                                <a-col>
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
    </a-row>
</template>
<script setup>
import { ref,reactive } from 'vue';
import { usePagination } from 'vue-request';
import {computed} from 'vue'
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
        title:'操作',
        key: 'action'
    }
];

const delconfirm = (res) => {

}

const queryData = params => {
    return getCredentailList(params).then(res => {
        console.log("Res================")
        console.log(res)
        if (res.data.code == 200) {
            lastSearchKeyword.value = res.config.params.search
            total.value = res.data.data.count
            return res.data.data.results
        } else {
            message.error("获取Credential列表失败")
            return []
        }

    });
};

const total = ref(0)

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
    run({ size: pageSize.value, search:keyword })
}

const onSelectChange = selectedRowKeys => {
    state.selectedRowKeys = selectedRowKeys;
};
const cancel = ()=> {

}

// Dialog
import Dialog from '@/views/assets/credential/components/Dialog.vue'
const itemAssignVisible = ref(false)
const item_assign_title= ref("错误界面")
const item_id = ref(-1)
const onSaveOrCreate = (id,name) => {
    console.log("打开")
    itemAssignVisible.value = true
    item_id.value = id
    item_assign_title.value = "编辑" + "-" + name
}
// 新增
const HandleAdd = ()=> {
    item_id.value = -1
    itemAssignVisible.value = true
    item_assign_title.value = "新增"
    console.log("新增")
}

</script>
<style scoped>
.AddBtn>:where(.css-dev-only-do-not-override-1p3hq3p).ant-btn-default {
    background-color: green;
    color: white;
}
BatchAddBtn>:where(.css-dev-only-do-not-override-1p3hq3p).ant-btn-default {
    background-color: green;
    color: white;
}


</style>