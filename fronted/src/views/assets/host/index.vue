<template>
    <a-row :gutter="16" class="host-page">
        <a-col :span="6">
            <a-card title="主机分组" size="small" class="group-card" :body-style="{ padding: '12px' }">
                <template #extra>
                    <a-space>
                        <a-button size="small" v-permission="'assets:hostgroups:create'" @click="handleCreateGroup">
                            <FontAwesomeIcon :icon="['fas', 'fa-plus']" />
                        </a-button>
                        <a-button size="small" type="primary" ghost class="refresh-btn" @click="refreshGroups">刷新</a-button>
                    </a-space>
                </template>

                <a-input-search
                    v-model:value="groupSearchText"
                    class="group-search"
                    placeholder="搜索分组名称"
                    allow-clear
                    @search="onGroupSearch"
                    @change="onGroupSearchChange"
                />

                <div class="group-toolbar">
                    <a-button size="small" block class="all-group-btn" @click="selectAllGroups">全部分组</a-button>
                </div>

                <a-spin :spinning="groupLoading">
                    <a-tree
                        block-node
                        show-line
                        :tree-data="filteredGroupTreeData"
                        :expanded-keys="groupTreeExpandedKeys"
                        :selected-keys="selectedGroupKeys"
                        @select="onGroupSelect"
                        @rightClick="onGroupRightClick"
                    />
                </a-spin>

                <!-- 右键上下文菜单 -->
                <div
                    v-if="groupContextMenu.visible"
                    class="group-context-menu"
                    :style="{ left: groupContextMenu.x + 'px', top: groupContextMenu.y + 'px' }"
                    @mouseleave="closeGroupContextMenu"
                >
                    <a-menu>
                        <a-menu-item key="create" @click="handleContextCreate">
                            <FontAwesomeIcon :icon="['fas', 'fa-plus']" />
                            <span>&nbsp;新增</span>
                        </a-menu-item>
                        <a-menu-item
                            key="edit"
                            :disabled="groupContextMenu.node?.key === 0"
                            @click="handleContextEdit"
                        >
                            <FontAwesomeIcon :icon="['fas', 'pen-to-square']" />
                            <span>&nbsp;修改</span>
                        </a-menu-item>
                        <a-menu-item
                            key="delete"
                            danger
                            :disabled="groupContextMenu.node?.key === 0"
                            @click="handleContextDelete"
                        >
                            <FontAwesomeIcon :icon="['fas', 'trash-can']" />
                            <span>&nbsp;删除</span>
                        </a-menu-item>
                    </a-menu>
                </div>
            </a-card>
        </a-col>

        <a-col :span="18">
            <a-row class="tools" :gutter="16">
                <a-col :span="10">
                    <a-input-search
                        class="tool-item"
                        v-model:value="searchText"
                        placeholder="主机名 / IP / 备注"
                        enter-button
                        size="large"
                        @search="onSearch"
                    />
                </a-col>
                <a-col :span="4">
                    <a-select
                        v-model:value="collectStatusFilter"
                        class="tool-item"
                        size="large"
                        style="width: 100%;"
                        :options="collectStatusOptions"
                        placeholder="采集状态"
                        allowClear
                        @change="onCollectStatusChange"
                    />
                </a-col>
                <a-col class="AddBtn tool-item" v-permission="'assets:hosts:create'">
                    <a-button size="large" @click="handleAdd">
                        <FontAwesomeIcon :icon="['fas', 'fa-plus-circle']" />
                        <span>&nbsp;新增主机</span>
                    </a-button>
                </a-col>
                <a-col class="tool-item" v-permission="'assets:hosts:view'">
                    <a-button size="large" type="primary" ghost class="refresh-btn" @click="refreshList" :disabled="loading">
                        <FontAwesomeIcon :icon="['fas', 'arrows-rotate']" :spin="loading" />
                        <span>&nbsp;刷新</span>
                    </a-button>
                </a-col>
                <a-col class="BatchCollectBtn tool-item" v-if="state.selectedRowKeys.length >= 1" v-permission="'assets:hosts:update'">
                    <a-button size="large" type="primary" :loading="state.collectLoading" @click="confirmBatchCollect">
                        <FontAwesomeIcon :icon="['fas', 'download']" />批量采集
                    </a-button>
                </a-col>
                <a-col class="BatchDelBtn tool-item" v-if="state.selectedRowKeys.length >= 1" v-permission="'assets:hosts:delete'">
                    <a-popconfirm
                        placement="top"
                        title="您确定要删除选中的主机么？"
                        ok-text="确认"
                        cancel-text="取消"
                        @confirm="confirmBatchDelete"
                        @cancel="cancel"
                        :overlayStyle="{ width: '300px', minHeight: '200px' }"
                    >
                        <a-button size="large" type="primary" :loading="state.loading" danger>
                            <FontAwesomeIcon :icon="['fas', 'trash-can']" />批量删除
                        </a-button>
                    </a-popconfirm>
                </a-col>
                <a-col>
                    <div class="selectedItems" v-if="state.selectedRowKeys.length >= 1">
                        <span style="height: 100%;">已选择{{ state.selectedRowKeys.length }}项</span>
                    </div>
                </a-col>
            </a-row>

            <a-card size="small" :bordered="false" class="list-card host-card">
                <template #title>
                    <span>主机列表</span>
                    <a-tag v-if="selectedGroupId" color="cyan" style="margin-left: 8px;">{{ selectedGroupName || '当前分组' }}</a-tag>
                    <a-tag v-else color="default" style="margin-left: 8px;">全部分组</a-tag>
                </template>

                <a-table
                    :scroll="{ x: 1600 }"
                    :row-selection="{ selectedRowKeys: state.selectedRowKeys, onChange: onSelectChange }"
                    rowKey="id"
                    :columns="columns"
                    :data-source="datasources"
                    :pagination="pagination"
                    :loading="loading"
                    :row-class-name="getRowClassName"
                    @change="handleTableChange"
                >
                    <template #bodyCell="{ column, record }">
                            <template v-if="column.key === 'hostname'">
                                <span class="hostname-value">{{ record.system?.hostname || '-' }}</span>
                            </template>
                        <template v-if="column.key === 'group_name'">
                            <a-tag color="blue">{{ getGroupName(record) }}</a-tag>
                        </template>
                        <template v-else-if="column.key === 'credential_name'">
                            <a v-if="getCredentialName(record) !== '-'" class="credential-link"
                                @click="goCredential(getCredentialName(record))">
                                <FontAwesomeIcon :icon="['fas', 'fa-key']" />&nbsp;{{ getCredentialName(record) }}
                            </a>
                            <span v-else>
                                <FontAwesomeIcon :icon="['fas', 'fa-key']" />&nbsp;-
                            </span>
                        </template>
                        <template v-else-if="column.key === 'collect_status'">
                            <a-tooltip v-if="record.collect_status === 'failed'">
                                <template #title>
                                    <div>{{ record.collect_message || '无法连接到该主机' }}</div>
                                    <div v-if="record.collect_time" style="margin-top: 4px; opacity: 0.85;">
                                        失败时间：{{ formatDateTime(record.collect_time) }}
                                    </div>
                                </template>
                                <a-tag color="error" style="cursor: default;">无法连接</a-tag>
                            </a-tooltip>
                            <a-tag v-else-if="record.collect_status === 'success'" color="success">正常</a-tag>
                            <a-tag v-else color="default">未采集</a-tag>
                        </template>
                        <template v-else-if="column.key === 'cpu_cores'">
                            <span>{{ record.hardware?.cpu_cores ?? '-' }} 核</span>
                        </template>
                        <template v-else-if="column.key === 'memory_gb'">
                            <span>{{ formatSize(record.hardware?.memory_gb) }}</span>
                        </template>
                        <template v-else-if="column.key === 'disk_total_gb'">
                            <span>{{ formatSize(record.hardware?.disk_total_gb) }}</span>
                        </template>
                        <template v-else-if="column.key === 'disk_used_percent'">
                            <span>{{ formatPercent(record.hardware?.disk_used_percent ?? record.disk_used_percent) }}</span>
                        </template>
                        <template v-else-if="column.key === 'action'">
                            <div :key="record.id">
                                <a-row :gutter="6" class="action_row" :wrap="false">
                                    <a-col v-permission="'assets:hosts:view'">
                                        <a-button @click="openDetail(record.id)">
                                            <FontAwesomeIcon :icon="['fas', 'fa-circle-info']" />
                                        </a-button>
                                    </a-col>
                                    <a-col v-permission="'assets:hosts:update'">
                                        <a-button @click="handleCollect(record.id)" :loading="rowLoadingStates[record.id]" type="default">
                                            <FontAwesomeIcon :icon="['fas', 'download']" />
                                        </a-button>
                                    </a-col>
                                    <a-col v-permission="'assets:hosts:update'">
                                        <a-button type="primary" @click="onSaveOrCreate(record.id)">
                                            <FontAwesomeIcon :icon="['fa', 'edit']" />
                                        </a-button>
                                    </a-col>
                                    <a-col v-permission="'assets:hosts:delete'">
                                        <a-popconfirm
                                            placement="bottom"
                                            title="您确定要删除么？"
                                            ok-text="确认"
                                            cancel-text="取消"
                                            @confirm="delconfirm(record.id)"
                                            @cancel="cancel"
                                            :overlayStyle="{ width: '200px', minHeight: '150px' }"
                                        >
                                            <a-button class="delBtn" :loading="rowLoadingStates['delete_' + record.id]" danger type="primary">
                                                <FontAwesomeIcon :icon="['fas', 'trash-can']" />
                                            </a-button>
                                        </a-popconfirm>
                                    </a-col>
                                </a-row>
                            </div>
                        </template>
                    </template>
                </a-table>
            </a-card>
        </a-col>
    </a-row>

    <Dialog
        :open="groupDialogVisible"
        @update:open="(value) => { groupDialogVisible = value }"
        :item_id="groupDialogId"
        :title="groupDialogTitle"
        :treeData="groupDialogTreeData"
        :default_parent_id="groupDialogDefaultParentId"
        :max_tree_depth="groupMaxTreeDepth"
        @initList="refreshGroups"
    />

    <a-drawer
        v-model:open="detailVisible"
        title="主机详情"
        width="760"
        destroyOnClose
        :closable="true"
    >
        <a-spin :spinning="detailLoading">
            <div v-if="detailHost" class="collect-time-banner">
                <div class="collect-time-title">最后采集时间</div>
                <div class="collect-time-value">{{ formatDateTime(detailHost.last_collect_time) }}</div>
            </div>

            <a-descriptions bordered :column="2" size="small" v-if="detailHost">
                <a-descriptions-item label="实例名">{{ detailHost.instance_name || '-' }}</a-descriptions-item>
                <a-descriptions-item label="IP 地址">{{ detailHost.ip || '-' }}</a-descriptions-item>
                <a-descriptions-item label="主机分组">{{ getGroupName(detailHost) }}</a-descriptions-item>
                <a-descriptions-item label="SSH 凭证">{{ getCredentialName(detailHost) }}</a-descriptions-item>
                <a-descriptions-item label="SSH 端口">{{ detailHost.port || 22 }}</a-descriptions-item>
                <a-descriptions-item label="OS 类型">{{ detailHost.system?.os_type || '-' }}</a-descriptions-item>
                <a-descriptions-item label="OS 版本">{{ detailHost.system?.os_version || '-' }}</a-descriptions-item>
                <a-descriptions-item label="内核版本">{{ detailHost.system?.kernel_version || '-' }}</a-descriptions-item>
                <a-descriptions-item label="主机名称">{{ detailHost.system?.hostname || '-' }}</a-descriptions-item>
                <a-descriptions-item label="CPU 核数">{{ detailHost.hardware?.cpu_cores ?? '-' }}</a-descriptions-item>
                <a-descriptions-item label="内存">{{ formatSize(detailHost.hardware?.memory_gb) }}</a-descriptions-item>
                <a-descriptions-item label="磁盘总量">{{ formatSize(detailHost.hardware?.disk_total_gb) }}</a-descriptions-item>
                <a-descriptions-item label="磁盘使用率">{{ formatPercent(detailHost.hardware?.disk_used_percent ?? detailHost.disk_used_percent) }}</a-descriptions-item>
                <a-descriptions-item label="备注" :span="2">{{ detailHost.remark || '-' }}</a-descriptions-item>
            </a-descriptions>

            <a-card title="磁盘信息" style="margin-top: 16px;" v-if="detailHost && getDisks(detailHost).length">
                <a-table :columns="diskColumns" :data-source="getDisks(detailHost)" :pagination="false" rowKey="device" size="small">
                    <template #bodyCell="{ column, record }">
                        <template v-if="column.key === 'size_gb'">
                            {{ formatSize(record.size_gb) }}
                        </template>
                        <template v-else-if="column.key === 'used_gb'">
                            {{ formatSize(record.used_gb) }}
                        </template>
                        <template v-else-if="column.key === 'usage_percent'">
                            {{ formatPercent(record.usage_percent) }}
                        </template>
                    </template>
                </a-table>
            </a-card>
        </a-spin>
    </a-drawer>

    <a-modal
        cancelText="取消"
        okText="保存"
        destroyOnClose
        :open="dialogVisible"
        v-model:title="dialogTitle"
        @ok="handleOk"
        @cancel="handleCancel"
        width="680px"
    >
        <a-spin :spinning="dialogLoading">
            <a-form
                v-if="!dialogLoading"
                :label-col="{ span: 6 }"
                :wrapper-col="{ span: 18 }"
                :model="form"
                ref="formRef"
                name="basic"
                autocomplete="off"
                :rules="rules"
            >
                <a-form-item name="instance_name" label="实例名">
                    <a-input v-model:value="form.instance_name" placeholder="例如：app-01" />
                </a-form-item>
                <a-form-item name="ip" label="IP 地址">
                    <a-input v-model:value="form.ip" placeholder="例如：192.168.1.10" />
                </a-form-item>
                <a-form-item name="group_id" label="主机分组">
                    <a-tree-select
                        v-model:value="form.group_id"
                        placeholder="请选择分组"
                        :tree-data="groupTreeSelectData"
                        allowClear
                        show-search
                        treeNodeFilterProp="title"
                        :dropdown-style="{ maxHeight: '300px', overflow: 'auto' }"
                    />
                </a-form-item>
                <a-form-item name="credential_id" label="SSH 凭证">
                    <a-select
                        v-model:value="form.credential_id"
                        placeholder="请选择凭证"
                        :options="credentialOptions"
                        allowClear
                        show-search
                        optionFilterProp="label"
                    />
                </a-form-item>
                <a-form-item name="port" label="SSH 端口">
                    <a-input-number v-model:value="form.port" :min="1" :max="65535" placeholder="默认：22" />
                </a-form-item>
                <a-form-item name="remark" label="备注">
                    <a-textarea v-model:value="form.remark" :rows="3" placeholder="可填写业务用途、负责人等信息" />
                </a-form-item>
            </a-form>
        </a-spin>
    </a-modal>
