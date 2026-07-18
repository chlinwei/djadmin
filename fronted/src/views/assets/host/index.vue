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
                <a-descriptions-item label="Agent 版本">{{ detailHost.system?.agent_version || '-' }}</a-descriptions-item>
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
                <a-form-item name="credential_ids" label="SSH 凭证（可选）">
                    <a-select
                        v-model:value="form.credential_ids"
                        :getPopupContainer="getPopupContainer"
                        placeholder="请选择一个或多个凭证"
                        :options="credentialOptions"
                        mode="multiple"
                        show-search
                        optionFilterProp="label"
                    />
                </a-form-item>
                <a-form-item name="default_credential_id" label="默认凭证（可选）">
                    <a-select
                        v-model:value="form.default_credential_id"
                        :getPopupContainer="getPopupContainer"
                        placeholder="请选择默认凭证"
                        :options="defaultCredentialOptions"
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

    <a-modal
        v-model:open="webSshVisible"
        :title="`Web SSH - ${webSshHostTitle}`"
        :footer="null"
        :width="980"
        destroyOnClose
        @cancel="closeWebSsh"
    >
        <a-alert
            v-if="webSshMessage"
            :message="webSshMessage"
            :type="webSshMessageType"
            show-icon
            style="margin-bottom: 10px"
        />
        <div ref="webSshContainerRef" class="webssh-terminal" />
    </a-modal>

    <a-modal
        v-model:open="webSshCredentialSelectorVisible"
        title="选择 WebSSH 凭证"
        ok-text="打开终端"
        cancel-text="取消"
        :confirm-loading="webSshCredentialOpening"
        @ok="confirmOpenWebSshWithCredential"
        @cancel="cancelOpenWebSshWithCredential"
    >
        <a-form layout="vertical">
            <a-form-item label="目标主机">
                <a-input :value="webSshHostTitle" disabled />
            </a-form-item>
            <a-form-item label="已绑定凭证（可选）">
                <a-select
                    v-model:value="webSshSelectedCredentialId"
                    :getPopupContainer="getPopupContainer"
                    :options="webSshCredentialOptions"
                    placeholder="留空则手动输入"
                    allowClear
                    show-search
                    optionFilterProp="label"
                    @change="onWebSshCredentialSelectionChange"
                />
            </a-form-item>
            <a-alert
                type="info"
                show-icon
                message="请确认本次登录认证方式"
                :description="isWebSshManualCredentialMode ? '当前为手动输入模式：将创建并绑定新凭证后再连接。' : '当前为已绑定凭证模式：将直接使用所选凭证连接。'"
                style="margin-bottom: 12px"
            />
            <a-form-item v-if="isWebSshManualCredentialMode" label="用户名" required>
                <a-input v-model:value="webSshInlineCredentialForm.username" placeholder="例如：root" />
            </a-form-item>
            <a-form-item v-if="isWebSshManualCredentialMode" label="认证方式" required>
                <a-radio-group v-model:value="webSshInlineCredentialForm.authType">
                    <a-radio :value="1">密码</a-radio>
                    <a-radio :value="2">密钥</a-radio>
                </a-radio-group>
            </a-form-item>
            <a-form-item v-if="isWebSshManualCredentialMode && webSshInlineCredentialForm.authType === 1" label="密码" required>
                <a-input-password
                    v-model:value="webSshInlineCredentialForm.password"
                    placeholder="请输入密码"
                />
            </a-form-item>
            <a-form-item v-if="isWebSshManualCredentialMode && webSshInlineCredentialForm.authType === 2" label="私钥" required>
                <a-textarea
                    v-model:value="webSshInlineCredentialForm.privateKey"
                    :rows="6"
                    placeholder="请输入 OpenSSH 私钥内容"
                />
                <a-space style="margin-top: 8px">
                    <a-button @click="triggerWebSshPrivateKeyFilePicker">上传密钥文件</a-button>
                    <span v-if="webSshInlineCredentialForm.privateKeyFileName">{{ webSshInlineCredentialForm.privateKeyFileName }}</span>
                </a-space>
                <input
                    ref="webSshPrivateKeyFileInputRef"
                    type="file"
                    style="display: none"
                    accept=".pem,.key,.txt"
                    @change="handleWebSshPrivateKeyFileChange"
                />
            </a-form-item>
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

