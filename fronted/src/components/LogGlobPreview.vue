<template>
  <!-- 日志路径通配的按需展开：含 * 的路径在承载实例上展开成真实文件清单。
       只在用户点「展开」时才去调 agent（逐台主机，慢且依赖在线），不进首屏加载。
       展开结果按实例分组，整条日志作为一格里的一块，不拆散到多行。 -->
  <span class="log-glob-preview">
    <a-button v-if="!loaded" type="link" size="small" :loading="loading" @click="load">
      展开文件
    </a-button>
    <template v-else>
      <a-button type="link" size="small" @click="reset">收起</a-button>
      <div v-if="error" class="glob-error">{{ error }}</div>
      <div v-else-if="!instances.length" class="glob-empty">该日志没有承载实例，无处展开</div>
      <div v-for="(instance, index) in instances" :key="index" class="glob-instance">
        <div class="glob-instance-title">{{ instanceLabel(instance) }}</div>
        <div v-if="instance.error" class="glob-error">{{ instance.error }}</div>
        <div v-else-if="!instance.matches?.length" class="glob-empty">未匹配到任何文件</div>
        <ul v-else class="glob-list">
          <li v-for="item in instance.matches" :key="item"><code>{{ item }}</code></li>
        </ul>
      </div>
    </template>
  </span>
</template>

<script setup>
import { ref } from 'vue'
import { message } from 'ant-design-vue'
import { previewApplicationServiceLogGlob } from '@/api/assets/application'

const props = defineProps({
  serviceId: { type: [Number, String], default: null },
  logDefinitionId: { type: [Number, String], default: null },
})

const loading = ref(false)
const loaded = ref(false)
const error = ref('')
const instances = ref([])

function instanceLabel(instance) {
  const name = instance.deployment_instance_name || instance.host_instance_name || '未知实例'
  const host = instance.host_instance_name && instance.host_instance_name !== name
    ? `（主机 ${instance.host_instance_name}）`
    : ''
  return name + host
}

async function load() {
  if (!props.serviceId || !props.logDefinitionId) return
  loading.value = true
  error.value = ''
  try {
    const response = await previewApplicationServiceLogGlob(props.serviceId, props.logDefinitionId)
    const data = response?.data?.data || {}
    instances.value = data.instances || []
    loaded.value = true
  } catch (err) {
    message.error(err?.response?.data?.msg || err?.message || '展开失败，请稍后重试')
  } finally {
    loading.value = false
  }
}

function reset() {
  loaded.value = false
  instances.value = []
  error.value = ''
}
</script>

<style scoped>
.log-glob-preview {
  display: inline-block;
}
.glob-instance {
  margin-top: 4px;
}
.glob-instance-title {
  color: rgba(0, 0, 0, 0.65);
  font-size: 12px;
}
.glob-list {
  margin: 2px 0 0;
  padding-left: 16px;
  font-size: 12px;
}
.glob-error {
  margin-top: 2px;
  color: #d4380d;
  font-size: 12px;
}
.glob-empty {
  margin-top: 2px;
  color: rgba(0, 0, 0, 0.45);
  font-size: 12px;
}
</style>
