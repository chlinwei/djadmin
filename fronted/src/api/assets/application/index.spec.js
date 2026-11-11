import { beforeEach, describe, expect, it, vi } from 'vitest'
import requestUtil from '@/util/request'
import { getApplicationServiceLogStatusSummary, refreshApplicationDeploymentRuntimeStatus, saveApplicationService, saveApplicationDeploymentTemplate } from './index'

vi.mock('@/util/request', () => ({
    default: { get: vi.fn(), post: vi.fn(), patch: vi.fn() },
}))

describe('当前范围实例状态检查', () => {
    beforeEach(() => vi.clearAllMocks())

    it('仅通过 GET URL 参数提交实例 ID 并保留超时和错误提示归属', async () => {
        const options = { suppressBusinessErrorMessage: true }
        await refreshApplicationDeploymentRuntimeStatus([60, 61], 120000, options)
        expect(requestUtil.get).toHaveBeenCalledWith(
            'assets/application-deployments/refresh-runtime-status/',
            { ids: '60,61' }, 120000, options,
        )
        expect(requestUtil.post).not.toHaveBeenCalled()
    })

    it('默认 HTTP 等待时间大于 Agent 执行和后端等待回包的期限', async () => {
        await refreshApplicationDeploymentRuntimeStatus([60])
        expect(requestUtil.get).toHaveBeenCalledWith(
            'assets/application-deployments/refresh-runtime-status/',
            { ids: '60' }, 150000, {},
        )
    })
})

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

describe('部署模板保存错误提示归属', () => {
    beforeEach(() => vi.clearAllMocks())

    it.each([null, 20])('保存 ID %s 的模板由调用方提示业务错误且保留失败拒绝', async (id) => {
        const payload = { id, name: 'template' }
        const failure = new Error('日志采集配置不自洽，已拒绝保存')
        const request = id ? requestUtil.patch : requestUtil.post
        request.mockRejectedValueOnce(failure)
        await expect(saveApplicationDeploymentTemplate(payload)).rejects.toBe(failure)
        if (id) {
            expect(request).toHaveBeenCalledWith('assets/application-deployment-templates/20/', payload, { suppressBusinessErrorMessage: true })
        } else {
            expect(request).toHaveBeenCalledWith('assets/application-deployment-templates/', payload, null, { suppressBusinessErrorMessage: true })
        }
    })
})

describe('逻辑服务保存错误提示归属', () => {
    beforeEach(() => vi.clearAllMocks())

    it.each([null, 20])('保存 ID %s 的服务由调用方提示业务错误且保留失败拒绝', async (id) => {
        const payload = { id, name: 'db' }
        const failure = new Error('同一业务系统与环境下的服务名称已存在')
        const request = id ? requestUtil.patch : requestUtil.post
        request.mockRejectedValueOnce(failure)
        await expect(saveApplicationService(payload)).rejects.toBe(failure)
        if (id) {
            expect(request).toHaveBeenCalledWith('assets/application-services/20/', payload, { suppressBusinessErrorMessage: true })
        } else {
            expect(request).toHaveBeenCalledWith('assets/application-services/', payload, null, { suppressBusinessErrorMessage: true })
        }
    })
})
