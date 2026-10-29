import { DOMWrapper, flushPromises, mount } from '@vue/test-utils'
import Antd, { Select, message } from 'ant-design-vue'
import { afterEach, describe, expect, it, vi } from 'vitest'

// 「个人信息」侧栏只放**真实数据**（2026-09-20 现场）：
// 这里原先有一行「告警媒介 → 在右侧"关联告警媒介"中选择」——把一句操作指引当成只读信息栏的"值"，
// 既不准确（真正的选择在右侧「告警媒介」tab 的弹窗里）也没有信息量。这条用例钉住"那行别再回来"。
vi.mock('@/api/user', () => ({
  getCurrentUser: vi.fn(() => ({
    id: 3, username: 'op', phonenumber: '13800000000', create_time: '2026-01-01T00:00:00Z',
  })),
  getCurrentUserAlertMediaBindings: vi.fn(() => Promise.resolve({ data: { data: { results: [] } } })),
  saveCurrentUser: vi.fn(),
  updateUserInfo: vi.fn(values => Promise.resolve({ data: { data: { user: { id: 3, ...values } } } })),
  updateUserPassword: vi.fn(),
}))
vi.mock('@/api/role', () => ({
  // 页面读的是 roleList（不是分页的 results）——mock 成 results 会让 onMounted 里 forEach 报错。
  getCurrentUserRoleList: vi.fn(() => Promise.resolve({ data: { data: { roleList: [{ name: '运维' }] } } })),
}))
vi.mock('@/api/sys/userTimezone', () => ({
  getCurrentUserInfo: vi.fn(() => Promise.resolve({ data: { data: { id: 3, phonenumber: '13800000000', timezone: 'Asia/Shanghai', create_time: '2026-01-01T00:00:00Z' } } })),
}))
vi.mock('@/api/monitor', () => ({
  getUserNotificationChain: vi.fn(() => Promise.resolve({ data: { data: { can_receive: true, bindings: [] } } })),
}))
vi.mock('@/util/deleteConfirm', () => ({ openDeleteConfirm: vi.fn(() => Promise.resolve(false)) }))
vi.mock('@/util/userTimezoneSync', () => ({ emitUserTimezoneChanged: vi.fn() }))
vi.mock('@/store', () => ({ default: { state: { user: { timezone: 'Asia/Shanghai' } }, commit: vi.fn(), dispatch: vi.fn() } }))

const originalGetComputedStyle = window.getComputedStyle.bind(window)
window.getComputedStyle = (element) => originalGetComputedStyle(element)

