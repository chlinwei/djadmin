<template>
    <div class="host-detail-page">
        <a-card :bordered="false" class="detail-shell">
            <template #title>
                <a-space>
                    <a-button @click="goBack">
                        <FontAwesomeIcon :icon="['fas', 'arrow-left']" />
                        <span>&nbsp;返回</span>
                    </a-button>
                    <span class="detail-title">主机详情</span>
                    <a-tag v-if="detailHost?.id" color="blue">ID: {{ detailHost.id }}</a-tag>
                    <a-tag v-if="detailHost?.id" :color="detailHost?.system?.agent_online ? 'success' : 'error'">
                        Agent {{ detailHost?.system?.agent_online ? '在线' : '离线' }}
                    </a-tag>
                </a-space>
            </template>
            <template #extra>
                <a-button type="primary" ghost @click="handleRefreshClick" :loading="loading || collectDispatching">
                    <FontAwesomeIcon :icon="['fas', 'arrows-rotate']" />
                    <span>&nbsp;刷新</span>
                </a-button>
            </template>

            <a-spin :spinning="loading">
                <a-alert
                    v-if="!loading && !detailHost"
                    type="warning"
                    show-icon
                    message="未找到主机详情"
                    description="请返回列表重试，或确认该主机是否已被删除。"
                />

                <template v-else-if="detailHost">
                    <a-row :gutter="16">
                        <a-col :xs="24" :xl="8">
                            <a-card size="small" class="top-card">
                                <template #title>
                                    <a-space size="small">
                                        <FontAwesomeIcon :icon="['fas', 'server']" />
                                        <span>主机基础信息</span>
                                    </a-space>
                                </template>
                                <div class="kv-line"><span class="k">实例名称</span><span class="v">{{ detailHost.instance_name || '-' }}</span></div>
                                <div class="kv-line"><span class="k">主机名称</span><span class="v">{{ detailHost.system?.hostname || '-' }}</span></div>
                                <div class="kv-line"><span class="k">IP</span><span class="v">{{ detailHost.ip || '-' }}</span></div>
                                <div class="kv-line"><span class="k">配置</span><span class="v">{{ hostConfigText }}</span></div>
                                <div class="kv-line multiline">
                                    <span class="k">系统运行天数</span>
                                    <span class="v">{{ runtimeText }}</span>
                                </div>
                            </a-card>
                        </a-col>

                        <a-col :xs="24" :xl="8">
                            <a-card size="small" class="top-card">
                                <template #title>
                                    <a-space size="small">
                                        <FontAwesomeIcon :icon="['fas', 'microchip']" />
                                        <span>CPU</span>
                                    </a-space>
                                </template>
                                <div class="usage-block">
                                    <div class="usage-value">{{ cpuUsageText }}</div>
                                    <div class="usage-label">总CPU使用率</div>
                                </div>
                                <div class="usage-detail-block">
                                    <div class="usage-detail-value">{{ cpuDetailText }}</div>
                                    <div class="usage-detail-label">CPU详细使用</div>
                                </div>
                            </a-card>
                        </a-col>

                        <a-col :xs="24" :xl="8">
                            <a-card size="small" class="top-card">
                                <template #title>
                                    <a-space size="small">
                                        <FontAwesomeIcon :icon="['fas', 'memory']" />
                                        <span>内存</span>
                                    </a-space>
                                </template>
                                <div class="usage-block">
                                    <div class="usage-value">{{ memoryUsageText }}</div>
                                    <div class="usage-label">总内存使用率</div>
                                </div>
                                <div class="usage-detail-block">
                                    <div class="usage-detail-value">{{ memoryDetailText }}</div>
                                    <div class="usage-detail-label">内存详细使用</div>
                                </div>
                            </a-card>
                        </a-col>
                    </a-row>

                    <a-card style="margin-top: 16px;" v-if="enhancedDiskRows.length">
                        <template #title>
                            <a-space size="small">
                                <FontAwesomeIcon :icon="['fas', 'hard-drive']" />
                                <span>磁盘分区详情</span>
                            </a-space>
                        </template>
                        <a-table :columns="diskColumns" :data-source="enhancedDiskRows" :pagination="false" rowKey="device" size="small">
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
                                <template v-else-if="column.key === 'read_speed'">
                                    {{ record.read_speed || '-' }}
                                </template>
                                <template v-else-if="column.key === 'write_speed'">
                                    {{ record.write_speed || '-' }}
                                </template>
                            </template>
                        </a-table>
                    </a-card>

                    <a-descriptions bordered :column="2" size="small" style="margin-top: 16px;">
                        <a-descriptions-item label="最后采集时间">{{ formatDateTime(detailHost.last_collect_time) }}</a-descriptions-item>
                        <a-descriptions-item label="OS 类型">{{ detailHost.system?.os_type || '-' }}</a-descriptions-item>
                        <a-descriptions-item label="OS 版本">{{ detailHost.system?.os_version || '-' }}</a-descriptions-item>
                        <a-descriptions-item label="内核版本">{{ detailHost.system?.kernel_version || '-' }}</a-descriptions-item>
                        <a-descriptions-item label="Agent 版本">{{ detailHost.system?.agent_version || '-' }}</a-descriptions-item>
                        <a-descriptions-item label="备注" :span="2">{{ detailHost.remark || '-' }}</a-descriptions-item>
                    </a-descriptions>
                </template>
            </a-spin>
        </a-card>
    </div>
