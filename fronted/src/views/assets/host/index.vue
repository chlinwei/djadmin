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
                        @expand="onGroupExpand"
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
                <a-col :span="7">
                    <a-input-search
                        class="tool-item"
                        v-model:value="searchText"
                        placeholder="实例名 / IP / 备注"
                        allow-clear
                        enter-button
                        size="large"
                        @search="onSearch"
                    />
                </a-col>
                <a-col :span="4">
                    <a-input-search
                        class="tool-item"
                        v-model:value="hostIdSearchText"
                        placeholder="按ID查找"
                        allow-clear
                        enter-button
                        size="large"
                        @search="onHostIdSearch"
                    />
                </a-col>
                <a-col :span="5" class="tool-item">
                    <a-radio-group
                        v-model:value="agentStatusFilter"
                        button-style="solid"
                        size="large"
                        @change="onAgentStatusChange"
                    >
                        <a-radio-button value="all">全部状态</a-radio-button>
                        <a-radio-button value="online">在线</a-radio-button>
                        <a-radio-button value="offline">离线</a-radio-button>
                    </a-radio-group>
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
                <a-col class="tool-item" v-permission="'assets:hosts:view'">
                    <a-button size="large" @click="resetAllFilters" :disabled="loading">
                        <FontAwesomeIcon :icon="['fas', 'rotate-left']" />
                        <span>&nbsp;重置</span>
                    </a-button>
                </a-col>
                <a-col class="BatchDelBtn tool-item" v-if="state.selectedRowKeys.length >= 1" v-permission="'assets:hosts:delete'">
                    <a-button size="large" type="primary" :loading="state.loading" danger @click="openBatchDeleteHostConfirm">
                        <FontAwesomeIcon :icon="['fas', 'trash-can']" />批量删除
                    </a-button>
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
                    :scroll="{ x: 1400 }"
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
                        <template v-else-if="column.key === 'agent_status'">
                            <a-tooltip v-if="record.system?.agent_online">
                                <template #title>
                                    <div>状态由心跳判定</div>
                                    <div v-if="record.system?.agent_last_seen_at" style="margin-top: 4px; opacity: 0.85;">
                                        最后心跳：{{ formatDateTime(record.system.agent_last_seen_at) }}
                                    </div>
                                </template>
                                <a-tag color="success">在线</a-tag>
                            </a-tooltip>
                            <a-tooltip v-else>
                                <template #title>
                                    <div>状态由心跳判定</div>
                                    <div v-if="record.system?.agent_last_seen_at" style="margin-top: 4px; opacity: 0.85;">
                                        最后心跳：{{ formatDateTime(record.system.agent_last_seen_at) }}
                                    </div>
                                    <div v-else style="margin-top: 4px; opacity: 0.85;">暂无心跳</div>
                                </template>
                                <a-tag color="error">离线</a-tag>
                            </a-tooltip>
                        </template>
                        <template v-else-if="column.key === 'monitor_enabled'">
                            <a-tag :color="record.monitor_enabled ? 'green' : 'default'">{{ record.monitor_enabled ? '开启' : '关闭' }}</a-tag>
                        </template>
                        <template v-else-if="column.key === 'monitor_install_status'">
                            <a-tag :color="monitorInstallStatusColor(record.monitor_install_status)">
                                {{ monitorInstallStatusText(record.monitor_install_status) }}
                            </a-tag>
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
                        <template v-else-if="column.key === 'agent_version'">
                            <span>{{ record.system?.agent_version || '-' }}</span>
                        </template>
                        <template v-else-if="column.key === 'action'">
                            <div :key="record.id">
                                <a-row :gutter="6" class="action_row" :wrap="false">
                                    <a-col v-permission="'assets:hosts:view'">
                                        <a-tooltip title="查看详情">
                                            <a-button @click="openDetail(record.id)">
                                                <FontAwesomeIcon :icon="['fas', 'fa-circle-info']" />
                                            </a-button>
                                        </a-tooltip>
                                    </a-col>
                                    <a-col v-permission="'assets:hosts:update'">
                                        <a-tooltip title="编辑">
                                            <a-button type="primary" @click="onSaveOrCreate(record.id)">
                                                <FontAwesomeIcon :icon="['fa', 'edit']" />
                                            </a-button>
                                        </a-tooltip>
                                    </a-col>
                                    <a-col v-permission="'assets:hosts:view'">
                                        <a-tooltip title="查看 Agent 状态">
                                            <a-button @click="openAgentRuntimePage(record)">
                                                <FontAwesomeIcon :icon="['fas', 'server']" />
                                            </a-button>
                                        </a-tooltip>
                                    </a-col>
                                    <a-col v-permission="'assets:hosts:view'">
                                        <a-tooltip :title="getWebSshActionTooltip(record)">
                                            <a-button :disabled="!canOpenWebSsh(record)" @click="openWebSsh(record)">
                                                <FontAwesomeIcon :icon="['fas', 'terminal']" />
                                            </a-button>
                                        </a-tooltip>
                                    </a-col>
                                    <a-col v-permission="'assets:hosts:delete'">
                                        <a-tooltip title="删除">
                                            <a-button class="delBtn" :loading="rowLoadingStates['delete_' + record.id]" danger type="primary"
                                                @click="openDeleteHostConfirm(record)">
                                                <FontAwesomeIcon :icon="['fas', 'trash-can']" />
                                            </a-button>
                                        </a-tooltip>
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
                <a-form-item name="webssh_default_username" label="WebSSH 默认用户">
                    <a-select
                        v-model:value="form.webssh_default_username"
                        :options="formWebSshDefaultUserOptions"
                        :getPopupContainer="getPopupContainer"
                        placeholder="请选择默认用户"
                        show-search
                    />
                </a-form-item>
                <a-form-item name="webssh_login_users" label="WebSSH 可选用户列表">
                    <div class="webssh-user-tags-wrap">
                        <a-tag
                            v-for="user in formWebSshUserTagList"
                            :key="user"
                            closable
                            @close.prevent="removeFormWebSshUserTag(user)"
                        >{{ user }}</a-tag>
                    </div>
                    <a-space style="margin-top: 8px; width: 100%">
                        <a-input
                            v-model:value="formWebSshNewUser"
                            placeholder="输入用户名后点击添加"
                            @pressEnter="addFormWebSshUserTag"
                        />
                        <a-button type="primary" @click="addFormWebSshUserTag">添加</a-button>
                    </a-space>
                </a-form-item>
                <a-form-item label="监控设置">
                    <div class="monitor-row" v-for="(item, index) in form.monitors" :key="index">
                        <a-select
                            v-model:value="item.name"
                            :getPopupContainer="getPopupContainer"
                            placeholder="请选择监控组件"
                            :options="monitorNameOptions"
                            show-search
                            optionFilterProp="label"
                            style="width: 160px"
                        />
                        <a-tooltip title="开启：保存后自动下发安装任务；关闭：保存后自动下发卸载任务（不再需要单独勾选）">
                            <a-switch
                                v-model:checked="item.enabled"
                                checked-children="开启"
                                un-checked-children="关闭"
                            />
                        </a-tooltip>
                        <a-tooltip
                            v-if="canDeleteMonitorRow(item)"
                            :title="item._persisted ? '删除该监控项（不可恢复）' : '仅移除本次编辑中还未保存的行'"
                        >
                            <a-button
                                class="delBtn"
                                type="link"
                                danger
                                :loading="item._persisted && monitorRowDeleteLoading[item.id]"
                                @click="removeMonitorRow(index)"
                            >删除</a-button>
                        </a-tooltip>
                        <a-tooltip
                            v-else
                            title="仍处于开启状态或卸载任务尚未结束，请先关闭（会自动卸载）并等待卸载完成后再删除"
                        >
                            <a-button type="link" danger disabled>删除</a-button>
                        </a-tooltip>
                    </div>
                    <a-button type="dashed" block @click="addMonitorRow">
                        + 添加监控项
                    </a-button>
                </a-form-item>
                <a-form-item name="remark" label="备注">
                    <a-textarea v-model:value="form.remark" :rows="3" placeholder="可填写业务用途、负责人等信息" />
                </a-form-item>
            </a-form>
        </a-spin>
    </a-modal>

    <a-modal
        v-model:open="webSshUserSelectorVisible"
        title="设置 WebSSH 登录用户"
        ok-text="打开终端"
        cancel-text="取消"
        :confirm-loading="webSshOpening"
        @ok="confirmOpenWebSshWithUser"
        @cancel="cancelOpenWebSshWithUser"
    >
        <a-form layout="vertical">
            <a-form-item label="目标主机">
                <a-input :value="webSshHostTitle" disabled />
            </a-form-item>
            <a-form-item label="登录用户" required>
                <a-select
                    v-model:value="webSshTargetUsername"
                    :options="webSshUserSelectOptions"
                    :getPopupContainer="getPopupContainer"
                    placeholder="请选择登录用户"
                    show-search
                />
            </a-form-item>
            <a-alert
                type="info"
                show-icon
                message="说明"
                description="可选用户列表和默认用户请在“编辑主机”中维护。终端会在 agent 侧尝试切换到该系统用户后再启动。"
                style="margin-bottom: 4px"
            />
        </a-form>
    </a-modal>

    <a-modal
        v-model:open="groupDeleteConfirmVisible"
        title="确认删除主机分组"
        ok-text="确认删除"
        cancel-text="取消"
        :ok-button-props="{ danger: true }"
        :confirm-loading="groupDeleteLoading"
        @ok="confirmDeleteGroupWithPreview"
    >
        <a-alert
            v-if="groupDeleteHostCount > 0"
            type="warning"
            show-icon
            :message="`该分组树下共有 ${groupDeleteHostCount} 台主机，删除前请确认影响清单`"
            description="提示按分组树展示，便于核对。"
            style="margin-bottom: 12px"
        />
        <a-alert
            v-else
            type="info"
            show-icon
            message="该分组树下没有主机"
            description="可直接确认删除分组。"
            style="margin-bottom: 12px"
        />
        <a-tree
            v-if="groupDeletePreviewTreeData.length"
            block-node
            show-line
            :expanded-keys="groupDeleteExpandedKeys"
            :auto-expand-parent="true"
            :tree-data="groupDeletePreviewTreeData"
            @expand="onGroupDeletePreviewExpand"
        />
    </a-modal>
