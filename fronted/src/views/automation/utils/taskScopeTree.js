function normalizeGroupTree(nodes, hostChildrenMap = { groupedHosts: {}, standaloneHosts: [] }) {
  return (nodes || []).map((node) => ({
    key: `group-${node.id}`,
    value: `group-${node.id}`,
    title: node.name,
    node_type: 'group',
    children: [
      ...normalizeGroupTree(node.children || [], hostChildrenMap),
      ...(hostChildrenMap.groupedHosts[node.id] || []),
    ],
  }))
}

function pruneGroupsWithoutHosts(nodes) {
  const normalizedNodes = Array.isArray(nodes) ? nodes : []
  const result = []

  normalizedNodes.forEach((node) => {
    if (!node || typeof node !== 'object') {
      return
    }

    if (node.node_type === 'host') {
      result.push({ ...node, children: undefined })
      return
    }

    const children = pruneGroupsWithoutHosts(node.children || [])
    if (children.length > 0) {
      result.push({ ...node, children })
    }
  })

  return result
}

function buildHostChildrenMap(hosts) {
  const groupedHosts = {}
  const standaloneHosts = []
  ;(hosts || []).forEach((item) => {
    if (!item || typeof item !== 'object') {
      return
    }
    const hostId = Number(item.id)
    if (!Number.isInteger(hostId) || hostId <= 0) {
      return
    }
    const hostNode = {
      key: `host-${hostId}`,
      value: `host-${hostId}`,
      title: formatHostTreeTitle(item),
      node_type: 'host',
      isLeaf: true,
      host_id: hostId,
      group_id: item.group_id || null,
    }
    if (item.group_id) {
      if (!groupedHosts[item.group_id]) {
        groupedHosts[item.group_id] = []
      }
      groupedHosts[item.group_id].push(hostNode)
      return
    }
    standaloneHosts.push(hostNode)
  })
  return { groupedHosts, standaloneHosts }
}

function parseGroupIdFromNode(node) {
  if (!node || typeof node !== 'object') {
    return null
  }
  if (Number.isInteger(Number(node.group_id)) && Number(node.group_id) > 0) {
    return Number(node.group_id)
  }
  const keyText = String(node.key || '')
  if (keyText.startsWith('group-')) {
    const groupId = Number(keyText.slice('group-'.length))
    return Number.isInteger(groupId) && groupId > 0 ? groupId : null
  }
  return null
}

export function formatHostTreeTitle(host) {
  if (!host || typeof host !== 'object') {
    return '-'
  }
  const instanceName = String(host.instance_name || '').trim()
  const ip = host.ip || '-'
  if (instanceName && instanceName !== ip) {
    return `${instanceName}(${ip})`
  }
  return ip
}

export function formatMatchedHostTitle(host) {
  const displayName = String(host?.host_name || '').trim()
  const ip = String(host?.host_ip || '').trim()
  if (displayName && ip && displayName !== ip) {
    return `${displayName}(${ip})`
  }
  return displayName || ip || '-'
}

export function buildGroupTreeWithAll(nodes, hosts = [], allGroupValue = '__all__') {
  const hostChildrenMap = buildHostChildrenMap(hosts)
  const normalizedGroups = normalizeGroupTree(nodes, hostChildrenMap)
  const prunedGroups = pruneGroupsWithoutHosts(normalizedGroups)
  return [{
    key: allGroupValue,
    value: allGroupValue,
    title: 'All',
    node_type: 'virtual',
    children: [...prunedGroups, ...hostChildrenMap.standaloneHosts],
  }]
}

export function stripGroupCountSuffix(title) {
  return String(title || '').replace(/\s*[（(](?:\d+个|\d+台|\d+组，\d+台)[）)]\s*$/, '')
}

export function appendGroupHostCount(nodes) {
  const normalizedNodes = Array.isArray(nodes) ? nodes : []

  const walk = (items) => {
    let hostCount = 0
    let groupCount = 0
    const mapped = items.map((node) => {
      if (!node || typeof node !== 'object') {
        return node
      }

      const nodeType = node.node_type || ''
      if (nodeType === 'host') {
        hostCount += 1
        return { ...node, children: undefined }
      }

      const children = Array.isArray(node.children) ? node.children : []
      const childResult = walk(children)
      const baseTitle = stripGroupCountSuffix(node.title || '-')
      let nextTitle = baseTitle

      if (nodeType === 'group') {
        groupCount += 1
        nextTitle = childResult.hostCount > 0
          ? `${baseTitle}（${childResult.hostCount}台）`
          : baseTitle
      }

      if (nodeType === 'virtual') {
        nextTitle = `${baseTitle}（${childResult.groupCount}组，${childResult.hostCount}台）`
      }

      hostCount += childResult.hostCount
      groupCount += childResult.groupCount
      return {
        ...node,
        title: nextTitle,
        children: childResult.nodes,
      }
    })

    return { nodes: mapped, hostCount, groupCount }
  }

  return walk(normalizedNodes).nodes
}

