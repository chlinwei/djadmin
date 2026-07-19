<template>
    <div class="agent-runtime-page">
        <a-page-header
            class="agent-runtime-header"
            :title="`Agent 运行状态 - ${hostDisplayTitle}`"
            @back="goBack"
        >
            <template #extra>
                <a-space>
                    <a-button size="middle" @click="goHostDetail">主机详情</a-button>
                    <a-button
                        size="middle"
                        type="primary"
                        @click="triggerAgentRuntimeCollect"
                        :loading="dispatching"
                        :disabled="!canDispatchCollect"
                    >
                        下发采集任务
                    </a-button>
                    <a-button size="middle" @click="refreshRuntimeStatus" :loading="loading">
                        刷新状态
                    </a-button>
                </a-space>
            </template>
        </a-page-header>

        <a-alert
            v-if="!canDispatchCollect"
            type="warning"
            show-icon
            message="该主机未绑定 agent 实例，无法下发采集任务"
            style="margin-bottom: 12px"
        />

        <a-card size="small" class="runtime-card">
            <a-spin :spinning="loading">
                <a-descriptions bordered :column="2" size="small" v-if="runtimeData">
                    <a-descriptions-item label="主机 ID">{{ hostId || '-' }}</a-descriptions-item>
                    <a-descriptions-item label="Agent ID">{{ runtimeData.agent_id || '-' }}</a-descriptions-item>
                    <a-descriptions-item label="Agent 版本">{{ runtimeData.version || '-' }}</a-descriptions-item>
                    <a-descriptions-item label="进程状态">{{ runtimeData.process?.running ? '运行中' : '已停止' }}</a-descriptions-item>
                    <a-descriptions-item label="运行时长">{{ formatUptimeDuration(runtimeData.process?.uptime_seconds) }}</a-descriptions-item>
                    <a-descriptions-item label="HTTP 监听">{{ runtimeData.http?.listen_addr || '-' }}</a-descriptions-item>
                    <a-descriptions-item label="HTTP 鉴权">{{ runtimeData.http?.auth_enabled ? '开启' : '关闭' }}</a-descriptions-item>
                    <a-descriptions-item label="主机上报间隔(当前)">{{ runtimeData.config?.host_report_interval_current_seconds ?? '-' }} 秒</a-descriptions-item>
                    <a-descriptions-item label="主机上报间隔(回退)">{{ runtimeData.config?.host_report_interval_fallback_seconds ?? '-' }} 秒</a-descriptions-item>
                    <a-descriptions-item label="心跳下次运行">{{ formatDateTime(runtimeData.schedulers?.heartbeat?.next_run_at) }}</a-descriptions-item>
                    <a-descriptions-item label="快照下次运行">{{ formatDateTime(runtimeData.schedulers?.host_snapshot?.next_run_at) }}</a-descriptions-item>
                </a-descriptions>

                <a-empty v-else description="暂无 Agent 状态数据" />

                <a-card title="当前注册任务" style="margin-top: 12px;" v-if="runtimeTasks.length">
                    <a-tabs v-model:activeKey="taskTabKey" size="small">
                        <a-tab-pane key="builtin" :tab="`内置任务 (${builtinTasks.length})`">
                            <a-table
                                :columns="builtinTaskColumns"
                                :data-source="builtinTasks"
                                :pagination="false"
                                size="small"
                                :scroll="{ x: 1300 }"
                                :rowKey="buildRuntimeTaskRowKey"
                            >
                                <template #bodyCell="{ column, record }">
                                    <template v-if="column.key === 'last_result_text'">
                                        <a-tag v-if="record.last_result_raw === 'success'" color="success">成功</a-tag>
                                        <a-tag v-else-if="record.last_result_raw === 'failed'" color="error">失败</a-tag>
                                        <a-tag v-else-if="record.last_result_raw === 'timeout'" color="warning">超时</a-tag>
                                        <a-tag v-else-if="record.last_result_raw === 'running'" color="processing">运行中</a-tag>
                                        <a-tag v-else>{{ record.last_result_text }}</a-tag>
                                    </template>
                                    <template v-else-if="column.key === 'enabled'">
                                        {{ record.enabled ? '开启' : '关闭' }}
                                    </template>
                                    <template v-else-if="column.key === 'last_run_at' || column.key === 'next_run_at' || column.key === 'updated_at'">
                                        {{ formatDateTime(record[column.key]) }}
                                    </template>
                                </template>
                            </a-table>
                            <a-empty v-if="!builtinTasks.length" description="暂无内置任务" />
                        </a-tab-pane>
                        <a-tab-pane key="dynamic" :tab="`动态任务 (${dynamicTaskTotal})`">
                            <div v-if="dynamicTaskGroups.length" class="dynamic-two-level-layout">
                                <div class="dynamic-level-one-pane">
                                    <a-menu
                                        mode="inline"
                                        :selectedKeys="selectedDynamicAction ? [selectedDynamicAction] : []"
                                        class="dynamic-action-menu"
                                        @click="onDynamicActionMenuClick"
                                    >
                                        <a-menu-item v-for="group in dynamicTaskGroups" :key="group.action">
                                            <div class="dynamic-task-group-header">
                                                <span class="dynamic-task-group-title">{{ group.action }}</span>
                                                <span class="dynamic-task-group-meta">{{ group.count }}</span>
                                            </div>
                                        </a-menu-item>
                                    </a-menu>
                                </div>

                                <div class="dynamic-level-two-pane">
                                    <a-tabs
                                        v-if="selectedDynamicGroup"
                                        v-model:activeKey="activeDynamicResultTab"
                                        size="small"
                                        class="dynamic-result-tabs"
                                    >
                                        <a-tab-pane key="all" :tab="`全部 (${selectedDynamicGroup.total || 0})`" />
                                        <a-tab-pane v-for="option in dynamicResultTabOptions" :key="option.key" :tab="`${option.label} (${selectedDynamicGroup.statusCounts[option.key] || 0})`" />
                                    </a-tabs>

                                    <a-table
                                        :columns="runtimeTaskColumns"
                                        :data-source="selectedDynamicDisplayRows"
                                        :loading="selectedDynamicGroup?.loading"
                                        :pagination="{
                                            current: selectedDynamicGroup?.page || 1,
                                            pageSize: selectedDynamicGroup?.pageSize || 10,
                                            total: selectedDynamicGroup?.total || 0,
                                            showSizeChanger: true,
                                            pageSizeOptions: ['10', '20', '50'],
                                            showTotal: (value) => `共 ${value} 条`,
                                        }"
                                        size="small"
                                        :scroll="{ x: 1300 }"
                                        :rowKey="buildRuntimeTaskRowKey"
                                        @change="(pagination) => onDynamicGroupTableChange(selectedDynamicGroup?.action || '', pagination)"
                                    >
                                        <template #bodyCell="{ column, record }">
                                            <template v-if="column.key === 'last_result_text'">
                                                <a-tag v-if="record.last_result_raw === 'success'" color="success">成功</a-tag>
                                                <a-tag v-else-if="record.last_result_raw === 'failed'" color="error">失败</a-tag>
                                                <a-tag v-else-if="record.last_result_raw === 'timeout'" color="warning">超时</a-tag>
                                                <a-tag v-else-if="record.last_result_raw === 'running'" color="processing">运行中</a-tag>
                                                <a-tag v-else>{{ record.last_result_text }}</a-tag>
                                            </template>
                                            <template v-else-if="column.key === 'enabled'">
                                                {{ record.enabled ? '开启' : '关闭' }}
                                            </template>
                                            <template v-else-if="column.key === 'last_run_at' || column.key === 'next_run_at' || column.key === 'updated_at'">
                                                {{ formatDateTime(record[column.key]) }}
                                            </template>
                                        </template>
                                    </a-table>
                                    <div class="dynamic-result-summary" v-if="selectedDynamicGroup?.statusSummaryText">
                                        结果分组：{{ selectedDynamicGroup.statusSummaryText }}
                                    </div>
                                </div>
                            </div>
                            <a-empty v-if="!dynamicTaskGroups.length && !dynamicGroupLoading" description="暂无动态任务" />
                            <a-spin v-if="dynamicGroupLoading" :spinning="true" />
                        </a-tab-pane>
                    </a-tabs>
                </a-card>
            </a-spin>
        </a-card>
    </div>