</template>

<script setup>
defineOptions({
    name: 'host'
})

import { computed, onMounted, onBeforeUnmount, reactive, ref } from 'vue'
import { message } from 'ant-design-vue'
import { useRouter } from 'vue-router'
import { batchDeleteHost, collectHostInfo, batchCollectHostInfo, deleteHostById, getHostById, getHostList, saveOrCreateHost } from '@/api/assets/host/index.js'
import { getHostGroupTree, deleteHostGroupById } from '@/api/assets/hostgroup/index.js'
import { getCredentailList } from '@/api/assets/credential/index.js'
import { getConfigByKey, CONFIG_KEYS } from '@/api/sys/sysconfig.js'
import Dialog from '@/views/assets/hostgroup/components/Dialog.vue'
import { formatTimeWithTimezone } from '@/util/timezone'

const searchText = ref('')
const groupSearchText = ref('')
const activeSearchText = ref('')
const collectStatusFilter = ref(undefined)
const selectedGroupId = ref(0)
const selectedGroupName = ref('全部分组')
const groupDialogVisible = ref(false)
const groupDialogTitle = ref('新增分组')
const groupDialogId = ref(-1)
const groupDialogDefaultParentId = ref(null)
const groupMaxTreeDepth = ref(5)
const loading = ref(false)
const groupLoading = ref(false)
const dialogVisible = ref(false)
const dialogTitle = ref('新增主机')
const dialogLoading = ref(false)
const detailVisible = ref(false)
const detailLoading = ref(false)
const detailHost = ref(null)
const formRef = ref(null)

