import dayjs from 'dayjs'
import {
  convertUTCToTimezone,
  formatTimeWithTimezone,
  getTimezoneLabel,
  getTimezoneOffset,
  getTimezoneOptions,
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

  // 日志查询要显示到毫秒：SSS 占位符取 Date.getMilliseconds()，且不受时区影响。
  it('supports millisecond placeholder SSS', () => {
    expect(formatTimeWithTimezone('2026-01-01T00:00:00.635Z', 'Asia/Shanghai', 'YYYY-MM-DD HH:mm:ss.SSS'))
      .toBe('2026-01-01 08:00:00.635')
    expect(formatTimeWithTimezone('2026-01-01T00:00:00Z', 'UTC', 'YYYY-MM-DD HH:mm:ss.SSS'))
      .toBe('2026-01-01 00:00:00.000')
  })

  it('formats Dayjs picker values in the requested timezone without errors', () => {
    const errorSpy = vi.spyOn(console, 'error')
    const value = dayjs('2026-01-01T00:00:00.635Z')
    expect(formatTimeWithTimezone(value, 'Asia/Shanghai')).toBe('2026-01-01 08:00:00')
    expect(formatTimeWithTimezone(value, 'Asia/Shanghai', 'YYYY-MM-DD HH:mm:ss.SSS'))
      .toBe('2026-01-01 08:00:00.635')
    expect(errorSpy).not.toHaveBeenCalled()
    errorSpy.mockRestore()
  })

  it('returns a placeholder for invalid time input without using the browser timezone', () => {
    const errorSpy = vi.spyOn(console, 'error').mockImplementation(() => {})
    const result = formatTimeWithTimezone('invalid-time-value', 'Asia/Shanghai')

    expect(result).toBe('-')
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
    expect(getTimezoneLabel('Asia/Shanghai')).toBe('UTC+08:00 · Asia/Shanghai')
  })

  it('reports an invalid timezone instead of treating its offset as UTC', () => {
    const errorSpy = vi.spyOn(console, 'error').mockImplementation(() => {})
    expect(() => getTimezoneOffset('Unknown/Zone')).toThrow(RangeError)
    expect(getTimezoneLabel('Unknown/Zone')).toBe('Unknown/Zone')
    expect(errorSpy).toHaveBeenCalled()
    errorSpy.mockRestore()
  })

  it.each([
    ['America/New_York', -5, -4],
    ['Europe/London', 0, 1],
    ['Australia/Sydney', 11, 10],
  ])('uses the date-specific DST offset for %s', (zone, winter, summer) => {
    expect(getTimezoneOffset(zone, new Date('2026-01-15T12:00:00Z'))).toBe(winter)
    expect(getTimezoneOffset(zone, new Date('2026-07-15T12:00:00Z'))).toBe(summer)
  })

  it.each([
    ['UTC', 0, 'UTC+00:00'],
    ['Asia/Kolkata', 5.5, 'UTC+05:30'],
    ['Asia/Kathmandu', 5.75, 'UTC+05:45'],
    ['America/St_Johns', -3.5, 'UTC-03:30'],
    ['Etc/GMT+5', -5, 'UTC-05:00'],
  ])('formats fractional and fixed offsets for %s', (zone, offset, prefix) => {
    const date = new Date('2026-01-15T12:00:00.635Z')
    expect(getTimezoneOffset(zone, date)).toBe(offset)
    expect(getTimezoneLabel(zone, date)).toBe(`${prefix} · ${zone}`)
  })

  it('does not shift the offset at local midnight or a date boundary', () => {
    expect(getTimezoneOffset('Asia/Shanghai', new Date('2026-01-01T16:00:00Z'))).toBe(8)
    expect(getTimezoneOffset('America/New_York', new Date('2026-01-01T01:00:00Z'))).toBe(-5)
  })

  it('lists every runtime IANA zone plus UTC without merging cities', () => {
    const enumerated = Intl.supportedValuesOf('timeZone')
    const options = getTimezoneOptions('Asia/Shanghai', new Date('2026-07-15T12:00:00Z'))
    const values = options.map(option => option.value)
    expect(values.length).toBeGreaterThan(300)
    expect(new Set(values)).toEqual(new Set(['UTC', ...enumerated, 'Asia/Shanghai']))
    expect(new Set(values).size).toBe(values.length)
    expect(values).toEqual(expect.arrayContaining(['Asia/Shanghai', 'Asia/Hong_Kong', 'Asia/Singapore', 'Asia/Tokyo', 'Asia/Seoul']))
    expect(options.find(option => option.value === 'America/New_York').label).toBe('UTC-04:00 · America/New_York')
  })

  it('preserves a valid saved identifier absent from the runtime enumeration', () => {
    const spy = vi.spyOn(Intl, 'supportedValuesOf').mockReturnValue(['Asia/Shanghai'])
    try {
      const options = getTimezoneOptions('Asia/Kolkata')
      expect(options.map(option => option.value)).toEqual(['Asia/Kolkata', 'Asia/Shanghai', 'UTC'])
      expect(() => getTimezoneOptions('Unknown/Zone')).toThrow(RangeError)
    } finally {
      spy.mockRestore()
    }
  })

  it('fails explicitly when timezone enumeration is unsupported', () => {
    vi.stubGlobal('Intl', { supportedValuesOf: undefined })
    try {
      expect(() => getTimezoneOptions('UTC')).toThrow('当前浏览器不支持时区枚举')
    } finally {
      vi.unstubAllGlobals()
    }
  })
})
