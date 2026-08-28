<template>
  <div class="log-retention-page">
    <div class="page-title-row">
      <h2>日志保留档位</h2>
    </div>

    <div class="toolbar">
      <a-button size="large" @click="openCreate">
        <FontAwesomeIcon :icon="['fas', 'plus-circle']" />
        <span>&nbsp;新增档位</span>
      </a-button>
      <a-tooltip title="刷新">
        <a-button size="large" @click="loadTiers">
          <FontAwesomeIcon :icon="['fas', 'rotate']" />
          <span>&nbsp;刷新</span>
        </a-button>
      </a-tooltip>
    </div>

    <a-alert
      class="page-hint"
      type="info"
      show-icon
      message="档位即 data stream 名称后缀：logs-<环境>-<业务系统>-<档位编码>；保存后 ISM 策略会自动下发到启用的集群。"
    />

    <a-table
      row-key="id"
      :columns="columns"
      :data-source="tiers"
      :loading="loading"
      :pagination="false"
      :scroll="{ x: 1180 }"
      :locale="{ emptyText: '暂无保留档位' }"
    >
      <template #bodyCell="{ column, record }">
        <template v-if="column.key === 'code'">
          <span class="tier-code">{{ record.code }}</span>
          <a-tag v-if="record.is_default" color="blue" class="default-tag">默认</a-tag>
        </template>
        <template v-else-if="column.key === 'daily_size_gb'">
          <span>{{ record.daily_size_gb }} GB/天</span>
        </template>
        <template v-else-if="column.key === 'retention_days'">
          <span>{{ record.retention_days }} 天</span>
        </template>
        <template v-else-if="column.key === 'estimated_total_gb'">
          <a-tooltip title="每天写入量 × 保留天数，用于容量规划">
            <span>{{ record.estimated_total_gb }} GB</span>
          </a-tooltip>
        </template>
        <template v-else-if="column.key === 'rollover'">
          <span>{{ record.rollover_min_index_age }} / {{ record.rollover_min_primary_shard_size }}</span>
        </template>
        <template v-else-if="column.key === 'enabled'">
          <a-tag :color="record.enabled ? 'green' : 'default'">{{ record.enabled ? '启用' : '停用' }}</a-tag>
        </template>
        <template v-else-if="column.key === 'service_count'">
          <a-tag :color="record.service_count ? 'purple' : 'default'">{{ record.service_count }}</a-tag>
        </template>
        <template v-else-if="column.key === 'action'">
          <a-space :size="6">
            <a-tooltip title="编辑">
              <a-button type="primary" @click="openEdit(record)">
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

    <a-modal
      v-model:open="editorOpen"
      :title="form.id ? `编辑保留档位：${form.code}` : '新增保留档位'"
      :width="640"
      centered
      :confirm-loading="saving"
      ok-text="保存"
      cancel-text="取消"
      @ok="submit"
    >
      <a-form ref="formRef" :model="form" :rules="formRules" layout="vertical">
        <a-form-item name="code" label="档位编码">
          <a-input
            v-model:value="form.code"
            :disabled="Boolean(form.id)"
            placeholder="例如 hot / std / cold，创建后不可修改"
          />
        </a-form-item>
        <a-form-item name="name" label="档位名称">
          <a-input v-model:value="form.name" placeholder="例如 热（7 天）" />
        </a-form-item>
        <a-row :gutter="16">
          <a-col :span="12">
            <a-form-item name="daily_size_gb" label="每天写入量（GB）">
              <a-input-number
                v-model:value="form.daily_size_gb"
                :min="0.01"
                :step="0.1"
                style="width: 100%"
              />
            </a-form-item>
          </a-col>
          <a-col :span="12">
            <a-form-item name="retention_days" label="保留天数">
              <a-input-number
                v-model:value="form.retention_days"
                :min="1"
                :max="3650"
                style="width: 100%"
              />
            </a-form-item>
          </a-col>
        </a-row>
        <a-form-item name="rollover_min_index_age" label="滚动最小时长">
          <a-input v-model:value="form.rollover_min_index_age" placeholder="例如 1d / 12h / 30m" />
        </a-form-item>
        <a-row :gutter="16">
          <a-col :span="12">
            <a-form-item label="启用">
              <a-switch v-model:checked="form.enabled" />
            </a-form-item>
          </a-col>
          <a-col :span="12">
            <a-form-item label="设为默认档位">
              <a-switch v-model:checked="form.is_default" />
            </a-form-item>
          </a-col>
        </a-row>
        <a-form-item label="备注">
          <a-textarea v-model:value="form.remark" :rows="2" placeholder="适用场景说明" />
        </a-form-item>
        <a-alert type="info" show-icon :message="`预计占用 ${estimatedTotal} GB`" />
      </a-form>
    </a-modal>
  </div>
