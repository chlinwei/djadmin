import { beforeEach, describe, expect, it, vi } from 'vitest'
import requestUtil from '@/util/request'
import { getApplicationServiceLogStatusSummary } from './index'

vi.mock('@/util/request', () => ({
    default: { get: vi.fn(), post: vi.fn() },
}))

describe('服务日志状态汇总查询', () => {
    beforeEach(() => vi.clearAllMocks())

    it('通过 GET 查询参数传递服务 ID，不创建变更请求', async () => {
        const response = { data: { code: 200, data: { items: [] } } }
        requestUtil.get.mockResolvedValue(response)
        expect(await getApplicationServiceLogStatusSummary([15, 16])).toBe(response)
        expect(requestUtil.get).toHaveBeenCalledWith(
            'assets/application-services/log-status-summary/',
            { service_ids: '15,16' },
        )
        expect(requestUtil.post).not.toHaveBeenCalled()
    })
})
