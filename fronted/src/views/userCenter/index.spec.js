import { flushPromises, mount } from '@vue/test-utils'
import Antd from 'ant-design-vue'
import { afterEach, describe, expect, it, vi } from 'vitest'

// 「个人信息」侧栏只放**真实数据**（2026-09-20 现场）：
// 这里原先有一行「告警媒介 → 在右侧"关联告警媒介"中选择」——把一句操作指引当成只读信息栏的"值"，
// 既不准确（真正的选择在右侧「告警媒介」tab 的弹窗里）也没有信息量。这条用例钉住"那行别再回来"。
vi.mock('@/api/user', () => ({
  getCurrentUser: vi.fn(() => Promise.resolve({ data: { data: { user: {
    id: 3, username: 'op', phonenumber: '13800000000', create_time: '2026-01-01T00:00:00Z',
  } } } })),
  getCurrentUserAlertMediaBindings: vi.fn(() => Promise.resolve({ data: { data: { results: [] } } })),
  saveCurrentUser: vi.fn(),
  updateUserInfo: vi.fn(),
  updateUserPassword: vi.fn(),
}))
vi.mock('@/api/role', () => ({
  // 页面读的是 roleList（不是分页的 results）——mock 成 results 会让 onMounted 里 forEach 报错。
  getCurrentUserRoleList: vi.fn(() => Promise.resolve({ data: { data: { roleList: [{ name: '运维' }] } } })),
}))
vi.mock('@/api/sys/userTimezone', () => ({
  getCurrentUserInfo: vi.fn(() => Promise.resolve({ data: { data: { timezone: 'Asia/Shanghai' } } })),
  updateUserTimezone: vi.fn(),
}))
vi.mock('@/api/monitor', () => ({
  getUserNotificationChain: vi.fn(() => Promise.resolve({ data: { data: { can_receive: true, bindings: [] } } })),
}))
vi.mock('@/util/deleteConfirm', () => ({ openDeleteConfirm: vi.fn(() => Promise.resolve(false)) }))
vi.mock('@/store', () => ({ default: { state: { user: { timezone: 'Asia/Shanghai' } }, commit: vi.fn(), dispatch: vi.fn() } }))

const originalGetComputedStyle = window.getComputedStyle.bind(window)
window.getComputedStyle = (element) => originalGetComputedStyle(element)

import UserCenter from './index.vue'

async function mountPage() {
  const wrapper = mount(UserCenter, {
    attachTo: document.body,
    global: { plugins: [Antd], stubs: { FontAwesomeIcon: true, SvgIcon: true, Avatar: true, UserNotificationChain: true } },
  })
  await flushPromises()
  return wrapper
}

describe('个人中心：个人信息侧栏', () => {
  afterEach(() => {
    document.body.innerHTML = ''
    vi.clearAllMocks()
  })

  it('lists real profile fields and no longer shows the media guidance text', async () => {
    const wrapper = await mountPage()

    const sidebar = wrapper.find('.profile-summary').text()
    // 真实数据照旧在。
    expect(sidebar).toContain('用户名称')
    expect(sidebar).toContain('电话')
    expect(sidebar).toContain('角色')
    expect(sidebar).toContain('创建时间')
    // 那句指引必须已经不在了（它曾经出现在"告警媒介"这一行的值里）。
    expect(sidebar).not.toContain('关联告警媒介')
    expect(document.body.textContent).not.toContain('在右侧')
    // 媒介的绑定入口仍在「告警媒介」tab 里（不是被误删掉了功能）——tab 内容懒挂载，切过去看。
    wrapper.vm.activeKey = '3'
    await flushPromises()
    expect(document.body.textContent).toContain('添加媒介绑定')
    wrapper.unmount()
  })
})
