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
                    <a-tag v-if="detailHost?.id" :color="detailHost?.agent_online ? 'success' : 'error'">
                        Agent {{ detailHost?.agent_online ? '在线' : '离线' }}
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
                    <a-tabs v-model:activeKey="activeDetailTab">
                        <a-tab-pane key="info" tab="主机信息">
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
                        <a-descriptions-item label="主机时区">{{ hostTimezoneText }}</a-descriptions-item>
                        <a-descriptions-item label="备注" :span="2">{{ detailHost.remark || '-' }}</a-descriptions-item>
                    </a-descriptions>
                        </a-tab-pane>

                        <a-tab-pane v-if="nodeExporterEnabled" key="performance" tab="性能监控">
                            <div class="performance-toolbar">
                                <a-range-picker
                                    v-model:value="performanceTimeRange"
                                    :show-time="performanceRangeShowTime"
                                    :presets="performanceRangePresets"
                                    :getPopupContainer="getPopupContainer"
                                    :placeholder="['开始时间', '结束时间']"
                                    class="performance-time-range"
                                    @openChange="onPerformanceRangeOpenChange"
                                    @change="onPerformanceTimeRangeChange"
                                />
                            </div>
                            <a-alert v-if="performanceError" type="warning" show-icon :message="performanceError" class="performance-error" />
                            <a-spin :spinning="performanceLoading">
                                <h3 class="performance-section-title">资源</h3>
                                <div class="performance-grid">
                                    <HostMetricChart title="CPU 使用率" :result="performanceData.cpu" :timezone="timezone" />
                                    <HostMetricChart title="内存使用率" :result="performanceData.memory" :timezone="timezone" />
                                    <HostMetricChart title="系统负载" :result="performanceData.load" :timezone="timezone" :series-label="metricNameSeriesLabel" unit="" :y-axis-max="0" />
                                    <HostMetricChart title="Swap 使用率" :result="performanceData.swap" :timezone="timezone" />
                                </div>

                                <h3 class="performance-section-title">存储</h3>
                                <div class="performance-grid">
                                    <HostMetricChart
                                        title="磁盘使用率"
                                        :result="performanceData.disk"
                                        :timezone="timezone"
                                        :series-label="diskSeriesLabel"
                                    />
                                    <HostMetricChart title="inode 使用率" :result="performanceData.inode" :timezone="timezone" :series-label="diskSeriesLabel" />
                                    <HostMetricChart
                                        title="磁盘读写速度"
                                        :result="performanceData.diskThroughput"
                                        :timezone="timezone"
                                        :series-label="directionDeviceSeriesLabel"
                                        unit="MiB/s"
                                        :y-axis-max="0"
                                    />
                                    <HostMetricChart title="磁盘 IOPS" :result="performanceData.diskIops" :timezone="timezone" :series-label="directionDeviceSeriesLabel" unit="ops/s" :y-axis-max="0" />
                                    <HostMetricChart title="磁盘平均延迟" :result="performanceData.diskLatency" :timezone="timezone" :series-label="directionDeviceSeriesLabel" unit="ms" :y-axis-max="0" />
                                </div>

                                <h3 class="performance-section-title">网络</h3>
                                <div class="performance-grid">
                                    <HostMetricChart title="网络收发速度" :result="performanceData.networkThroughput" :timezone="timezone" :series-label="directionDeviceSeriesLabel" unit="MiB/s" :y-axis-max="0" />
                                    <HostMetricChart title="网络错误包" :result="performanceData.networkErrors" :timezone="timezone" :series-label="directionDeviceSeriesLabel" unit="个/s" :y-axis-max="0" />
                                    <HostMetricChart title="TCP 已建立连接" :result="performanceData.tcpConnections" :timezone="timezone" unit="个" :y-axis-max="0" />
                                </div>

                                <h3 class="performance-section-title">系统</h3>
                                <div class="performance-grid">
                                    <HostMetricChart title="文件描述符使用率" :result="performanceData.fileDescriptor" :timezone="timezone" />
                                    <HostMetricChart title="进程状态" :result="performanceData.processes" :timezone="timezone" :series-label="stateSeriesLabel" unit="个" :y-axis-max="0" />
                                    <HostMetricChart title="上下文切换" :result="performanceData.contextSwitches" :timezone="timezone" unit="次/s" :y-axis-max="0" />
                                    <HostMetricChart title="CPU 中断" :result="performanceData.interrupts" :timezone="timezone" unit="次/s" :y-axis-max="0" />
                                    <HostMetricChart title="时间同步偏差" :result="performanceData.timeOffset" :timezone="timezone" unit="秒" :y-axis-max="0" />
                                </div>
                            </a-spin>
                        </a-tab-pane>
                    </a-tabs>
                </template>
            </a-spin>
        </a-card>
    </div>