</template>

<script setup>
defineOptions({
    name: 'HostAgentRuntimePage',
})

import { computed, onMounted, ref, watch } from 'vue'
import { message } from 'ant-design-vue'
import { useRoute, useRouter } from 'vue-router'
import { createAgentJob, getHostAgentRuntimeStatus, getHostById, queryHostDynamicTasks } from '@/api/assets/host/index.js'
import { formatDateTimeWithTimezone } from './hostDisplayUtils'
import { getHostDisplayName } from './hostGroupTreeUtils'
import { formatTimeWithTimezone } from '@/util/timezone'
import store from '@/store'

const route = useRoute()
const router = useRouter()

const loading = ref(false)
const dispatching = ref(false)
const runtimeData = ref(null)
const hostRecord = ref(null)
const hostDisplayTitle = ref('-')
const taskTabKey = ref('builtin')
const dynamicGroupLoading = ref(false)
const dynamicTaskGroupsRaw = ref([])
const dynamicTaskPageState = ref({})
const selectedDynamicAction = ref('')
const activeDynamicResultTab = ref('all')

const dynamicResultTabOptions = [
    { key: 'running', label: '运行中' },
    { key: 'queued', label: '排队' },
    { key: 'failed', label: '失败' },
    { key: 'timeout', label: '超时' },
    { key: 'canceled', label: '已取消' },
    { key: 'success', label: '成功' },
    { key: 'other', label: '其他' },
]

