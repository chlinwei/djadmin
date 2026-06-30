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
        <a-space>
          <a-radio-group v-model:value="valueFilter" button-style="solid" size="small">
            <a-radio-button value="all">全部</a-radio-button>
            <a-radio-button value="changed">仅已修改</a-radio-button>
          </a-radio-group>
          <a-button type="primary" ghost class="refresh-btn" @click="loadConfigs" :disabled="loading">
            <FontAwesomeIcon :icon="['fas', 'arrows-rotate']" :spin="loading" />
            <span>&nbsp;刷新</span>
          </a-button>
        </a-space>
      </a-col>
    </a-row>

    <a-card size="small">
      <a-table
        :columns="columns"
        :data-source="filteredConfigs"
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
            <a-space>
              <a-button
                size="small"
                type="primary"
                :disabled="record.is_readonly"
                @click="openEdit(record)"
              >
                编辑
              </a-button>
              <a-popconfirm
                title="确认重置为默认值吗？"
                ok-text="确认"
                cancel-text="取消"
                @confirm="handleResetDefault(record)"
              >
                <a-button size="small" :disabled="record.is_readonly">重置默认</a-button>
              </a-popconfirm>
            </a-space>
          </template>
        </template>
      </a-table>
    </a-card>

    <!-- 编辑弹窗 -->
    <a-modal
      v-model:open="editVisible"
      :width="520"
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
        <a-form-item label="默认值">
          <a-input v-model:value="editForm.default_value" placeholder="留空表示不设置默认值" />
        </a-form-item>
      </a-form>
    </a-modal>
  </div>
</template>

<script setup>
import { ref, reactive, computed } from 'vue'
import { message } from 'ant-design-vue'
import { getConfigList, updateConfig, resetConfigDefault } from '@/api/sys/sysconfig'
import { getCurrentUserInfo } from '@/api/sys/userTimezone'
import { formatTimeWithTimezone } from '@/util/timezone'

const filterText = ref('')
const loading = ref(false)
const configs = ref([])
const valueFilter = ref('all')
const editVisible = ref(false)
const saveLoading = ref(false)
const userTimezone = ref(Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC')
const editForm = reactive({ id: null, name: '', key: '', value: '', default_value: '', description: '' })

const toComparableValue = (value) => {
  if (value === null || value === undefined) {
    return ''
  }
  return String(value)
}

const isChangedFromDefault = (item) => {
  return toComparableValue(item.value) !== toComparableValue(item.default_value)
}

const filteredConfigs = computed(() => {
  if (valueFilter.value === 'changed') {
    return configs.value.filter(isChangedFromDefault)
  }
  return configs.value
})

const columns = [
  { title: '参数名称', dataIndex: 'name', key: 'name', width: 180 },
  { title: '参数键', dataIndex: 'key', key: 'key', width: 220 },
  { title: '当前值', dataIndex: 'value', key: 'value' },
  { title: '默认值', dataIndex: 'default_value', key: 'default_value' },
  { title: '类型', dataIndex: 'value_type', key: 'value_type', width: 100 },
  { title: '属性', dataIndex: 'is_readonly', key: 'is_readonly', width: 100 },
  { title: '修改时间', dataIndex: 'update_time', key: 'update_time', width: 180 },
  { title: '说明', dataIndex: 'description', key: 'description' },
  { title: '操作', key: 'action', width: 180 },
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
      const data = res.data.data
      const list = Array.isArray(data) ? data : (data?.results || [])
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
  editForm.default_value = record.default_value ?? ''
  editForm.description = record.description || ''
  editVisible.value = true
}

const saveEdit = () => {
  saveLoading.value = true
  updateConfig(editForm.id, {
    value: editForm.value,
    default_value: editForm.default_value === '' ? null : editForm.default_value,
  })
    .then(() => {
      message.success('保存成功')
      editVisible.value = false
      loadConfigs()
    })
    .catch((error) => {
      const msg = error?.response?.data?.error || '保存失败'
      message.error(msg)
    })
    .finally(() => {
      saveLoading.value = false
    })
}

const handleResetDefault = (record) => {
  resetConfigDefault(record.id)
    .then(() => {
      message.success('已重置为默认值')
      loadConfigs()
    })
    .catch((error) => {
      const msg = error?.response?.data?.error || '重置失败'
      message.error(msg)
    })
}

loadUserTimezone()
loadConfigs()
</script>

<style scoped>
.sysconfig-page {
  padding: 16px;
  position: relative;
}
.tools {
  margin-bottom: 16px;
}
.right-actions {
  text-align: right;
}
</style>

