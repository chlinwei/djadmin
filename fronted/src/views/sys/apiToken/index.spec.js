import { shallowMount } from '@vue/test-utils'
import Antd from 'ant-design-vue'
import { expect, it, vi } from 'vitest'
import ApiTokenPage from './index.vue'

vi.mock('@/store', () => ({ default: { state: { user: { timezone: 'Asia/Kuala_Lumpur' } } } }))
vi.mock('@/api/user/apiToken', () => ({
  getApiTokenList: vi.fn().mockResolvedValue({ data: { data: { results: [] } } }),
  createApiToken: vi.fn(),
  deleteApiToken: vi.fn(),
  disableApiToken: vi.fn(),
  rotateApiToken: vi.fn(),
}))

it('未使用的 Token 不触发日期错误，有效时间按当前用户时区显示', () => {
  const reportError = vi.spyOn(console, 'error')
  const wrapper = shallowMount(ApiTokenPage, {
    global: {
      plugins: [Antd],
      directives: { permission: () => {} },
      stubs: { FontAwesomeIcon: true },
    },
  })
  try {
    expect(wrapper.vm.formatDateTime({ Time: '0001-01-01T00:00:00Z', Valid: false })).toBe('-')
    expect(wrapper.vm.formatDateTime(null)).toBe('-')
    expect(wrapper.vm.formatDateTime({ Time: '2026-09-01T05:49:37Z', Valid: true })).toBe('2026-09-01 13:49:37')
    expect(wrapper.vm.formatDateTime('2026-09-01T05:49:37Z')).toBe('2026-09-01 13:49:37')
    expect(reportError).not.toHaveBeenCalled()
  } finally {
    wrapper.unmount()
    reportError.mockRestore()
  }
})