</template>

<script setup>
defineOptions({
    name: 'host-detail',
})

import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import dayjs from 'dayjs'
import { message } from 'ant-design-vue'
import { useRoute, useRouter } from 'vue-router'
import { refreshHostInfo, getHostById } from '@/api/assets/host/index.js'
import { queryPrometheusRange } from '@/api/monitor'
import { getConfigByKey, CONFIG_KEYS } from '@/api/sys/sysconfig'
import { formatTimeWithTimezone } from '@/util/timezone'
import { buildUserTimezoneRangePresets, buildUserTimezoneShowTime, toUtcQueryISOStringByUserTimezone } from '@/util/timezoneRange'
import { resolvePopupContainerByContext } from '@/util/popupContainer'
import store from '@/store'
import { formatDateTimeWithTimezone, formatPercent, formatSize, getDisks } from '../utils/hostDisplayUtils'
import HostMetricChart from './components/HostMetricChart.vue'

const route = useRoute()
const router = useRouter()
const getPopupContainer = (triggerNode) => resolvePopupContainerByContext(triggerNode)

const loading = ref(false)
const detailHost = ref(null)
const collectDispatching = ref(false)
const timezone = computed(() => store.state.user?.timezone || 'Asia/Shanghai')
const hostRuntime = computed(() => detailHost.value?.runtime || {})
const hostTimezoneText = computed(() => {
    const timezoneName = String(detailHost.value?.system?.timezone_name || '').trim()
    const utcOffset = String(detailHost.value?.system?.utc_offset || '').trim()
    if (!timezoneName) return '-'
    return utcOffset ? `${timezoneName}（${utcOffset}）` : timezoneName
})
const activeDetailTab = ref('info')
const nodeExporterMonitor = computed(() => (detailHost.value?.monitors || []).find((monitor) => {
    const monitorName = String(monitor?.name || monitor?.exporter_type || '').trim()
    const enabled = monitor?.enabled ?? monitor?.managed_enabled
    return monitorName === 'node_exporter' && enabled === true
}))
const nodeExporterEnabled = computed(() => Boolean(nodeExporterMonitor.value))
const performanceNow = dayjs().tz(timezone.value)
const performanceTimeRange = ref([performanceNow.subtract(1, 'hour'), performanceNow])
const performanceRangePresets = ref(buildUserTimezoneRangePresets(timezone.value))
const performanceRangeShowTime = computed(() => buildUserTimezoneShowTime(timezone.value))
const performanceLoading = ref(false)
const performanceError = ref('')
const performanceMetricKeys = [
    'cpu', 'memory', 'load', 'swap', 'disk', 'inode', 'diskThroughput', 'diskIops',
    'diskLatency', 'networkThroughput', 'networkErrors', 'tcpConnections',
    'fileDescriptor', 'processes', 'contextSwitches', 'interrupts', 'timeOffset',
]
const performanceData = reactive(Object.fromEntries(performanceMetricKeys.map((key) => [key, []])))

const escapePrometheusLabelValue = (value) => String(value || '')
    .replaceAll('\\', '\\\\')
    .replaceAll('"', '\\"')

const parsePrometheusResult = (response) => {
    const data = response?.data?.data || {}
    if (String(data.status || '').toLowerCase() === 'error') {
        throw new Error(data.error || 'Prometheus 查询失败')
    }
    return Array.isArray(data.result) ? data.result : []
}

const buildPerformanceQueryParams = (query) => {
    const [start, end] = performanceTimeRange.value || []
    const durationSeconds = Math.max(0, (end?.valueOf?.() - start?.valueOf?.()) / 1000)
    let step = '30s'
    if (durationSeconds > 7 * 86400) step = '1h'
    else if (durationSeconds > 2 * 86400) step = '15m'
    else if (durationSeconds > 12 * 3600) step = '5m'
    else if (durationSeconds > 2 * 3600) step = '2m'
    return {
        query,
        start: toUtcQueryISOStringByUserTimezone(start, timezone.value),
        end: toUtcQueryISOStringByUserTimezone(end, timezone.value),
        step,
    }
}

const onPerformanceRangeOpenChange = (open) => {
    if (open) {
        performanceRangePresets.value = buildUserTimezoneRangePresets(timezone.value)
    }
}

const onPerformanceTimeRangeChange = (dates) => {
    if (!Array.isArray(dates) || !dates[0] || !dates[1]) {
        performanceTimeRange.value = []
        return
    }
    performanceTimeRange.value = [dayjs(dates[0]), dayjs(dates[1])]
    loadPerformanceData()
}

