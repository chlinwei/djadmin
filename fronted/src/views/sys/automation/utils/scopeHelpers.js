export const LIMIT_INPUT_PLACEHOLDER = '例如：host:pvg-esb4-204 10.25.* path:wuhan/pvg4'

export function formatMatchedHostTitle(host) {
  const displayName = String(host?.host_name || '').trim()
  const ip = String(host?.host_ip || '').trim()
  if (displayName && ip && displayName !== ip) {
    return `${displayName}(${ip})`
  }
  return displayName || ip || '-'
}

export function mapInventoryOptions(items) {
  const normalizedItems = Array.isArray(items) ? items : []
  return normalizedItems
    .filter((item) => Number(item?.id || 0) > 0)
    .map((item) => ({
      label: String(item?.name || ''),
      value: Number(item.id),
    }))
}

export function splitLimitTokens(limitText) {
  return String(limitText || '')
    .replace(/,/g, ' ')
    .split(/\s+/)
    .map((item) => item.trim())
    .filter(Boolean)
}

export function resolveMatchedHostLimitToken(host) {
  const hostName = String(host?.host_name || '').trim()
  const hostIp = String(host?.host_ip || '').trim()
  if (hostName && !/[\s,]/.test(hostName)) {
    return `host:${hostName}`
  }
  if (hostIp) {
    return hostIp
  }
  const hostId = Number(host?.host_id || 0)
  if (Number.isInteger(hostId) && hostId > 0) {
    return String(hostId)
  }
  return ''
}

export function hasLimitToken(limitText, tokenText) {
  const token = String(tokenText || '').trim()
  if (!token) {
    return false
  }
  const tokens = splitLimitTokens(limitText)
  const lowerToken = token.toLowerCase()
  return tokens.some((item) => String(item || '').trim().toLowerCase() === lowerToken)
}

export function toggleLimitToken(limitText, tokenText) {
  const token = String(tokenText || '').trim()
  const tokens = splitLimitTokens(limitText)
  if (!token) {
    return String(limitText || '').trim()
  }

  const lowerToken = token.toLowerCase()
  const negativeToken = `!${token}`.toLowerCase()
  const normalized = tokens.filter((item) => {
    const text = String(item || '').trim().toLowerCase()
    return text !== negativeToken
  })

  const exists = normalized.some((item) => String(item || '').trim().toLowerCase() === lowerToken)
  if (exists) {
    return normalized
      .filter((item) => String(item || '').trim().toLowerCase() !== lowerToken)
      .join(' ')
      .trim()
  }

  return [...normalized, token].join(' ').trim()
}

export function removeLimitToken(limitText, tokenText) {
  const token = String(tokenText || '').trim()
  if (!token) {
    return String(limitText || '').trim()
  }

  const lowerToken = token.toLowerCase()
  return splitLimitTokens(limitText)
    .filter((item) => String(item || '').trim().toLowerCase() !== lowerToken)
    .join(' ')
    .trim()
}

export const ASSET_HOST_ROUTE_CANDIDATES = ['/assets/hosts', '/assets/host', '/assets/hosts/index', '/assets/host/index']

export function resolveAssetHostListPath(router) {
  for (const path of ASSET_HOST_ROUTE_CANDIDATES) {
    const resolved = router.resolve({ path })
    if (Array.isArray(resolved?.matched) && resolved.matched.length > 0) {
      return path
    }
  }
  return ''
}

export function goToAssetHost(router, messageApi, hostId, hostName = '') {
  const normalizedHostId = Number(hostId)
  if (!Number.isInteger(normalizedHostId) || normalizedHostId <= 0) {
    return
  }

  const hostListPath = resolveAssetHostListPath(router)
  if (!hostListPath) {
    messageApi.warning('未找到资产主机页面路由')
    return
  }

  const searchText = String(hostName || '').trim()
  router.push({
    path: hostListPath,
    query: {
      host_id: String(normalizedHostId),
      ...(searchText ? { search: searchText } : {}),
    },
  })
}
