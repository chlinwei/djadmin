import { flushPromises, mount } from '@vue/test-utils'
import Antd from 'ant-design-vue'
import { describe, expect, it, vi } from 'vitest'

import UserNotificationChain from '../UserNotificationChain.vue'

const getUserNotificationChain = vi.fn()

vi.mock('@/api/monitor', () => ({
  getUserNotificationChain: (...args) => getUserNotificationChain(...args),
}))

function mountChain(props = {}) {
  return mount(UserNotificationChain, {
    props,
    global: { plugins: [Antd] },
  })
}

describe('UserNotificationChain', () => {
  it('can_receive=false 时展示红色横幅和 summary_issues，并在绑定/路由旁展示 issue tag', async () => {
    getUserNotificationChain.mockResolvedValue({
      data: {
        data: {
          user: { id: 3, username: 'op' },
          can_receive: false,
          summary_issues: ['未绑定任何告警媒介', '警告：媒介 email 已禁用'],
          bindings: [
            {
              binding_id: 11,
              enabled: false,
              recipients: ['op@example.com'],
              media: { id: 2, name: 'email', media_type: 'email', enabled: true },
              issues: ['该绑定已禁用'],
              routes: [
                {
                  route_id: 5,
                  name: ' critical 路由',
                  enabled: true,
                  notify_on_firing: true,
                  notify_on_resolved: false,
                  matchers: 'severity="critical"',
                  issues: ['该路由不会在恢复时通知'],
                },
              ],
            },
          ],
        },
      },
    })

    const wrapper = mountChain({ userId: 3 })
    await flushPromises()

    // 管理员查看指定用户时应带上 user_id 参数
    expect(getUserNotificationChain).toHaveBeenCalledWith(3)

    const text = wrapper.text()
    expect(text).toContain('你当前不会收到告警通知')
    expect(text).toContain('未绑定任何告警媒介')
    expect(text).toContain('警告：媒介 email 已禁用')
    expect(text).toContain('该绑定已禁用')
    expect(text).toContain('该路由不会在恢复时通知')
    expect(text).toContain('email')
    expect(text).toContain('op@example.com')
    expect(text).toContain('critical 路由')
  })

  it('can_receive=true 时展示绿色横幅且不调用链路问题列表', async () => {
    getUserNotificationChain.mockResolvedValue({
      data: {
        data: {
          can_receive: true,
          summary_issues: [],
          bindings: [
            {
              binding_id: 1,
              enabled: true,
              recipients: ['a@b.com'],
              media: { id: 2, name: 'email', media_type: 'email', enabled: true },
              issues: [],
              routes: [],
            },
          ],
        },
      },
    })

    const wrapper = mountChain()
    await flushPromises()

    // 不传 userId 时以默认值 null 调用（组件内部不拼 user_id 参数，后端按当前登录用户处理）
    expect(getUserNotificationChain).toHaveBeenCalledWith(null)
    expect(wrapper.text()).toContain('你当前会收到告警通知')
    expect(wrapper.text()).not.toContain('不会收到告警通知')
  })
})