</template>

<script setup>
import { computed, onMounted, reactive, ref } from 'vue'
import { message } from 'ant-design-vue'
import {
  deleteLogRetentionTier,
  getLogRetentionTiers,
  saveLogRetentionTier,
} from '@/api/monitor'
import { openDeleteConfirm } from '@/util/deleteConfirm'

const tiers = ref([])
const loading = ref(false)
const saving = ref(false)
const editorOpen = ref(false)
const formRef = ref(null)

const form = reactive({
  id: null,
  code: '',
  name: '',
  daily_size_gb: 5,
  retention_days: 30,
  rollover_min_index_age: '1d',
  enabled: true,
  is_default: false,
  remark: '',
})

const columns = [
  { title: '档位编码', key: 'code', width: 160, fixed: 'left' },
  { title: '档位名称', key: 'name', dataIndex: 'name', width: 180 },
  { title: '每天写入量', key: 'daily_size_gb', width: 130, align: 'center' },
  { title: '保留天数', key: 'retention_days', width: 110, align: 'center' },
  { title: '预计占用', key: 'estimated_total_gb', width: 120, align: 'center' },
  { title: '滚动条件（时长/分片）', key: 'rollover', width: 180, align: 'center' },
  { title: '状态', key: 'enabled', width: 90, align: 'center' },
  { title: '引用服务', key: 'service_count', width: 100, align: 'center' },
  { title: '操作', key: 'action', width: 130, fixed: 'right' },
]

const formRules = {
  code: [
    { required: true, message: '请输入档位编码' },
    { pattern: /^[a-z][a-z0-9-]*$/, message: '只能以小写字母开头，使用小写字母、数字和连字符' },
  ],
  name: [{ required: true, message: '请输入档位名称' }],
  daily_size_gb: [{ required: true, message: '请输入每天写入量' }],
  retention_days: [{ required: true, message: '请输入保留天数' }],
  rollover_min_index_age: [
    { required: true, message: '请输入滚动最小时长' },
    { pattern: /^\d+[mhd]$/, message: '格式如 30m / 12h / 1d' },
  ],
}

const estimatedTotal = computed(() =>
  Math.round((Number(form.daily_size_gb) || 0) * (Number(form.retention_days) || 0) * 100) / 100,
)

async function loadTiers() {
  loading.value = true
  try {
    const response = await getLogRetentionTiers({ page_size: 100 })
    tiers.value = response?.data?.data?.results || []
  } catch (error) {
    tiers.value = []
    message.error(error?.response?.data?.msg || error?.message || '获取保留档位失败')
  } finally {
    loading.value = false
  }
}

function resetForm() {
  Object.assign(form, {
    id: null,
    code: '',
    name: '',
    daily_size_gb: 5,
    retention_days: 30,
    rollover_min_index_age: '1d',
    enabled: true,
    is_default: false,
    remark: '',
  })
}

function openCreate() {
  resetForm()
  editorOpen.value = true
}

function openEdit(record) {
  resetForm()
  Object.assign(form, {
    id: record.id,
    code: record.code,
    name: record.name,
    daily_size_gb: record.daily_size_gb,
    retention_days: record.retention_days,
    rollover_min_index_age: record.rollover_min_index_age,
    enabled: record.enabled,
    is_default: record.is_default,
    remark: record.remark || '',
  })
  editorOpen.value = true
}

async function submit() {
  await formRef.value?.validate()
  saving.value = true
  try {
    await saveLogRetentionTier({ ...form })
    message.success(form.id ? '保存成功，ISM 策略已下发' : '新增成功，ISM 策略已下发')
    editorOpen.value = false
    await loadTiers()
  } catch (error) {
    message.error(error?.response?.data?.msg || error?.message || '保存失败')
  } finally {
    saving.value = false
  }
}

function confirmDelete(record) {
  openDeleteConfirm({
    title: '删除保留档位',
    summary: '删除后该档位对应的 data stream 将不再有新日志写入，已存在的索引不受影响。',
    items: [`档位: ${record.name}（${record.code}）`],
    onConfirm: async () => {
      await deleteLogRetentionTier(record.id)
      message.success('删除成功')
      await loadTiers()
    },
  })
}

onMounted(loadTiers)
</script>

<style scoped>
.log-retention-page {
  padding: 16px;
}
.page-title-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  margin-bottom: 16px;
}
.page-title-row h2 {
  margin: 0;
  font-size: 20px;
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
.page-hint {
  margin-bottom: 12px;
}
.tier-code {
  font-family: "JetBrains Mono", "Cascadia Code", monospace;
  font-size: 13px;
}
.default-tag {
  margin-left: 8px;
}
</style>