export function cloneTreeNodes(nodes) {
  return (nodes || []).map((item) => ({
    ...item,
    children: cloneTreeNodes(item.children || []),
  }))
}

export function collectTreeScopeStats(nodes) {
  const stats = { groupCount: 0, hostCount: 0 }

  const walk = (items) => {
    ;(Array.isArray(items) ? items : []).forEach((node) => {
      if (!node || typeof node !== 'object') {
        return
      }
      const nodeType = node.node_type || ''
      if (nodeType === 'group') {
        stats.groupCount += 1
      }
      if (nodeType === 'host') {
        stats.hostCount += 1
      }
      walk(node.children || [])
    })
  }

  walk(nodes)
  return stats
}

export function filterTreeByKeyword(nodes, keyword) {
  const normalizedNodes = Array.isArray(nodes) ? nodes : []
  const kw = String(keyword || '').trim().toLowerCase()
  if (!kw) {
    return normalizedNodes
  }

  const walk = (items, keepAllDescendants = false) => {
    const result = []
    items.forEach((node) => {
      if (!node || typeof node !== 'object') {
        return
      }
      const rawChildren = Array.isArray(node.children) ? node.children : []
      const titleText = String(stripGroupCountSuffix(node.title || '')).toLowerCase()
      const selfMatched = keepAllDescendants || titleText.includes(kw)
      const children = walk(rawChildren, selfMatched)
      if (selfMatched || children.length > 0) {
        result.push({ ...node, children })
      }
    })
    return result
  }

  return walk(normalizedNodes, false)
}

export function collectExpandedGroupKeys(nodes) {
  const keys = []
  const walk = (items) => {
    ;(Array.isArray(items) ? items : []).forEach((node) => {
      if (!node || typeof node !== 'object') {
        return
      }
      const keyText = String(node.key || '')
      const nodeType = String(node.node_type || '')
      const isHostNode = nodeType === 'host' || keyText.includes('host-')
      if (!isHostNode && keyText) {
        keys.push(keyText)
      }
      walk(node.children || [])
    })
  }
  walk(nodes)
  return keys
}

export function buildScopePreviewTreeData(hosts) {
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

export function collectCheckedHostCountByNode(nodes, checkedKeySet, collector = {}, inheritedSelected = false) {
  const normalizedNodes = Array.isArray(nodes) ? nodes : []
  let subtreeHostCount = 0

  normalizedNodes.forEach((node) => {
    if (!node || typeof node !== 'object') {
      return
    }

    const key = String(node.key || '')
    const nodeType = node.node_type || ''
    const selfSelected = inheritedSelected || checkedKeySet.has(key)

    if (nodeType === 'host') {
      if (selfSelected) {
        subtreeHostCount += 1
      }
      return
    }

    const childCount = collectCheckedHostCountByNode(node.children || [], checkedKeySet, collector, selfSelected)
    collector[key] = childCount
    subtreeHostCount += childCount
  })

  return subtreeHostCount
}

export function toNumericSet(values) {
  const set = new Set()
  ;(Array.isArray(values) ? values : []).forEach((item) => {
    const num = Number(item)
    if (Number.isInteger(num) && num > 0) {
      set.add(num)
    }
  })
  return set
}

export function pruneTreeToCheckedNodes(nodes, selectedGroupSet, selectedHostSet, inheritedGroupSelected = false) {
  const normalizedNodes = Array.isArray(nodes) ? nodes : []
  const result = []

  normalizedNodes.forEach((node) => {
    if (!node || typeof node !== 'object') {
      return
    }

    const nodeType = node.node_type || ''
    if (nodeType === 'host') {
      const hostId = Number(node.host_id)
      const keepByHost = Number.isInteger(hostId) && selectedHostSet.has(hostId)
      if (keepByHost || inheritedGroupSelected) {
        result.push({ ...node, children: undefined })
      }
      return
    }

    const groupId = parseGroupIdFromNode(node)
    const groupSelected = inheritedGroupSelected || (groupId !== null && selectedGroupSet.has(groupId))
    const children = pruneTreeToCheckedNodes(node.children || [], selectedGroupSet, selectedHostSet, groupSelected)

    if (children.length > 0) {
      result.push({ ...node, children })
    }
  })

  return result
}

export function collectCheckedScope(keys, allGroupValue = '__all__') {
  const checkedGroupIds = []
  const checkedHostIds = []
  ;(Array.isArray(keys) ? keys : []).forEach((key) => {
    if (key === allGroupValue) {
      checkedGroupIds.push(allGroupValue)
      return
    }
    if (typeof key !== 'string') {
      return
    }
    if (key.startsWith('group-')) {
      const groupId = Number(key.replace('group-', ''))
      if (Number.isInteger(groupId) && groupId > 0) {
        checkedGroupIds.push(groupId)
      }
      return
    }
    if (key.startsWith('host-')) {
      const hostId = Number(key.replace('host-', ''))
      if (Number.isInteger(hostId) && hostId > 0) {
        checkedHostIds.push(hostId)
      }
    }
  })
  return { checkedGroupIds, checkedHostIds }
}
