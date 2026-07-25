export const buildTreeData = (nodes, maxDepth, depth = 1) => {
  return (nodes || []).map((item) => ({
    title: `${item.name}${item.host_count !== undefined && item.host_count !== null ? ` (${item.host_count})` : ''}`,
    key: item.id,
    name: item.name,
    host_count: item.host_count,
    depth,
    children: item.children && item.children.length && depth < maxDepth ? buildTreeData(item.children, maxDepth, depth + 1) : undefined,
  }))
}

export const buildGroupTreeSelectData = (nodes = []) => {
  return nodes.map((item) => ({
    title: item.name,
    value: item.key,
    key: item.key,
    children: item.children && item.children.length ? buildGroupTreeSelectData(item.children) : undefined,
  }))
}

export const filterGroupTree = (nodes, keyword) => {
  const search = String(keyword || '').trim().toLowerCase()
  if (!search) {
    return nodes
  }

  return (nodes || [])
    .map((item) => {
      const children = item.children ? filterGroupTree(item.children, search) : []
      const matched = String(item.name || '').toLowerCase().includes(search)
      if (matched || children.length) {
        return {
          ...item,
          children: children.length ? children : undefined,
        }
      }
      return null
    })
    .filter(Boolean)
}

export const collectExpandedKeys = (nodes) => {
  const keys = [0]
  const walk = (items) => {
    items.forEach((item) => {
      if (item.children && item.children.length) {
        keys.push(item.key)
        walk(item.children)
      }
    })
  }
  walk(nodes || [])
  return Array.from(new Set(keys))
}

export const findGroupNodeByKey = (nodes, key) => {
  for (const node of nodes || []) {
    if (Number(node?.key) === Number(key)) {
      return node
    }
    const childNode = findGroupNodeByKey(node?.children || [], key)
    if (childNode) {
      return childNode
    }
  }
  return null
}

export const getGroupNodeLabel = (node) => {
  if (!node) {
    return '未命名分组'
  }
  const rawTitle = String(node.title || node.name || '未命名分组')
  return rawTitle.replace(/\s*\(\d+\)\s*$/, '')
}

export const getHostDisplayName = (host) => {
  const instanceName = String(host?.instance_name || '').trim()
  const hostname = String(host?.system?.hostname || '').trim()
  const ip = String(host?.ip || '').trim()
  return instanceName || hostname || ip || `Host-${host?.id || '-'}`
}

export const getHostGroupId = (host) => {
  if (Number.isInteger(Number(host?.group_id)) && Number(host?.group_id) > 0) {
    return Number(host.group_id)
  }
  if (Number.isInteger(Number(host?.group)) && Number(host?.group) > 0) {
    return Number(host.group)
  }
  if (Number.isInteger(Number(host?.group?.id)) && Number(host?.group?.id) > 0) {
    return Number(host.group.id)
  }
  return null
}

export const buildDeletePreviewTree = (groupNode, hosts) => {
  const hostRows = Array.isArray(hosts) ? hosts : []
  const groupHostsMap = new Map()
  hostRows.forEach((host) => {
    const gid = getHostGroupId(host)
    if (!gid) {
      return
    }
    const list = groupHostsMap.get(gid) || []
    list.push(host)
    groupHostsMap.set(gid, list)
  })

  const buildNode = (node) => {
    const nodeId = Number(node?.key || 0)
    const groupLabel = getGroupNodeLabel(node)
    const childGroups = Array.isArray(node?.children) ? node.children.map((item) => buildNode(item)) : []
    const groupHosts = (groupHostsMap.get(nodeId) || []).map((host) => ({
      title: `${getHostDisplayName(host)} (${host.ip || '-'})`,
      key: `host-${host.id}`,
      isLeaf: true,
    }))

    return {
      title: `${groupLabel}`,
      key: `group-${nodeId}`,
      children: [...childGroups, ...groupHosts],
    }
  }

  return groupNode ? [buildNode(groupNode)] : []
}

export const collectTreeKeys = (nodes) => {
  const keys = []
  const walk = (items) => {
    ;(items || []).forEach((item) => {
      keys.push(item.key)
      if (Array.isArray(item.children) && item.children.length) {
        walk(item.children)
      }
    })
  }
  walk(nodes)
  return keys
}