</template>

<script setup>
defineOptions({
    name: 'host'
})

import { computed, onMounted, onBeforeUnmount, reactive, ref, watch } from 'vue'
import { message } from 'ant-design-vue'
import { useRoute, useRouter } from 'vue-router'
import {
    batchDeleteHost,
    batchRefreshHostInfo,
    deleteHostById,
    getHostById,
    getHostList,
    saveOrCreateHost,
} from '@/api/assets/host/index.js'
import { getHostGroupTree, deleteHostGroupById } from '@/api/assets/hostgroup/index.js'
import { getConfigByKey, CONFIG_KEYS } from '@/api/sys/sysconfig.js'
import { deleteManagedTarget, getSoftwarePackages } from '@/api/sys/monitor.js'
import { openDeleteConfirm } from '@/util/deleteConfirm'
import { resolvePopupContainerByContext } from '@/util/popupContainer'
import Dialog from '@/views/assets/hostgroup/components/Dialog.vue'
import { formatTimeWithTimezone } from '@/util/timezone'
import store from '@/store'
import {
    buildDeletePreviewTree,
    buildGroupTreeSelectData,
    buildTreeData,
    collectExpandedKeys,
    collectTreeKeys,
    filterGroupTree,
    findGroupNodeByKey,
    getGroupNodeLabel,
    getHostDisplayName,
    getHostGroupId,
} from './hostGroupTreeUtils'
import {
    formatDateTimeWithTimezone,
    formatPercent,
    formatSize,
    getGroupName,
} from './hostDisplayUtils'