const state = reactive({
    selectedRowKeys: [],
    loading: false,
    collectLoading: false,
})

const rowLoadingStates = reactive({})
const groupTreeData = ref([])
const expandedGroupKeys = ref([])
const credentialOptions = ref([])
const datasources = ref([])

const form = reactive({
    id: -1,
    ip: '',
    group_id: undefined,
    credential_id: undefined,
    port: 22,
    remark: '',
    instance_name: '',
})

const rules = {
    instance_name: [{ required: true, message: '请输入实例名' }],
    ip: [{ required: true, message: '请输入 IP 地址' }],
    credential_id: [{ required: true, message: '请选择 SSH 凭证' }],
    port: [{ required: true, message: '请输入 SSH 端口' }],
}

const columns = [
    { title: '实例名', dataIndex: 'instance_name', key: 'instance_name', width: 160 },
    { title: '采集状态', dataIndex: 'collect_status', key: 'collect_status', width: 110 },
    { title: '主机名称', dataIndex: 'hostname', key: 'hostname', width: 160 },
    { title: '主机分组', dataIndex: 'group_name', key: 'group_name', width: 130 },
    { title: 'IP 地址', dataIndex: 'ip', key: 'ip', width: 150 },
    { title: 'SSH 端口', dataIndex: 'port', key: 'port', width: 100 },
    { title: 'SSH 凭证', dataIndex: 'credential_name', key: 'credential_name', width: 170 },
    { title: 'OS 类型', dataIndex: 'os_type', key: 'os_type', width: 120 },
    { title: 'OS 版本', dataIndex: 'os_version', key: 'os_version', width: 160 },
    { title: '内核版本', dataIndex: 'kernel_version', key: 'kernel_version', width: 160 },
    { title: 'CPU 核数', dataIndex: 'cpu_cores', key: 'cpu_cores', width: 100 },
    { title: '内存', dataIndex: 'memory_gb', key: 'memory_gb', width: 110 },
    { title: '磁盘总量', dataIndex: 'disk_total_gb', key: 'disk_total_gb', width: 110 },
    { title: '磁盘使用率', dataIndex: 'disk_used_percent', key: 'disk_used_percent', width: 120 },
    { title: '备注', dataIndex: 'remark', key: 'remark', ellipsis: true },
    { title: '操作', key: 'action', fixed: 'right', width: 220 },
]