</template>

<script setup>
defineOptions({
    name: 'host-detail',
})

import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { message } from 'ant-design-vue'
import { useRoute, useRouter } from 'vue-router'
import { refreshHostInfo, getHostById } from '@/api/assets/host/index.js'
import { getConfigByKey, CONFIG_KEYS } from '@/api/sys/sysconfig'
import { formatTimeWithTimezone } from '@/util/timezone'
import store from '@/store'
import { formatDateTimeWithTimezone, formatPercent, formatSize, getDisks } from './hostDisplayUtils'

const route = useRoute()
const router = useRouter()

const loading = ref(false)
const detailHost = ref(null)
const collectDispatching = ref(false)
const timezone = computed(() => store.state.user?.timezone || 'Asia/Shanghai')
const hostSnapshot = computed(() => detailHost.value?.host_snapshot || {})

const DEFAULT_COLLECT_DISPATCH_INTERVAL_SECONDS = 8
const MIN_COLLECT_DISPATCH_INTERVAL_SECONDS = 3
const MAX_COLLECT_DISPATCH_INTERVAL_SECONDS = 600
const collectDispatchIntervalSeconds = ref(DEFAULT_COLLECT_DISPATCH_INTERVAL_SECONDS)
let collectDispatchTimer = null

const diskColumns = [
    { title: '设备', dataIndex: 'device', key: 'device' },
    { title: '挂载点', dataIndex: 'mount_point', key: 'mount_point' },
    { title: '容量', dataIndex: 'size_gb', key: 'size_gb' },
    { title: '已用', dataIndex: 'used_gb', key: 'used_gb' },
    { title: '使用率', dataIndex: 'usage_percent', key: 'usage_percent' },
    { title: '读取速度', dataIndex: 'read_speed', key: 'read_speed' },
    { title: '写入速度', dataIndex: 'write_speed', key: 'write_speed' },
    { title: '文件系统', dataIndex: 'filesystem', key: 'filesystem' },
]

const diskRows = computed(() => getDisks(detailHost.value || {}))

const toNumber = (value) => {
    const num = Number(value)
    if (!Number.isFinite(num)) {
        return null
    }
    return num
}

const formatRuntimeDuration = (secondsValue) => {
    const seconds = toNumber(secondsValue)
    if (seconds === null || seconds < 0) {
        return '-'
    }
    const days = Math.floor(seconds / 86400)
    const hours = Math.floor((seconds % 86400) / 3600)
    const minutes = Math.floor((seconds % 3600) / 60)
    return `${days}天${hours}小时${minutes}分钟`
}

const parseTimeLikeValue = (value) => {
    if (!value) {
        return null
    }
    if (typeof value === 'number') {
        const msValue = value > 1e12 ? value : value * 1000
        return new Date(msValue).toISOString()
    }
    if (typeof value === 'string') {
        return value
    }
    return null
}

const hostConfigText = computed(() => {
    const cores = detailHost.value?.hardware?.cpu_cores
    const memory = detailHost.value?.hardware?.memory_gb
    if (cores === null || cores === undefined || memory === null || memory === undefined) {
        return '-'
    }
    return `${cores}核${Number(memory).toFixed(0)}GB内存`
})

const runtimeText = computed(() => {
    // 仅使用 OS 字段，禁止回退到其他兼容字段，避免口径混乱。
    const uptimeSeconds = hostSnapshot.value?.os_uptime_seconds
    const startAtRaw = parseTimeLikeValue(
        hostSnapshot.value?.os_boot_time,
    )

    // 没有 OS 启动时间时不展示，避免 started_at/start_time 混入产生错误认知。
    if (!startAtRaw) {
        return '-'
    }

    const duration = formatRuntimeDuration(uptimeSeconds)
    if (duration === '-') {
        return '-'
    }
    const startedText = startAtRaw ? formatDateTime(startAtRaw) : '-'
    return `${duration}(运行开始时间:${startedText})`
})

const cpuUsageText = computed(() => {
    const usage = hostSnapshot.value?.cpu_usage_percent
    if (usage === null || usage === undefined || usage === '') {
        return '-'
    }
    return formatPercent(usage)
})