const searchText = ref('')
const groupSearchText = ref('')
const activeSearchText = ref('')
const hostIdSearchText = ref('')
const activeHostIdFilter = ref(null)
const agentStatusFilter = ref('all')
const selectedGroupId = ref(0)
const selectedGroupName = ref('全部分组')
const groupDialogVisible = ref(false)
const groupDialogTitle = ref('新增分组')
const groupDialogId = ref(-1)
const groupDialogDefaultParentId = ref(null)
const groupMaxTreeDepth = ref(5)
const loading = ref(false)
const groupLoading = ref(false)
const groupDeleteLoading = ref(false)
const dialogVisible = ref(false)
const dialogTitle = ref('新增主机')
const dialogLoading = ref(false)
const formRef = ref(null)
const webSshHostTitle = ref('')
const webSshUserSelectorVisible = ref(false)
const webSshTargetUsername = ref('root')
const webSshUserList = ref(['root'])
const webSshOpening = ref(false)
const webSshPendingHostRecord = ref(null)
const syncingRouteFilters = ref(false)

let hostListAutoRefreshTimer = null

const DEFAULT_HOST_LIST_REFRESH_INTERVAL_SECONDS = 5
const MIN_HOST_LIST_REFRESH_INTERVAL_SECONDS = 3
const MAX_HOST_LIST_REFRESH_INTERVAL_SECONDS = 300
const hostListRefreshIntervalSeconds = ref(DEFAULT_HOST_LIST_REFRESH_INTERVAL_SECONDS)

const getPopupContainer = (triggerNode) => resolvePopupContainerByContext(triggerNode)

const state = reactive({
    selectedRowKeys: [],
    loading: false,
})

const rowLoadingStates = reactive({})
const groupTreeData = ref([])
const groupDeleteConfirmVisible = ref(false)
const groupDeletePreviewTreeData = ref([])
const groupDeleteExpandedKeys = ref([])
const pendingDeleteGroupNode = ref(null)
const expandedGroupKeys = ref([])
const datasources = ref([])
const hostListQueryToken = ref(0)

const WEBSSH_USERNAME_REGEXP = /^[a-zA-Z_][a-zA-Z0-9._-]{0,63}$/

const normalizeWebSshUserList = (rawInput) => {
    const rawItems = Array.isArray(rawInput)
        ? rawInput
        : String(rawInput || '').split(/\s+/)
    const items = rawItems
        .flatMap((item) => String(item || '').split(/\s+/))
        .map((item) => item.trim())
        .filter((item) => item && WEBSSH_USERNAME_REGEXP.test(item))
    const uniqueItems = [...new Set(items)]
    return uniqueItems.length ? uniqueItems : ['root']
}

const webSshUserSelectOptions = computed(() => {
    return (webSshUserList.value || []).map((item) => ({ label: item, value: item }))
})