const collectStatusOptions = [
    { label: '全部状态', value: undefined },
    { label: '无法连接', value: 'failed' },
    { label: '正常', value: 'success' },
    { label: '未采集', value: 'unknown' },
]

const diskColumns = [
    { title: '设备', dataIndex: 'device', key: 'device' },
    { title: '挂载点', dataIndex: 'mount_point', key: 'mount_point' },
    { title: '容量', dataIndex: 'size_gb', key: 'size_gb' },
    { title: '已用', dataIndex: 'used_gb', key: 'used_gb' },
    { title: '使用率', dataIndex: 'usage_percent', key: 'usage_percent' },
    { title: '文件系统', dataIndex: 'filesystem', key: 'filesystem' },
]

const pagination = reactive({
    current: 1,
    pageSize: 10,
    total: 0,
    showTotal: (value) => `共有${value}条数据`,
    pageSizeOptions: ['10', '20', '30'],
    showQuickJumper: true,
})

const buildTreeData = (nodes, depth = 1) => {
    return nodes.map((item) => ({
        title: `${item.name}${item.host_count !== undefined && item.host_count !== null ? ` (${item.host_count})` : ''}`,
        key: item.id,
        name: item.name,
        host_count: item.host_count,
        depth,
        children: item.children && item.children.length && depth < groupMaxTreeDepth.value ? buildTreeData(item.children, depth + 1) : undefined,
    }))
}