const runtimeTaskColumns = [
    { title: '任务名', dataIndex: 'name', key: 'name', width: 180 },
    { title: '任务类型', dataIndex: 'task_type_text', key: 'task_type_text', width: 110 },
    { title: '任务来源', dataIndex: 'source_text', key: 'source_text', width: 110 },
    { title: '上次结果', dataIndex: 'last_result_text', key: 'last_result_text', width: 120 },
    { title: '任务状态', dataIndex: 'status_text', key: 'status_text', width: 110 },
    { title: '启用状态', dataIndex: 'enabled', key: 'enabled', width: 110 },
    { title: '间隔(秒)', dataIndex: 'interval_seconds_text', key: 'interval_seconds_text', width: 110 },
    { title: '作业ID', dataIndex: 'job_id', key: 'job_id', width: 220 },
    { title: '最近运行', dataIndex: 'last_run_at', key: 'last_run_at', width: 180 },
    { title: '下次运行', dataIndex: 'next_run_at', key: 'next_run_at', width: 180 },
    { title: '最近更新时间', dataIndex: 'updated_at', key: 'updated_at', width: 180 },
]

const builtinTaskColumns = runtimeTaskColumns.filter((column) => column.key !== 'updated_at')

const hostId = computed(() => Number(route.params.id || 0))

const canDispatchCollect = computed(() => {
    const agentId = String(hostRecord.value?.instance_name || '').trim()
    return Boolean(hostId.value > 0 && agentId)
})