const applyWebSshUserList = (users) => {
    const normalizedUsers = normalizeWebSshUserList(users)
    webSshUserList.value = normalizedUsers
    if (!normalizedUsers.includes(webSshTargetUsername.value)) {
        webSshTargetUsername.value = normalizedUsers[0] || 'root'
    }
}

const resolveHostPreferredWebSshUser = (record) => {
    const users = normalizeWebSshUserList(record?.webssh_login_users)
    const defaultUser = String(record?.webssh_default_username || '').trim()
    if (defaultUser && users.includes(defaultUser)) {
        return defaultUser
    }
    return users[0] || 'root'
}

const form = reactive({
    id: -1,
    ip: '',
    group_id: undefined,
    monitors: [],
    remark: '',
    instance_name: '',
    webssh_default_username: 'root',
    webssh_login_users: 'root',
})
const formWebSshNewUser = ref('')
const formWebSshUserTagList = computed(() => normalizeWebSshUserList(form.webssh_login_users))
const formWebSshDefaultUserOptions = computed(() => {
    return formWebSshUserTagList.value.map((item) => ({ label: item, value: item }))
})

const addFormWebSshUserTag = () => {
    const candidate = String(formWebSshNewUser.value || '').trim()
    if (!candidate) {
        return
    }
    if (!WEBSSH_USERNAME_REGEXP.test(candidate)) {
        message.warning('用户名格式非法，请输入合法 Linux 用户名')
        return
    }
    const nextUsers = normalizeWebSshUserList([...formWebSshUserTagList.value, candidate])
    form.webssh_login_users = nextUsers.join(' ')
    formWebSshNewUser.value = ''
    normalizeFormWebSshUserSettings()
}

const removeFormWebSshUserTag = (user) => {
    const nextUsers = normalizeWebSshUserList(
        formWebSshUserTagList.value.filter((item) => item !== user),
    )
    form.webssh_login_users = nextUsers.join(' ')
    normalizeFormWebSshUserSettings()
}

const normalizeFormWebSshUserSettings = () => {
    const users = normalizeWebSshUserList(form.webssh_login_users)
    const defaultUser = String(form.webssh_default_username || '').trim()
    form.webssh_login_users = users.join(' ')
    form.webssh_default_username = users.includes(defaultUser) ? defaultUser : (users[0] || 'root')
}

// 监控名称下拉选项：仅来自监控软件仓库中 enabled=true 的包（按名称去重）
const monitorNameOptions = ref([])

const loadMonitorNameOptions = async () => {
    try {
        const res = await getSoftwarePackages({ enabled: true, page_size: 200, ordering: '-id' })
        const results = Array.isArray(res?.data?.data?.results) ? res.data.data.results : []
        const names = [...new Set(results.map((item) => item.name).filter(Boolean))]
        monitorNameOptions.value = names.map((name) => ({ label: name, value: name }))
    } catch (error) {
        monitorNameOptions.value = []
    }
}

const addMonitorRow = () => {
    // _persisted 仅作为前端本地标记，不提交给后端（提交 payload 时只取 name/enabled）：
    // 新增的行在后端没有对应 MonitorTarget，所以可以真正本地移除（无需删除按钮限制）。
    form.monitors.push({ name: undefined, enabled: true, _persisted: false })
}

// 已保存的行是否满足真删除条件：必须已经是关闭状态，且没有卸载任务还在进行中——
// 与 monitor/views.py MonitorViewSet.destroy() 的后端前置校验保持一致，避免点了按钮却 400。
const canDeleteMonitorRow = (item) => !item._persisted || (!item.enabled && item.install_status !== 'pending')

const monitorRowDeleteLoading = reactive({})

const removeMonitorRow = (index) => {
    const item = form.monitors[index]
    if (!item?._persisted) {
        // 本次编辑中新增、还没保存过的行，后端没有对应记录，本地移除即可。
        form.monitors.splice(index, 1)
        return
    }
    // 已保存的行走真删除：必须复用统一的删除确认弹窗（openDeleteConfirm），不能用 a-popconfirm 自行拼。
    openDeleteConfirm({
        title: '确认删除监控项',
        summary: '删除后将无法再追踪该监控项的安装/卸载历史记录，如需重新纳管请重新添加。',
        items: [`${item.name}`],
        onConfirm: async () => {
            monitorRowDeleteLoading[item.id] = true
            try {
                await deleteManagedTarget(item.id)
                message.success('删除成功')
                form.monitors.splice(index, 1)
            } catch (error) {
                message.error(error?.response?.data?.msg || error?.message || '删除失败')
            } finally {
                monitorRowDeleteLoading[item.id] = false
            }
        },
    })
}

const rules = {
    instance_name: [{ required: true, message: '请输入实例名' }],
    ip: [{ required: true, message: '请输入 IP 地址' }],
}

