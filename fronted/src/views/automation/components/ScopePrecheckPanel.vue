<template>
  <div v-if="showLimitToggle && limitTokens.length > 0" class="scope-limit-token-box">
    <a-tag
      v-for="token in limitTokens"
      :key="token"
      closable
      class="scope-limit-token"
      @close.prevent="onRemoveLimitToken(token)"
    >
      {{ token }}
    </a-tag>
  </div>

  <a-alert
    :type="precheckOk ? 'success' : 'warning'"
    show-icon
    :message="message"
    class="scope-precheck-alert"
  />

  <div class="scope-precheck-head">
    <span>匹配预览</span>
    <a-space size="small">
      <a-checkbox v-if="showTargetFilter" v-model:checked="targetOnly">仅看目标主机</a-checkbox>
      <a-spin size="small" :spinning="prechecking" />
    </a-space>
  </div>

  <a-input
    v-model:value="hostSearchKeyword"
    allow-clear
    size="small"
    class="scope-precheck-search"
    placeholder="搜索主机名 / IP / 分组路径 / 主机ID"
  />

  <div class="scope-precheck-body">
    <a-empty v-if="displayHosts.length === 0 && !prechecking" :description="emptyText" />
    <ul v-else class="scope-precheck-list">
      <li
        v-for="item in displayHosts"
        :key="`${item.host_id}-${item.host_ip}`"
        :class="{ 'is-target': isHostMatched(item), 'is-not-target': !isHostMatched(item) }"
      >
        <span class="scope-host-main">
          <a-button
            v-if="showHostLink && Number(item.host_id) > 0"
            type="link"
            size="small"
            class="scope-host-link-btn"
            @click.stop="$emit('hostClick', item)"
          >
            {{ formatMatchedHostTitle(item) }}
          </a-button>
          <span v-else>{{ formatMatchedHostTitle(item) }}</span>
          <span class="scope-host-group"> [{{ item.group_path || item.group_name || '-' }}]</span>
          <a-tag v-if="item.agent_online === false" color="default" class="scope-agent-offline-tag">Agent离线</a-tag>
        </span>
        <a-button
          v-if="showLimitToggle"
          size="small"
          type="link"
          class="scope-limit-toggle-btn"
          @click.stop="onToggleLimit(item)"
        >
          {{ isTokenSelected(item) ? '移出Limit' : '加入Limit' }}
        </a-button>
      </li>
    </ul>
  </div>
</template>

<script setup>
import { computed, ref } from 'vue'
import { formatMatchedHostTitle, hasLimitToken, resolveMatchedHostLimitToken, splitLimitTokens } from '../utils/scopeHelpers'

const props = defineProps({
  precheckOk: {
    type: Boolean,
    default: false,
  },
  prechecking: {
    type: Boolean,
    default: false,
  },
  message: {
    type: String,
    default: '',
  },
  hosts: {
    type: Array,
    default: () => [],
  },
  matchedHosts: {
    type: Array,
    default: () => [],
  },
  emptyText: {
    type: String,
    default: '暂无匹配主机',
  },
  showHostLink: {
    type: Boolean,
    default: false,
  },
  showLimitToggle: {
    type: Boolean,
    default: false,
  },
  limitText: {
    type: String,
    default: '',
  },
  showTargetFilter: {
    type: Boolean,
    default: false,
  },
})

const emit = defineEmits(['hostClick', 'toggleLimitHost', 'removeLimitToken'])

const limitTokens = computed(() => {
  const seen = new Set()
  const result = []
  splitLimitTokens(props.limitText).forEach((token) => {
    const key = String(token || '').toLowerCase()
    if (!key || seen.has(key)) {
      return
    }
    seen.add(key)
    result.push(token)
  })
  return result
})

const selectedTokenSet = computed(() => {
  const map = new Set()
  const normalizedHosts = Array.isArray(props.hosts) && props.hosts.length > 0
    ? props.hosts
    : (Array.isArray(props.matchedHosts) ? props.matchedHosts : [])
  normalizedHosts.forEach((item) => {
    const token = resolveMatchedHostLimitToken(item)
    if (token && hasLimitToken(props.limitText, token)) {
      map.add(token)
    }
  })
  return map
})