const buildGroupTreeSelectData = (nodes = []) => {
    return nodes.map((item) => ({
        title: item.name,
        value: item.key,
        key: item.key,
        children: item.children && item.children.length ? buildGroupTreeSelectData(item.children) : undefined,
    }))
}

const filterGroupTree = (nodes, keyword) => {
    const search = keyword.trim().toLowerCase()
    if (!search) {
        return nodes
    }

    return nodes
        .map((item) => {
            const children = item.children ? filterGroupTree(item.children, search) : []
            const matched = String(item.name || '').toLowerCase().includes(search)
            if (matched || children.length) {
                return {
                    ...item,
                    children: children.length ? children : undefined,
                }
            }
            return null
        })
        .filter(Boolean)
}

const collectExpandedKeys = (nodes) => {
    const keys = [0]
    const walk = (items) => {
        items.forEach((item) => {
            if (item.children && item.children.length) {
                keys.push(item.key)
                walk(item.children)
            }
        })
    }
    walk(nodes)
    return Array.from(new Set(keys))
}

const groupTreeExpandedKeys = computed(() => {
    return expandedGroupKeys.value
})

const filteredGroupTreeData = computed(() => {
    return filterGroupTree(groupTreeData.value, groupSearchText.value)
})

const groupDialogTreeData = computed(() => {
    return groupTreeData.value[0]?.children || []
})

const groupTreeSelectData = computed(() => {
    const children = groupTreeData.value[0]?.children || []
    return buildGroupTreeSelectData(children)
})

const loadGroupTree = async () => {
    groupLoading.value = true
    try {
        const res = await getHostGroupTree()
        if (res.data.code === 200) {
            const data = res.data.data || []
            groupTreeData.value = [
                {
                    title: '全部分组',
                    key: 0,
                    name: '全部分组',
                    host_count: null,
                    children: buildTreeData(data, 1),
                },
            ]
            expandedGroupKeys.value = collectExpandedKeys(groupTreeData.value)
        } else {
            message.error(res.data.msg || '获取分组失败')
        }
    } finally {
        groupLoading.value = false
    }
}

const loadCredentialOptions = async () => {
    try {
        const res = await getCredentailList({ page: 1, size: 1000 })
        if (res.data.code === 200) {
            const rows = res.data.data.results || []
            credentialOptions.value = rows.map((item) => ({
                label: `${item.name} (${item.username}@${item.port})`,
                value: item.id,
            }))
        }
    } catch (error) {
        console.log(error)
    }
}

const loadHostList = async () => {
    loading.value = true
    try {
        const params = {
            page: pagination.current,
            size: pagination.pageSize,
            search: activeSearchText.value,
        }
        if (collectStatusFilter.value) {
            params.collect_status = collectStatusFilter.value
        }
        if (selectedGroupId.value && selectedGroupId.value !== 0) {
            params.group_id = selectedGroupId.value
        }

        const res = await getHostList(params)
        if (res.data.code === 200) {
            datasources.value = res.data.data.results || []
            pagination.total = res.data.data.count || 0
        } else {
            message.error(res.data.msg || '获取主机列表失败')
            datasources.value = []
            pagination.total = 0
        }
    } finally {
        loading.value = false
    }
}

const refreshList = async () => {
    await loadHostList()
}

const refreshGroups = async () => {
    await loadGroupTree()
    await refreshList()
}

const selectedGroupKeys = computed(() => [selectedGroupId.value])

const selectAllGroups = async () => {
    selectedGroupId.value = 0
    selectedGroupName.value = '全部分组'
    pagination.current = 1
    await refreshList()
}

// 右键菜单状态
const groupContextMenu = reactive({
    visible: false,
    x: 0,
    y: 0,
    node: null,
})

const onGroupRightClick = ({ event, node }) => {
    event.preventDefault()
    groupContextMenu.node = node
    groupContextMenu.x = event.clientX
    groupContextMenu.y = event.clientY
    groupContextMenu.visible = true
}

const closeGroupContextMenu = () => {
    groupContextMenu.visible = false
}

const handleContextCreate = () => {
    const node = groupContextMenu.node
    closeGroupContextMenu()
    groupDialogId.value = -1
    groupDialogTitle.value = '新增分组'
    groupDialogDefaultParentId.value = (node && node.key !== 0) ? node.key : null
    groupDialogVisible.value = true
}