import UserCenter from './index.vue'
import { getCurrentUserInfo } from '@/api/sys/userTimezone'
import { saveCurrentUser, updateUserInfo } from '@/api/user'
import { emitUserTimezoneChanged } from '@/util/userTimezoneSync'

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
    vi.restoreAllMocks()
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

  it('offers searchable IANA zones without saving a selection automatically', async () => {
    const wrapper = await mountPage()
    try {
      vi.clearAllMocks()
      const select = wrapper.findAllComponents(Select).find(component => component.props('showSearch'))
      expect(select).toBeTruthy()
      expect(select.props('filterOption')).not.toBe(false)
      expect(select.props('optionFilterProp')).toBe('label')
      expect(select.props('getPopupContainer')).toBeTypeOf('function')
      expect(select.props('virtual')).toBe(false)
      expect(select.props('options').length).toBeGreaterThan(300)
      expect(select.props('options').map(option => option.value)).toContain('Asia/Hong_Kong')
      const input = select.find('input[role="combobox"]')
      await input.trigger('mousedown')
      await input.setValue('hong_kong')
      await flushPromises()
      const labels = [...document.querySelectorAll('.ant-select-item-option-content')].map(node => node.textContent)
      expect(labels).toEqual(['UTC+08:00 · Asia/Hong_Kong'])
      await new DOMWrapper(document.querySelector('.ant-select-item-option')).trigger('click')
      await flushPromises()
      expect(wrapper.vm.formState.timezone).toBe('Asia/Hong_Kong')
      expect(wrapper.vm.currentUser.user.timezone).toBe('Asia/Shanghai')
      expect(updateUserInfo).not.toHaveBeenCalled()
      expect(saveCurrentUser).not.toHaveBeenCalled()
      expect(emitUserTimezoneChanged).not.toHaveBeenCalled()
    } finally {
      wrapper.unmount()
    }
  })

  it('keeps the saved IANA alias and refreshes the list on opening', async () => {
    getCurrentUserInfo.mockResolvedValueOnce({ data: { data: { id: 3, timezone: 'Asia/Kolkata' } } })
    const wrapper = await mountPage()
    try {
      expect(wrapper.vm.formState.timezone).toBe('Asia/Kolkata')
      const select = wrapper.findAllComponents(Select).find(component => component.props('showSearch'))
      expect(select.props('options').map(option => option.value)).toContain('Asia/Kolkata')
      const spy = vi.spyOn(Intl, 'supportedValuesOf')
      select.vm.$emit('dropdownVisibleChange', true)
      await flushPromises()
      expect(spy).toHaveBeenCalledWith('timeZone')
      expect(wrapper.vm.formState.timezone).toBe('Asia/Kolkata')
    } finally {
      wrapper.unmount()
    }
  })

  it('reports enumeration errors and disables the selector', async () => {
    vi.spyOn(Intl, 'supportedValuesOf').mockImplementation(() => { throw new Error('unavailable') })
    vi.spyOn(console, 'error').mockImplementation(() => {})
    const errorSpy = vi.spyOn(message, 'error').mockImplementation(() => {})
    const wrapper = await mountPage()
    try {
      const select = wrapper.findAllComponents(Select).find(component => component.props('showSearch'))
      expect(select.props('disabled')).toBe(true)
      expect(select.props('options')).toEqual([])
      expect(errorSpy).toHaveBeenCalledWith(expect.stringContaining('加载时区列表失败'))
    } finally {
      wrapper.unmount()
    }
  })

  it('saves phone and timezone in a single self-profile request', async () => {
    const wrapper = await mountPage()
    try {
      vi.clearAllMocks()
      wrapper.vm.formState.phonenumber = '256'
      wrapper.vm.formState.timezone = 'Asia/Hong_Kong'
      await wrapper.find('form.settings-form').trigger('submit')
      await flushPromises()
      expect(updateUserInfo).toHaveBeenCalledOnce()
      expect(updateUserInfo).toHaveBeenCalledWith({ phonenumber: '256', timezone: 'Asia/Hong_Kong' })
      expect(saveCurrentUser).toHaveBeenCalledWith({ id: 3, phonenumber: '256', timezone: 'Asia/Hong_Kong' })
      expect(emitUserTimezoneChanged).toHaveBeenCalledOnce()
      expect(emitUserTimezoneChanged).toHaveBeenCalledWith('Asia/Hong_Kong')
      expect(wrapper.vm.profileSaving).toBe(false)
    } finally {
      wrapper.unmount()
    }
  })

  it('keeps phone edits and displayed times unchanged while choosing a draft timezone', async () => {
    const wrapper = await mountPage()
    try {
      wrapper.vm.formState.phonenumber = 'unsaved-phone'
      wrapper.vm.formState.timezone = 'Asia/Tokyo'
      await flushPromises()
      expect(wrapper.vm.formState.phonenumber).toBe('unsaved-phone')
      expect(wrapper.find('.profile-summary').text()).toContain('2026-01-01 08:00:00')
    } finally {
      wrapper.unmount()
    }
  })

  it('reports save errors without replacing the cached profile', async () => {
    const wrapper = await mountPage()
    try {
      vi.clearAllMocks()
      vi.spyOn(console, 'error').mockImplementation(() => {})
      const errorSpy = vi.spyOn(message, 'error').mockImplementation(() => {})
      updateUserInfo.mockRejectedValueOnce(new Error('保存被拒绝'))
      wrapper.vm.formState.phonenumber = '256'
      wrapper.vm.formState.timezone = 'Asia/Hong_Kong'
      await wrapper.vm.onFinish_updateUserInfo({ phonenumber: '256', timezone: 'Asia/Hong_Kong' })
      expect(errorSpy).toHaveBeenCalledWith('保存被拒绝')
      expect(saveCurrentUser).not.toHaveBeenCalled()
      expect(emitUserTimezoneChanged).not.toHaveBeenCalled()
      expect(wrapper.vm.formState.timezone).toBe('Asia/Hong_Kong')
      expect(wrapper.vm.currentUser.user.timezone).toBe('Asia/Shanghai')
      expect(wrapper.vm.profileSaving).toBe(false)
    } finally {
      wrapper.unmount()
    }
  })

  it('does not publish the timezone until the save succeeds', async () => {
    const wrapper = await mountPage()
    try {
      vi.clearAllMocks()
      let finishSave
      updateUserInfo.mockReturnValueOnce(new Promise(resolve => { finishSave = resolve }))
      const saving = wrapper.vm.onFinish_updateUserInfo({ phonenumber: '256', timezone: 'Asia/Tokyo' })
      expect(wrapper.vm.profileSaving).toBe(true)
      expect(saveCurrentUser).not.toHaveBeenCalled()
      expect(emitUserTimezoneChanged).not.toHaveBeenCalled()
      finishSave({ data: { data: { user: { id: 3, phonenumber: '256', timezone: 'Asia/Tokyo' } } } })
      await saving
      expect(saveCurrentUser).toHaveBeenCalledOnce()
      expect(emitUserTimezoneChanged).toHaveBeenCalledWith('Asia/Tokyo')
      expect(wrapper.vm.profileSaving).toBe(false)
    } finally {
      wrapper.unmount()
    }
  })
})
