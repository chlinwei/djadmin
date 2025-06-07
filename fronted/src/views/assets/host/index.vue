<template>
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
        <a-col class="BatchAddBtn tool-item">
            <a-button size="large" @click="HandleAdd">
                <FontAwesomeIcon :icon="['fas', 'fa-plus-circle']" />
                <span>&nbsp批量导入</span>
            </a-button>
        </a-col>
    </a-row>
    <a-row>
        <a-col :span="24">
            <a-table :row-selection="{ selectedRowKeys: state.selectedRowKeys, onChange: onSelectChange }" rowKey="id"
                :columns="columns" :data-source="datasources" :pagination="pagination" :loading="loading"
                @change="handleTableChange">
                <template #bodyCell="{ column, record }">
                    <template v-if="column.key === 'action'">
                        <div :key="record.id">
                            <a-row :gutter="6" class="action_row">
                                <a-col v-if="record.name != 'admin'">
                                    <a-button type="primary" @click="onSaveorChanageRole(record.id)">
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
const SearchText =  ref('')
const state = reactive({
    selectedRowKeys: [],
    // Check here to configure the default column
    loading: false,
});
const datasources = ref([])
const columns = [
    {
        title: '类别',
        dataIndex: 'host_type',
        key: 'host_type',
    },
    {
        title: '主机名',
        dataIndex: 'hostname',
        key: 'hostname',
    },
    {
        title: '连接地址',
        dataIndex: 'ssh_ip',
        key: 'ssh_ip',
    },
    {
        title: '连接端口',
        dataIndex: 'ssh_port',
        key: 'ssh_port',
    },
        {
        title: '备注',
        dataIndex: 'remark',
        key: 'remark',
    }
];
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