const handleContextEdit = () => {
    const node = groupContextMenu.node
    closeGroupContextMenu()
    if (!node || node.key === 0) return
    groupDialogId.value = node.key
    groupDialogTitle.value = '编辑分组'
    groupDialogVisible.value = true
}

const handleContextDelete = async () => {
    const node = groupContextMenu.node
    closeGroupContextMenu()
    if (!node || node.key === 0) return
    try {
        const res = await deleteHostGroupById(node.key)
        if (res.data.code === 200 || res.status === 204) {
            message.success('删除成功')
            await loadGroupTree()
            if (selectedGroupId.value === node.key) {
                selectedGroupId.value = 0
                selectedGroupName.value = '全部分组'
                await refreshList()
            }
        } else {
            message.error(res.data?.msg || '删除失败')
        }
    } catch (e) {
        message.error('删除失败')
    }
}

const handleCreateGroup = () => {
    groupDialogId.value = -1
    groupDialogTitle.value = '新增分组'
    groupDialogDefaultParentId.value = null
    groupDialogVisible.value = true
}

const onGroupSearch = () => {}

const onGroupSearchChange = () => {}

const onGroupSelect = async (selectedKeys, info) => {
    const key = selectedKeys && selectedKeys.length ? selectedKeys[0] : 0
    selectedGroupId.value = key || 0
    selectedGroupName.value = key === 0 ? '全部分组' : (info?.node?.dataRef?.name || '当前分组')
    searchText.value = ''
    activeSearchText.value = ''
    pagination.current = 1
    await refreshList()
}

const resetForm = () => {
    form.id = -1
    form.instance_name = ''
    form.ip = ''
    form.group_id = selectedGroupId.value && selectedGroupId.value !== 0 ? selectedGroupId.value : undefined
    form.credential_id = undefined
    form.port = 22
    form.remark = ''
}

const handleAdd = () => {
    resetForm()
    dialogTitle.value = '新增主机'
    dialogVisible.value = true
}

const handleApiError = (err) => {
    // 从响应中提取错误数据
    const responseData = err?.response?.data || err?.data
    // 如果是统一格式的错误响应（code 和 msg），data 字段才是实际错误内容
    let errorObj = responseData
    if (responseData && responseData.data && typeof responseData.data === 'object') {
        errorObj = responseData.data
    }
    
    if (errorObj && typeof errorObj === 'object') {
        const firstKey = Object.keys(errorObj)[0]
        const msg = Array.isArray(errorObj[firstKey]) ? errorObj[firstKey][0] : errorObj[firstKey]
        if (msg) { message.error(msg); return }
    }
    message.error('操作失败，请稍后重试')
}

const onSaveOrCreate = (id) => {
    dialogTitle.value = '编辑主机'
    dialogVisible.value = true
    dialogLoading.value = true
    getHostById(id)
        .then((res) => {
            if (res.data.code === 200) {
                const data = res.data.data || {}
                form.id = data.id ?? id
                form.instance_name = data.instance_name || ''
                form.ip = data.ip || ''
                form.group_id = data.group ?? data.group_id ?? undefined
                form.credential_id = data.credential?.id ?? undefined
                form.port = data.port ?? 22
                form.remark = data.remark || ''
            } else {
                message.error(res.data.msg || '获取主机详情失败')
            }
        })
        .finally(() => {
            dialogLoading.value = false
        })
}

const handleOk = () => {
    formRef.value?.validate().then(() => {
        dialogLoading.value = true
        const payload = { ...form }
        delete payload.name
        saveOrCreateHost(payload)
            .then((res) => {
                if (res.data.code === 200) {
                    message.success(form.id === -1 ? '新增主机成功' : '保存主机成功')
                    dialogVisible.value = false
                    refreshGroups()
                } else {
                    // 任何非 200 的返回都认为是错误
                    handleApiError({data: res.data})
                }
            })
            .catch(handleApiError)
            .finally(() => {
                dialogLoading.value = false
            })
    })
}

const handleCancel = () => {
    dialogVisible.value = false
}

const onSearch = async () => {
    activeSearchText.value = searchText.value.trim()
    pagination.current = 1
    await refreshList()
}

const onCollectStatusChange = async () => {
    pagination.current = 1
    await refreshList()
}

const handleTableChange = async (pageConfig) => {
    pagination.current = pageConfig.current
    pagination.pageSize = pageConfig.pageSize
    await refreshList()
}

const onSelectChange = (selectedRowKeys) => {
    state.selectedRowKeys = selectedRowKeys
}

const cancel = () => {}

const delconfirm = (id) => {
    rowLoadingStates['delete_' + id] = true
    deleteHostById(id)
        .then((res) => {
            if (res.data.code === 200) {
                message.success('删除成功')
                refreshGroups()
            } else {
                message.error(res.data.msg || '删除失败')
            }
        })
        .catch((error) => {
            message.error(error?.response?.data?.msg || error?.message || '删除失败')
        })
        .finally(() => {
            rowLoadingStates['delete_' + id] = false
        })
}

