<template>
  <div><a-select style="width:100%" mode="multiple" :value="modelValue" allow-clear show-search :filter-option="false"
    :options="options" :disabled="disabled" :loading="loading" :getPopupContainer="getPopupContainer"
    placeholder="输入名称查找，可多选" @focus="load('')" @search="load"
    @update:value="$emit('update:modelValue', $event)" />
  <small v-if="error" role="alert">{{ error }}</small>
  <small v-if="nameError" role="alert">{{ nameError }}</small></div>
</template>

<script setup>
import { computed, onMounted, reactive, ref } from 'vue'
import { getHostById, getHostList } from '@/api/assets/host'
import { resolvePopupContainerByContext } from '@/util/popupContainer'

const props = defineProps({ modelValue: { type: Array, required: true }, disabled: Boolean, scopeNames: Object })
defineEmits(['update:modelValue'])
const getPopupContainer = (triggerNode) => resolvePopupContainerByContext(triggerNode)
const results = ref([])
const selectedLabels = reactive(new Map())
const nameError = ref('')
function hostLabel(row) {
  const name = row.name || row.instance_name
  return name && row.ip ? `${name}(${row.ip})` : name || row.ip || '未命名主机'
}
const options = computed(() => [...new Set([...props.modelValue, ...results.value.map(row => row.value)])]
  .map(value => ({ value, label: selectedLabels.get(value) || props.scopeNames?.[`host:${value}`] || '正在读取主机名称…' })))
// 编辑和告警预填只携带 ID；所选主机不一定出现在搜索的第一页。
onMounted(async () => {
  if (props.disabled) return
  await Promise.all(props.modelValue.filter(id => !props.scopeNames?.[`host:${id}`]).map(async id => {
    try { selectedLabels.set(id, hostLabel((await getHostById(id)).data.data)) }
    catch (e) {
      selectedLabels.set(id, '名称不可用')
      nameError.value = e.message || '无法读取所选主机名称'
    }
  }))
})
const loading = ref(false)
const error = ref('')
let sequence = 0
async function load(search) {
  if (props.disabled) return
  const current = ++sequence
  loading.value = true
  error.value = ''
  try {
    const res = await getHostList({ page: 1, page_size: 50, search })
    if (current !== sequence) return
    results.value = res.data.data.results.map(row => ({
      value: row.id, label: hostLabel(row),
    }))
    for (const row of results.value) selectedLabels.set(row.value, row.label)
  } catch (err) {
    if (current === sequence) {
      results.value = []
      error.value = err.message || '资产选项加载失败'
    }
  } finally {
    if (current === sequence) loading.value = false
  }
}
</script>
