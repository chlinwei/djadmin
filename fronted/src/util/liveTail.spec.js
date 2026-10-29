import { describe, expect, it } from 'vitest'
import { createTailBuffer, liveSourceError, liveSourceFromLog, liveSourceKey } from './liveTail'

describe('live tail bounded plaintext buffer', () => {
  it('decodes UTF-8 across chunks and never interprets markup', () => {
    const buffer = createTailBuffer()
    const bytes = new TextEncoder().encode('日志<script>alert(1)</script>')
    buffer.pushBase64(btoa(String.fromCharCode(...bytes.slice(0, 2))))
    buffer.pushBase64(btoa(String.fromCharCode(...bytes.slice(2))))
    expect(buffer.state.lines).toEqual(['日志<script>alert(1)</script>'])
  })

  it('bounds lines and UTF-8 bytes and distinguishes eviction from truncation', () => {
    const buffer = createTailBuffer({ maxLines: 3, maxBytes: 20, maxLineBytes: 8 })
    buffer.append('一二三四五\nnext\nlast\nnew')
    expect(buffer.state.truncated).toBe(true)
    expect(buffer.state.evicted).toBe(true)
    expect(buffer.state.lines.length).toBeLessThanOrEqual(3)
    expect(buffer.state.bytes).toBeLessThanOrEqual(20)
    buffer.clear()
    expect(buffer.state).toEqual({ lines: [''], bytes: 0, evicted: false, truncated: false })
  })

  it('preserves displayed text but resets partial UTF-8 at a new session boundary', () => {
    const buffer = createTailBuffer()
    const bytes = new TextEncoder().encode('日志')
    buffer.append('previous output')
    buffer.pushBase64(btoa(String.fromCharCode(...bytes.slice(0, 2))))
    buffer.beginSession('manual continuation; possible gap or duplicate')
    buffer.pushBase64(btoa(String.fromCharCode(...bytes)))
    expect(buffer.state.lines).toEqual([
      'previous output',
      '--- manual continuation; possible gap or duplicate ---',
      '日志',
    ])
    expect(buffer.state.lines.join('\n')).not.toContain('\uFFFD')
  })

  it('keeps continuation markers within the same line and byte budgets', () => {
    const buffer = createTailBuffer({ maxLines: 3, maxBytes: 40 })
    for (let i = 0; i < 10; i++) {
      buffer.beginSession('restart')
      buffer.append('new output')
    }
    expect(buffer.state.lines.length).toBeLessThanOrEqual(3)
    expect(buffer.state.bytes).toBeLessThanOrEqual(40)
    expect(buffer.state.evicted).toBe(true)
  })

  it('does not buffer a continuing oversized line', () => {
    const buffer = createTailBuffer()
    for (let i = 0; i < 100; i++) buffer.append('x'.repeat(32768))
    expect(buffer.state.bytes).toBe(65536)
    expect(buffer.state.lines[0]).toHaveLength(65536)
    buffer.append('\nnew')
    expect(buffer.state.lines[1]).toBe('new')
  })

  it('handles newline-heavy packets without retaining earlier rows', () => {
    const buffer = createTailBuffer()
    buffer.pushBase64(btoa('\n'.repeat(32768)))
    expect(buffer.state.lines).toHaveLength(5000)
    expect(buffer.state.bytes).toBe(4999)
    expect(buffer.state.evicted).toBe(true)
    expect(() => buffer.pushBase64(btoa('x'.repeat(32769)))).toThrow('32 KiB')
  })

  it('uses existing deployment fields without requiring a host ID', () => {
    expect(liveSourceFromLog({ host_ip: '127.0.0.1', log_path: '/app.log' })).toBe(null)
    const log = { service_id: '2', instance: 'app', host_ip: '10.0.0.1', log_name: 'main', log_path: '/app.log' }
    expect(liveSourceFromLog({ ...log, shared_source_id: '3' })).toBe(null)
    const source = liveSourceFromLog(log)
    expect(source).toEqual(log)
    expect(source).not.toHaveProperty('host_id')
    expect(liveSourceFromLog({ ...log, host_id: '999' })).toEqual(source)
    expect(liveSourceKey(source)).toBe('10.0.0.1:/app.log')
  })

  it('identifies missing fields instead of assuming a shared source or old record', () => {
    const log = { service_id: '2', instance: 'app', log_name: 'main', log_path: '/app.log' }
    const missingHost = liveSourceError(log)
    expect(missingHost).toContain('缺少 host_ip')
    expect(missingHost).not.toContain('共享')
    expect(missingHost).not.toContain('旧日志')
    expect(missingHost).not.toContain('下发')
    expect(liveSourceError({ ...log, service_id: '', host_ip: '10.0.0.1' })).toContain('缺少 service_id')
    expect(liveSourceError({ ...log, instance: '', host_ip: '10.0.0.1' })).toContain('缺少 instance')
    expect(liveSourceError({ ...log, log_name: '', host_ip: '10.0.0.1' })).toContain('缺少 log_name')
  })

  it('distinguishes missing paths and historical shared-source markers from identity errors', () => {
    const log = { service_id: '2', instance: 'app', host_ip: '10.0.0.1', log_name: 'main' }
    expect(liveSourceError(log)).toContain('缺少 log_path')
    expect(liveSourceError({ shared_source_id: '3' })).toContain('共享采集源历史标记')
    expect(liveSourceError(null)).toBe('请先选择一条日志记录。')
    expect(liveSourceError({ ...log, log_path: '/app.log' })).toBe('')
  })
})
