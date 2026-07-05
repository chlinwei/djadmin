import {
  convertUTCToTimezone,
  formatTimeWithTimezone,
  getTimezoneLabel,
  getTimezoneOffset,
  toUtcQueryISOString,
} from '../timezone'

describe('timezone utils', () => {
  it('keeps Date object identity for convertUTCToTimezone', () => {
    const date = new Date('2026-01-01T00:00:00Z')

    const result = convertUTCToTimezone(date, 'Asia/Shanghai')
    expect(result).toBe(date)
  })

  it('supports timestamp input for convertUTCToTimezone', () => {
    const result = convertUTCToTimezone(1767225600000, 'UTC')

    expect(result instanceof Date).toBe(true)
    expect(result.toISOString()).toBe('2026-01-01T00:00:00.000Z')
  })

  it('formats UTC time with user timezone', () => {
    const result = formatTimeWithTimezone('2026-01-01T00:00:00Z', 'Asia/Shanghai')

    expect(result).toBe('2026-01-01 08:00:00')
  })

  it('returns local string for invalid time input', () => {
    const errorSpy = vi.spyOn(console, 'error').mockImplementation(() => {})
    const result = formatTimeWithTimezone('invalid-time-value', 'Asia/Shanghai')

    expect(typeof result).toBe('string')
    expect(result.length).toBeGreaterThan(0)
    errorSpy.mockRestore()
  })

  it('returns ISO string for Date input', () => {
    const date = new Date('2026-01-01T00:00:00Z')

    expect(toUtcQueryISOString(date)).toBe('2026-01-01T00:00:00.000Z')
  })

  it('returns undefined for empty input', () => {
    expect(toUtcQueryISOString(null)).toBeUndefined()
  })

  it('returns ISO string for object with toDate method', () => {
    const result = toUtcQueryISOString({
      toDate: () => new Date('2026-01-01T00:00:00Z'),
    })

    expect(result).toBe('2026-01-01T00:00:00.000Z')
  })

  it('returns UTC offset and label for known timezone', () => {
    expect(getTimezoneOffset('Asia/Shanghai')).toBe(8)
    expect(getTimezoneLabel('Asia/Shanghai')).toContain('GMT+8')
  })

  it('returns fallback values for unknown timezone', () => {
    expect(getTimezoneOffset('Unknown/Zone')).toBe(0)
    expect(getTimezoneLabel('Unknown/Zone')).toBe('Unknown/Zone')
  })
})
