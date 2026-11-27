<template>
  <div class="workspace">
    <header class="workspace-hero">
      <div>
        <p class="eyebrow"><AppstoreOutlined /> DJADMIN · OPS CONSOLE</p>
        <h1>运维工作台</h1>
        <p class="welcome">{{ username ? `你好，${username}。` : '你好。' }}在这里查看资产、告警与执行动态。</p>
      </div>
      <time v-if="overview" :datetime="overview.generated_at" class="snapshot">
        <ClockCircleOutlined /> 更新于 {{ formatTime(overview.generated_at) }}
      </time>
    </header>

    <a-skeleton v-if="loading && !overview" active />
    <a-alert v-if="error" type="error" show-icon :message="error" />
    <a-empty v-if="overview && !visibleSections.length" description="当前账号暂无业务概览权限，可前往文档中心了解平台功能。" />

    <section v-if="visibleSections.length" aria-labelledby="home-overview">
      <h2 id="home-overview">运行概览</h2>
      <div class="metrics">
        <a-card v-for="section in visibleSections" :key="section.key" class="metric" :class="section.color">
          <div class="metric-title">
            <span class="tile-icon"><component :is="section.icon" /></span>
            {{ section.label }}
          </div>
          <strong class="metric-value">{{ section.data.count ?? '—' }}</strong>
          <div class="muted">{{ section.data.status === 'ok' ? section.scope : '暂时无法获取' }}</div>
        </a-card>
      </div>
    </section>

    <section v-if="activities.length" aria-labelledby="home-activity">
      <h2 id="home-activity">近期动态</h2>
      <div class="activity-grid">
        <a-card v-for="section in activities" :key="section.key" :title="section.activity">
          <template #extra><router-link :to="section.path">查看全部 <RightOutlined /></router-link></template>
          <p v-if="section.data.status !== 'ok'" class="panel-empty">暂时无法获取记录</p>
          <p v-else-if="!section.data.recent?.length" class="panel-empty">{{ section.empty }}</p>
          <ul v-else class="activity-list">
            <li v-for="row in section.data.recent" :key="row.id">
              <div class="activity-copy">
                <strong>{{ row.name || `#${row.id}` }}</strong>
                <span class="muted">
                  <time :datetime="row.time">{{ formatTime(row.time) }}</time>
                  <span v-if="row.detail"> · {{ row.detail }}</span>
                </span>
              </div>
              <a-tag :color="statuses[row.status]?.color">{{ statuses[row.status]?.label || row.status || '未分级' }}</a-tag>
            </li>
          </ul>
        </a-card>
      </div>
    </section>

    <section aria-labelledby="home-shortcuts">
      <h2 id="home-shortcuts">快捷入口</h2>
      <nav class="shortcuts" aria-labelledby="home-shortcuts">
        <router-link v-for="entry in shortcuts" :key="entry.path" :to="entry.path" class="shortcut" :class="entry.color">
          <span class="tile-icon"><component :is="entry.icon" /></span>
          <span class="shortcut-copy"><strong>{{ entry.title }}</strong><span class="muted">{{ entry.description }}</span></span>
          <RightOutlined class="shortcut-arrow" />
        </router-link>
      </nav>
    </section>
  </div>
</template>

<script setup>
import { computed, onActivated, ref } from 'vue'
import {
  AlertOutlined, AppstoreOutlined, BookOutlined, CheckCircleOutlined, ClockCircleOutlined,
  CodeOutlined, DatabaseOutlined, FileSearchOutlined, RightOutlined,
} from '@ant-design/icons-vue'
import { getHomeOverview } from '@/api/home'
import store from '@/store'
import { formatTimeWithTimezone } from '@/util/timezone'

const sections = [
  { key: 'hosts', label: '主机资产', scope: '平台已登记主机', title: '主机管理', description: '查看主机资产与 Agent', path: '/assets/hosts', icon: DatabaseOutlined, color: 'blue' },
  { key: 'alerts', label: '触发中告警', scope: '平台已接收且尚未恢复', title: '监控告警', description: '查看实时告警与历史', path: '/monitor/alerting/alerts', icon: AlertOutlined, color: 'orange', activity: '触发中告警', empty: '暂无已接收的触发中告警' },
  { key: 'jobs', label: '运行中作业', scope: '当前状态为运行中', title: '运行记录中心', description: '查看作业结果与日志', path: '/sys/automation/logs', icon: CodeOutlined, color: 'purple', activity: '最近作业', empty: '暂无作业记录' },
  { key: 'inspections', label: '失败巡检', scope: '近 24 小时发起的失败执行', title: '巡检中心', description: '查看巡检任务与报告', path: '/sys/inspection', icon: CheckCircleOutlined, color: 'teal', activity: '最近巡检', empty: '暂无巡检记录' },
]
const statuses = {
  critical: { label: '严重', color: 'red' },
  warning: { label: '警告', color: 'orange' },
  info: { label: '信息', color: 'blue' },
  pending: { label: '等待中' },
  queued: { label: '排队中' },
  running: { label: '运行中', color: 'processing' },
  success: { label: '成功', color: 'green' },
  failed: { label: '失败', color: 'red' },
  canceled: { label: '已取消' },
  skipped: { label: '已跳过' },
  timeout: { label: '超时', color: 'orange' },
}

