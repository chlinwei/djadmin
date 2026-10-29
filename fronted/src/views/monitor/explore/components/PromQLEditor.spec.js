import { mount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { PromQLExtension } from '@prometheus-io/codemirror-promql'
import PromQLEditor from './PromQLEditor.vue'

vi.mock('@prometheus-io/codemirror-promql', () => ({
  PromQLExtension: vi.fn().mockImplementation(() => ({
    setComplete: vi.fn().mockReturnThis(),
    asExtension: () => [],
  })),
}))

describe('PromQL 自动补全查询', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
    localStorage.removeItem('token')
    vi.clearAllMocks()
  })

  it('通过 SDK 配置使用 GET，保留鉴权和 URL 归一化', async () => {
    const fetch = vi.fn().mockResolvedValue({})
    vi.stubGlobal('fetch', fetch)
    localStorage.setItem('token', 'test-token')
    const wrapper = mount(PromQLEditor, {
      props: { completionRemoteUrl: '/monitor/targets/prometheus/proxy/' },
    })
    try {
      const extension = PromQLExtension.mock.results[0].value
      const config = extension.setComplete.mock.calls[0][0]
      expect(config.remote.httpMethod).toBe('GET')
      await config.remote.fetchFn('/monitor/targets/prometheus/proxy//api/v1/labels?match%5B%5D=up', { method: 'GET' })
      expect(fetch).toHaveBeenCalledWith('/monitor/targets/prometheus/proxy/api/v1/labels?match%5B%5D=up', expect.objectContaining({
        method: 'GET', credentials: 'same-origin',
      }))
      const options = fetch.mock.calls[0][1]
      expect(options.headers.get('AUTHORIZATION')).toBe('test-token')
      expect(options.body).toBeUndefined()
    } finally {
      wrapper.unmount()
    }
  })
})
