/**
 * 主机范围勾选树的公共构造/过滤逻辑，供巡检任务的「编辑范围」与「查看范围」复用。
 * 范围的唯一真相是主机 ID 列表；分组节点只是批量勾选入口，不会被保存。
 */

const GROUP_PREFIX = 'group-'
const HOST_PREFIX = 'host-'
const COUNT_SUFFIX_PATTERN = /\s*[（(]\d+台[）)]\s*$/

function toPositiveIntSet(values) {
  const result = new Set()
  ;(Array.isArray(values) ? values : []).forEach((item) => {
    const num = Number(item)
    if (Number.isInteger(num) && num > 0) result.add(num)
  })
  return result
}

function hostTitle(host) {
  const name = String(host?.instance_name || '').trim()
  const ip = String(host?.ip || '').trim()
  if (name && ip && name !== ip) return `${name}(${ip})`
  return name || ip || `host-${host?.id ?? ''}`
}

export function buildHostScopeTree(groups, hosts) {
  const hostsByGroup = new Map()
  ;(Array.isArray(hosts) ? hosts : []).forEach((host) => {
    const groupId = Number(host?.group_id)
    if (!Number.isInteger(groupId) || groupId <= 0) return
    const bucket = hostsByGroup.get(groupId) || []
    bucket.push({
      key: `${HOST_PREFIX}${host.id}`,
      value: `${HOST_PREFIX}${host.id}`,
      title: hostTitle(host),
      host_id: host.id,
      isLeaf: true,
    })
    hostsByGroup.set(groupId, bucket)
  })

  const walk = (nodes) => (Array.isArray(nodes) ? nodes : []).map((node) => ({
    key: `${GROUP_PREFIX}${node.id}`,
    value: `${GROUP_PREFIX}${node.id}`,
    title: String(node.name || `分组#${node.id}`),
    children: [...walk(node.children), ...(hostsByGroup.get(Number(node.id)) || [])],
  }))
  return walk(groups)
}

export function stripHostCountSuffix(title) {
  return String(title || '').replace(COUNT_SUFFIX_PATTERN, '')
}

export function appendHostCount(nodes) {
  const walk = (items) => {
    let hostCount = 0
    const mapped = (Array.isArray(items) ? items : []).map((node) => {
      if (String(node?.key || '').startsWith(HOST_PREFIX)) {
        hostCount += 1
        return { ...node, children: undefined }
      }
      const child = walk(node?.children)
      hostCount += child.hostCount
      const base = stripHostCountSuffix(node?.title)
      return { ...node, title: child.hostCount > 0 ? `${base}（${child.hostCount}台）` : base, children: child.nodes }
    })
    return { nodes: mapped, hostCount }
  }
  return walk(nodes).nodes
}

export function filterHostScopeTree(nodes, keyword) {
  const kw = String(keyword || '').trim().toLowerCase()
  if (!kw) return Array.isArray(nodes) ? nodes : []

  // 命中的分组保留整棵子树，否则只保留命中链路，避免搜索后勾选范围看起来"变小"。
  const walk = (items, keepAll) => {
    const result = []
    ;(Array.isArray(items) ? items : []).forEach((node) => {
      const matched = keepAll || stripHostCountSuffix(node?.title).toLowerCase().includes(kw)
      const children = walk(node?.children, matched)
      if (matched || children.length > 0) result.push({ ...node, children })
    })
    return result
  }
  return walk(nodes, false)
}

export function pruneHostScopeTree(nodes, selectedHostIds) {
  const hostSet = toPositiveIntSet(selectedHostIds)

  const walk = (items) => {
    const result = []
    ;(Array.isArray(items) ? items : []).forEach((node) => {
      const key = String(node?.key || '')
      if (key.startsWith(HOST_PREFIX)) {
        if (hostSet.has(Number(key.slice(HOST_PREFIX.length)))) result.push({ ...node, children: undefined })
        return
      }
      const children = walk(node?.children)
      if (children.length > 0) result.push({ ...node, children })
    })
    return result
  }
  return walk(nodes)
}

export function collectGroupKeys(nodes) {
  const keys = []
  const walk = (items) => {
    ;(Array.isArray(items) ? items : []).forEach((node) => {
      if (String(node?.key || '').startsWith(GROUP_PREFIX)) keys.push(node.key)
      walk(node?.children)
    })
  }
  walk(nodes)
  return keys
}

export function countScopeHosts(nodes) {
  let total = 0
  const walk = (items) => {
    ;(Array.isArray(items) ? items : []).forEach((node) => {
      if (String(node?.key || '').startsWith(HOST_PREFIX)) total += 1
      walk(node?.children)
    })
  }
  walk(nodes)
  return total
}

export function collectHostIds(nodes) {
  const ids = []
  const walk = (items) => {
    ;(Array.isArray(items) ? items : []).forEach((node) => {
      const key = String(node?.key || '')
      if (key.startsWith(HOST_PREFIX)) {
        const id = Number(key.slice(HOST_PREFIX.length))
        if (Number.isInteger(id) && id > 0) ids.push(id)
      }
      walk(node?.children)
    })
  }
  walk(nodes)
  return ids
}

export function pickHostIds(keys) {
  const list = Array.isArray(keys) ? keys : keys?.checked || []
  return list
    .filter((item) => typeof item === 'string' && item.startsWith(HOST_PREFIX))
    .map((item) => Number(item.slice(HOST_PREFIX.length)))
    .filter((item) => Number.isInteger(item) && item > 0)
}

export function toHostKeys(selectedHostIds) {
  return (Array.isArray(selectedHostIds) ? selectedHostIds : []).map((id) => `${HOST_PREFIX}${id}`)
}