const overview = ref(null)
const loading = ref(false)
const error = ref('')
const username = computed(() => store.state.user?.username)
const timezone = computed(() => store.state.user?.timezone || 'Asia/Shanghai')
const formatTime = (time) => formatTimeWithTimezone(time, timezone.value, 'MM-DD HH:mm')
const visibleSections = computed(() => sections
  .filter(({ key }) => overview.value?.[key])
  .map((section) => ({ ...section, data: overview.value[section.key] })))
const activities = computed(() => visibleSections.value.filter(({ activity }) => activity))
const shortcuts = computed(() => [
  ...visibleSections.value,
  ...(overview.value?.alerts ? [{ title: '日志中心', description: '按服务查看日志与采集配置', path: '/monitor/logging/center', icon: FileSearchOutlined, color: 'teal' }] : []),
  { title: '文档中心', description: '查阅功能说明与排障手册', path: '/docs', icon: BookOutlined, color: 'blue' },
])

async function loadOverview() {
  if (loading.value) return
  loading.value = true
  error.value = ''
  try {
    const { data } = await getHomeOverview()
    if (data?.code !== 200) throw new Error(data?.msg || '运维概览加载失败。')
    if (!data.data?.generated_at) throw new Error('运维概览响应不完整。')
    overview.value = data.data
  } catch (err) {
    overview.value = null
    error.value = err?.response?.data?.msg || err?.message || '运维概览加载失败，请重新进入首页。'
  } finally {
    loading.value = false
  }
}

// 首页由布局的 KeepAlive 托管；首次进入与切回时查询，后台不轮询。
onActivated(loadOverview)
</script>

<style scoped>
.workspace { display: grid; gap: 20px; padding: 8px 0 16px; color: #17243b; }
.workspace-hero {
  display: flex; flex-wrap: wrap; align-items: flex-end; justify-content: space-between; gap: 16px;
  padding: clamp(20px, 3vw, 36px); border-radius: 14px; color: #fff;
  background: linear-gradient(110deg, #10213b, #1a3e64);
}
.eyebrow { margin: 0 0 10px; color: #b7d4f5; font-size: 12px; font-weight: 600; letter-spacing: .12em; }
h1 { margin: 0; color: inherit; font-size: clamp(26px, 3vw, 36px); line-height: 1.2; }
.welcome { margin: 10px 0 0; color: #d3e2f5; }
.snapshot { color: #d3e2f5; font-size: 12px; white-space: nowrap; }
h2 { margin: 0 0 10px; color: inherit; font-size: 17px; font-weight: 650; }
.metrics, .activity-grid, .shortcuts { display: grid; gap: 12px; }
.metrics { grid-template-columns: repeat(auto-fit, minmax(min(100%, 170px), 1fr)); }
.activity-grid { grid-template-columns: repeat(auto-fit, minmax(min(100%, 270px), 1fr)); }
.shortcuts { grid-template-columns: repeat(auto-fill, minmax(min(100%, 220px), 1fr)); }
.blue { --tile-bg: #edf4ff; --tile-ink: #1958aa; }
.orange { --tile-bg: #fff2e8; --tile-ink: #aa4814; }
.purple { --tile-bg: #f3edff; --tile-ink: #6841a7; }
.teal { --tile-bg: #e6f7f4; --tile-ink: #13776c; }
.tile-icon {
  display: inline-grid; flex: 0 0 auto; width: 34px; height: 34px; place-items: center;
  border-radius: 9px; background: var(--tile-bg); color: var(--tile-ink); font-size: 16px;
}
.metric-title { display: flex; align-items: center; gap: 10px; font-weight: 600; }
.metric-value { display: block; margin: 10px 0 2px; font-size: 30px; line-height: 1.15; font-variant-numeric: tabular-nums; }
.metric :deep(.ant-card-body) { padding: 16px 18px; }
.muted { color: #596a81; font-size: 12px; }
.activity-grid :deep(.ant-card-head) { padding: 0 16px; }
.activity-grid :deep(.ant-card-body) { padding: 2px 16px 8px; }
.activity-grid :deep(.ant-card-extra) { font-size: 12px; }
.panel-empty { margin: 0; padding: 24px 0; text-align: center; color: #596a81; }
.activity-list { margin: 0; padding: 0; list-style: none; }
.activity-list li { display: flex; align-items: center; justify-content: space-between; gap: 8px; padding: 9px 0; }
.activity-list li + li { border-top: 1px solid #eef0f4; }
.activity-copy, .shortcut-copy { display: grid; gap: 2px; min-width: 0; overflow-wrap: anywhere; }
.activity-list :deep(.ant-tag) { margin: 0; white-space: normal; }
.shortcut {
  display: flex; align-items: center; gap: 10px; padding: 12px; color: inherit;
  border: 1px solid #e8ebf0; border-radius: 9px; background: #fff;
}
.shortcut:hover, .shortcut:focus-visible { color: #1958aa; border-color: #91caff; }
.shortcut-arrow { margin-left: auto; font-size: 11px; }
</style>