const runtimeTasks = computed(() => {
    const explicitTasks = runtimeData.value?.registered_tasks
    if (!Array.isArray(explicitTasks)) {
        return []
    }

    return explicitTasks
        .filter((item) => item && typeof item === 'object')
        .map((item) => ({
            name: String(item.name || '-'),
            task_type_text: String(item.task_type || '').trim() === 'periodic' ? '周期' : '事件',
            source_text: String(item.source || '').trim() === 'builtin' ? '内置' : '动态',
            status_text: ({
                queued: '排队',
                running: '运行中',
                success: '成功',
                failed: '失败',
                timeout: '超时',
                canceled: '已取消',
            }[String(item.status || '').trim().toLowerCase()] || '-'),
            last_result_text: ({
                success: '成功',
                failed: '失败',
                timeout: '超时',
                canceled: '已取消',
                running: '运行中',
                queued: '排队',
            }[String(item.last_result || item.status || '').trim().toLowerCase()] || '-'),
            enabled: Boolean(item.enabled),
            interval_seconds_text: Number(item.interval_seconds ?? 0) > 0 ? Number(item.interval_seconds ?? 0) : '-',
            job_id: String(item.job_id || '-'),
            last_run_at: item.last_run_at || null,
            next_run_at: item.next_run_at || null,
            updated_at: item.updated_at || null,
            last_result_raw: String(item.last_result || item.status || '').trim().toLowerCase(),
        }))
})

const builtinTasks = computed(() => {
    return runtimeTasks.value.filter((item) => item.source_text === '内置')
})

const dynamicTaskTotal = computed(() => {
    return dynamicTaskGroupsRaw.value.reduce((sum, item) => sum + Number(item?.count || 0), 0)
})

const dynamicTaskGroups = computed(() => {
    return dynamicTaskGroupsRaw.value.map((item) => {
        const action = String(item.action || '-').trim() || '-'
        const state = dynamicTaskPageState.value[action] || {
            page: 1,
            pageSize: 10,
            total: Number(item.count || 0),
            tasks: [],
            loading: false,
        }
        const statusBuckets = {
            running: [],
            queued: [],
            failed: [],
            timeout: [],
            canceled: [],
            success: [],
            other: [],
        }

        ;(Array.isArray(state.tasks) ? state.tasks : []).forEach((row) => {
            const key = String(row?.last_result_raw || '').trim().toLowerCase()
            if (statusBuckets[key]) {
                statusBuckets[key].push(row)
                return
            }
            statusBuckets.other.push(row)
        })

        const displayRows = [
            ...statusBuckets.running,
            ...statusBuckets.queued,
            ...statusBuckets.failed,
            ...statusBuckets.timeout,
            ...statusBuckets.canceled,
            ...statusBuckets.success,
            ...statusBuckets.other,
        ]

        const summaryParts = []
        if (statusBuckets.running.length) summaryParts.push(`运行中 ${statusBuckets.running.length}`)
        if (statusBuckets.queued.length) summaryParts.push(`排队 ${statusBuckets.queued.length}`)
        if (statusBuckets.failed.length) summaryParts.push(`失败 ${statusBuckets.failed.length}`)
        if (statusBuckets.timeout.length) summaryParts.push(`超时 ${statusBuckets.timeout.length}`)
        if (statusBuckets.canceled.length) summaryParts.push(`已取消 ${statusBuckets.canceled.length}`)
        if (statusBuckets.success.length) summaryParts.push(`成功 ${statusBuckets.success.length}`)
        if (statusBuckets.other.length) summaryParts.push(`其他 ${statusBuckets.other.length}`)

        return {
            key: action,
            action,
            count: Number(item.count || 0),
            page: state.page,
            pageSize: state.pageSize,
            total: state.total,
            tasks: state.tasks,
            displayRows,
            statusBuckets,
            statusCounts: {
                running: statusBuckets.running.length,
                queued: statusBuckets.queued.length,
                failed: statusBuckets.failed.length,
                timeout: statusBuckets.timeout.length,
                canceled: statusBuckets.canceled.length,
                success: statusBuckets.success.length,
                other: statusBuckets.other.length,
            },
            statusSummaryText: summaryParts.join(' | '),
            loading: state.loading,
        }
    })
})