import { computed, nextTick, onMounted, onBeforeUnmount, reactive, ref, watch } from 'vue'
import { message } from 'ant-design-vue'
import { useRoute, useRouter } from 'vue-router'
import { getToken } from '@/api/user/index.js'
import { batchDeleteHost, deleteHostById, getHostById, getHostList, saveOrCreateHost } from '@/api/assets/host/index.js'
import { getHostGroupTree, deleteHostGroupById } from '@/api/assets/hostgroup/index.js'
import { getCredentailList, SaveOrCreateCredential } from '@/api/assets/credential/index.js'
import { getConfigByKey, CONFIG_KEYS } from '@/api/sys/sysconfig.js'
import { getWebSocketBaseUrl } from '@/util/request'
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
    getCredentialName,
    getDisks,
    getGroupName,
    hasHostCredential,
} from './hostDisplayUtils'
import { Terminal } from '@xterm/xterm'
import { FitAddon } from '@xterm/addon-fit'
import '@xterm/xterm/css/xterm.css'

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
const detailVisible = ref(false)
const detailLoading = ref(false)
const detailHost = ref(null)
const formRef = ref(null)
const webSshVisible = ref(false)
const webSshHostTitle = ref('')
const webSshContainerRef = ref(null)
const webSshMessage = ref('')
const webSshMessageType = ref('info')
const webSshCredentialSelectorVisible = ref(false)
const webSshSelectedCredentialId = ref(undefined)
const webSshCredentialOpening = ref(false)
const webSshPendingHostRecord = ref(null)
const webSshDefaultCredentialId = ref(undefined)
const webSshPrivateKeyFileInputRef = ref(null)
const webSshInlineCredentialForm = reactive({
    username: 'root',
    authType: 1,
    password: '',
    privateKey: '',
    privateKeyFileName: '',
})
const WEBSSH_DEFAULT_CREDENTIALS_STORAGE_KEY = 'webssh-default-credentials-by-host'
const webSshDefaultCredentialsByHost = ref({})
const syncingRouteFilters = ref(false)

let webSshSocket = null
let webSshTerminal = null
let webSshFitAddon = null
let webSshOnDataDisposable = null
let webSshOnResizeDisposable = null

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
const credentialOptions = ref([])
const datasources = ref([])
const defaultCredentialOptions = computed(() => {
    const selectedIds = Array.isArray(form.credential_ids) ? form.credential_ids : []
    if (!selectedIds.length) {
        return []
    }
    const selectedSet = new Set(selectedIds)
    return credentialOptions.value.filter((item) => selectedSet.has(item.value))
})

const webSshCredentialOptions = computed(() => {
    const record = webSshPendingHostRecord.value
    const hostCredentials = Array.isArray(record?.credentials) ? record.credentials : []
    const optionMap = new Map((credentialOptions.value || []).map((item) => [item.value, item]))

    const scopedOptions = hostCredentials.length
        ? hostCredentials.map((item) => {
            const fallback = optionMap.get(item.id)
            const label = (item?.name && item?.username)
                ? `${item.name} (${item.username}@${item.port || 22})`
                : (fallback?.label || `Credential-${item.id}`)
            return {
                label,
                value: item.id,
            }
        })
        : []

    return scopedOptions.map((item) => {
        if (item.value === webSshDefaultCredentialId.value) {
            return {
                ...item,
                label: `${item.label}（默认）`,
            }
        }
        return item
    })
})

const isWebSshManualCredentialMode = computed(() => !webSshSelectedCredentialId.value)

const resolveHostDefaultCredentialMeta = (record) => {
    const hostCredentials = Array.isArray(record?.credentials) ? record.credentials : []
    const defaultByList = hostCredentials.find((item) => item?.is_default)
    if (defaultByList) {
        return defaultByList
    }
    if (record?.credential?.id) {
        return record.credential
    }
    return null
}

const getWebSshCredentialMetaById = (record, credentialId) => {
    const id = Number(credentialId || 0)
    if (id <= 0) {
        return null
    }
    const hostCredentials = Array.isArray(record?.credentials) ? record.credentials : []
    return hostCredentials.find((item) => Number(item?.id || 0) === id) || null
}

const form = reactive({
    id: -1,
    ip: '',
    group_id: undefined,
    credential_ids: [],
    default_credential_id: undefined,
    port: 22,
    remark: '',
    instance_name: '',
})

const rules = {
    instance_name: [{ required: true, message: '请输入实例名' }],
    ip: [{ required: true, message: '请输入 IP 地址' }],
    port: [{ required: true, message: '请输入 SSH 端口' }],
}

