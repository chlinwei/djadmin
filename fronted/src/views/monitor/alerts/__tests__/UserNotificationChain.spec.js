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
  it('can_receive=false 时展示红色横幅和 summary_issues，并展示关联策略与绑定问题', async () => {
    getUserNotificationChain.mockResolvedValue({
      data: {
        data: {
          user: { id: 3, username: 'op' },
          can_receive: false,
          summary_issues: ['媒介绑定「email」：该绑定已禁用', '警告：媒介 email 已禁用'],
          bindings: [
            {
              binding_id: 11,
              enabled: false,
              recipients: ['op@example.com'],
              media: { id: 2, name: 'email', media_type: 'email', enabled: true },
              issues: ['该绑定已禁用'],
              policies: [
                {
                  id: 5,
                  name: 'critical 路由',
                  path: '默认策略 / critical',
                  notify_on_firing: true,
                  notify_on_resolved: false,
                  matchers: [{ type: 'label', label: 'severity', operator: '=', value: 'critical' }],
                  matcher_summary: '标签「severity」等于「critical」',
                  user_group_ids: [7],
                  user_group_names: ['运维组'],
                  user_in_group: true,
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
    expect(text).toContain('媒介绑定「email」：该绑定已禁用')
    expect(text).toContain('警告：媒介 email 已禁用')
    expect(text).toContain('该绑定已禁用')
    expect(text).toContain('email')
    expect(text).toContain('op@example.com')
    expect(text).toContain('critical 路由')
    expect(text).toContain('默认策略 / critical')
    expect(text).toContain('运维组')
    expect(text).not.toContain('当前用户不在组内，收不到')
    expect(text).toContain('resolved 关')
    expect(text).toContain('匹配条件：标签「severity」等于「critical」')
    expect(text).not.toContain('matchers：')
    expect(wrapper.find('.chain-node-title').text()).toBe('媒介绑定：email')
    expect(text).not.toContain('绑定 #11')
    wrapper.unmount()
  })

  it('匹配条件使用实际名称和中文描述，保留上级约束且按纯文本展示', async () => {
    const policies = [
      { id: 1, matchers: [], matcher_summary: '不限' },
      {
        id: 2,
        matchers: [{ type: 'tree', node_type: 'project', id: 1 }],
        matcher_summary: '项目「kul项目」',
      },
      {
        id: 3,
        matchers: [{ type: 'label', label: 'severity', operator: '=', value: 'critical' }],
        matcher_summary: '项目「kul项目」 且 标签「severity」等于「critical」',
      },
      { id: 4, matchers: [], matcher_summary: '项目不存在（原编号：99）' },
      {
        id: 5,
        matchers: [{ type: 'tree', node_type: 'service', id: 2 }],
        matcher_summary: '服务「<b>支付服务</b>」',
      },
    ].map((policy) => ({
      ...policy, name: `策略${policy.id}`, path: `默认策略 / 策略${policy.id}`,
      notify_on_firing: true, notify_on_resolved: true,
      user_group_ids: null, user_group_names: [], user_in_group: true,
    }))
    getUserNotificationChain.mockResolvedValue({
      data: { data: {
        can_receive: true,
        summary_issues: [],
        bindings: [{
          binding_id: 5, enabled: true, recipients: ['first@example.com'],
          media: { id: 20, name: 'wuhan-smtp', media_type: 'email', enabled: true },
          issues: [], policies,
        }],
      } },
    })
    const wrapper = mountChain()
    await flushPromises()
    const descriptions = wrapper.findAll('.chain-matchers')
      .map((item) => item.text()).filter((text) => text.startsWith('匹配条件：'))
    expect(descriptions).toEqual(
      policies.map((policy) => `匹配条件：${policy.matcher_summary}`),
    )
    expect(wrapper.text()).not.toContain('matchers：')
    expect(wrapper.text()).not.toContain('node_type')
    expect(wrapper.text()).toContain('服务「<b>支付服务</b>」')
    expect(wrapper.find('.chain-matchers b').exists()).toBe(false)
    wrapper.unmount()
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
              policies: [],
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
    expect(wrapper.find('.chain-node-title').text()).toBe('媒介绑定：email')
    wrapper.unmount()
  })

  it('多个媒介绑定按媒介名称区分，保留各自的收件地址和状态，不展示内部编号', async () => {
    getUserNotificationChain.mockResolvedValue({
      data: { data: {
        can_receive: false,
        summary_issues: ['媒介绑定「wuhan-smtp」：该绑定已禁用'],
        bindings: [
          {
            binding_id: 5, enabled: false, recipients: ['first@example.com'],
            media: { id: 20, name: 'wuhan-smtp', media_type: 'email', enabled: false },
            issues: ['该绑定已禁用', '媒介已停用'], policies: [],
          },
          {
            binding_id: 42, enabled: true, recipients: ['second@example.com'],
            media: { id: 21, name: 'prod-smtp', media_type: 'email', enabled: true },
            issues: [], policies: [],
          },
        ],
      } },
    })
    const wrapper = mountChain()
    await flushPromises()
    const nodes = wrapper.findAll('.chain-node')
    expect(nodes).toHaveLength(2)
    expect(nodes[0].find('.chain-node-title').text()).toBe('媒介绑定：wuhan-smtp')
    expect(nodes[0].text()).toContain('first@example.com')
    expect(nodes[0].get('button[role="switch"]').attributes('aria-checked')).toBe('false')
    expect(nodes[0].get('button[role="switch"]').attributes('disabled')).toBeDefined()
    expect(nodes[1].find('.chain-node-title').text()).toBe('媒介绑定：prod-smtp')
    expect(nodes[1].text()).toContain('second@example.com')
    expect(nodes[1].get('button[role="switch"]').attributes('aria-checked')).toBe('true')
    expect(nodes[1].get('button[role="switch"]').attributes('disabled')).toBeDefined()
    expect(wrapper.text()).not.toContain('绑定 #')
    expect(wrapper.text()).not.toContain('绑定 5')
    expect(wrapper.text()).not.toContain('绑定 42')
    wrapper.unmount()
  })

  it('无适用策略时提示当前用户无生效策略，不误称媒介全局没有策略出口', async () => {
    getUserNotificationChain.mockResolvedValue({
      data: { data: {
        can_receive: false,
        summary_issues: ['媒介 wuhan-smtp 暂无对当前用户生效的通知策略'],
        bindings: [{
          binding_id: 5, enabled: true, recipients: ['first@example.com'],
          media: { id: 20, name: 'wuhan-smtp', media_type: 'email', enabled: true },
          issues: [], policies: [],
        }],
      } },
    })
    const wrapper = mountChain()
    await flushPromises()
    expect(wrapper.find('.chain-no-route').text()).toBe('暂无对当前用户生效的通知策略')
    expect(wrapper.text()).toContain('媒介 wuhan-smtp 暂无对当前用户生效的通知策略')
    expect(wrapper.text()).not.toContain('未被任何通知策略出口命中')
    expect(wrapper.text()).not.toContain('当前用户不在组内，收不到')
    wrapper.unmount()
  })
})