const selectedDynamicGroup = computed(() => {
    if (!selectedDynamicAction.value) {
        return null
    }
    return dynamicTaskGroups.value.find((item) => item.action === selectedDynamicAction.value) || null
})

const selectedDynamicDisplayRows = computed(() => {
    const group = selectedDynamicGroup.value
    if (!group) {
        return []
    }
    if (activeDynamicResultTab.value === 'all') {
        return group.displayRows
    }
    return group.statusBuckets[activeDynamicResultTab.value] || []
})

const buildRuntimeTaskRowKey = (record) => {
    if (record.job_id && record.job_id !== '-') {
        return record.job_id
    }
    return `${record.name}-${record.source_text}-${record.updated_at || ''}`
}

const formatDateTime = (value) => {
    return formatDateTimeWithTimezone(value, formatTimeWithTimezone, store.state.user?.timezone || 'Asia/Shanghai')
}

const formatUptimeDuration = (seconds) => {
    const totalSeconds = Number(seconds)
    if (!Number.isFinite(totalSeconds) || totalSeconds < 0) {
        return '-'
    }

    const normalizedSeconds = Math.floor(totalSeconds)
    const days = Math.floor(normalizedSeconds / 86400)
    const hours = Math.floor((normalizedSeconds % 86400) / 3600)
    const minutes = Math.floor((normalizedSeconds % 3600) / 60)
    return `${days}天${hours}小时${minutes}分钟`
}

const loadHost = async () => {
    if (hostId.value <= 0) {
        hostRecord.value = null
        hostDisplayTitle.value = '-'
        return
    }

    try {
        const res = await getHostById(hostId.value)
        if (res?.data?.code === 200) {
            hostRecord.value = res.data.data || null
            hostDisplayTitle.value = getHostDisplayName(hostRecord.value)
            return
        }
        hostRecord.value = null
        hostDisplayTitle.value = `ID:${hostId.value}`
    } catch (error) {
        hostRecord.value = null
        hostDisplayTitle.value = `ID:${hostId.value}`
    }
}

const loadRuntimeStatus = async () => {
    if (hostId.value <= 0) {
        runtimeData.value = null
        return
    }

    loading.value = true
    try {
        const res = await getHostAgentRuntimeStatus(hostId.value)
        if (res?.data?.code === 200) {
            runtimeData.value = res.data.data || {}
            return
        }
        runtimeData.value = null
        message.error(res?.data?.msg || '获取 Agent 状态失败')
    } catch (error) {
        runtimeData.value = null
        message.error(error?.response?.data?.msg || error?.message || '获取 Agent 状态失败')
    } finally {
        loading.value = false
    }
}

const mapDynamicJobRow = (item) => {
    const normalizedStatus = String(item?.status || '').trim().toLowerCase()
    return {
        name: String(item?.action || '-'),
        task_type_text: '事件',
        source_text: '动态',
        status_text: ({
            queued: '排队',
            running: '运行中',
            success: '成功',
            failed: '失败',
            timeout: '超时',
            canceled: '已取消',
        }[normalizedStatus] || '-'),
        last_result_text: ({
            success: '成功',
            failed: '失败',
            timeout: '超时',
            canceled: '已取消',
            running: '运行中',
            queued: '排队',
        }[normalizedStatus] || '-'),
        last_result_raw: normalizedStatus,
        enabled: true,
        interval_seconds_text: '-',
        job_id: String(item?.job_id || '-'),
        last_run_at: item?.finished_at || item?.picked_at || null,
        next_run_at: null,
        updated_at: item?.finished_at || item?.picked_at || item?.create_time || null,
    }
}

const setDynamicTaskState = (action, nextState) => {
    dynamicTaskPageState.value = {
        ...dynamicTaskPageState.value,
        [action]: {
            ...(dynamicTaskPageState.value[action] || {}),
            ...nextState,
        },
    }
}