const handleCollect = (id) => {
    rowLoadingStates[id] = true
    collectHostInfo(id)
        .then((res) => {
            if (res.data.code === 200) {
                const result = res.data.data
                // 检查采集状态：status 应该是 'collected' 或 'failed'
                if (result.status === 'collected') {
                    message.success(result.message || '采集成功')
                } else if (result.status === 'failed') {
                    message.error(result.message || result.error || '采集失败')
                } else {
                    message.success(result.message || '采集成功')
                }
                refreshList()
            } else {
                message.error(res.data.msg || '采集失败')
            }
        })
        .catch((error) => {
            message.error(error?.response?.data?.msg || error?.message || '采集失败')
        })
        .finally(() => {
            rowLoadingStates[id] = false
        })
}

const confirmBatchCollect = () => {
    state.collectLoading = true
    batchCollectHostInfo(state.selectedRowKeys)
        .then((res) => {
            if (res.data.code === 200) {
                const successCount = (res.data.data.results || []).filter((item) => item.status === 'collected').length
                const failedCount = (res.data.data.results || []).filter((item) => item.status === 'failed').length
                message.success(`已采集 ${successCount} 台主机${failedCount ? `，${failedCount} 台失败` : ''}`)
                refreshList()
            } else {
                message.error(res.data.msg || '批量采集失败')
            }
        })
        .catch((error) => {
            message.error(error?.response?.data?.msg || error?.message || '批量采集失败')
        })
        .finally(() => {
            state.collectLoading = false
        })
}

const confirmBatchDelete = () => {
    state.loading = true
    batchDeleteHost(state.selectedRowKeys)
        .then((res) => {
            if (res.data.code === 200) {
                message.success('批量删除成功')
                state.selectedRowKeys = []
                refreshGroups()
            } else {
                message.error(res.data.msg || '批量删除失败')
            }
        })
        .finally(() => {
            state.loading = false
        })
}

const openDetail = (id) => {
    detailVisible.value = true
    detailLoading.value = true
    getHostById(id)
        .then((res) => {
            if (res.data.code === 200) {
                detailHost.value = res.data.data || null
            } else {
                message.error(res.data.msg || '获取主机详情失败')
            }
        })
        .finally(() => {
            detailLoading.value = false
        })
}

const getGroupName = (record) => {
    return record.group_name || record.group?.name || '-'
}

const getCredentialName = (record) => {
    return record.credential_name || record.credential?.name || '-'
}

const router = useRouter()
const goCredential = (name) => {
    if (!name || name === '-') return
    router.push({ path: '/assets/credentials/index', query: { search: name } })
}

const getRowClassName = (record) => {
    return record.collect_status === 'failed' ? 'row-collect-failed' : ''
}

const getDisks = (record) => {
    return record.disks || []
}

const formatSize = (value) => {
    if (value === null || value === undefined || value === '') {
        return '-'
    }
    return `${value} GB`
}

const formatPercent = (value) => {
    if (value === null || value === undefined || value === '') {
        return '-'
    }
    return `${Number(value).toFixed(2)}%`
}

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
        const timezone = Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC'
        return formatTimeWithTimezone(normalizeUtcTime(value), timezone, 'YYYY-MM-DD HH:mm:ss')
    } catch (error) {
        return value
    }
}

onMounted(async () => {
    // 先加载主机分组最大层级配置，再构建树（buildTreeData 依赖此值）
    await getConfigByKey(CONFIG_KEYS.HOSTGROUP_MAX_TREE_DEPTH).then(res => {
        if (res.data?.value) groupMaxTreeDepth.value = Number(res.data.value) || 5
    }).catch(() => {})
    await Promise.all([loadGroupTree(), loadCredentialOptions()])
    await refreshList()
    document.addEventListener('click', closeGroupContextMenu)
})

onBeforeUnmount(() => {
    document.removeEventListener('click', closeGroupContextMenu)
})
</script>

<style scoped>
.credential-link {
    color: #1677ff;
    cursor: pointer;
}

.credential-link:hover {
    text-decoration: underline;
}

:deep(.row-collect-failed) > td {
    background-color: #fff1f0;
}

:deep(.row-collect-failed:hover) > td {
    background-color: #ffe0de !important;
}

