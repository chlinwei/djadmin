<template>
  <div class="alert-route-page">
    <a-card title="告警路由" size="small">
      <template #extra>
        <a-button size="large" @click="openCreateModal">
          <FontAwesomeIcon :icon="['fas', 'fa-plus-circle']" />
          <span>&nbsp;新增</span>
        </a-button>
      </template>

      <a-table
        rowKey="id"
        :columns="columns"
        :data-source="routeList"
        :loading="listLoading"
        :scroll="{ x: 1200 }"
        :pagination="{ showSizeChanger: true, showQuickJumper: true }"
        size="small"
      >
        <template #bodyCell="{ column, record }">
          <template v-if="column.key === 'enabled'">
            <a-tag :color="record.enabled ? 'green' : 'default'">{{ record.enabled ? '已启用' : '已停用' }}</a-tag>
          </template>
          <template v-else-if="column.key === 'matchers'">
            <a-space v-if="Object.keys(record.matchers || {}).length" wrap>
              <a-tag v-for="(value, key) in record.matchers" :key="key" color="blue">{{ key }}={{ value }}</a-tag>
            </a-space>
            <a-tag v-else>匹配全部告警</a-tag>
          </template>
          <template v-else-if="column.key === 'events'">
            <a-space>
              <a-tag v-if="record.notify_on_firing" color="red">告警</a-tag>
              <a-tag v-if="record.notify_on_resolved" color="green">恢复</a-tag>
            </a-space>
          </template>
          <template v-else-if="column.key === 'media'">
            <a-space wrap>
              <a-tag v-for="mediaId in record.media" :key="mediaId">{{ mediaNameMap[mediaId] || `媒介 #${mediaId}` }}</a-tag>
            </a-space>
          </template>
          <template v-else-if="column.key === 'operation'">
            <a-row :gutter="6" class="action-row">
              <a-col>
                <a-tooltip title="编辑" placement="top">
                  <a-button type="primary" @click="openEditModal(record)">编辑</a-button>
                </a-tooltip>
              </a-col>
              <a-col>
                <a-tooltip title="删除" placement="top">
                  <a-button
                    class="delBtn"
                    danger
                    type="primary"
                    :loading="rowDeleteLoading[record.id]"
                    @click="handleDelete(record)"
                  >删除</a-button>
                </a-tooltip>
              </a-col>
            </a-row>
          </template>
        </template>
      </a-table>
    </a-card>

    <a-modal
      v-model:open="modalVisible"
      :title="editingId ? '编辑告警路由' : '新增告警路由'"
      width="760px"
      :footer="null"
      @cancel="closeModal"
    >
      <a-form :model="routeForm" :label-col="{ span: 5 }" :wrapper-col="{ span: 17 }">
        <a-form-item label="名称" required>
          <a-input v-model:value="routeForm.name" placeholder="例如：生产环境严重告警" />
        </a-form-item>
        <a-form-item label="通知事件" required>
          <a-checkbox-group v-model:value="routeForm.eventTypes" :options="eventTypeOptions" />
        </a-form-item>
        <a-form-item label="标签条件">
          <div class="matcher-list">
            <div v-for="(matcher, index) in routeForm.matchers" :key="matcher.id" class="matcher-row">
              <a-input v-model:value="matcher.key" placeholder="标签名，例如 severity" />
              <span class="matcher-equals">=</span>
              <a-input v-model:value="matcher.value" placeholder="标签值，例如 critical" />
              <a-button danger @click="removeMatcher(index)">移除</a-button>
            </div>
            <a-button @click="addMatcher">
              <FontAwesomeIcon :icon="['fas', 'fa-plus-circle']" />
              <span>&nbsp;添加条件</span>
            </a-button>
            <div class="matcher-tip">不添加条件时匹配全部告警；多个条件必须同时满足。</div>
          </div>
        </a-form-item>
        <a-form-item label="通知媒介" required>
          <a-select
            v-model:value="routeForm.mediaIds"
            mode="multiple"
            :options="mediaOptions"
            :getPopupContainer="getPopupContainer"
            placeholder="选择告警媒介"
          />
        </a-form-item>
        <a-form-item label="描述">
          <a-textarea v-model:value="routeForm.remark" :rows="3" />
        </a-form-item>
        <a-form-item label="已启用">
          <a-checkbox v-model:checked="routeForm.enabled" />
        </a-form-item>
      </a-form>

      <div class="modal-actions">
        <a-button @click="closeModal">取消</a-button>
        <a-button type="primary" :loading="saveLoading" @click="saveRoute">保存</a-button>
      </div>
    </a-modal>
  </div>
</template>

<script setup>
import { computed, onMounted, reactive, ref } from 'vue'
import { message } from 'ant-design-vue'
import {
  createAlertRoute,
  deleteAlertRoute,
  getAlertMediaList,
  getAlertRouteList,
  updateAlertRoute,
} from '@/api/monitor'
import { openDeleteConfirm } from '@/util/deleteConfirm'
import { resolvePopupContainerByContext } from '@/util/popupContainer'

const getPopupContainer = (triggerNode) => resolvePopupContainerByContext(triggerNode)

const columns = [
  { title: '名称', dataIndex: 'name', key: 'name', width: 220 },
  { title: '状态', key: 'enabled', width: 110 },
  { title: '标签条件', key: 'matchers', width: 320 },
  { title: '通知事件', key: 'events', width: 160 },
  { title: '通知媒介', key: 'media', width: 240 },
  { title: '描述', dataIndex: 'remark', key: 'remark', width: 240 },
  { title: '操作', key: 'operation', fixed: 'right', width: 180 },
]

