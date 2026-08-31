<template>
  <div class="log-storage-page">
    <div class="page-header">
      <h2>日志存储</h2>
      <p>配置 OpenSearch 连接，供日志采集与日志检索使用。</p>
    </div>

    <div class="toolbar">
      <!-- 日志存储为全局单例配置：已存在集群时禁用新增，仅允许编辑/删除现有记录 -->
      <a-tooltip :title="clusters.length ? '仅支持配置一个集群，请直接编辑现有记录' : '新增集群'">
        <a-button size="large" :disabled="clusters.length > 0" @click="openDialog()">
          <FontAwesomeIcon :icon="['fas', 'plus-circle']" />
          <span>&nbsp;新增集群</span>
        </a-button>
      </a-tooltip>
      <a-tooltip title="刷新">
        <a-button size="large" @click="loadClusters">
          <FontAwesomeIcon :icon="['fas', 'rotate']" />
          <span>&nbsp;刷新</span>
        </a-button>
      </a-tooltip>
      <a-tooltip title="逐层比对期望配置与集群、主机实际状态">
        <a-button size="large" :disabled="!clusters.length" :loading="healthLoading" @click="loadHealth">
          <FontAwesomeIcon :icon="['fas', 'stethoscope']" />
          <span>&nbsp;链路体检</span>
        </a-button>
      </a-tooltip>
    </div>

    <a-table
      row-key="id"
      :columns="columns"
      :data-source="clusters"
      :loading="loading"
      :pagination="false"
      :scroll="{ x: 1650 }"
    >
      <template #bodyCell="{ column, record }">
        <template v-if="column.key === 'name'">
          <span>{{ record.name }}</span>
          <a-tag v-if="record.is_default" color="blue">默认</a-tag>
        </template>
        <template v-else-if="column.key === 'enabled'">
          <a-tag :color="record.enabled ? 'green' : 'default'">{{ record.enabled ? '启用' : '停用' }}</a-tag>
        </template>
        <template v-else-if="column.key === 'verify_tls'">
          <a-tag :color="record.verify_tls ? 'green' : 'orange'">{{ record.verify_tls ? '校验' : '不校验' }}</a-tag>
        </template>
        <template v-else-if="column.key === 'check'">
          <a-tag v-if="record.last_check_success === null" color="default">未测试</a-tag>
          <a-tooltip v-else :title="record.last_check_message">
            <a-tag :color="record.last_check_success ? 'green' : 'red'">
              {{ record.last_check_success ? '正常' : '失败' }}
            </a-tag>
          </a-tooltip>
          <span v-if="record.last_check_time" class="check-time">{{ formatTime(record.last_check_time) }}</span>
        </template>
        <template v-else-if="column.key === 'storage_sync'">
          <a-tooltip v-if="record.storage_sync_error" :title="record.storage_sync_error">
            <a-tag :color="STORAGE_SYNC_COLOR[record.storage_sync_status] || 'default'">
              {{ STORAGE_SYNC_TEXT[record.storage_sync_status] || '未同步' }}
            </a-tag>
          </a-tooltip>
          <a-tag v-else :color="STORAGE_SYNC_COLOR[record.storage_sync_status] || 'default'">
            {{ STORAGE_SYNC_TEXT[record.storage_sync_status] || '未同步' }}
          </a-tag>
          <span v-if="record.storage_sync_time" class="check-time">{{ formatTime(record.storage_sync_time) }}</span>
        </template>
        <template v-else-if="column.key === 'action'">
          <a-space :size="6">
            <a-tooltip title="连接测试">
              <a-button :loading="testingId === record.id" @click="testConnection(record)">
                <FontAwesomeIcon :icon="['fas', 'plug']" />
              </a-button>
            </a-tooltip>
            <a-tooltip title="编辑">
              <a-button type="primary" @click="openDialog(record)">
                <FontAwesomeIcon :icon="['fa', 'edit']" />
              </a-button>
            </a-tooltip>
            <a-tooltip title="删除">
              <a-button class="delBtn" danger type="primary" @click="confirmDelete(record)">
                <FontAwesomeIcon :icon="['fas', 'trash-can']" />
              </a-button>
            </a-tooltip>
          </a-space>
        </template>
      </template>
    </a-table>

    <div v-if="health" class="health-panel">
      <a-alert :type="OVERALL_ALERT[health.status] || 'info'" show-icon :message="overallMessage">
        <template #description>
          <span class="health-time">检查时间 {{ formatTime(health.checked_at) }}</span>
        </template>
      </a-alert>

      <a-collapse v-model:activeKey="openLayers" class="health-layers">
        <a-collapse-panel v-for="layer in health.layers" :key="layer.key">
          <template #header>
            <a-tag :color="STATUS_COLOR[layer.status]">{{ STATUS_TEXT[layer.status] }}</a-tag>
            <span class="layer-name">{{ layer.name }}</span>
            <span class="layer-summary">{{ layer.summary }}</span>
          </template>
          <a-empty v-if="!layer.items.length" :image="simpleEmptyImage" description="无明细" />
          <div v-for="item in layer.items" :key="item.name" class="health-item">
            <a-tag :color="STATUS_COLOR[item.status]">{{ STATUS_TEXT[item.status] }}</a-tag>
            <span class="item-name">{{ item.name }}</span>
            <span class="item-detail">{{ item.detail }}</span>
          </div>
        </a-collapse-panel>
      </a-collapse>
    </div>

    <a-modal
      v-model:open="dialogOpen"
      :title="form.id ? `编辑集群：${form.name}` : '新增集群'"
      :width="720"
      centered
      :confirm-loading="saving"
      ok-text="保存"
      cancel-text="取消"
      @ok="submit"
    >
      <a-form ref="formRef" :model="form" :rules="rules" layout="vertical">
        <a-row :gutter="16">
          <a-col :span="12">
            <a-form-item name="name" label="集群名称">
              <a-input v-model:value="form.name" placeholder="例如 日志集群" />
            </a-form-item>
          </a-col>
          <a-col :span="12">
            <a-form-item name="index_prefix" label="索引前缀">
              <a-input v-model:value="form.index_prefix" placeholder="logs" />
            </a-form-item>
          </a-col>
          <a-col :span="24">
            <a-form-item name="hosts" label="连接地址">
              <a-input v-model:value="form.hosts" placeholder="https://10.0.0.1:9200，多个用逗号分隔" />
            </a-form-item>
          </a-col>
          <a-col :span="12">
            <a-form-item name="username" label="用户名">
              <a-input v-model:value="form.username" autocomplete="off" />
            </a-form-item>
          </a-col>
          <a-col :span="12">
            <a-form-item name="password" label="密码">
              <a-input-password
                v-model:value="form.password"
                autocomplete="new-password"
                :placeholder="form.id && form.password_configured ? '不修改请保留占位符' : ''"
              />
            </a-form-item>
          </a-col>
          <a-col :span="12">
            <a-form-item name="request_timeout" label="请求超时（秒）">
              <a-input-number v-model:value="form.request_timeout" :min="1" :max="300" style="width: 100%" />
            </a-form-item>
          </a-col>
          <a-col :span="6">
            <a-form-item label="校验 TLS">
              <a-switch v-model:checked="form.verify_tls" />
            </a-form-item>
          </a-col>
          <a-col :span="6">
            <a-form-item label="设为默认">
              <a-switch v-model:checked="form.is_default" />
            </a-form-item>
          </a-col>
          <a-col v-if="form.verify_tls" :span="24">
            <a-form-item name="ca_cert" label="CA 证书">
              <a-textarea v-model:value="form.ca_cert" :rows="4" placeholder="-----BEGIN CERTIFICATE-----" />
            </a-form-item>
          </a-col>
          <a-col :span="6">
            <a-form-item label="启用">
              <a-switch v-model:checked="form.enabled" />
            </a-form-item>
          </a-col>
          <a-col :span="24">
            <a-form-item name="remark" label="备注">
              <a-textarea v-model:value="form.remark" :rows="2" />
            </a-form-item>
          </a-col>
        </a-row>
      </a-form>
    </a-modal>
  </div>
