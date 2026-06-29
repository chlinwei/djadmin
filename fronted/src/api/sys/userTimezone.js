import requestUtil from '@/util/request'

/**
 * 获取当前用户信息（包含时区）
 */
export function getCurrentUserInfo() {
    return requestUtil.get('sys/users/current/')
}

/**
 * 更新用户时区
 * @param {number} userId - 用户ID
 * @param {string} timezone - 新的时区值
 */
export function updateUserTimezone(userId, timezone) {
    return requestUtil.patch(`sys/users/${userId}/`, {
        timezone
    })
}