const eventTypeOptions = [
  { label: '告警触发', value: 'firing' },
  { label: '告警恢复', value: 'resolved' },
]

const routeList = ref([])
const mediaList = ref([])
const listLoading = ref(false)
const saveLoading = ref(false)
const modalVisible = ref(false)
const editingId = ref(null)
const rowDeleteLoading = reactive({})
let matcherSequence = 0
const routeForm = ref(createDefaultForm())

const mediaOptions = computed(() => mediaList.value
  .filter((item) => item.enabled && item.media_type === 'email')
  .map((item) => ({ label: item.name, value: item.id })))
const mediaNameMap = computed(() => Object.fromEntries(mediaList.value.map((item) => [item.id, item.name])))

function createDefaultForm() {
  return {
    name: '',
    enabled: true,
    eventTypes: ['firing', 'resolved'],
    matchers: [],
    mediaIds: [],
    remark: '',
  }
}

function parseApiData(response) {
  return response?.data?.data || {}
}

function addMatcher() {
  matcherSequence += 1
  routeForm.value.matchers.push({ id: matcherSequence, key: '', value: '' })
}

function removeMatcher(index) {
  routeForm.value.matchers.splice(index, 1)
}

function openCreateModal() {
  editingId.value = null
  routeForm.value = createDefaultForm()
  modalVisible.value = true
}

function openEditModal(record) {
  editingId.value = record.id
  routeForm.value = {
    name: record.name,
    enabled: record.enabled,
    eventTypes: [
      ...(record.notify_on_firing ? ['firing'] : []),
      ...(record.notify_on_resolved ? ['resolved'] : []),
    ],
    matchers: Object.entries(record.matchers || {}).map(([key, value]) => {
      matcherSequence += 1
      return { id: matcherSequence, key, value }
    }),
    mediaIds: [...(record.media || [])],
    remark: record.remark || '',
  }
  modalVisible.value = true
}

function closeModal() {
  modalVisible.value = false
}

async function saveRoute() {
  const name = routeForm.value.name.trim()
  if (!name) {
    message.warning('请填写路由名称')
    return
  }
  if (!routeForm.value.eventTypes.length) {
    message.warning('至少选择一种通知事件')
    return
  }
  if (!routeForm.value.mediaIds.length) {
    message.warning('至少选择一个通知媒介')
    return
  }

  const matchers = {}
  for (const matcher of routeForm.value.matchers) {
    const key = matcher.key.trim()
    const value = matcher.value.trim()
    if (!key || !value) {
      message.warning('标签名和值不能为空')
      return
    }
    if (Object.prototype.hasOwnProperty.call(matchers, key)) {
      message.warning(`标签 ${key} 不能重复`)
      return
    }
    matchers[key] = value
  }

  const payload = {
    name,
    enabled: routeForm.value.enabled,
    matchers,
    notify_on_firing: routeForm.value.eventTypes.includes('firing'),
    notify_on_resolved: routeForm.value.eventTypes.includes('resolved'),
    media: routeForm.value.mediaIds,
    remark: routeForm.value.remark.trim(),
  }

  saveLoading.value = true
  try {
    if (editingId.value) {
      await updateAlertRoute(editingId.value, payload)
    } else {
      await createAlertRoute(payload)
    }
    modalVisible.value = false
    message.success(editingId.value ? '告警路由已更新' : '告警路由已添加')
    await loadRoutes()
  } finally {
    saveLoading.value = false
  }
}

async function loadRoutes() {
  listLoading.value = true
  try {
    const response = await getAlertRouteList({ page: 1, page_size: 100 })
    const data = parseApiData(response)
    routeList.value = data.results || data || []
  } finally {
    listLoading.value = false
  }
}

async function loadMedia() {
  const response = await getAlertMediaList({ page: 1, page_size: 100 })
  const data = parseApiData(response)
  mediaList.value = data.results || data || []
}

async function handleDelete(record) {
  await openDeleteConfirm({
    title: '确认删除告警路由',
    summary: '删除后，符合该路由条件的告警将不再通过其媒介发送。',
    items: [record.name],
    onConfirm: async () => {
      rowDeleteLoading[record.id] = true
      try {
        await deleteAlertRoute(record.id)
        message.success('告警路由已删除')
        await loadRoutes()
      } finally {
        rowDeleteLoading[record.id] = false
      }
    },
  })
}

onMounted(async () => {
  try {
    await Promise.all([loadRoutes(), loadMedia()])
  } catch (error) {
    message.error(error?.response?.data?.msg || '告警路由数据加载失败')
  }
})
</script>

<style scoped>
.alert-route-page {
  padding: 8px;
}

.action-row {
  flex-wrap: nowrap;
}

.matcher-list {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.matcher-row {
  display: grid;
  grid-template-columns: minmax(160px, 1fr) 20px minmax(160px, 1fr) 64px;
  align-items: center;
  gap: 8px;
}

.matcher-equals {
  text-align: center;
  color: rgba(0, 0, 0, 0.65);
}

.matcher-tip {
  color: rgba(0, 0, 0, 0.45);
  font-size: 12px;
}

.modal-actions {
  display: flex;
  justify-content: flex-end;
  gap: 10px;
  margin-top: 22px;
  padding-top: 16px;
  border-top: 1px solid #f0f0f0;
}

@media (max-width: 640px) {
  .matcher-row {
    grid-template-columns: 1fr;
  }

  .matcher-equals {
    display: none;
  }
}
</style>
