<template>
  <a-tooltip v-if="canView && editable && canUpdate" title="编辑" placement="top">
    <a-button size="small" type="primary" @click="emit('open', false)">
      <FontAwesomeIcon :icon="['fas', 'pen-to-square']" />
    </a-button>
  </a-tooltip>
  <ViewRecordButton v-else-if="canView" :disabled="false" @click="emit('open', true)" />
</template>

<script setup>
import ViewRecordButton from '@/components/ViewRecordButton.vue'
import { canView, canUpdate } from './assetPermissions'

defineProps({ editable: { type: Boolean, default: true } })
const emit = defineEmits(['open'])
</script>
