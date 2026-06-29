<template>
  <div class="sysconfig-page">
    <a-row class="tools" :gutter="16">
      <a-col :span="16">
        <a-input-search
          v-model:value="filterText"
          placeholder="搜索参数名称 / 参数键"
          allow-clear
          enter-button
          @search="loadConfigs"
        />
      </a-col>
      <a-col :span="8" class="right-actions">
        <a-button type="primary" @click="loadConfigs" :loading="loading">刷新</a-button>
      </a-col>
    </a-row>

    <a-card size="small">
      <a-table
        :columns="columns"
        :data-source="configs"
        :loading="loading"
        rowKey="id"
        :pagination="false"
      >
        <template #bodyCell="{ column, record }">
          <template v-if="column.key === 'value_type'">
            <a-tag>{{ valueTypeLabel(record.value_type) }}</a-tag>
          </template>
          <template v-else-if="column.key === 'is_readonly'">
            <a-tag :color="record.is_readonly ? 'orange' : 'green'">
              {{ record.is_readonly ? '只读' : '可编辑' }}
            </a-tag>
          </template>
          <template v-else-if="column.key === 'update_time'">
            {{ formatDateTime(record.update_time) }}
          </template>
          <template v-else-if="column.key === 'action'">
            <a-button
              size="small"
              type="primary"
              :disabled="record.is_readonly"
              @click="openEdit(record)"
            >
              编辑
            </a-button>
          </template>
        </template>
      </a-table>
    </a-card>

    <!-- 编辑弹窗 -->
    <a-modal
      v-model:open="editVisible"
      title="修改参数"
      @ok="saveEdit"
      :confirm-loading="saveLoading"
      ok-text="保存"
      cancel-text="取消"
    >
      <a-form layout="vertical" style="margin-top: 16px">
        <a-form-item label="参数名称">
          <a-input :value="editForm.name" disabled />
        </a-form-item>
        <a-form-item label="参数键">
          <a-input :value="editForm.key" disabled />
        </a-form-item>
        <a-form-item label="说明">
          <a-input :value="editForm.description" disabled />
        </a-form-item>
        <a-form-item label="参数值" required>
          <a-input v-model:value="editForm.value" />
        </a-form-item>
      </a-form>
    </a-modal>
  </div>
</template>

<script setup>
import { ref, reactive } from 'vue'
import { message } from 'ant-design-vue'
import { getConfigList, updateConfig } from '@/api/sys/sysconfig'
import { getCurrentUserInfo } from '@/api/sys/userTimezone'
import { formatTimeWithTimezone } from '@/util/timezone'

const filterText = ref('')
const loading = ref(false)
const configs = ref([])
const editVisible = ref(false)
const saveLoading = ref(false)
const userTimezone = ref(Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC')
const editForm = reactive({ id: null, name: '', key: '', value: '', description: '' })

const columns = [
  { title: '参数名称', dataIndex: 'name', key: 'name', width: 180 },
  { title: '参数键', dataIndex: 'key', key: 'key', width: 220 },
  { title: '当前值', dataIndex: 'value', key: 'value' },
  { title: '类型', dataIndex: 'value_type', key: 'value_type', width: 100 },
  { title: '属性', dataIndex: 'is_readonly', key: 'is_readonly', width: 100 },
  { title: '修改时间', dataIndex: 'update_time', key: 'update_time', width: 180 },
  { title: '说明', dataIndex: 'description', key: 'description' },
  { title: '操作', key: 'action', width: 100 },
]

const valueTypeLabel = (type) => {
  const map = { string: '字符串', int: '整数', bool: '布尔值', json: 'JSON' }
  return map[type] || type
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
  if (!value) return '-'
  try {
    return formatTimeWithTimezone(normalizeUtcTime(value), userTimezone.value, 'YYYY-MM-DD HH:mm:ss')
  } catch (error) {
    return value
  }
}

const loadUserTimezone = () => {
  getCurrentUserInfo()
    .then((res) => {
      const timezone = res?.data?.data?.timezone
      if (timezone) {
        userTimezone.value = timezone
      }
    })
    .catch(() => {})
}

const loadConfigs = () => {
  loading.value = true
  const params = {}
  if (filterText.value) params.search = filterText.value
  getConfigList(params)
    .then((res) => {
      const list = res.data.data || res.data || []
      configs.value = [...list].sort((a, b) => String(a.key || '').localeCompare(String(b.key || '')))
    })
    .catch(() => {
      message.error('加载失败')
    })
    .finally(() => {
      loading.value = false
    })
}

const openEdit = (record) => {
  editForm.id = record.id
  editForm.name = record.name
  editForm.key = record.key
  editForm.value = record.value
  editForm.description = record.description || ''
  editVisible.value = true
}

const saveEdit = () => {
  saveLoading.value = true
  updateConfig(editForm.id, editForm.value)
    .then(() => {
      message.success('保存成功')
      editVisible.value = false
      loadConfigs()
    })
    .catch(() => {
      message.error('保存失败')
    })
    .finally(() => {
      saveLoading.value = false
    })
}

loadUserTimezone()
loadConfigs()
</script>

<style scoped>
.sysconfig-page {
  padding: 16px;
}
.tools {
  margin-bottom: 16px;
}
.right-actions {
  text-align: right;
}
</style>