const columns = [
    { title: 'ID', dataIndex: 'id', key: 'id', width: 90 },
    { title: '实例名', dataIndex: 'instance_name', key: 'instance_name', width: 160 },
    { title: '状态', dataIndex: 'agent_status', key: 'agent_status', width: 110 },
    { title: '主机名称', dataIndex: 'hostname', key: 'hostname', width: 160 },
    { title: '主机分组', dataIndex: 'group_name', key: 'group_name', width: 130 },
    { title: 'IP 地址', dataIndex: 'ip', key: 'ip', width: 150 },
    { title: '监控', dataIndex: 'monitor_enabled', key: 'monitor_enabled', width: 90 },
    { title: '安装状态', dataIndex: 'monitor_install_status', key: 'monitor_install_status', width: 120 },
    { title: 'OS 类型', dataIndex: 'os_type', key: 'os_type', width: 120 },
    { title: 'OS 版本', dataIndex: 'os_version', key: 'os_version', width: 160 },
    { title: '内核版本', dataIndex: 'kernel_version', key: 'kernel_version', width: 160 },
    { title: 'Agent 版本', dataIndex: 'agent_version', key: 'agent_version', width: 160 },
    { title: 'CPU 核数', dataIndex: 'cpu_cores', key: 'cpu_cores', width: 100 },
    { title: '内存', dataIndex: 'memory_gb', key: 'memory_gb', width: 110 },
    { title: '磁盘总量', dataIndex: 'disk_total_gb', key: 'disk_total_gb', width: 110 },
    { title: '磁盘使用率', dataIndex: 'disk_used_percent', key: 'disk_used_percent', width: 120 },
    { title: '备注', dataIndex: 'remark', key: 'remark', ellipsis: true },
    { title: '操作', key: 'action', fixed: 'right', width: 340 },
]

const pagination = reactive({
    current: 1,
    pageSize: 10,
    total: 0,
    showTotal: (value) => `共有${value}条数据`,
    pageSizeOptions: ['10', '20', '30'],
    showQuickJumper: true,
})

const groupDeleteHostCount = computed(() => {
    const rootNodes = Array.isArray(groupDeletePreviewTreeData.value) ? groupDeletePreviewTreeData.value : []
    let total = 0
    const walk = (nodes) => {
        nodes.forEach((node) => {
            const children = Array.isArray(node.children) ? node.children : []
            if (String(node.key || '').startsWith('host-')) {
                total += 1
                return
            }
            if (children.length) {
                walk(children)
            }
        })
    }
    walk(rootNodes)
    return total
})

const groupTreeExpandedKeys = computed(() => {
    return expandedGroupKeys.value
})

const onGroupExpand = (expandedKeys) => {
    expandedGroupKeys.value = Array.isArray(expandedKeys) ? expandedKeys : []
}

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
                    children: buildTreeData(data, groupMaxTreeDepth.value, 1),
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

const mergeUpdatedHosts = (updatedHosts) => {
    const rows = Array.isArray(updatedHosts) ? updatedHosts : []
    if (!rows.length) {
        return
    }
    const incoming = new Map(rows.map((item) => [item.id, item]))
    datasources.value = (datasources.value || []).map((item) => incoming.get(item.id) || item)
}

const loadHostList = async ({ refreshRuntime = true } = {}) => {
    const queryToken = Date.now()
    hostListQueryToken.value = queryToken
    loading.value = true
    try {
        const params = {
            page: pagination.current,
            size: pagination.pageSize,
            search: activeSearchText.value,
        }
        if (activeHostIdFilter.value) {
            params.host_id = activeHostIdFilter.value
        }
        if (agentStatusFilter.value && agentStatusFilter.value !== 'all') {
            params.agent_status = agentStatusFilter.value
        }
        if (selectedGroupId.value && selectedGroupId.value !== 0) {
            params.group_id = selectedGroupId.value
        }

        const res = await getHostList(params)
        if (hostListQueryToken.value !== queryToken) {
            return
        }
        if (res.data.code === 200) {
            const rows = Array.isArray(res.data.data.results) ? res.data.data.results : []
            datasources.value = rows
            pagination.total = res.data.data.count || 0

            // 两阶段加载：先渲染缓存列表，再异步刷新当前页在线主机的最新信息。
            if (refreshRuntime && rows.length) {
                const onlineIds = rows
                    .filter((item) => item?.system?.agent_online)
                    .map((item) => item.id)
                    .filter((id) => Number.isInteger(id) || /^\d+$/.test(String(id)))
                    .map((id) => Number(id))
                if (onlineIds.length) {
                    batchRefreshHostInfo(onlineIds)
                        .then((refreshRes) => {
                            if (hostListQueryToken.value !== queryToken) {
                                return
                            }
                            if (refreshRes?.data?.code === 200) {
                                mergeUpdatedHosts(refreshRes?.data?.data?.hosts)
                            }
                        })
                        .catch(() => {
                            // 二阶段刷新失败不打断列表渲染，避免频繁提示干扰操作。
                        })
                }
            }
        } else {
            message.error(res.data.msg || '获取主机列表失败')
            datasources.value = []
            pagination.total = 0
        }
    } finally {
        loading.value = false
    }
}