</template>

<script setup>
defineOptions({
  name: 'monitorLogStorage'
})

import { onMounted, reactive, ref, computed } from 'vue'
import { message, Empty } from 'ant-design-vue'
import {
  deleteOpenSearchCluster,
  getLogPipelineHealth,
  getOpenSearchClusterList,
  saveOpenSearchCluster,
  testOpenSearchCluster,
} from '@/api/monitor'
import { openDeleteConfirm } from '@/util/deleteConfirm'
import { formatTimeWithTimezone } from '@/util/timezone'
import store from '@/store'

// 与后端 PASSWORD_MASK 约定一致：提交该占位符表示不修改已保存的密码。
const PASSWORD_MASK = '******'

const clusters = ref([])
const loading = ref(false)
const saving = ref(false)
const testingId = ref(null)
const dialogOpen = ref(false)
const formRef = ref(null)

const emptyForm = () => ({
  id: null,
  name: '',
  hosts: '',
  username: '',
  password: '',
  password_configured: false,
  verify_tls: false,
  ca_cert: '',
  index_prefix: 'logs',
  request_timeout: 10,
  enabled: true,
  is_default: false,
  remark: '',
})
const form = reactive(emptyForm())

const rules = {
  name: [{ required: true, message: '请输入集群名称' }],
  hosts: [{ required: true, message: '请输入连接地址' }],
  index_prefix: [
    { required: true, message: '请输入索引前缀' },
    { pattern: /^[a-z0-9][a-z0-9_-]*$/, message: '仅支持小写字母、数字、下划线和连字符' },
  ],
}

