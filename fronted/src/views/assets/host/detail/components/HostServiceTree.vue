<template>
  <a-space direction="vertical" style="width: 100%" :size="12">
    <div>所属环境：{{ host.environment_name || (host.environment ? `环境 ${host.environment}` : '未设置') }}</div>
    <a-alert v-if="!host.environment" type="warning" show-icon message="该主机尚未设置所属环境" />
    <a-alert v-if="error" type="error" show-icon :message="error">
      <template #action><a-button size="small" @click="load">重试</a-button></template>
    </a-alert>
    <a-spin :spinning="loading">
      <template v-if="!error && !loading">
        <div>关联服务：{{ result.serviceCount }}　部署实例：{{ result.deploymentCount }}</div>
        <a-tree v-if="result.tree.length" :key="revision" :tree-data="result.tree" default-expand-all :virtual="false">
          <template #title="node">
            <a-tooltip v-if="node.route" title="在服务树中查看" placement="top">
              <a @click.stop="router.push(node.route)">{{ node.title }}</a>
            </a-tooltip>
            <span v-else>{{ node.title }}</span>
          </template>
        </a-tree>
        <a-empty v-if="!result.deploymentCount" description="该主机暂无关联的服务部署" />
        <a-card v-if="result.unlinked.length" title="未关联服务的部署" size="small">
          <div v-for="item in result.unlinked" :key="item.id">{{ item.instance_name || `部署 ${item.id}` }}</div>
        </a-card>
        <a-alert v-if="result.anomalies.length" type="warning" show-icon message="关联异常">
          <template #description>
            <div v-for="item in result.anomalies" :key="item.key">{{ item.title }}</div>
          </template>
        </a-alert>
      </template>
    </a-spin>
  </a-space>
</template>

<script setup>
import { ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { getApplicationDeploymentList, getApplicationServiceList, getBusinessSystemList } from '@/api/assets/application'
import { fetchAllPages } from '@/util/fetchAllPages'
import { buildHostServiceTree } from './hostServiceTree'

const props = defineProps({ host: { type: Object, required: true } })
const router = useRouter()
const loading = ref(false)
const error = ref('')
const result = ref({ tree: [], unlinked: [], anomalies: [], serviceCount: 0, deploymentCount: 0 })
const revision = ref(0)
let requestId = 0

async function load() {
  const token = ++requestId
  const host = props.host
  loading.value = true
  error.value = ''
  try {
    // 与完整服务树共用分页接口和鉴权；全部页面加载后才归集，避免漏掉跨页关联。
    const [deployments, services, systems] = await Promise.all([
      fetchAllPages(getApplicationDeploymentList),
      fetchAllPages(getApplicationServiceList),
      fetchAllPages(getBusinessSystemList),
    ])
    if (token !== requestId) return
    result.value = buildHostServiceTree(host, deployments, services, systems)
    revision.value++
  } catch (err) {
    if (token === requestId) error.value = err?.response?.data?.msg || err?.message || '加载服务树归属失败'
  } finally {
    if (token === requestId) loading.value = false
  }
}

// 主机运行指标会定时刷新，不随快照对象替换重复拉取整棵资产关系。
watch([
  () => props.host.id,
  () => props.host.environment,
  () => props.host.environment_name,
], load, { immediate: true })
</script>