const cpuDetailText = computed(() => {
    const cpuTimes = hostSnapshot.value?.cpu_times
    if (cpuTimes && typeof cpuTimes === 'object') {
        const getPart = (name) => {
            const value = toNumber(cpuTimes[name])
            return value === null ? '0.0' : value.toFixed(1)
        }
        return `${getPart('us')} us, ${getPart('sy')} sy, ${getPart('ni')} ni, ${getPart('id')} id, ${getPart('wa')} wa, ${getPart('hi')} hi, ${getPart('si')} si, ${getPart('st')} st`
    }
    if (typeof hostSnapshot.value?.cpu_detail === 'string' && hostSnapshot.value.cpu_detail.trim()) {
        return hostSnapshot.value.cpu_detail.trim()
    }
    return '-'
})

const memoryUsageText = computed(() => {
    const usage = hostSnapshot.value?.memory_usage_percent
    if (usage === null || usage === undefined || usage === '') {
        return '-'
    }
    return formatPercent(usage)
})

const memoryDetailText = computed(() => {
    const memory = hostSnapshot.value?.memory
    if (memory && typeof memory === 'object') {
        const total = memory.total ?? '-'
        const used = memory.used ?? '-'
        const free = memory.free ?? '-'
        const available = memory.available ?? '-'
        return `total=${total}, used=${used}, free=${free}, available=${available}`
    }
    return '-'
})

const diskSpeedMap = computed(() => {
    const source = hostSnapshot.value?.disk_io
    const result = {}
    if (Array.isArray(source)) {
        source.forEach((item) => {
            const device = String(item?.device || '').trim()
            if (!device) {
                return
            }
            result[device] = {
                read_speed: item?.read_speed || item?.read || '-',
                write_speed: item?.write_speed || item?.write || '-',
            }
        })
    } else if (source && typeof source === 'object') {
        Object.keys(source).forEach((device) => {
            const item = source[device] || {}
            result[device] = {
                read_speed: item?.read_speed || item?.read || '-',
                write_speed: item?.write_speed || item?.write || '-',
            }
        })
    }
    return result
})

const buildDiskSpeedCandidates = (device) => {
    const raw = String(device || '').trim()
    if (!raw) {
        return []
    }

    const candidates = new Set([raw])
    if (raw.startsWith('/dev/')) {
        candidates.add(raw.slice('/dev/'.length))
    }
    if (raw.startsWith('/dev/mapper/')) {
        candidates.add(raw.slice('/dev/mapper/'.length))
    }
    const slashIndex = raw.lastIndexOf('/')
    if (slashIndex >= 0 && slashIndex < raw.length - 1) {
        candidates.add(raw.slice(slashIndex + 1))
    }
    return Array.from(candidates)
}

const enhancedDiskRows = computed(() => {
    return diskRows.value.map((row) => {
        const speed = buildDiskSpeedCandidates(row.device)
            .map((candidate) => diskSpeedMap.value[candidate])
            .find((item) => !!item) || {}
        return {
            ...row,
            read_speed: speed.read_speed || '-',
            write_speed: speed.write_speed || '-',
        }
    })
})

const getHostIdFromRoute = () => {
    const hostId = Number(route.params.id)
    if (!Number.isFinite(hostId) || hostId <= 0) {
        return null
    }
    return hostId
}

const formatDateTime = (value) => {
    return formatDateTimeWithTimezone(value, formatTimeWithTimezone, timezone.value)
}

const resolveConfigIntValue = (response, fallbackValue) => {
    const rawValue = response?.data?.value ?? response?.data?.data?.value
    const parsed = Number(rawValue)
    if (!Number.isFinite(parsed) || parsed <= 0) {
        return fallbackValue
    }
    return Math.floor(parsed)
}

const loadCollectDispatchIntervalConfig = async () => {
    try {
        const res = await getConfigByKey(CONFIG_KEYS.HOST_DETAIL_COLLECT_DISPATCH_INTERVAL_SECONDS)
        const configValue = resolveConfigIntValue(res, DEFAULT_COLLECT_DISPATCH_INTERVAL_SECONDS)
        collectDispatchIntervalSeconds.value = Math.max(
            MIN_COLLECT_DISPATCH_INTERVAL_SECONDS,
            Math.min(MAX_COLLECT_DISPATCH_INTERVAL_SECONDS, configValue),
        )
    } catch (error) {
        collectDispatchIntervalSeconds.value = DEFAULT_COLLECT_DISPATCH_INTERVAL_SECONDS
    }
}

const stopCollectDispatchTimer = () => {
    if (collectDispatchTimer) {
        window.clearInterval(collectDispatchTimer)
        collectDispatchTimer = null
    }
}