.host-page {
    align-items: flex-start;
    font-size: 15px;
    padding: 12px 6px 18px;
    background:
        radial-gradient(circle at top left, rgba(15, 23, 42, 0.04), transparent 28%),
        linear-gradient(180deg, #f8fafc 0%, #f4f7fb 100%);
    border-radius: 16px;
}

.group-toolbar {
    margin-bottom: 12px;
}

.collect-time-banner {
    margin-bottom: 14px;
    padding: 12px 14px;
    border-radius: 10px;
    border: 1px solid #91caff;
    background: linear-gradient(90deg, #e6f4ff 0%, #f0f5ff 100%);
}

.collect-time-title {
    font-size: 12px;
    color: #1d39c4;
    margin-bottom: 4px;
}

.collect-time-value {
    font-size: 20px;
    font-weight: 700;
    color: #0f172a;
    letter-spacing: 0.3px;
}

.list-card {
    margin-top: 12px;
}

.group-card,
.host-card {
    border: 1px solid #e5e7eb;
    border-radius: 14px;
    box-shadow: 0 10px 30px rgba(15, 23, 42, 0.06);
    overflow: hidden;
}

.group-card {
    background: linear-gradient(180deg, #ffffff 0%, #f8fafc 100%);
    border-color: #e2e8f0;
}

.group-card :deep(.ant-card-head) {
    border-bottom: 1px solid #eef2f7;
    background: rgba(255, 255, 255, 0.9);
}

.group-card :deep(.ant-card-head-title),
.group-card :deep(.ant-card-extra) {
    color: #1f2937;
}

.group-card :deep(.ant-card-body) {
    background: rgba(255, 255, 255, 0.96);
    color: #334155;
}

.group-card :deep(.ant-card-head),
.host-card :deep(.ant-card-head) {
    border-bottom: 1px solid #eef2f7;
    background: rgba(255, 255, 255, 0.9);
}

.group-card :deep(.ant-card-body),
.host-card :deep(.ant-card-body) {
    background: rgba(255, 255, 255, 0.96);
}

.group-search {
    margin-bottom: 10px;
}

.group-card :deep(.ant-input),
.group-card :deep(.ant-input-affix-wrapper),
.group-card :deep(.ant-input-search .ant-input),
.group-card :deep(.ant-input-search .ant-btn) {
    background: #ffffff;
    border-color: #dbe4ef;
    color: #0f172a;
}

.group-card :deep(.ant-input::placeholder) {
    color: #94a3b8;
}

.all-group-btn {
    border-color: #dbe4ef;
    background: #f8fafc;
    color: #334155;
}

.all-group-btn:hover {
    border-color: #94a3b8;
    color: #0f172a;
    background: #eef2f7;
}

.host-page :deep(.ant-card-head-title) {
    color: #0f172a;
    font-weight: 700;
}

.host-page :deep(.ant-tree .ant-tree-treenode) {
    color: #475569;
}

.host-page :deep(.ant-tree .ant-tree-node-content-wrapper) {
    border-radius: 6px;
    transition: background-color 0.15s ease, color 0.15s ease, box-shadow 0.15s ease;
}

.host-page :deep(.ant-tree .ant-tree-node-content-wrapper:hover) {
    background: #eef2ff;
    color: #1e293b;
}

.host-page :deep(.ant-tree .ant-tree-node-selected),
.host-page :deep(.ant-tree .ant-tree-node-content-wrapper.ant-tree-node-selected) {
    background: #eff6ff !important;
    color: #1d4ed8 !important;
    font-weight: 600;
    box-shadow: inset 3px 0 0 #3b82f6;
}

.host-page :deep(.ant-tree .ant-tree-node-content-wrapper.ant-tree-node-selected .ant-tree-title) {
    color: #1d4ed8 !important;
}

.host-page :deep(.ant-tag) {
    border: 0;
}

.host-page :deep(.ant-tag-blue) {
    background: #1677ff;
    color: #ffffff;
}

.host-page :deep(.ant-tag-green) {
    background: #16a34a;
    color: #ffffff;
}

.host-page :deep(.ant-tag-cyan) {
    background: #1677ff;
    color: #ffffff;
}

.host-page :deep(.ant-tag-default) {
    background: #e2e8f0;
    color: #334155;
    border: 0;
}

/* 分组右键菜单 */
.group-context-menu {
    position: fixed;
    z-index: 9999;
    background: #fff;
    border-radius: 6px;
    box-shadow: 0 4px 16px rgba(0, 0, 0, 0.14);
    min-width: 120px;
    padding: 4px 0;
    user-select: none;
}

.group-context-menu :deep(.ant-menu) {
    border: none;
    box-shadow: none;
}

.host-page :deep(.ant-card-head-title),
.host-page :deep(.ant-card-extra),
.host-page :deep(.ant-tabs-tab),
.host-page :deep(.ant-btn),
.host-page :deep(.ant-input),
.host-page :deep(.ant-select-selector),
.host-page :deep(.ant-tree),
.host-page :deep(.ant-table),
.host-page :deep(.ant-form-item-label > label),
.host-page :deep(.ant-modal-title),
.host-page :deep(.ant-drawer-title),
.host-page :deep(.ant-tag) {
    font-size: 15px;
}

.host-page :deep(.ant-table-thead > tr > th),
.host-page :deep(.ant-table-tbody > tr > td) {
    font-size: 15px;
}

.action_row {
    flex-wrap: nowrap;
    align-items: center;
    white-space: nowrap;
}

.action_row :deep(.ant-col) {
    flex: 0 0 auto;
}

.host-page :deep(.ant-btn-lg),
.host-page :deep(.ant-input-search .ant-input),
.host-page :deep(.ant-input-search .ant-btn) {
    font-size: 15px;
}
</style>