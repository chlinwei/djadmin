export function getTimezoneOptions(selectedTimezone, date = new Date()) {
    if (typeof Intl.supportedValuesOf !== 'function') {
        throw new Error('当前浏览器不支持时区枚举，请升级浏览器')
    }
    const zones = new Set(['UTC', ...Intl.supportedValuesOf('timeZone')])
    // 枚举不包含全部别名；保存的有效 IANA 标识必须原样回显，不能自动改写用户设置。
    if (selectedTimezone) {
        new Intl.DateTimeFormat(undefined, { timeZone: selectedTimezone })
        zones.add(selectedTimezone)
    }
    return [...zones].sort().map(value => ({
        value,
        label: formatTimezoneLabel(value, date),
    }))
}

/**
 * 将UTC时间转换为指定时区的本地时间
 * @param {Date|string|number} utcTime - UTC时间 (Date对象、ISO字符串或时间戳)
 * @param {string} timezone - 时区标识符 (e.g., 'UTC', 'Asia/Shanghai')
 * @returns {Date} 转换后的本地时间
 */
export function convertUTCToTimezone(utcTime, timezone = 'UTC') {
    try {
        // 如果是字符串或数字，转换为Date对象
        let date
        if (typeof utcTime === 'string' || typeof utcTime === 'number') {
            date = new Date(utcTime)
        } else {
            date = utcTime
        }

        // 使用 Intl API 格式化时间（支持时区）
        return date
    } catch (error) {
        console.error('时区转换错误:', error)
        return new Date(utcTime)
    }
}

/**
 * 格式化时间为指定时区的字符串
 * @param {Date|string|number} utcTime - UTC时间
 * @param {string} timezone - 时区标识符
 * @param {string} format - 格式化格式 (默认: 'YYYY-MM-DD HH:mm:ss')
 * @returns {string} 格式化后的时间字符串
 *
 * 支持 `SSS` 毫秒占位符（日志查询要显示到毫秒）。毫秒与时区无关（时区偏移都是整分钟），
 * 所以直接取 `Date.getMilliseconds()`；Intl 的 formatToParts 拿不到毫秒。
 */
export function formatTimeWithTimezone(utcTime, timezone = 'UTC', format = 'YYYY-MM-DD HH:mm:ss') {
    try {
        // 日期选择器返回 Dayjs；先归一为 Date，毫秒读取与 Intl 才使用同一个时间值。
        const date = typeof utcTime?.toDate === 'function'
            ? utcTime.toDate()
            : new Date(utcTime)

        // 使用 Intl API 获取特定时区的时间
        const formatter = new Intl.DateTimeFormat('zh-CN', {
            timeZone: timezone,
            year: 'numeric',
            month: '2-digit',
            day: '2-digit',
            hour: '2-digit',
            minute: '2-digit',
            second: '2-digit',
            hour12: false
        })

        const parts = formatter.formatToParts(date)
        const values = {}
        parts.forEach(part => {
            values[part.type] = part.value
        })

        // 根据格式字符串组合
        let result = format
        result = result.replace('YYYY', values.year)
        result = result.replace('MM', values.month)
        result = result.replace('DD', values.day)
        result = result.replace('HH', values.hour)
        result = result.replace('mm', values.minute)
        result = result.replace('ss', values.second)
        result = result.replace('SSS', String(date.getMilliseconds()).padStart(3, '0'))

        return result
    } catch (error) {
        console.error('时间格式化错误:', error)
        return '-'
    }
}

/**
 * 将日期对象转换为 UTC ISO 字符串，供后端查询参数使用。
 * 仅返回标准 ISO（含 Z），避免后端把无时区字符串误判为 UTC。
 * @param {any} value - dayjs/moment/Date
 * @returns {string|undefined}
 */
export function toUtcQueryISOString(value) {
    if (!value) {
        return undefined
    }

    if (typeof value?.toDate === 'function') {
        return value.toDate().toISOString()
    }

    if (value instanceof Date) {
        return value.toISOString()
    }

    return undefined
}

/**
 * 获取时区的偏移小时数
 * @param {string} timezone - 时区标识符
 * @param {Date} date - 偏移计算时刻，包含该时刻的夏令时规则
 * @returns {number} 相对于UTC的偏移小时数
 */
export function getTimezoneOffset(timezone, date = new Date()) {
    // 数值字段不依赖浏览器语言；h23 避免午夜被输出为次日 24 点。
    const parts = new Intl.DateTimeFormat(undefined, {
        timeZone: timezone,
        calendar: 'iso8601',
        numberingSystem: 'latn',
        hourCycle: 'h23',
        year: 'numeric',
        month: '2-digit',
        day: '2-digit',
        hour: '2-digit',
        minute: '2-digit',
        second: '2-digit',
    }).formatToParts(date)
    const values = Object.fromEntries(parts.map(part => [part.type, Number(part.value)]))
    const localAsUtc = Date.UTC(values.year, values.month - 1, values.day, values.hour, values.minute, values.second)
    return (localAsUtc - Math.floor(date.getTime() / 1000) * 1000) / 3600000
}

function formatTimezoneLabel(timezone, date) {
    const minutes = Math.round(getTimezoneOffset(timezone, date) * 60)
    const sign = minutes < 0 ? '-' : '+'
    const hoursText = String(Math.floor(Math.abs(minutes) / 60)).padStart(2, '0')
    const minutesText = String(Math.abs(minutes) % 60).padStart(2, '0')
    return `UTC${sign}${hoursText}:${minutesText} · ${timezone}`
}

/**
 * 获取时区显示标签
 * @param {string} timezone - 时区标识符
 * @returns {string} 时区标签
 */
export function getTimezoneLabel(timezone, date = new Date()) {
    try {
        return formatTimezoneLabel(timezone, date)
    } catch (error) {
        if (!(error instanceof RangeError)) {
            throw error
        }
        console.error('时区标签生成失败:', error)
        return timezone
    }
}