const diskSeriesLabel = (metric) => metric.mountpoint || metric.device || '磁盘'
const metricNameSeriesLabel = (metric) => metric.__name__ || '指标'
const directionDeviceSeriesLabel = (metric) => [metric.direction, metric.device].filter(Boolean).join(' / ') || '指标'
const stateSeriesLabel = (metric) => metric.state || '进程'

const clearPerformanceData = () => {
    performanceMetricKeys.forEach((key) => {
        performanceData[key] = []
    })
}

const loadPerformanceData = async () => {
    if (!nodeExporterEnabled.value || performanceLoading.value) {
        return
    }
    const [startTime, endTime] = performanceTimeRange.value || []
    if (!startTime || !endTime || endTime.valueOf() <= startTime.valueOf()) {
        performanceError.value = '请选择有效的性能查询时间范围'
        return
    }
    const port = Number(nodeExporterMonitor.value?.port || 9100)
    const instance = `${detailHost.value?.ip}:${port}`
    const selector = `instance="${escapePrometheusLabelValue(instance)}"`
    const queries = {
        cpu: `100 * (1 - avg by (instance) (rate(node_cpu_seconds_total{${selector},mode="idle"}[5m])))`,
        memory: `100 * (1 - node_memory_MemAvailable_bytes{${selector}} / node_memory_MemTotal_bytes{${selector}})`,
        load: `node_load1{${selector}} or node_load5{${selector}} or node_load15{${selector}}`,
        swap: `100 * (1 - node_memory_SwapFree_bytes{${selector}} / node_memory_SwapTotal_bytes{${selector}}) and on(instance) (node_memory_SwapTotal_bytes{${selector}} > 0)`,
        disk: `100 * (1 - node_filesystem_avail_bytes{${selector},fstype!~"tmpfs|overlay|squashfs|nsfs|tracefs",mountpoint!~".*/var/snap.*"} / node_filesystem_size_bytes{${selector},fstype!~"tmpfs|overlay|squashfs|nsfs|tracefs",mountpoint!~".*/var/snap.*"})`,
        inode: `100 * (1 - node_filesystem_files_free{${selector},fstype!~"tmpfs|overlay|squashfs|nsfs|tracefs",mountpoint!~".*/var/snap.*"} / node_filesystem_files{${selector},fstype!~"tmpfs|overlay|squashfs|nsfs|tracefs",mountpoint!~".*/var/snap.*"})`,
        diskThroughput: `label_replace(sum by (device) (rate(node_disk_read_bytes_total{${selector},device!~"loop.*|ram.*|fd.*|sr.*"}[5m])) / 1024 / 1024, "direction", "读取", "", "") or label_replace(sum by (device) (rate(node_disk_written_bytes_total{${selector},device!~"loop.*|ram.*|fd.*|sr.*"}[5m])) / 1024 / 1024, "direction", "写入", "", "")`,
        diskIops: `label_replace(sum by (device) (rate(node_disk_reads_completed_total{${selector},device!~"loop.*|ram.*|fd.*|sr.*"}[5m])), "direction", "读取", "", "") or label_replace(sum by (device) (rate(node_disk_writes_completed_total{${selector},device!~"loop.*|ram.*|fd.*|sr.*"}[5m])), "direction", "写入", "", "")`,
        diskLatency: `label_replace(1000 * rate(node_disk_read_time_seconds_total{${selector},device!~"loop.*|ram.*|fd.*|sr.*"}[5m]) / clamp_min(rate(node_disk_reads_completed_total{${selector},device!~"loop.*|ram.*|fd.*|sr.*"}[5m]), 0.001), "direction", "读取", "", "") or label_replace(1000 * rate(node_disk_write_time_seconds_total{${selector},device!~"loop.*|ram.*|fd.*|sr.*"}[5m]) / clamp_min(rate(node_disk_writes_completed_total{${selector},device!~"loop.*|ram.*|fd.*|sr.*"}[5m]), 0.001), "direction", "写入", "", "")`,
        networkThroughput: `label_replace(sum by (device) (rate(node_network_receive_bytes_total{${selector},device!~"lo|docker.*|veth.*|br-.*"}[5m])) / 1024 / 1024, "direction", "接收", "", "") or label_replace(sum by (device) (rate(node_network_transmit_bytes_total{${selector},device!~"lo|docker.*|veth.*|br-.*"}[5m])) / 1024 / 1024, "direction", "发送", "", "")`,
        networkErrors: `label_replace(sum by (device) (rate(node_network_receive_errs_total{${selector},device!~"lo|docker.*|veth.*|br-.*"}[5m])), "direction", "接收", "", "") or label_replace(sum by (device) (rate(node_network_transmit_errs_total{${selector},device!~"lo|docker.*|veth.*|br-.*"}[5m])), "direction", "发送", "", "")`,
        tcpConnections: `node_netstat_Tcp_CurrEstab{${selector}}`,
        fileDescriptor: `100 * node_filefd_allocated{${selector}} / node_filefd_maximum{${selector}}`,
        processes: `label_replace(node_procs_running{${selector}}, "state", "运行", "", "") or label_replace(node_procs_blocked{${selector}}, "state", "阻塞", "", "")`,
        contextSwitches: `rate(node_context_switches_total{${selector}}[5m])`,
        interrupts: `rate(node_intr_total{${selector}}[5m])`,
        timeOffset: `abs(node_timex_offset_seconds{${selector}})`,
    }

    performanceLoading.value = true
    performanceError.value = ''
    try {
        const queryEntries = Object.entries(queries)
        const settledResults = await Promise.allSettled(queryEntries.map(async ([key, query]) => {
            const response = await queryPrometheusRange(buildPerformanceQueryParams(query))
            return [key, parsePrometheusResult(response)]
        }))
        let failedCount = 0
        settledResults.forEach((result, index) => {
            const key = queryEntries[index][0]
            if (result.status === 'fulfilled') {
                performanceData[key] = result.value[1]
            } else {
                performanceData[key] = []
                failedCount += 1
            }
        })
        if (failedCount) {
            performanceError.value = `有 ${failedCount} 项指标暂时无法查询，其余指标已正常加载`
        }
    } catch (error) {
        clearPerformanceData()
        performanceError.value = error?.response?.data?.msg || error?.message || '加载主机性能数据失败'
    } finally {
        performanceLoading.value = false
    }
}

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
    const uptimeSeconds = hostRuntime.value?.os_uptime_seconds
    const startAtRaw = parseTimeLikeValue(
        hostRuntime.value?.os_boot_time,
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
    const usage = hostRuntime.value?.cpu_usage_percent
    if (usage === null || usage === undefined || usage === '') {
        return '-'
    }
    return formatPercent(usage)
})