function isTokenSelected(item) {
  const token = resolveMatchedHostLimitToken(item)
  if (!token) {
    return false
  }
  return selectedTokenSet.value.has(token)
}

function onToggleLimit(item) {
  emit('toggleLimitHost', item)
}

function onRemoveLimitToken(token) {
  emit('removeLimitToken', token)
}

const targetOnly = ref(false)
const hostSearchKeyword = ref('')

const matchedHostKeySet = computed(() => {
  const set = new Set()
  ;(Array.isArray(props.matchedHosts) ? props.matchedHosts : []).forEach((item) => {
    const hostId = Number(item?.host_id || 0)
    const hostIp = String(item?.host_ip || '').trim()
    if (hostId > 0) {
      set.add(`id:${hostId}`)
    }
    if (hostIp) {
      set.add(`ip:${hostIp}`)
    }
  })
  return set
})

const baseHosts = computed(() => {
  const allHosts = Array.isArray(props.hosts) ? props.hosts : []
  if (allHosts.length > 0) {
    return allHosts
  }
  return Array.isArray(props.matchedHosts) ? props.matchedHosts : []
})

function isHostMatched(item) {
  const set = matchedHostKeySet.value
  if (set.size === 0) {
    return true
  }

  const hostId = Number(item?.host_id || 0)
  if (hostId > 0 && set.has(`id:${hostId}`)) {
    return true
  }

  const hostIp = String(item?.host_ip || '').trim()
  if (hostIp && set.has(`ip:${hostIp}`)) {
    return true
  }

  return false
}

const displayHosts = computed(() => {
  let normalized = baseHosts.value
  if (targetOnly.value) {
    normalized = normalized.filter((item) => isHostMatched(item))
  }

  const keyword = String(hostSearchKeyword.value || '').trim().toLowerCase()
  if (!keyword) {
    return normalized
  }

  return normalized.filter((item) => {
    const hostName = String(item?.host_name || '').toLowerCase()
    const hostIp = String(item?.host_ip || '').toLowerCase()
    const groupPath = String(item?.group_path || item?.group_name || '').toLowerCase()
    const hostId = String(item?.host_id || '').toLowerCase()
    return hostName.includes(keyword)
      || hostIp.includes(keyword)
      || groupPath.includes(keyword)
      || hostId.includes(keyword)
  })
})
</script>

<style scoped>
.scope-precheck-alert {
  margin-top: 8px;
}

.scope-limit-token-box {
  margin-top: 8px;
  padding: 6px 8px;
  border: 1px solid #d9d9d9;
  border-radius: 6px;
  background: #fafafa;
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}

.scope-precheck-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-top: 10px;
  color: #595959;
  font-size: 12px;
}

.scope-precheck-search {
  margin-top: 6px;
}

.scope-limit-token {
  margin-inline-end: 0;
}

.scope-precheck-body {
  margin-top: 6px;
  max-height: 170px;
  overflow-y: auto;
  border: 1px solid #f0f0f0;
  border-radius: 4px;
  padding: 8px;
  background: #fafafa;
}

.scope-precheck-list {
  margin: 0;
  padding-left: 16px;
}

.scope-precheck-list li {
  line-height: 1.8;
  display: flex;
  justify-content: space-between;
  gap: 8px;
  align-items: center;
}

.scope-host-main {
  display: inline-flex;
  align-items: center;
  flex: 1;
  min-width: 0;
  gap: 2px;
  flex-wrap: nowrap;
  overflow: hidden;
}

.scope-agent-offline-tag {
  flex-shrink: 0;
  font-size: 11px;
  margin-inline-end: 0;
  line-height: 18px;
  padding: 0 4px;
}

.scope-limit-toggle-btn {
  flex-shrink: 0;
}

.scope-precheck-list li.is-target {
  background: #f6ffed;
}

.scope-precheck-list li.is-not-target {
  background: #fff7e6;
}
</style>