const loadDynamicGroupPage = async (action, page = 1, pageSize = 10) => {
    if (hostId.value <= 0 || !action) {
        return
    }

    setDynamicTaskState(action, { loading: true, page, pageSize })
    try {
        const res = await queryHostDynamicTasks(hostId.value, {
            action,
            page,
            size: pageSize,
        })
        if (res?.data?.code === 200) {
            const payload = res.data.data || {}
            const rows = Array.isArray(payload.results) ? payload.results : []
            setDynamicTaskState(action, {
                loading: false,
                page: Number(payload.pageNumber || page),
                pageSize: Number(payload.pageSize || pageSize),
                total: Number(payload.total || 0),
                tasks: rows.filter((item) => item && typeof item === 'object').map((item) => mapDynamicJobRow(item)),
            })
            return
        }

        setDynamicTaskState(action, { loading: false, tasks: [] })
        message.error(res?.data?.msg || '获取动态任务失败')
    } catch (error) {
        setDynamicTaskState(action, { loading: false, tasks: [] })
        message.error(error?.response?.data?.msg || error?.message || '获取动态任务失败')
    }
}

const onDynamicGroupTableChange = async (action, pagination) => {
    if (!action) {
        return
    }
    const nextPage = Number(pagination?.current || 1)
    const nextPageSize = Number(pagination?.pageSize || 10)
    await loadDynamicGroupPage(action, nextPage, nextPageSize)
}

const ensureDynamicGroupLoaded = async (action) => {
    if (!action) {
        return
    }
    const currentState = dynamicTaskPageState.value[action]
    if (currentState?.tasks?.length) {
        return
    }
    await loadDynamicGroupPage(action, Number(currentState?.page || 1), Number(currentState?.pageSize || 10))
}

const onDynamicActionMenuClick = async ({ key }) => {
    const action = String(key || '').trim()
    if (!action) {
        return
    }
    selectedDynamicAction.value = action
    activeDynamicResultTab.value = 'all'
    await ensureDynamicGroupLoaded(action)
}

const loadDynamicTasks = async () => {
    if (hostId.value <= 0) {
        dynamicTaskGroupsRaw.value = []
        dynamicTaskPageState.value = {}
        selectedDynamicAction.value = ''
        return
    }

    dynamicGroupLoading.value = true
    try {
        const res = await queryHostDynamicTasks(hostId.value, { groupBy: 'action' })
        if (res?.data?.code === 200) {
            const rows = Array.isArray(res?.data?.data?.results) ? res.data.data.results : []
            dynamicTaskGroupsRaw.value = rows.filter((item) => item && typeof item === 'object')

            const nextPageState = {}
            dynamicTaskGroupsRaw.value.forEach((item) => {
                const action = String(item.action || '-').trim() || '-'
                const previous = dynamicTaskPageState.value[action] || {}
                nextPageState[action] = {
                    page: Number(previous.page || 1),
                    pageSize: Number(previous.pageSize || 10),
                    total: Number(item.count || 0),
                    tasks: Array.isArray(previous.tasks) ? previous.tasks : [],
                    loading: false,
                }
            })
            dynamicTaskPageState.value = nextPageState

            if (dynamicTaskGroupsRaw.value.length) {
                const firstAction = String(dynamicTaskGroupsRaw.value[0].action || '-').trim() || '-'
                const nextSelectedAction = nextPageState[selectedDynamicAction.value]
                    ? selectedDynamicAction.value
                    : firstAction
                selectedDynamicAction.value = nextSelectedAction
                activeDynamicResultTab.value = 'all'
                await ensureDynamicGroupLoaded(nextSelectedAction)
            } else {
                selectedDynamicAction.value = ''
            }
            return
        }
        dynamicTaskGroupsRaw.value = []
        dynamicTaskPageState.value = {}
        message.error(res?.data?.msg || '获取动态任务失败')
    } catch (error) {
        dynamicTaskGroupsRaw.value = []
        dynamicTaskPageState.value = {}
        message.error(error?.response?.data?.msg || error?.message || '获取动态任务失败')
    } finally {
        dynamicGroupLoading.value = false
    }
}