const cpuDetailText = computed(() => {
    const cpuTimes = hostRuntime.value?.cpu_times
    if (cpuTimes && typeof cpuTimes === 'object') {
        const getPart = (name) => {
            const value = toNumber(cpuTimes[name])
            return value === null ? '0.0' : value.toFixed(1)
        }
        return `${getPart('us')} us, ${getPart('sy')} sy, ${getPart('ni')} ni, ${getPart('id')} id, ${getPart('wa')} wa, ${getPart('hi')} hi, ${getPart('si')} si, ${getPart('st')} st`
    }
    return '-'
})

const memoryUsageText = computed(() => {
    const usage = hostRuntime.value?.memory_usage_percent
    if (usage === null || usage === undefined || usage === '') {
        return '-'
    }
    return formatPercent(usage)
})

const memoryDetailText = computed(() => {
    const memory = hostRuntime.value?.memory
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
    const source = hostRuntime.value?.disk_io
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
    if (activeDetailTab.value === 'performance') {
        await loadPerformanceData()
    }
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
        activeDetailTab.value = 'info'
        clearPerformanceData()
        loadDetail()
    },
)

watch(activeDetailTab, (tab) => {
    if (tab === 'performance') {
        loadPerformanceData()
    }
})

watch(nodeExporterEnabled, (enabled) => {
    if (!enabled && activeDetailTab.value === 'performance') {
        activeDetailTab.value = 'info'
    }
})

onMounted(() => {
    // 挂载后先同步置 loading=true：避免在等待“采集间隔配置”这个与详情无关的请求
    // 返回之前，出现 !loading && !detailHost 的空窗期而误闪一次“未找到主机详情”。
    loading.value = true
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

.performance-toolbar {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 12px;
    margin-bottom: 12px;
}

.performance-time-range {
    width: min(100%, 420px);
}

.performance-error {
    margin-bottom: 12px;
}

.performance-section-title {
    margin: 18px 0 10px;
    padding-left: 10px;
    border-left: 3px solid #1677ff;
    color: #1f1f1f;
    font-size: 15px;
}

.performance-section-title:first-of-type {
    margin-top: 4px;
}

.performance-grid {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: 12px;
}

@media (max-width: 1000px) {
    .performance-toolbar {
        align-items: stretch;
        flex-direction: column;
    }

    .performance-time-range {
        width: 100%;
    }

    .performance-grid {
        grid-template-columns: 1fr;
    }

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
