<template>
  <a-modal
    :title="title"
    :open="open"
    :width="760"
    :footer="null"
    @cancel="emit('close')"
  >
    <div class="group-scope-viewer">
      <div class="group-scope-editor__desc">按实例名 / IP / 主机组路径搜索</div>
      <div class="group-scope-viewer__summary">
        共 {{ normalizedTotal }} 台，当前展示 {{ visibleHostCount }} 台
      </div>
      <a-input
        v-model:value="keyword"
        allow-clear
        placeholder="搜索实例名 / IP / 主机组路径"
        class="group-scope-viewer__search"
      />
      <div class="group-scope-editor__tree-wrap scope-preview-list-wrap">
        <a-spin :spinning="loading">
          <a-tree
            v-if="filteredTreeData.length > 0"
            block-node
            :expanded-keys="expandedKeys"
            :auto-expand-parent="true"
            :tree-data="filteredTreeData"
            :selectable="false"
            :show-line="{ showLeafIcon: false }"
            class="group-scope-tree"
          >
            <template #title="node">
              <a-button
                v-if="isHostTreeNode(node)"
                type="link"
                size="small"
                class="scope-host-link-btn"
                @click.stop="emitHostClick(node)"
              >
                <span :class="getTreeNodeClass(node)">{{ getTreeNode(node).title }}</span>
              </a-button>
              <span v-else :class="getTreeNodeClass(node)">{{ getTreeNode(node).title }}</span>
            </template>
          </a-tree>
          <a-empty v-else description="暂无已勾选范围" />
        </a-spin>
      </div>
    </div>
  </a-modal>
</template>

<script setup>
import { computed, ref, watch } from 'vue'

const props = defineProps({
  open: {
    type: Boolean,
    default: false,
  },
  title: {
    type: String,
    default: '执行范围主机预览',
  },
  hosts: {
    type: Array,
    default: () => [],
  },
  total: {
    type: Number,
    default: 0,
  },
  loading: {
    type: Boolean,
    default: false,
  },
})

const emit = defineEmits(['close', 'host-click'])
const keyword = ref('')

watch(
  () => props.open,
  (visible) => {
    if (visible) {
      keyword.value = ''
    }
  },
)

const normalizedTotal = computed(() => Number(props.total || 0))

const treeData = computed(() => buildScopePreviewTreeData(props.hosts))

const filteredTreeData = computed(() => {
  return filterTreeByKeyword(treeData.value, keyword.value)
})

const expandedKeys = computed(() => {
  return collectAllTreeNodeKeys(filteredTreeData.value)
})

const visibleHostCount = computed(() => {
  return collectTreeScopeStats(filteredTreeData.value).hostCount
})

function getTreeNode(node) {
  if (!node || typeof node !== 'object') {
    return {}
  }
  return node.dataRef || node
}

function getTreeNodeClass(node) {
  const currentNode = getTreeNode(node)
  if (currentNode.node_type === 'host') {
    return 'scope-node-host'
  }
  if (currentNode.node_type === 'group') {
    return 'scope-node-group'
  }
  return 'scope-node-virtual'
}

function isHostTreeNode(node) {
  return getTreeNode(node).node_type === 'host'
}

function emitHostClick(node) {
  const currentNode = getTreeNode(node)
  emit('host-click', {
    host_id: currentNode.host_id,
    host_name: String(currentNode.title || ''),
  })
}

function formatMatchedHostTitle(host) {
  const displayName = String(host?.host_name || '').trim()
  const ip = String(host?.host_ip || '').trim()
  if (displayName && ip && displayName !== ip) {
    return `${displayName}(${ip})`
  }
  return displayName || ip || '-'
}

