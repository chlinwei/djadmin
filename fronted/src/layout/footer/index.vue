<template>
    <a-layout-footer style="text-align: center">
          {{ systemTitle }}
    </a-layout-footer>
</template>
<script setup>
import { ref } from 'vue'
import { getConfigByKey, CONFIG_KEYS } from '@/api/sys/sysconfig'

const systemTitle = ref('Djadmin')

getConfigByKey(CONFIG_KEYS.SYSTEM_TITLE).then(res => {
  const v = res.data?.data?.value
  if (v) systemTitle.value = v
}).catch(() => {})
</script>
<style scoped>
</style>