const refreshRuntimeStatus = async () => {
    await Promise.all([loadRuntimeStatus(), loadDynamicTasks()])
}

watch(
    () => [builtinTasks.value.length, dynamicTaskTotal.value],
    ([builtinLen, dynamicLen]) => {
        if (taskTabKey.value === 'builtin' && builtinLen === 0 && dynamicLen > 0) {
            taskTabKey.value = 'dynamic'
            return
        }
        if (taskTabKey.value === 'dynamic' && dynamicLen === 0 && builtinLen > 0) {
            taskTabKey.value = 'builtin'
        }
    }
)

const triggerAgentRuntimeCollect = async () => {
    const agentId = String(hostRecord.value?.instance_name || '').trim()
    if (hostId.value <= 0 || !agentId) {
        message.warning('主机未绑定 agent 实例，无法下发采集任务')
        return
    }

    if (dispatching.value) {
        return
    }

    dispatching.value = true
    try {
        const res = await createAgentJob({
            target_type: 'single',
            target_value: agentId,
            type: 'inventory',
            action: 'get_host_info',
            params: {},
            timeout_seconds: 30,
        })

        if (res?.data?.code !== 200) {
            message.error(res?.data?.msg || '下发采集任务失败')
            return
        }

        message.success('采集任务已下发')
        await Promise.all([loadRuntimeStatus(), loadDynamicTasks()])
    } catch (error) {
        message.error(error?.response?.data?.msg || error?.message || '下发采集任务失败')
    } finally {
        dispatching.value = false
    }
}

const goBack = () => {
    router.push({ path: '/assets/hosts/index' })
}

const goHostDetail = () => {
    if (hostId.value <= 0) {
        return
    }
    router.push({ path: `/assets/hosts/detail/${hostId.value}` })
}

const initPage = async () => {
    await loadHost()
    await Promise.all([loadRuntimeStatus(), loadDynamicTasks()])
}

watch(
    () => route.params.id,
    async () => {
        await initPage()
    }
)

onMounted(async () => {
    await initPage()
})
</script>

<style scoped>
.agent-runtime-page {
    padding: 12px;
}

.agent-runtime-header {
    padding: 0 0 12px;
}

.runtime-card {
    border-radius: 12px;
}

.dynamic-task-group-collapse {
    margin-top: 4px;
}

.dynamic-two-level-layout {
    display: grid;
    grid-template-columns: 260px minmax(0, 1fr);
    gap: 12px;
    align-items: start;
}

.dynamic-level-one-pane,
.dynamic-level-two-pane {
    border: 1px solid #e2e8f0;
    border-radius: 10px;
    padding: 10px;
    background: #fff;
}

.dynamic-pane-title {
    margin-bottom: 8px;
    font-size: 13px;
    font-weight: 600;
    color: #0f172a;
}

.dynamic-action-menu {
    border-inline-end: 0;
}

.dynamic-result-tabs {
    margin-bottom: 8px;
}

.dynamic-task-group-mode {
    margin: 0 0 8px;
    color: #64748b;
    font-size: 12px;
}

.dynamic-task-group-header {
    display: flex;
    align-items: center;
    gap: 12px;
    flex-wrap: wrap;
}

.dynamic-task-group-title {
    font-weight: 600;
    color: #0f172a;
}

.dynamic-task-group-meta {
    font-size: 12px;
    color: #64748b;
}

.dynamic-result-summary {
    margin-top: 8px;
    font-size: 12px;
    color: #64748b;
}

@media (max-width: 992px) {
    .dynamic-two-level-layout {
        grid-template-columns: 1fr;
    }
}
</style>
