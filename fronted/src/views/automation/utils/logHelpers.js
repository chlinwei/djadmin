export function formatInventoryHostLabel(host) {
  const instanceName = String(host?.instance_name || host?.host_name || '').trim()
  const hostIp = String(host?.host_ip || '').trim()
  if (instanceName && hostIp && instanceName !== hostIp) {
    return `${instanceName}(${hostIp})`
  }
  return instanceName || hostIp || '-'
}

export function buildHostScopedLogText(jobOutput, record) {
  const source = String(jobOutput || '')
  if (!source.trim() || !record) {
    return ''
  }

  const hostName = String(record.host_name || '').trim()
  const hostIp = String(record.host_ip || '').trim()
  const hostLabelCandidates = []
  if (hostName && hostIp && hostName !== hostIp) {
    hostLabelCandidates.push(`${hostName}(${hostIp})`)
  }
  if (hostName) {
    hostLabelCandidates.push(hostName)
  }
  if (hostIp) {
    hostLabelCandidates.push(hostIp)
  }

  const uniqCandidates = Array.from(new Set(hostLabelCandidates.filter((item) => !!item)))
  if (!uniqCandidates.length) {
    return ''
  }

  const displayHostLabel = formatInventoryHostLabel({
    host_name: hostName,
    host_ip: hostIp,
  })

  const escapedHostExpr = uniqCandidates
    .map((item) => item.replace(/[.*+?^${}()|[\]\\]/g, '\\$&'))
    .join('|')
  // Support both formats and keep timestamp/stream markers in detailed logs:
  // 1) [host][stdout] ...
  // 2) [YYYY-MM-DD HH:mm:ss][host][stdout] ...
  const hostLineRegex = new RegExp(
    `^(?:\\[(\\d{4}-\\d{2}-\\d{2} \\d{2}:\\d{2}:\\d{2})\\])?\\[(?:${escapedHostExpr})\\](?:\\[(stdout|stderr)\\])?\\s?(.*)$`
  )

  const lines = source
    .split('\n')
    .filter((line) => hostLineRegex.test(line))
    .map((line) => {
      const matched = line.match(hostLineRegex)
      if (!matched) {
        return line
      }
      const timestampText = matched[1] || ''
      const streamText = matched[2] || ''
      const contentText = String(matched[3] || '')
      const normalizedContentText = displayHostLabel
        ? contentText.replace(/\bhost_\d+\b/g, displayHostLabel)
        : contentText

      const prefixes = []
      if (timestampText) {
        prefixes.push(`[${timestampText}]`)
      }
      if (streamText) {
        prefixes.push(`[${streamText}]`)
      }

      if (!prefixes.length) {
        return normalizedContentText
      }
      if (!normalizedContentText) {
        return prefixes.join('')
      }
      return `${prefixes.join('')} ${normalizedContentText}`
    })

  return lines.join('\n').trim()
}

export function normalizeUnifiedLogAliases(text) {
  const source = String(text || '')
  if (!source) {
    return ''
  }

  const linePattern = /^(\[\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}\])\[([^\]]+)\](\[(?:stdout|stderr)\]\s?)(.*)$/
  return source
    .split('\n')
    .map((line) => {
      const matched = line.match(linePattern)
      if (!matched) {
        return line
      }

      const timestampPart = matched[1] || ''
      const hostPart = matched[2] || ''
      const streamPart = matched[3] || ''
      const contentPart = String(matched[4] || '')
      const normalizedContent = hostPart
        ? contentPart.replace(/\bhost_\d+\b/g, hostPart)
        : contentPart

      return `${timestampPart}[${hostPart}]${streamPart}${normalizedContent}`
    })
    .join('\n')
}

// 复制文本的实现已下沉到 @/util/clipboard（monitor 的日志详情也要用，不应让监控域反向依赖自动化域）。
export { copyTextWithFallback } from '@/util/clipboard'