// 同步经 gRPC 让 agent 执行 get_host_info 并落库，一次调用即返回最新主机数据。
// 用于「打开详情页即刷」「点击刷新」以及在页面停留期间的定时刷新（保持动态指标实时）。
const refreshHostRuntime = async ({ showError = false } = {}) => {
    if (collectDispatching.value) {
        return
    }

    const hostId = detailHost.value?.id || getHostIdFromRoute()
    if (!hostId) {
        return
    }

    collectDispatching.value = true
    try {
        const res = await refreshHostInfo(hostId)
        if (res?.data?.code === 200) {
            const payload = res.data.data || {}
            if (payload.host) {
                detailHost.value = payload.host
            }
            const result = payload.result || {}
            // agent 离线/未配置实例名时后端返回 skipped，提示但不视为错误
            if (showError && result.skipped && result.error) {
                message.warning(result.error)
            } else if (showError && !result.updated && result.error) {
                message.error(result.error)
            }
            return
        }
        if (showError) {
            message.error(res?.data?.msg || '刷新主机信息失败')
        }
    } catch (error) {
        if (showError) {
            message.error(error?.response?.data?.msg || error?.message || '刷新主机信息失败')
        }
    } finally {
        collectDispatching.value = false
    }
}

const startCollectDispatchTimer = () => {
    stopCollectDispatchTimer()
    collectDispatchTimer = window.setInterval(async () => {
        if (loading.value || collectDispatching.value) {
            return
        }
        await refreshHostRuntime({ showError: false })
    }, collectDispatchIntervalSeconds.value * 1000)
}

const loadDetail = async () => {
    const hostId = getHostIdFromRoute()
    if (!hostId) {
        detailHost.value = null
        message.error('主机 ID 不合法')
        return
    }

    loading.value = true
    try {
        const res = await getHostById(hostId)
        if (res?.data?.code === 200) {
            detailHost.value = res.data.data || null
            return
        }
        detailHost.value = null
        message.error(res?.data?.msg || '获取主机详情失败')
    } catch (error) {
        detailHost.value = null
        message.error(error?.response?.data?.msg || error?.message || '获取主机详情失败')
    } finally {
        loading.value = false
    }
}

const handleRefreshClick = async () => {
    await refreshHostRuntime({ showError: true })
}

const goBack = () => {
    // Prefer native history back so list filters/pagination can be preserved.
    if (window.history.length > 1) {
        router.back()
        return
    }
    router.push('/assets/host/index')
}

watch(
    () => route.params.id,
    () => {
        loadDetail()
    },
)

onMounted(() => {
    loadCollectDispatchIntervalConfig().finally(async () => {
        await loadDetail()
        // 打开详情页即触发一次同步刷新，确保展示的是最新采集结果
        await refreshHostRuntime({ showError: false })
        startCollectDispatchTimer()
    })
})

onBeforeUnmount(() => {
    stopCollectDispatchTimer()
})
</script>

<style scoped>
.host-detail-page {
    padding: 12px 8px 18px;
    background:
        radial-gradient(circle at top left, rgba(15, 23, 42, 0.04), transparent 28%),
        linear-gradient(180deg, #f8fafc 0%, #f4f7fb 100%);
    min-height: calc(100vh - 120px);
}

.detail-shell {
    border: 1px solid #e5e7eb;
    border-radius: 14px;
    box-shadow: 0 10px 30px rgba(15, 23, 42, 0.06);
}

.detail-title {
    font-size: 16px;
    font-weight: 600;
    color: #0f172a;
}

.top-card {
    height: 100%;
    border-radius: 12px;
}

.kv-line {
    display: flex;
    justify-content: space-between;
    align-items: flex-start;
    gap: 8px;
    padding: 6px 0;
    border-bottom: 1px dashed #e5e7eb;
}

.kv-line.multiline {
    align-items: flex-start;
}

.kv-line:last-child {
    border-bottom: none;
}

.kv-line .k {
    color: #475569;
    min-width: 96px;
    flex-shrink: 0;
}

.kv-line .v {
    color: #0f172a;
    text-align: right;
    word-break: break-word;
}

.usage-block {
    padding: 10px 0 14px;
    border-bottom: 1px dashed #e5e7eb;
    margin-bottom: 2px;
    text-align: center;
}

.usage-value {
    font-size: 40px;
    line-height: 1;
    font-weight: 700;
    color: #0f172a;
    letter-spacing: 0.2px;
}

.usage-label {
    margin-top: 8px;
    font-size: 13px;
    color: #64748b;
}

.usage-detail-block {
    padding: 10px 0 0;
    text-align: center;
}

.usage-detail-value {
    color: #0f172a;
    font-size: 15px;
    line-height: 1.6;
    word-break: break-word;
}

.usage-detail-label {
    margin-top: 6px;
    font-size: 13px;
    color: #64748b;
}
</style>
