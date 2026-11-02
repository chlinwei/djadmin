import { flushPromises, mount } from '@vue/test-utils'
import Antd, { message } from 'ant-design-vue'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import Dialog from './Dialog.vue'
import { addUser, checkUserName, getUserById, saveUserInfo } from '@/api/user/index.js'

vi.mock('@/api/user/index.js', () => ({
  addUser: vi.fn(),
  checkUserName: vi.fn(),
  getUserById: vi.fn(),
  saveUserInfo: vi.fn(),
}))
let wrapper
beforeEach(() => {
  vi.resetAllMocks()
  vi.spyOn(message, 'success').mockImplementation(() => {})
  checkUserName.mockResolvedValue({ data: { data: { exists: false } } })
  getUserById.mockResolvedValue({ data: { data: { id: 7, username: 'view', status: 1, phonenumber: '', remark: '' } } })
  addUser.mockResolvedValue({ data: { code: 200 } })
  saveUserInfo.mockResolvedValue({ data: { code: 200 } })
})
afterEach(() => { wrapper?.unmount(); document.body.innerHTML = ''; vi.restoreAllMocks() })
describe('user status form', () => {
  it.each([
    [-1, false, 1, addUser],
    [7, true, 0, saveUserInfo],
  ])('keeps numeric status and only saves the draft on submit for user %s', async (id, initial, target, save) => {
    wrapper = mount(Dialog, {
      props: { open: false, title: '用户', user_id: id },
      attachTo: document.body,
      global: { plugins: [Antd] },
    })
    await wrapper.setProps({ open: true })
    await flushPromises()
    const button = document.body.querySelector('button[role="switch"]')
    expect(button.getAttribute('aria-checked')).toBe(String(initial))
    button.click()
    await flushPromises()
    expect(wrapper.vm.form.status).toBe(target)
    expect(typeof wrapper.vm.form.status).toBe('number')
    expect(addUser).not.toHaveBeenCalled()
    expect(saveUserInfo).not.toHaveBeenCalled()
    wrapper.vm.form.username = 'view'
    document.body.querySelector('.ant-modal-footer .ant-btn-primary').click()
    await flushPromises()
    expect(save).toHaveBeenCalledWith(expect.objectContaining({ status: target }))
  })
})