const resolveConfigIntValue = (response, fallbackValue) => {
    const rawValue = response?.data?.value ?? response?.data?.data?.value
    const parsed = Number(rawValue)
    if (!Number.isFinite(parsed) || parsed <= 0) {
        return fallbackValue
    }
    return Math.floor(parsed)
}

const stopHostListAutoRefresh = () => {
    if (hostListAutoRefreshTimer) {
        window.clearInterval(hostListAutoRefreshTimer)
        hostListAutoRefreshTimer = null
    }
}

const startHostListAutoRefresh = () => {
    stopHostListAutoRefresh()
    // 轮询用于实时同步 agent 在线状态，间隔由系统参数控制。
    hostListAutoRefreshTimer = window.setInterval(() => {
        if (loading.value || dialogVisible.value) {
            return
        }
        // 自动轮询只刷新列表与在线状态，不触发按需主机信息拉取，避免高频采集。
        loadHostList({ refreshRuntime: false })
    }, hostListRefreshIntervalSeconds.value * 1000)
}

const loadHostListRefreshIntervalConfig = async () => {
    try {
        const res = await getConfigByKey(CONFIG_KEYS.HOST_MANAGE_REFRESH_INTERVAL_SECONDS)
        const configValue = resolveConfigIntValue(res, DEFAULT_HOST_LIST_REFRESH_INTERVAL_SECONDS)
        hostListRefreshIntervalSeconds.value = Math.max(
            MIN_HOST_LIST_REFRESH_INTERVAL_SECONDS,
            Math.min(MAX_HOST_LIST_REFRESH_INTERVAL_SECONDS, configValue),
        )
    } catch (error) {
        hostListRefreshIntervalSeconds.value = DEFAULT_HOST_LIST_REFRESH_INTERVAL_SECONDS
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
    activeHostIdFilter.value = null
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

const loadHostsByGroupTree = async (groupId) => {
    const allHosts = []
    const pageSize = 200
    let page = 1
    let total = 0

    do {
        const res = await getHostList({ page, size: pageSize, group_id: groupId })
        if (res.data.code !== 200) {
            throw new Error(res.data.msg || '获取主机分组清单失败')
        }
        const payload = res.data.data || {}
        const rows = Array.isArray(payload.results) ? payload.results : []
        total = Number(payload.count || 0)
        allHosts.push(...rows)
        page += 1
    } while (allHosts.length < total)

    return allHosts
}

const onGroupDeletePreviewExpand = (expandedKeys) => {
    groupDeleteExpandedKeys.value = Array.isArray(expandedKeys) ? expandedKeys : []
}

const resolveSelectedHostDeleteItems = () => {
    const selectedIds = Array.isArray(state.selectedRowKeys) ? state.selectedRowKeys : []
    const rows = Array.isArray(datasources.value) ? datasources.value : []
    const selectedRows = rows.filter((item) => selectedIds.includes(item.id))
    if (selectedRows.length) {
        return selectedRows.map((item) => `主机: ${getHostDisplayName(item)} (${item.ip || '-'})`)
    }
    return selectedIds.map((id) => `主机ID: ${id}`)
}

const openBatchDeleteHostConfirm = () => {
    const selectedIds = Array.isArray(state.selectedRowKeys) ? state.selectedRowKeys : []
    openDeleteConfirm({
        title: '确认批量删除主机',
        summary: `即将删除 ${selectedIds.length} 台主机，请确认影响清单。`,
        items: resolveSelectedHostDeleteItems(),
        onConfirm: () => confirmBatchDelete(),
    })
}

const openDeleteHostConfirm = (record) => {
    openDeleteConfirm({
        title: '确认删除主机',
        summary: '删除后不可恢复。',
        items: [`主机: ${getHostDisplayName(record)} (${record?.ip || '-'})`],
        onConfirm: () => delconfirm(record.id),
    })
}

const openGroupDeleteConfirm = async (groupNode) => {
    pendingDeleteGroupNode.value = groupNode
    groupDeleteLoading.value = true
    try {
        const hosts = await loadHostsByGroupTree(groupNode.key)
        groupDeletePreviewTreeData.value = buildDeletePreviewTree(groupNode, hosts)
        groupDeleteConfirmVisible.value = true
    } catch (error) {
        message.error(error?.message || '获取分组主机清单失败')
    } finally {
        groupDeleteLoading.value = false
    }
}

const confirmDeleteGroupWithPreview = async () => {
    const node = pendingDeleteGroupNode.value
    if (!node || Number(node.key) === 0) {
        groupDeleteConfirmVisible.value = false
        return
    }

    groupDeleteLoading.value = true
    try {
        const res = await deleteHostGroupById(node.key)
        if (res.data.code === 200 || res.status === 204) {
            message.success('删除成功')
            groupDeleteConfirmVisible.value = false
            groupDeletePreviewTreeData.value = []
            pendingDeleteGroupNode.value = null
            await loadGroupTree()
            if (selectedGroupId.value === node.key) {
                selectedGroupId.value = 0
                selectedGroupName.value = '全部分组'
                await refreshList()
            }
        } else {
            message.error(res.data?.msg || '删除失败')
        }
    } catch (error) {
        message.error(error?.response?.data?.msg || error?.message || '删除失败')
    } finally {
        groupDeleteLoading.value = false
    }
}

watch(groupDeletePreviewTreeData, (nodes) => {
    groupDeleteExpandedKeys.value = collectTreeKeys(nodes)
}, { deep: true })

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
    const groupNode = findGroupNodeByKey(groupTreeData.value, node.key)
    if (!groupNode) {
        message.error('分组信息不存在，请刷新后重试')
        return
    }
    await openGroupDeleteConfirm(groupNode)
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
    if (syncingRouteFilters.value) {
        return
    }
    const isUserTriggered = !!info?.event
    if (!isUserTriggered) {
        return
    }
    const key = selectedKeys && selectedKeys.length ? selectedKeys[0] : 0
    selectedGroupId.value = key || 0
    selectedGroupName.value = key === 0 ? '全部分组' : (info?.node?.dataRef?.name || '当前分组')
    searchText.value = ''
    activeSearchText.value = ''
    hostIdSearchText.value = ''
    activeHostIdFilter.value = null
    pagination.current = 1
    await refreshList()
}

const resetForm = () => {
    form.id = -1
    form.instance_name = ''
    form.ip = ''
    form.group_id = selectedGroupId.value && selectedGroupId.value !== 0 ? selectedGroupId.value : undefined
    form.monitors = []
    form.remark = ''
    form.webssh_default_username = 'root'
    form.webssh_login_users = 'root'
    formWebSshNewUser.value = ''
}

const handleAdd = async () => {
    await loadMonitorNameOptions()
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

const onSaveOrCreate = async (id) => {
    await loadMonitorNameOptions()
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
                const monitorRows = Array.isArray(data.monitors) ? data.monitors : []
                // 标记为已持久化行：携带 id/install_status，用于判断是否满足后端真删除的前置条件
                // （managed_enabled=false 且 install_status!=='pending'）——不满足时只能先用开关关闭。
                form.monitors = monitorRows.map((item) => ({
                    id: item.id,
                    name: item.name,
                    enabled: Boolean(item.enabled),
                    install_status: item.install_status,
                    _persisted: true,
                }))
                form.remark = data.remark || ''
                form.webssh_default_username = String(data.webssh_default_username || 'root').trim() || 'root'
                form.webssh_login_users = String(data.webssh_login_users || 'root').trim() || 'root'
                formWebSshNewUser.value = ''
                normalizeFormWebSshUserSettings()
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
        // 提交前校验：监控行必须选择名称，避免提交空名称导致后端静默跳过该行
        const invalidRow = form.monitors.find((item) => !item.name)
        if (invalidRow) {
            message.warning('请为每个监控项选择监控组件名称，或删除该行')
            return
        }
        dialogLoading.value = true
        const payload = { ...form }
        normalizeFormWebSshUserSettings()
        payload.webssh_default_username = form.webssh_default_username
        payload.webssh_login_users = form.webssh_login_users
        delete payload.name
        payload.monitors = form.monitors.map((item) => ({
            name: item.name,
            enabled: Boolean(item.enabled),
        }))
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
    hostIdSearchText.value = ''
    activeHostIdFilter.value = null
    pagination.current = 1
    await refreshList()
}

const onHostIdSearch = async () => {
    const text = String(hostIdSearchText.value || '').trim()
    if (!text) {
        activeHostIdFilter.value = null
    } else if (!/^\d+$/.test(text) || Number(text) <= 0) {
        message.warning('请输入有效的主机ID（正整数）')
        return
    } else {
        activeHostIdFilter.value = Number(text)
    }
    searchText.value = ''
    activeSearchText.value = ''
    pagination.current = 1
    await refreshList()
}

const resetAllFilters = async () => {
    searchText.value = ''
    activeSearchText.value = ''
    hostIdSearchText.value = ''
    activeHostIdFilter.value = null
    agentStatusFilter.value = 'all'
    selectedGroupId.value = 0
    selectedGroupName.value = '全部分组'
    pagination.current = 1

    const hasRouteFilters = Boolean(
        route.query.group_id || route.query.search || route.query.host_id
    )
    if (hasRouteFilters) {
        await router.replace({ path: route.path, query: {} })
        return
    }
    await refreshList()
}

const onAgentStatusChange = async () => {
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
    router.push({ path: `/assets/hosts/detail/${id}` })
}

const openWebSsh = async (record) => {
    if (!canOpenWebSsh(record)) {
        message.warning('该主机未绑定 agent 实例，无法打开 WebSSH')
        return
    }

    webSshPendingHostRecord.value = record
    webSshHostTitle.value = getHostDisplayName(record)
    const users = normalizeWebSshUserList(record?.webssh_login_users)
    applyWebSshUserList(users)
    const preferredUser = resolveHostPreferredWebSshUser(record)
    webSshTargetUsername.value = preferredUser
    webSshUserSelectorVisible.value = true
}

const openAgentRuntimePage = (record) => {
    if (!record?.id) {
        message.warning('主机信息不存在，请刷新后重试')
        return
    }

    router.push({
        path: `/assets/hosts/agent-runtime/${record.id}`,
        query: {
            instance_name: record.instance_name || '',
            ip: record.ip || '',
        },
    })
}

const confirmOpenWebSshWithUser = async () => {
    const record = webSshPendingHostRecord.value
    if (!record?.id) {
        message.error('主机信息不存在，请刷新后重试')
        return
    }

    const targetUser = String(webSshTargetUsername.value || '').trim()
    if (!targetUser) {
        message.warning('请输入登录用户')
        return
    }
    if (!WEBSSH_USERNAME_REGEXP.test(targetUser)) {
        message.warning('登录用户格式非法，请输入合法 Linux 用户名')
        return
    }
    if (!webSshUserList.value.includes(targetUser)) {
        message.warning('请从当前主机配置的可选用户列表中选择登录用户')
        return
    }

    webSshOpening.value = true

    try {
        const routeData = router.resolve({
            path: '/assets/hosts/webssh',
            query: {
                host_id: record.id,
                instance_name: record.instance_name || '',
                ip: record.ip || '',
                target_user: targetUser,
            },
        })
        window.open(routeData.href, '_blank', 'noopener,noreferrer,width=1280,height=820')
        webSshUserSelectorVisible.value = false
        webSshPendingHostRecord.value = null
    } finally {
        webSshOpening.value = false
    }
}

const cancelOpenWebSshWithUser = () => {
    webSshUserSelectorVisible.value = false
    webSshOpening.value = false
    webSshPendingHostRecord.value = null
}

const router = useRouter()
const route = useRoute()

const applyRouteFilters = async () => {
    syncingRouteFilters.value = true
    const initialHostId = Number(route.query.host_id || 0)
    const normalizedHostId = Number.isInteger(initialHostId) && initialHostId > 0 ? initialHostId : 0

    const initialGroupId = Number(route.query.group_id || 0)
    selectedGroupId.value = Number.isInteger(initialGroupId) && initialGroupId > 0 ? initialGroupId : 0
    selectedGroupName.value = selectedGroupId.value > 0 ? '当前分组' : '全部分组'

    const initialSearchText = String(route.query.search || '').trim()

    searchText.value = initialSearchText
    activeSearchText.value = initialSearchText
    hostIdSearchText.value = normalizedHostId ? String(normalizedHostId) : ''
    activeHostIdFilter.value = normalizedHostId || null

    pagination.current = 1
    try {
        await refreshList()
    } finally {
        syncingRouteFilters.value = false
    }
}

const getRowClassName = (record) => {
    return record.collect_status === 'failed' ? 'row-collect-failed' : ''
}

const monitorInstallStatusColor = (status) => {
    const normalized = String(status || '').toLowerCase()
    if (normalized === 'success') return 'green'
    if (normalized === 'failed') return 'red'
    if (normalized === 'pending') return 'orange'
    return 'default'
}

const monitorInstallStatusText = (status) => {
    const normalized = String(status || '').toLowerCase()
    if (normalized === 'success') return '成功'
    if (normalized === 'failed') return '失败'
    if (normalized === 'pending') return '处理中'
    return '未知'
}

const canOpenWebSsh = (record) => {
    const hasAgentInstance = Boolean(String(record?.instance_name || '').trim())
    const isAgentOnline = Boolean(record?.system?.agent_online)
    return hasAgentInstance && isAgentOnline
}

const getWebSshActionTooltip = (record) => {
    if (!String(record?.instance_name || '').trim()) {
        return '未绑定 agent 实例，无法打开 WebSSH'
    }
    if (!record?.system?.agent_online) {
        return 'Agent 离线，禁止打开 WebSSH'
    }
    return '打开WebSSH 终端'
}

const formatDateTime = (value) => {
    return formatDateTimeWithTimezone(value, formatTimeWithTimezone, store.state.user?.timezone || 'Asia/Shanghai')
}

onMounted(async () => {
    // 先加载主机分组最大层级配置，再构建树（buildTreeData 依赖此值）
    await getConfigByKey(CONFIG_KEYS.HOSTGROUP_MAX_TREE_DEPTH).then(res => {
        groupMaxTreeDepth.value = resolveConfigIntValue(res, 5)
    }).catch(() => {})
    await loadHostListRefreshIntervalConfig()
    await loadGroupTree()
    startHostListAutoRefresh()
    document.addEventListener('click', closeGroupContextMenu)
})

watch(
    () => [route.query.group_id, route.query.search, route.query.host_id],
    async () => {
        await applyRouteFilters()
    },
    { immediate: true }
)

onBeforeUnmount(() => {
    stopHostListAutoRefresh()
    document.removeEventListener('click', closeGroupContextMenu)
})
</script>

<style scoped>
.monitor-row {
    display: flex;
    align-items: center;
    gap: 8px;
    margin-bottom: 8px;
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