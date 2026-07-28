import dayjs from 'dayjs'
import utc from 'dayjs/plugin/utc'
import timezone from 'dayjs/plugin/timezone'
import { toUtcQueryISOString } from '@/util/timezone'

dayjs.extend(utc)
dayjs.extend(timezone)

function resolveTimezone(timezoneName) {
  return String(timezoneName || 'Asia/Shanghai')
}

export function buildUserTimezoneShowTime(timezoneName) {
  const tz = resolveTimezone(timezoneName)
  const now = dayjs().tz(tz)
  return {
    defaultValue: [now.startOf('day'), now.endOf('day')],
    defaultOpenValue: [now.startOf('day'), now.endOf('day')],
  }
}

export function buildUserTimezoneRangePresets(timezoneName) {
  const tz = resolveTimezone(timezoneName)
  const now = dayjs().tz(tz)
  return [
    { label: '过去 5 分钟', value: [now.subtract(5, 'minute'), now] },
    { label: '过去 15 分钟', value: [now.subtract(15, 'minute'), now] },
    { label: '过去 30 分钟', value: [now.subtract(30, 'minute'), now] },
    { label: '过去 1 小时', value: [now.subtract(1, 'hour'), now] },
    { label: '过去 3 小时', value: [now.subtract(3, 'hour'), now] },
    { label: '过去 6 小时', value: [now.subtract(6, 'hour'), now] },
    { label: '过去 12 小时', value: [now.subtract(12, 'hour'), now] },
    { label: '过去 1 天', value: [now.subtract(1, 'day'), now] },
    { label: '过去 2 天', value: [now.subtract(2, 'day'), now] },
    { label: '过去 7 天', value: [now.subtract(7, 'day'), now] },
    { label: '过去 30 天', value: [now.subtract(30, 'day'), now] },
    { label: '过去 3 个月', value: [now.subtract(3, 'month'), now] },
    { label: '过去 6 个月', value: [now.subtract(6, 'month'), now] },
    { label: '过去 1 年', value: [now.subtract(1, 'year'), now] },
  ]
}

export function toUtcQueryISOStringByUserTimezone(value, timezoneName) {
  if (!value) {
    return undefined
  }

  const tz = resolveTimezone(timezoneName)

  if (dayjs.isDayjs(value)) {
    return value.tz(tz, true).utc().toISOString()
  }

  const parsed = new Date(value)
  if (Number.isNaN(parsed.getTime())) {
    return toUtcQueryISOString(value)
  }

  return dayjs(parsed).tz(tz, true).utc().toISOString()
}