watch(
    () => form.credential_ids,
    (nextValue) => {
        const selectedIds = Array.isArray(nextValue) ? nextValue : []
        if (!selectedIds.length) {
            form.default_credential_id = undefined
            return
        }
        if (!selectedIds.includes(form.default_credential_id)) {
            form.default_credential_id = selectedIds[0]
        }
    },
    { deep: true },
)

const columns = [
    { title: 'ID', dataIndex: 'id', key: 'id', width: 90 },
    { title: '实例名', dataIndex: 'instance_name', key: 'instance_name', width: 160 },
    { title: '状态', dataIndex: 'agent_status', key: 'agent_status', width: 110 },
    { title: '主机名称', dataIndex: 'hostname', key: 'hostname', width: 160 },
    { title: '主机分组', dataIndex: 'group_name', key: 'group_name', width: 130 },
    { title: 'IP 地址', dataIndex: 'ip', key: 'ip', width: 150 },
    { title: 'SSH 端口', dataIndex: 'port', key: 'port', width: 100 },
    { title: 'SSH 凭证', dataIndex: 'credential_name', key: 'credential_name', width: 170 },
    { title: 'OS 类型', dataIndex: 'os_type', key: 'os_type', width: 120 },
    { title: 'OS 版本', dataIndex: 'os_version', key: 'os_version', width: 160 },
    { title: '内核版本', dataIndex: 'kernel_version', key: 'kernel_version', width: 160 },
    { title: 'Agent 版本', dataIndex: 'agent_version', key: 'agent_version', width: 160 },
    { title: 'CPU 核数', dataIndex: 'cpu_cores', key: 'cpu_cores', width: 100 },
    { title: '内存', dataIndex: 'memory_gb', key: 'memory_gb', width: 110 },
    { title: '磁盘总量', dataIndex: 'disk_total_gb', key: 'disk_total_gb', width: 110 },
    { title: '磁盘使用率', dataIndex: 'disk_used_percent', key: 'disk_used_percent', width: 120 },
    { title: '备注', dataIndex: 'remark', key: 'remark', ellipsis: true },
    { title: '操作', key: 'action', fixed: 'right', width: 280 },
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

const loadWebSshDefaultCredentialMap = () => {
    try {
        const raw = localStorage.getItem(WEBSSH_DEFAULT_CREDENTIALS_STORAGE_KEY)
        if (!raw) {
            webSshDefaultCredentialsByHost.value = {}
            return
        }
        const parsed = JSON.parse(raw)
        if (parsed && typeof parsed === 'object' && !Array.isArray(parsed)) {
            webSshDefaultCredentialsByHost.value = parsed
            return
        }
        webSshDefaultCredentialsByHost.value = {}
    } catch (error) {
        webSshDefaultCredentialsByHost.value = {}
    }
}

const saveWebSshDefaultCredentialMap = () => {
    try {
        localStorage.setItem(
            WEBSSH_DEFAULT_CREDENTIALS_STORAGE_KEY,
            JSON.stringify(webSshDefaultCredentialsByHost.value || {}),
        )
    } catch (error) {
        // ignore storage failure
    }
}

const resolveWebSshDefaultCredentialId = (record) => {
    const hostId = Number(record?.id || 0)
    const hostCredentials = Array.isArray(record?.credentials) ? record.credentials : []
    const scopedIds = hostCredentials.length
        ? hostCredentials.map((item) => item.id)
        : (record?.credential?.id ? [record.credential.id] : [])
    const availableIds = new Set(scopedIds)
    const fromStorage = webSshDefaultCredentialsByHost.value[String(hostId)]
    if (fromStorage && availableIds.has(fromStorage)) {
        return fromStorage
    }
    const fromHost = record?.credential?.id
    if (fromHost && availableIds.has(fromHost)) {
        return fromHost
    }
    const first = scopedIds[0]
    return first || undefined
}

const loadHostList = async () => {
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
    await Promise.all([loadHostList(), loadCredentialOptions()])
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
    form.credential_ids = []
    form.default_credential_id = undefined
    form.port = 22
    form.remark = ''
}

const handleAdd = async () => {
    await loadCredentialOptions()
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
    await loadCredentialOptions()
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
                const credentialRows = Array.isArray(data.credentials) ? data.credentials : []
                form.credential_ids = credentialRows.map((item) => item.id)
                form.default_credential_id = (
                    credentialRows.find((item) => item.is_default)?.id
                    || data.credential?.id
                    || form.credential_ids[0]
                    || undefined
                )
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
        payload.credential_ids = Array.isArray(form.credential_ids) ? form.credential_ids : []
        payload.default_credential_id = form.default_credential_id
        payload.credential_id = form.default_credential_id
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

const buildWebSocketUrl = (hostId) => {
    const token = getToken() || ''
    return `${getWebSocketBaseUrl()}/ws/assets/hosts/${hostId}/webssh/?token=${encodeURIComponent(token)}`
}

const WEBSSH_BLOCKED_SHORTCUTS = new Set(['w', 'r', 't', 'n', 'l', 'p', 'j', 'k'])

const handleWebSshGlobalKeydown = (event) => {
    if (!webSshVisible.value) return

    const key = String(event.key || '').toLowerCase()
    const ctrlOrMeta = event.ctrlKey || event.metaKey

    if (event.key === 'F5' || (ctrlOrMeta && WEBSSH_BLOCKED_SHORTCUTS.has(key))) {
        // Prevent browser actions like close tab / refresh while terminal is active.
        event.preventDefault()
        event.stopPropagation()
        if (event.stopImmediatePropagation) {
            event.stopImmediatePropagation()
        }
        event.returnValue = false
        if (webSshTerminal) {
            webSshTerminal.focus()
        }
        return false
    }

    return true
}

const bindWebSshShortcutGuard = () => {
    if (!webSshTerminal) return

    // Keep browser-level shortcuts from hijacking terminal key combos.
    webSshTerminal.attachCustomKeyEventHandler((event) => {
        const key = String(event.key || '').toLowerCase()
        const ctrlOrMeta = event.ctrlKey || event.metaKey
        const blockedCtrlMetaKeys = ['w', 'r', 't', 'n', 'l', 'p', 'j', 'k']

        if (event.key === 'F5' || (ctrlOrMeta && blockedCtrlMetaKeys.includes(key))) {
            event.preventDefault()
            event.stopPropagation()
            if (event.stopImmediatePropagation) {
                event.stopImmediatePropagation()
            }
            event.returnValue = false
            return false
        }

        return true
    })
}

const disposeWebSshTerminal = () => {
    if (webSshOnDataDisposable) {
        webSshOnDataDisposable.dispose()
        webSshOnDataDisposable = null
    }
    if (webSshOnResizeDisposable) {
        webSshOnResizeDisposable.dispose()
        webSshOnResizeDisposable = null
    }
    if (webSshTerminal) {
        webSshTerminal.dispose()
        webSshTerminal = null
    }
    webSshFitAddon = null
}

const initWebSshTerminal = async () => {
    await nextTick()
    if (!webSshContainerRef.value) return

    // Recreate terminal instance every session to avoid stale renderer/socket state.
    disposeWebSshTerminal()

    webSshTerminal = new Terminal({
        cursorBlink: true,
        fontFamily: 'Consolas, Menlo, monospace',
        fontSize: 13,
        theme: {
            background: '#0b1220',
            foreground: '#e2e8f0',
        },
        convertEol: true,
        scrollback: 5000,
    })
    webSshFitAddon = new FitAddon()
    webSshTerminal.loadAddon(webSshFitAddon)
    bindWebSshShortcutGuard()

    webSshContainerRef.value.innerHTML = ''
    webSshTerminal.open(webSshContainerRef.value)
    webSshFitAddon.fit()
    webSshTerminal.focus()

    webSshOnDataDisposable = webSshTerminal.onData((data) => {
        if (webSshSocket && webSshSocket.readyState === WebSocket.OPEN) {
            webSshSocket.send(JSON.stringify({ type: 'input', data }))
        }
    })

    webSshOnResizeDisposable = webSshTerminal.onResize(({ cols, rows }) => {
        if (webSshSocket && webSshSocket.readyState === WebSocket.OPEN) {
            webSshSocket.send(JSON.stringify({ type: 'resize', cols, rows }))
        }
    })
}

const openWebSsh = async (record) => {
    if (!canOpenWebSsh(record)) {
        message.warning('该主机未绑定 agent 实例，无法打开 WebSSH')
        return
    }

    const hostCredentials = Array.isArray(record?.credentials) ? record.credentials : []
    if (!hostCredentials.length && !credentialOptions.value.length) {
        await loadCredentialOptions()
    }

    webSshPendingHostRecord.value = record
    webSshHostTitle.value = getHostDisplayName(record)
    webSshDefaultCredentialId.value = resolveWebSshDefaultCredentialId(record)
    webSshSelectedCredentialId.value = webSshDefaultCredentialId.value
    const defaultCredentialMeta = getWebSshCredentialMetaById(record, webSshDefaultCredentialId.value) || resolveHostDefaultCredentialMeta(record)
    webSshInlineCredentialForm.username = String(defaultCredentialMeta?.username || 'root')
    webSshInlineCredentialForm.authType = Number(defaultCredentialMeta?.auth_type || 1)
    webSshInlineCredentialForm.password = ''
    webSshInlineCredentialForm.privateKey = ''
    webSshInlineCredentialForm.privateKeyFileName = ''
    webSshCredentialSelectorVisible.value = true
}

const onWebSshCredentialSelectionChange = (value) => {
    const credentialId = Number(value || 0)
    if (credentialId <= 0) {
        return
    }

    const record = webSshPendingHostRecord.value
    const selectedCredential = getWebSshCredentialMetaById(record, credentialId)
    if (!selectedCredential) {
        return
    }

    webSshInlineCredentialForm.username = String(selectedCredential?.username || webSshInlineCredentialForm.username || 'root')
    webSshInlineCredentialForm.authType = Number(selectedCredential?.auth_type || webSshInlineCredentialForm.authType || 1)
    webSshInlineCredentialForm.password = ''
    webSshInlineCredentialForm.privateKey = ''
    webSshInlineCredentialForm.privateKeyFileName = ''
}

const triggerWebSshPrivateKeyFilePicker = () => {
    webSshPrivateKeyFileInputRef.value?.click()
}

const handleWebSshPrivateKeyFileChange = async (event) => {
    const target = event?.target
    const file = target?.files?.[0]
    if (!file) {
        return
    }
    try {
        const content = await file.text()
        webSshInlineCredentialForm.privateKey = String(content || '')
        webSshInlineCredentialForm.privateKeyFileName = String(file.name || '')
    } catch (error) {
        message.error('读取密钥文件失败')
    } finally {
        target.value = ''
    }
}

const buildCredentialLabel = (credential) => {
    const name = credential?.name || 'webssh-credential'
    const username = credential?.username || 'root'
    const port = credential?.port || 22
    return `${name} (${username}@${port})`
}

const createAndBindWebSshCredential = async (record) => {
    const username = String(webSshInlineCredentialForm.username || '').trim()
    if (!username) {
        message.warning('请输入用户名')
        return null
    }

    const authType = Number(webSshInlineCredentialForm.authType || 1)
    if (authType === 1) {
        const password = String(webSshInlineCredentialForm.password || '')
        if (!password) {
            message.warning('请输入密码')
            return null
        }
    } else {
        const privateKey = String(webSshInlineCredentialForm.privateKey || '').trim()
        if (!privateKey) {
            message.warning('请填写或上传私钥')
            return null
        }
    }

    const credentialPayload = {
        id: -1,
        name: `webssh-${record.instance_name || record.ip || record.id}-${Date.now()}`,
        username,
        auth_type: authType,
        password: authType === 1 ? String(webSshInlineCredentialForm.password || '') : '',
        private_key: authType === 2 ? String(webSshInlineCredentialForm.privateKey || '') : '',
        port: Number(record?.port || 22),
    }
    const credentialRes = await SaveOrCreateCredential(credentialPayload)
    if (credentialRes?.data?.code !== 200 || !credentialRes?.data?.data?.id) {
        message.error(credentialRes?.data?.msg || '创建凭证失败')
        return null
    }

    const createdCredential = credentialRes.data.data
    const existingIds = Array.isArray(record?.credentials)
        ? record.credentials.map((item) => item.id)
        : (record?.credential?.id ? [record.credential.id] : [])
    const mergedIds = Array.from(new Set([...existingIds, createdCredential.id]))
    const hostPayload = {
        id: record.id,
        credential_ids: mergedIds,
        default_credential_id: createdCredential.id,
        credential_id: createdCredential.id,
    }
    const hostRes = await saveOrCreateHost(hostPayload)
    if (hostRes?.data?.code !== 200) {
        message.error(hostRes?.data?.msg || '绑定凭证到主机失败')
        return null
    }

    const createdOption = {
        value: createdCredential.id,
        label: buildCredentialLabel(createdCredential),
    }
    credentialOptions.value = [...credentialOptions.value, createdOption]

    const refreshedHost = hostRes?.data?.data || {}
    record.credentials = Array.isArray(refreshedHost.credentials) ? refreshedHost.credentials : [
        {
            id: createdCredential.id,
            name: createdCredential.name,
            username: createdCredential.username,
            port: createdCredential.port,
            auth_type: createdCredential.auth_type,
            is_default: true,
        },
    ]
    record.credential = refreshedHost.credential || {
        id: createdCredential.id,
        name: createdCredential.name,
        username: createdCredential.username,
        port: createdCredential.port,
        auth_type: createdCredential.auth_type,
    }

    return createdCredential.id
}

const confirmOpenWebSshWithCredential = async () => {
    const record = webSshPendingHostRecord.value
    if (!record?.id) {
        message.error('主机信息不存在，请刷新后重试')
        return
    }

    webSshCredentialOpening.value = true
    let credentialId = Number(webSshSelectedCredentialId.value || 0) || null

    try {
        if (!credentialId) {
            credentialId = await createAndBindWebSshCredential(record)
            if (!credentialId) {
                return
            }
        }

        const hostId = String(record.id)
        webSshDefaultCredentialsByHost.value = {
            ...(webSshDefaultCredentialsByHost.value || {}),
            [hostId]: credentialId,
        }
        webSshDefaultCredentialId.value = credentialId
        saveWebSshDefaultCredentialMap()

        const routeData = router.resolve({
            path: '/assets/hosts/webssh',
            query: {
                host_id: record.id,
                instance_name: record.instance_name || '',
                ip: record.ip || '',
                credential_id: credentialId,
            },
        })
        window.open(routeData.href, '_blank', 'noopener,noreferrer,width=1280,height=820')
        webSshCredentialSelectorVisible.value = false
        webSshPendingHostRecord.value = null
    } finally {
        webSshCredentialOpening.value = false
    }
}

const cancelOpenWebSshWithCredential = () => {
    webSshCredentialSelectorVisible.value = false
    webSshCredentialOpening.value = false
    webSshPendingHostRecord.value = null
}

const closeWebSsh = () => {
    webSshVisible.value = false
    if (webSshSocket) {
        try {
            if (webSshSocket.readyState === WebSocket.OPEN) {
                webSshSocket.send(JSON.stringify({ type: 'close' }))
            }
            webSshSocket.close()
        } catch (error) {
            // ignore close exception
        }
        webSshSocket = null
    }
    disposeWebSshTerminal()
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

const goCredential = (name) => {
    if (!name || name === '-') return
    router.push({ path: '/assets/credentials/index', query: { search: name } })
}

const getRowClassName = (record) => {
    if (!hasHostCredential(record)) {
        return 'row-no-credential'
    }
    return record.collect_status === 'failed' ? 'row-collect-failed' : ''
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
    return '打开 Agent WebSSH 终端（需先选择凭证）'
}

const formatDateTime = (value) => {
    return formatDateTimeWithTimezone(value, formatTimeWithTimezone, store.state.user?.timezone || 'Asia/Shanghai')
}

onMounted(async () => {
    loadWebSshDefaultCredentialMap()
    // 先加载主机分组最大层级配置，再构建树（buildTreeData 依赖此值）
    await getConfigByKey(CONFIG_KEYS.HOSTGROUP_MAX_TREE_DEPTH).then(res => {
        if (res.data?.value) groupMaxTreeDepth.value = Number(res.data.value) || 5
    }).catch(() => {})
    await Promise.all([loadGroupTree(), loadCredentialOptions()])
    document.addEventListener('click', closeGroupContextMenu)
    window.addEventListener('keydown', handleWebSshGlobalKeydown, true)
    document.addEventListener('keydown', handleWebSshGlobalKeydown, true)
})

watch(
    () => [route.query.group_id, route.query.search, route.query.host_id],
    async () => {
        await applyRouteFilters()
    },
    { immediate: true }
)

onBeforeUnmount(() => {
    document.removeEventListener('click', closeGroupContextMenu)
    window.removeEventListener('keydown', handleWebSshGlobalKeydown, true)
    document.removeEventListener('keydown', handleWebSshGlobalKeydown, true)
    closeWebSsh()
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

:deep(.row-no-credential) > td {
    background-color: #fffbe6;
}

:deep(.row-no-credential:hover) > td {
    background-color: #fff1b8 !important;
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

.webssh-terminal {
    width: 100%;
    height: 460px;
    border-radius: 8px;
    overflow: hidden;
    border: 1px solid #1f2937;
}

.host-page :deep(.ant-btn-lg),
.host-page :deep(.ant-input-search .ant-input),
.host-page :deep(.ant-input-search .ant-btn) {
    font-size: 15px;
}
</style>