// Text is never interpreted as HTML/ANSI. The decoder persists across transport
// chunks; limits count UTF-8 bytes rather than JS UTF-16 code units.
const encoder = new TextEncoder()
export const MAX_TAIL_TABS = 5
export const TAIL_ROW_HEIGHT = 22

export function createTailBuffer({ maxLines = 5000, maxBytes = 5 * 1024 * 1024, maxLineBytes = 64 * 1024 } = {}) {
  const state = { lines: [''], bytes: 0, evicted: false, truncated: false }
  let lineBytes = 0
  let discardLine = false
  let decoder = new TextDecoder()
  function append(text) {
    let removed = 0
    const parts = text.split('\n')
    parts.forEach((part, index) => {
      if (index) {
        state.lines.push('')
        lineBytes = 0
        discardLine = false
      }
      if (!discardLine) {
        let content = part
        const bytes = encoder.encode(part)
        if (lineBytes + bytes.length > maxLineBytes) {
          // Decode only complete characters at the truncation boundary.
          content = new TextDecoder().decode(bytes.subarray(0, Math.max(0, maxLineBytes - lineBytes)), { stream: true })
          discardLine = true
          state.truncated = true
        }
        state.lines[state.lines.length - 1] += content
        const added = encoder.encode(content).length
        lineBytes += added
        state.bytes += added
      }
      // Include separators and advance a logical head while ingesting. Compact
      // once per packet, avoiding quadratic Array.shift on newline-heavy logs.
      if (index) state.bytes++
      while (state.lines.length - removed > maxLines || state.bytes > maxBytes) {
        if (state.lines.length - removed === 1) break
        state.bytes -= encoder.encode(state.lines[removed++]).length + 1
        state.evicted = true
      }
    })
    if (removed) state.lines.splice(0, removed)
  }
  return {
    state,
    append,
    beginSession(notice) {
      // Separate independent snapshots and discard any unfinished UTF-8 rune
      // from the previous connection instead of decoding it with the new one.
      decoder = new TextDecoder()
      append(`\n--- ${notice} ---\n`)
    },
    pushBase64(data) {
      if (!data) return
      const binary = atob(data)
      if (binary.length > 32768) throw new Error('live-tail frame exceeds 32 KiB')
      const bytes = Uint8Array.from(binary, (char) => char.charCodeAt(0))
      append(decoder.decode(bytes, { stream: true }))
    },
    clear() {
      state.lines = ['']
      state.bytes = 0
      state.evicted = false
      state.truncated = false
      lineBytes = 0
      discardLine = false
    },
  }
}

export function liveSourceError(log) {
  if (!log) return '请先选择一条日志记录。'
  if (log.shared_source_id) return '该记录带有共享采集源历史标记，暂不支持实时跟踪。'
  const missingIdentities = ['service_id', 'instance', 'host_ip', 'log_name'].filter((field) => !log[field])
  if (missingIdentities.length) {
    return `当前日志记录缺少 ${missingIdentities.join('、')}，无法关联部署实例定位源文件，请检查日志来源字段。`
  }
  if (!log.log_path) return '当前日志记录缺少 log_path（真实文件路径），无法实时跟踪；请检查采集事件中的 log.file.path。'
  return ''
}

export function liveSourceFromLog(log) {
  if (liveSourceError(log)) return null
  return {
    service_id: String(log.service_id),
    host_ip: log.host_ip,
    instance: log.instance || '',
    log_name: log.log_name || '',
    log_path: log.log_path,
  }
}

export function liveSourceKey(source) {
  return `${source.host_ip}:${source.log_path}`
}
