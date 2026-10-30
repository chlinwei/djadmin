<template>
  <a-row :gutter="16">
    <a-col :span="12">
      <a-form-item label="交互类型" :name="[prefix, 'interaction_type_id']" :rules="[{ required: true, message: '请选择交互类型' }]">
        <a-select v-model:value="model.interaction_type_id" :options="typeOptions" :getPopupContainer="getPopupContainer" placeholder="请选择交互类型" />
      </a-form-item>
    </a-col>
    <a-col :span="12">
      <a-form-item label="依赖强度" :name="[prefix, 'strength']">
        <a-select v-model:value="model.strength" :options="strengthOptions" :getPopupContainer="getPopupContainer" />
      </a-form-item>
    </a-col>
  </a-row>
  <a-form-item label="功能范围" :name="[prefix, 'function_scope']" :rules="[{ required: true, whitespace: true, message: '请说明哪个功能需要对方' }]">
    <a-input v-model:value="model.function_scope" :maxlength="120" placeholder="例如：订单创建、登录态校验、商品缓存" />
  </a-form-item>
  <a-collapse ghost>
    <a-collapse-panel key="more" header="更多说明（选填）">
      <a-form-item label="协议 / 通道标识"><a-input v-model:value="model.protocol" :maxlength="128" placeholder="例如 HTTPS、gRPC、MySQL、订单消息队列" /></a-form-item>
      <a-form-item label="故障影响"><a-textarea v-model:value="model.failure_impact" :maxlength="500" :rows="2" placeholder="对方不可用时，哪些功能受影响？" /></a-form-item>
      <a-form-item label="故障处理说明"><a-textarea v-model:value="model.fallback_plan" :maxlength="500" :rows="2" placeholder="例如缓存失败后查数据库。仅记录已有行为，不自动执行。" /></a-form-item>
    </a-collapse-panel>
  </a-collapse>
</template>

<script setup>
import { resolvePopupContainerByContext } from '@/util/popupContainer'
import { strengthOptions } from './dependencyGraph'

defineProps({
  model: { type: Object, required: true },
  prefix: { type: String, required: true },
  typeOptions: { type: Array, required: true },
})
const getPopupContainer = (triggerNode) => resolvePopupContainerByContext(triggerNode)
</script>