const columns = [
  { title: '集群名称', key: 'name', width: 180, fixed: 'left' },
  { title: '连接地址', dataIndex: 'hosts', key: 'hosts', width: 280 },
  { title: '用户名', dataIndex: 'username', key: 'username', width: 120 },
  { title: '索引前缀', dataIndex: 'index_prefix', key: 'index_prefix', width: 110 },
  { title: 'TLS', key: 'verify_tls', width: 100 },
  { title: '状态', key: 'enabled', width: 90 },
  { title: '连接测试', key: 'check', width: 220 },
  { title: '配置同步', key: 'storage_sync', width: 180 },
  { title: '备注', dataIndex: 'remark', key: 'remark', width: 200 },
  { title: '操作', key: 'action', fixed: 'right', width: 150 },
]

const timezone = store.state.user?.timezone || 'Asia/Shanghai'
const formatTime = (value) => (value ? formatTimeWithTimezone(value, timezone) : '-')

// drift 与 error 的区别：前者是「期望与实际不一致，重新下发即可修复」，后者是「检查本身失败」。
const STATUS_COLOR = { ok: 'green', warn: 'orange', drift: 'volcano', error: 'red' }
const STATUS_TEXT = { ok: '一致', warn: '注意', drift: '待下发', error: '异常' }
const STORAGE_SYNC_COLOR = { pending: 'processing', success: 'green', failed: 'red', unknown: 'default' }
const STORAGE_SYNC_TEXT = { pending: '同步中', success: '已同步', failed: '同步失败', unknown: '未同步' }
const OVERALL_ALERT = { ok: 'success', warn: 'warning', drift: 'warning', error: 'error' }
const OVERALL_TEXT = {
  ok: '链路各层一致，日志采集正常',
  warn: '链路基本一致，有需要关注的项',
  drift: '存在未下发的配置，展开查看需要处理的项',
  error: '链路存在异常，展开查看失败原因',
}

