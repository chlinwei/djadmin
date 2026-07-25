<template>
  <div class="alert-rule-groups-page">
    <a-card title="规则组管理" size="small">
      <a-alert
        type="info"
        show-icon
        message="维护 Prometheus groups（名称、interval、启停、排序）。告警规则页会引用这些规则组。"
        style="margin-bottom: 12px"
      />

      <a-space style="margin-bottom: 12px">
        <a-tooltip title="新增">
          <a-button type="primary" @click="openCreateModal">新增规则组</a-button>
        </a-tooltip>
        <a-tooltip title="刷新">
          <a-button type="primary" ghost :loading="loading" @click="loadGroups">刷新</a-button>
        </a-tooltip>
      </a-space>

      <a-table
        rowKey="id"
        :columns="columns"
        :data-source="groupList"
        :loading="loading"
        size="small"
        :scroll="{ x: 900 }"
        :pagination="pagination"
        @change="handleTableChange"
      >
        <template #bodyCell="{ column, record }">
          <template v-if="column.key === 'enabled'">
            <a-tag :color="record.enabled ? 'green' : 'default'">{{ record.enabled ? '启用' : '禁用' }}</a-tag>
          </template>
          <template v-else-if="column.key === 'action'">
            <a-space>
              <a-tooltip title="编辑">
                <a-button type="primary" ghost size="small" @click="openEditModal(record)">编辑</a-button>
              </a-tooltip>
              <a-tooltip title="删除">
                <a-button class="delBtn" danger type="primary" size="small" :loading="deleteLoading[record.id]" @click="openDeleteGroupConfirm(record)">删除</a-button>
              </a-tooltip>
            </a-space>
          </template>
        </template>
      </a-table>
    </a-card>

    <a-modal
      :title="isEdit ? '编辑规则组' : '新增规则组'"
      :open="modalVisible"
      :confirm-loading="submitting"
      ok-text="保存"
      cancel-text="取消"
      @ok="submitForm"
      @cancel="modalVisible = false"
    >
      <a-form layout="vertical">
        <a-form-item label="组名" required>
          <a-input v-model:value="formModel.name" placeholder="例如 host-baseline" />
        </a-form-item>
        <a-form-item label="interval" required>
          <a-input v-model:value="formModel.interval" placeholder="例如 30s / 1m" />
        </a-form-item>
        <a-form-item label="排序">
          <a-input-number v-model:value="formModel.order_num" :min="1" :precision="0" style="width: 100%" />
        </a-form-item>
        <a-form-item label="是否启用">
          <a-switch v-model:checked="formModel.enabled" checked-children="启用" un-checked-children="禁用" />
        </a-form-item>
      </a-form>
    </a-modal>
  </div>
</template>

<script setup>
import { onMounted, reactive, ref } from 'vue'
import { message } from 'ant-design-vue'
import {
  createAlertRuleGroup,
  deleteAlertRuleGroup,
  getAlertRuleGroups,
  updateAlertRuleGroup,
} from '@/api/sys/monitor'
import { openDeleteConfirm } from '@/util/deleteConfirm'

const loading = ref(false)
const submitting = ref(false)
const modalVisible = ref(false)
const isEdit = ref(false)
const currentId = ref(null)
const groupList = ref([])
const deleteLoading = reactive({})

const pagination = reactive({
  current: 1,
  pageSize: 10,
  total: 0,
  showSizeChanger: true,
  showQuickJumper: true,
  showTotal: (total) => `共有 ${total} 条数据`,
})

const columns = [
  { title: 'ID', dataIndex: 'id', key: 'id', width: 90 },
  { title: '组名', dataIndex: 'name', key: 'name', width: 220 },
  { title: 'Interval', dataIndex: 'interval', key: 'interval', width: 130 },
  { title: '启用', dataIndex: 'enabled', key: 'enabled', width: 100 },
  { title: '排序', dataIndex: 'order_num', key: 'order_num', width: 100 },
  { title: '更新时间', dataIndex: 'update_time', key: 'update_time', width: 180 },
  { title: '操作', key: 'action', width: 180, fixed: 'right' },
]

const formModel = reactive({
  name: '',
  interval: '30s',
  enabled: true,
  order_num: 100,
})

function parseApiData(res) {
  const payload = res?.data || res
  if (payload && typeof payload === 'object' && payload.data !== undefined) {
    return payload.data
  }
  return payload || {}
}

function resetForm() {
  formModel.name = ''
  formModel.interval = '30s'
  formModel.enabled = true
  formModel.order_num = 100
}

function openCreateModal() {
  isEdit.value = false
  currentId.value = null
  resetForm()
  modalVisible.value = true
}

function openEditModal(record) {
  isEdit.value = true
  currentId.value = record.id
  formModel.name = record.name || ''
  formModel.interval = record.interval || '30s'
  formModel.enabled = Boolean(record.enabled)
  formModel.order_num = Number(record.order_num || 100)
  modalVisible.value = true
}

function buildPayload() {
  return {
    name: String(formModel.name || '').trim(),
    interval: String(formModel.interval || '').trim(),
    enabled: Boolean(formModel.enabled),
    order_num: Number(formModel.order_num || 100),
  }
}

async function loadGroups() {
  loading.value = true
  try {
    const res = await getAlertRuleGroups({
      page: pagination.current,
      page_size: pagination.pageSize,
      ordering: 'order_num',
    })
    const data = parseApiData(res)
    groupList.value = Array.isArray(data.results) ? data.results : []
    pagination.total = Number(data.count || 0)
  } catch (error) {
    message.warning(error?.response?.data?.msg || error?.message || '加载规则组失败')
  } finally {
    loading.value = false
  }
}

async function submitForm() {
  const payload = buildPayload()
  submitting.value = true
  try {
    if (isEdit.value && currentId.value) {
      await updateAlertRuleGroup(currentId.value, payload)
      message.success('规则组更新成功')
    } else {
      await createAlertRuleGroup(payload)
      message.success('规则组创建成功')
    }
    modalVisible.value = false
    await loadGroups()
  } catch (error) {
    message.warning(error?.response?.data?.msg || error?.message || '保存规则组失败')
  } finally {
    submitting.value = false
  }
}

function openDeleteGroupConfirm(record) {
  openDeleteConfirm({
    title: '删除规则组',
    contentTitle: '确认删除以下规则组吗？',
    items: [{ name: record.name || `ID:${record.id}` }],
    async onConfirm() {
      deleteLoading[record.id] = true
      try {
        await deleteAlertRuleGroup(record.id)
        message.success('删除成功')
        await loadGroups()
      } catch (error) {
        message.warning(error?.response?.data?.msg || error?.message || '删除规则组失败')
      } finally {
        deleteLoading[record.id] = false
      }
    },
  })
}

function handleTableChange(nextPagination) {
  pagination.current = Number(nextPagination?.current || 1)
  pagination.pageSize = Number(nextPagination?.pageSize || 10)
  loadGroups()
}

onMounted(() => {
  loadGroups()
})
</script>

<style scoped>
.alert-rule-groups-page {
  padding: 12px;
}
</style>
