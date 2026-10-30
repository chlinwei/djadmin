<template>
    <a-layout-footer style="text-align: center">
          {{ systemTitle }}
    </a-layout-footer>
</template>
<script setup>
import { ref } from 'vue'
import { getConfigByKey, CONFIG_KEYS } from '@/api/sys/sysconfig'
import { checkPermission } from '@/directives/permission/permission'

const systemTitle = ref('Djadmin')

// 页脚不依赖系统配置权限；无读取权限时直接使用产品默认标题。
if (checkPermission('sys:config:list')) getConfigByKey(CONFIG_KEYS.SYSTEM_TITLE).then(res => {
  const v = res.data?.data?.value
  if (v) systemTitle.value = v
}).catch(() => {})
</script>
<style scoped>
</style>