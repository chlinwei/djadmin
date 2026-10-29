import requestUtil from '@/util/request'

/**
 * 获取当前用户信息（包含时区）
 */
export function getCurrentUserInfo() {
    return requestUtil.get('sys/users/current/')
}