const health = ref(null)
const healthLoading = ref(false)
const openLayers = ref([])
const simpleEmptyImage = Empty.PRESENTED_IMAGE_SIMPLE

const overallMessage = computed(() => OVERALL_TEXT[health.value?.status] || '检查完成')

async function loadHealth() {
  const cluster = clusters.value[0]
  if (!cluster) return
  healthLoading.value = true
  try {
    const response = await getLogPipelineHealth(cluster.id)
    health.value = response?.data?.data || null
    // 默认只展开有问题的层，全绿时保持收起，避免刷屏。
    openLayers.value = (health.value?.layers || [])
      .filter((layer) => layer.status !== 'ok')
      .map((layer) => layer.key)
  } catch (error) {
    message.error(error?.response?.data?.msg || error?.message || '体检失败')
  } finally {
    healthLoading.value = false
  }
}

async function loadClusters() {
  loading.value = true
  try {
    const response = await getOpenSearchClusterList({ page: 1, page_size: 100 })
    clusters.value = response?.data?.data?.results || []
  } finally {
    loading.value = false
  }
}

function openDialog(record = null) {
  Object.assign(form, emptyForm())
  if (record) {
    Object.assign(form, record)
    // 已保存密码时回填占位符，用户不改就不会覆盖原密码。
    form.password = record.password_configured ? PASSWORD_MASK : ''
  }
  dialogOpen.value = true
}

async function submit() {
  await formRef.value.validate()
  saving.value = true
  try {
    const payload = { ...form }
    delete payload.password_configured
    if (!payload.password) delete payload.password
    await saveOpenSearchCluster(payload)
    message.success('保存成功，索引模板与保留策略正在后台同步')
    dialogOpen.value = false
    await loadClusters()
  } catch (error) {
    message.error(error?.response?.data?.msg || error?.message || '保存失败')
  } finally {
    saving.value = false
  }
}

async function testConnection(record) {
  testingId.value = record.id
  try {
    const response = await testOpenSearchCluster(record.id)
    const info = response?.data?.data || {}
    message.success(`连接成功：${info.distribution || 'opensearch'} ${info.version}，状态 ${info.status}`)
  } catch (error) {
    message.error(error?.response?.data?.msg || error?.message || '连接失败')
  } finally {
    testingId.value = null
    await loadClusters()
  }
}

function confirmDelete(record) {
  openDeleteConfirm({
    title: '删除集群',
    summary: '删除后依赖该集群的日志采集与查询将不可用。',
    items: [`${record.name} (${record.hosts})`],
    onConfirm: async () => {
      await deleteOpenSearchCluster(record.id)
      message.success('删除成功')
      await loadClusters()
    },
  })
}

onMounted(loadClusters)
</script>

<style scoped>
.log-storage-page {
  padding: 16px;
}
.page-header h2 {
  margin: 0;
  font-size: 20px;
}
.page-header p {
  margin: 4px 0 16px;
  color: #7a8697;
  font-size: 13px;
}
.toolbar {
  display: flex;
  gap: 8px;
  margin-bottom: 12px;
}
.toolbar .ant-btn {
  display: inline-flex;
  align-items: center;
}
.check-time {
  margin-left: 6px;
  color: #7a8697;
  font-size: 12px;
}
.health-panel {
  margin-top: 16px;
}
.health-time {
  color: #7a8697;
  font-size: 12px;
}
.health-layers {
  margin-top: 12px;
}
.layer-name {
  font-weight: 600;
  margin-right: 12px;
}
.layer-summary {
  color: #7a8697;
  font-size: 13px;
}
.health-item {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 4px 0;
  font-size: 13px;
}
.item-name {
  min-width: 220px;
  font-family: 'JetBrains Mono', Consolas, monospace;
}
.item-detail {
  color: #7a8697;
}
</style>