function buildScopePreviewTreeData(hosts) {
  const rootNodes = []
  const groupNodeMap = {}
  const rootHosts = []

  const ensureGroupNode = (pathParts) => {
    if (!Array.isArray(pathParts) || pathParts.length === 0) {
      return null
    }
    const pathKey = pathParts.join('/')
    const groupKey = `scope-preview-group-${pathKey}`
    if (groupNodeMap[groupKey]) {
      return groupNodeMap[groupKey]
    }

    const parentParts = pathParts.slice(0, -1)
    const parentNode = ensureGroupNode(parentParts)
    const label = pathParts[pathParts.length - 1]
    const nextNode = {
      key: groupKey,
      value: groupKey,
      title: label,
      node_type: 'group',
      children: [],
    }

    groupNodeMap[groupKey] = nextNode
    if (parentNode) {
      parentNode.children.push(nextNode)
    } else {
      rootNodes.push(nextNode)
    }

    return nextNode
  }

  ;(Array.isArray(hosts) ? hosts : []).forEach((item) => {
    if (!item || typeof item !== 'object') {
      return
    }
    const hostId = Number(item.host_id)
    if (!Number.isInteger(hostId) || hostId <= 0) {
      return
    }
    const groupPathText = String(item.group_path || item.group_name || '').trim()
    const hostTitle = formatMatchedHostTitle(item)
    const hostNode = {
      key: `scope-preview-host-${hostId}`,
      value: `scope-preview-host-${hostId}`,
      title: hostTitle,
      node_type: 'host',
      host_id: hostId,
      isLeaf: true,
    }

    if (!groupPathText) {
      rootHosts.push(hostNode)
      return
    }

    const pathParts = groupPathText
      .split('/')
      .map((part) => String(part || '').trim())
      .filter(Boolean)
    if (pathParts.length === 0) {
      rootHosts.push(hostNode)
      return
    }

    const leafGroupNode = ensureGroupNode(pathParts)
    if (!leafGroupNode) {
      rootHosts.push(hostNode)
      return
    }
    leafGroupNode.children.push(hostNode)
  })

  const sortNodes = (nodes) => {
    return (Array.isArray(nodes) ? nodes : [])
      .map((node) => {
        if (!node || typeof node !== 'object') {
          return node
        }
        const children = Array.isArray(node.children) ? sortNodes(node.children) : []
        return { ...node, children }
      })
      .sort((a, b) => {
        const aType = a?.node_type || ''
        const bType = b?.node_type || ''
        if (aType === bType) {
          return String(a?.title || '').localeCompare(String(b?.title || ''))
        }
        if (aType === 'group') {
          return -1
        }
        if (bType === 'group') {
          return 1
        }
        return String(a?.title || '').localeCompare(String(b?.title || ''))
      })
  }

  return appendGroupHostCount(sortNodes([...rootNodes, ...rootHosts]))
}

function appendGroupHostCount(nodes) {
  return (Array.isArray(nodes) ? nodes : []).map((node) => {
    if (!node || typeof node !== 'object') {
      return node
    }
    const children = appendGroupHostCount(node.children || [])
    if (node.node_type !== 'group') {
      return { ...node, children }
    }
    const hostCount = collectTreeScopeStats(children).hostCount
    const baseTitle = String(node.title || '')
    const normalizedTitle = baseTitle.replace(/\s*\(\d+\)$/, '')
    return {
      ...node,
      title: `${normalizedTitle} (${hostCount})`,
      children,
    }
  })
}

function filterTreeByKeyword(nodes, keywordText) {
  const normalizedKeyword = String(keywordText || '').trim().toLowerCase()
  const normalizedNodes = Array.isArray(nodes) ? nodes : []
  if (!normalizedKeyword) {
    return normalizedNodes
  }

  const includesKeyword = (value) => String(value || '').toLowerCase().includes(normalizedKeyword)
  const walk = (nodeList) => {
    const result = []
    ;(Array.isArray(nodeList) ? nodeList : []).forEach((node) => {
      if (!node || typeof node !== 'object') {
        return
      }

      const children = walk(node.children || [])
      const nodeText = String(node.title || '')
      if (includesKeyword(nodeText) || children.length > 0) {
        result.push({ ...node, children })
      }
    })
    return result
  }

  return walk(normalizedNodes)
}

function collectTreeScopeStats(nodes) {
  const stats = {
    hostCount: 0,
  }

  const visit = (nodeList) => {
    ;(Array.isArray(nodeList) ? nodeList : []).forEach((node) => {
      if (!node || typeof node !== 'object') {
        return
      }
      if (node.node_type === 'host') {
        stats.hostCount += 1
      }
      visit(node.children || [])
    })
  }

  visit(nodes)
  return stats
}

function collectAllTreeNodeKeys(nodes) {
  const keys = []
  const visit = (nodeList) => {
    ;(Array.isArray(nodeList) ? nodeList : []).forEach((node) => {
      if (!node || typeof node !== 'object') {
        return
      }
      if (node.key) {
        keys.push(node.key)
      }
      visit(node.children || [])
    })
  }
  visit(nodes)
  return keys
}
</script>

<style scoped>
.group-scope-viewer {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.group-scope-editor__desc {
  color: rgba(0, 0, 0, 0.45);
}

.group-scope-viewer__summary {
  padding: 8px 12px;
  border: 1px solid #d6e4ff;
  border-radius: 6px;
  background: #f5f8ff;
  color: rgba(0, 0, 0, 0.85);
}

.group-scope-viewer__search {
  max-width: 360px;
}

.group-scope-editor__tree-wrap {
  max-height: 440px;
  min-height: 280px;
  overflow: auto;
  border: 1px solid #f0f0f0;
  border-radius: 6px;
  padding: 8px;
}

.scope-host-link-btn {
  padding: 0;
  height: auto;
}

.scope-node-group {
  font-weight: 500;
}

.scope-node-host {
  color: rgba(0, 0, 0, 0.88);
}

.scope-node-virtual {
  color: rgba(0, 0, 0, 0.65);
}
</style>
