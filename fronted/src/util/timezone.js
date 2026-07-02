// 常见时区列表
export const TIMEZONE_LIST = [
    { label: 'UTC (协调世界时)', value: 'UTC', offset: 0 },
    { label: 'GMT+8 (北京/香港/新加坡)', value: 'Asia/Shanghai', offset: 8 },
    { label: 'GMT+9 (东京/首尔)', value: 'Asia/Tokyo', offset: 9 },
    { label: 'GMT+5:30 (印度标准时)', value: 'Asia/Kolkata', offset: 5.5 },
    { label: 'GMT+8 (澳大利亚/珀斯)', value: 'Australia/Perth', offset: 8 },
    { label: 'GMT+10 (澳大利亚/悉尼)', value: 'Australia/Sydney', offset: 10 },
    { label: 'GMT+0 (伦敦)', value: 'Europe/London', offset: 0 },
    { label: 'GMT+1 (柏林/巴黎)', value: 'Europe/Berlin', offset: 1 },
    { label: 'GMT-5 (纽约)', value: 'America/New_York', offset: -5 },
    { label: 'GMT-8 (洛杉矶)', value: 'America/Los_Angeles', offset: -8 },
    { label: 'GMT-6 (芝加哥)', value: 'America/Chicago', offset: -6 },
    { label: 'GMT-3 (圣保罗)', value: 'America/Sao_Paulo', offset: -3 },
]

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
 */
export function formatTimeWithTimezone(utcTime, timezone = 'UTC', format = 'YYYY-MM-DD HH:mm:ss') {
    try {
        let date
        if (typeof utcTime === 'string' || typeof utcTime === 'number') {
            date = new Date(utcTime)
        } else {
            date = utcTime
        }

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

        return result
    } catch (error) {
        console.error('时间格式化错误:', error)
        return new Date(utcTime).toLocaleString()
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
 * @returns {number} 相对于UTC的偏移小时数
 */
export function getTimezoneOffset(timezone) {
    const found = TIMEZONE_LIST.find(tz => tz.value === timezone)
    return found ? found.offset : 0
}

/**
 * 获取时区显示标签
 * @param {string} timezone - 时区标识符
 * @returns {string} 时区标签
 */
export function getTimezoneLabel(timezone) {
    const found = TIMEZONE_LIST.find(tz => tz.value === timezone)
    return found ? found.label : timezone
}
