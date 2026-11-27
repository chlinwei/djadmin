<template>
  <a-tooltip title="查看" placement="top">
    <a-button v-bind="$attrs" size="small" type="link" class="detail-action" aria-label="查看" @click="$emit('click', $event)">
      <FontAwesomeIcon :icon="['fas', 'eye']" /><span>查看</span>
    </a-button>
  </a-tooltip>
</template>
<script setup>
defineOptions({ inheritAttrs: false })
defineEmits(['click'])
</script>
