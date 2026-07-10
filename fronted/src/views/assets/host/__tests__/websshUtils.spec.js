import { describe, it, expect, vi, beforeEach } from 'vitest'

// mock timezone util — 以固定格式验证入参，避免受本地时区影响
vi.mock('@/util/timezone', () => ({
    formatTimeWithTimezone: vi.fn((utcStr, tz, fmt) => `formatted:${utcStr}|${tz}|${fmt}`),
}))

import {
    normalizeUtcTime,
    formatFileSize,
    resolveFileParentDirectory,
    formatDateTime,
    formatFileMtime,
} from '../websshUtils.js'

// ─── normalizeUtcTime ────────────────────────────────────────────────────────

describe('normalizeUtcTime', () => {
    it('非字符串原样返回', () => {
        expect(normalizeUtcTime(null)).toBeNull()
        expect(normalizeUtcTime(undefined)).toBeUndefined()
        expect(normalizeUtcTime(12345)).toBe(12345)
    })

    it('空字符串原样返回', () => {
        expect(normalizeUtcTime('')).toBe('')
        expect(normalizeUtcTime('   ')).toBe('   ')
    })

    it('已带 Z 后缀的字符串原样返回', () => {
        expect(normalizeUtcTime('2026-01-01T00:00:00Z')).toBe('2026-01-01T00:00:00Z')
        expect(normalizeUtcTime('2026-01-01T00:00:00z')).toBe('2026-01-01T00:00:00z')
    })

    it('已带时区偏移的字符串原样返回', () => {
        expect(normalizeUtcTime('2026-01-01T08:00:00+08:00')).toBe('2026-01-01T08:00:00+08:00')
        expect(normalizeUtcTime('2026-01-01T00:00:00-05:00')).toBe('2026-01-01T00:00:00-05:00')
    })

    it('裸日期时间字符串（空格分隔）补全为 ISO+Z', () => {
        expect(normalizeUtcTime('2026-01-01 00:00:00')).toBe('2026-01-01T00:00:00Z')
    })

    it('裸 ISO 字符串（无时区）补全 Z 后缀', () => {
        expect(normalizeUtcTime('2026-06-15T12:30:00')).toBe('2026-06-15T12:30:00Z')
    })
})

// ─── formatFileSize ──────────────────────────────────────────────────────────

describe('formatFileSize', () => {
    it('null / undefined 返回 -', () => {
        expect(formatFileSize(null)).toBe('-')
        expect(formatFileSize(undefined)).toBe('-')
    })

    it('负数返回 -', () => {
        expect(formatFileSize(-1)).toBe('-')
    })

    it('NaN / Infinity 返回 -', () => {
        expect(formatFileSize(NaN)).toBe('-')
        expect(formatFileSize(Infinity)).toBe('-')
    })

    it('0 字节', () => {
        expect(formatFileSize(0)).toBe('0 B')
    })

    it('小于 1 KB 显示 B', () => {
        expect(formatFileSize(512)).toBe('512 B')
        expect(formatFileSize(1023)).toBe('1023 B')
    })

    it('1 KB 边界', () => {
        expect(formatFileSize(1024)).toBe('1.0 KB')
    })

    it('KB 范围', () => {
        expect(formatFileSize(1536)).toBe('1.5 KB')
    })

    it('1 MB 边界', () => {
        expect(formatFileSize(1024 * 1024)).toBe('1.0 MB')
    })

    it('MB 范围', () => {
        expect(formatFileSize(1024 * 1024 * 2.5)).toBe('2.5 MB')
    })

    it('1 GB 边界', () => {
        expect(formatFileSize(1024 * 1024 * 1024)).toBe('1.0 GB')
    })

    it('GB 范围', () => {
        expect(formatFileSize(1024 * 1024 * 1024 * 3)).toBe('3.0 GB')
    })

    it('字符串数字可正确解析', () => {
        expect(formatFileSize('2048')).toBe('2.0 KB')
    })
})

// ─── resolveFileParentDirectory ──────────────────────────────────────────────

describe('resolveFileParentDirectory', () => {
    it('null / undefined / 空字符串 返回 .', () => {
        expect(resolveFileParentDirectory(null)).toBe('.')
        expect(resolveFileParentDirectory(undefined)).toBe('.')
        expect(resolveFileParentDirectory('')).toBe('.')
    })

    it('. 返回 .', () => {
        expect(resolveFileParentDirectory('.')).toBe('.')
    })

    it('根目录 / 返回 /', () => {
        expect(resolveFileParentDirectory('/')).toBe('/')
    })

    it('一级路径返回 /', () => {
        expect(resolveFileParentDirectory('/home')).toBe('/')
        expect(resolveFileParentDirectory('/etc')).toBe('/')
    })

    it('多级路径返回父级', () => {
        expect(resolveFileParentDirectory('/home/esb')).toBe('/home')
        expect(resolveFileParentDirectory('/home/esb/files')).toBe('/home/esb')
    })

    it('末尾带斜杠也能正确解析', () => {
        expect(resolveFileParentDirectory('/home/esb/')).toBe('/home')
    })

    it('无斜杠的相对路径返回 .', () => {
        expect(resolveFileParentDirectory('somefile')).toBe('.')
    })

    it('相对路径正常返回父级', () => {
        expect(resolveFileParentDirectory('dir/subdir')).toBe('dir')
    })
})

// ─── formatDateTime ──────────────────────────────────────────────────────────

describe('formatDateTime', () => {
    beforeEach(() => {
        vi.clearAllMocks()
    })

    it('空值返回 -', () => {
        expect(formatDateTime(null)).toBe('-')
        expect(formatDateTime(undefined)).toBe('-')
        expect(formatDateTime('')).toBe('-')
    })

    it('使用传入的时区调用 formatTimeWithTimezone', () => {
        const result = formatDateTime('2026-01-01 00:00:00', 'Asia/Tokyo')
        expect(result).toBe('formatted:2026-01-01T00:00:00Z|Asia/Tokyo|YYYY-MM-DD HH:mm:ss')
    })

    it('未传时区时使用默认 Asia/Shanghai', () => {
        const result = formatDateTime('2026-01-01T00:00:00Z')
        expect(result).toContain('Asia/Shanghai')
    })
})

// ─── formatFileMtime ─────────────────────────────────────────────────────────

describe('formatFileMtime', () => {
    beforeEach(() => {
        vi.clearAllMocks()
    })

    it('null / undefined 返回 -', () => {
        expect(formatFileMtime(null)).toBe('-')
        expect(formatFileMtime(undefined)).toBe('-')
    })

    it('0 不应返回 - (0 是合法时间戳)', () => {
        // Unix epoch 0 是 1970-01-01T00:00:00Z，应调用 formatTimeWithTimezone
        const result = formatFileMtime(0, 'UTC')
        expect(result).not.toBe('-')
        expect(result).toContain('1970-01-01T00:00:00.000Z')
    })

    it('将 Unix 秒级时间戳转换为 ISO 后传给 formatTimeWithTimezone', () => {
        // 1720396800 = 2024-07-08T00:00:00Z
        const result = formatFileMtime(1720396800, 'Asia/Shanghai')
        expect(result).toBe('formatted:2024-07-08T00:00:00.000Z|Asia/Shanghai|YYYY-MM-DD HH:mm:ss')
    })

    it('字符串时间戳也能正确处理', () => {
        const result = formatFileMtime('1720396800', 'UTC')
        expect(result).toContain('2024-07-08T00:00:00.000Z')
    })
})
