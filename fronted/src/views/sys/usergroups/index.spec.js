import { flushPromises, mount } from '@vue/test-utils'
import Antd, { Modal, Tooltip, message } from 'ant-design-vue'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import UserGroups from './index.vue'
import { batchDeleteUserGroups, getUserGroupList, updateUserGroup } from '@/api/userGroup'

vi.mock('@/api/userGroup', () => ({
  getUserGroupList: vi.fn(),
  createUserGroup: vi.fn(),
  updateUserGroup: vi.fn(),
  batchDeleteUserGroups: vi.fn(),
}))
vi.mock('@/api/user/index.js', () => ({
  getUserList: vi.fn(() => Promise.resolve({
    data: { data: { results: [
      { id: 65, username: 'admin' },
      { id: 66, username: 'alice' },
      { id: 77, username: 'bob' },
    ], totalPages: 1 } },
  })),
}))

let wrapper
let groups

beforeEach(() => {
  vi.clearAllMocks()
  groups = [
    { id: 11, name: '值班组', remark: '告警通知', member_count: 2, members: [
      { user_id: 65, username: 'admin' }, { user_id: 66, username: 'alice' },
    ] },
    { id: 22, name: '研发组', remark: '研发通知', member_count: 1, members: [{ user_id: 77, username: 'bob' }] },
  ]
  getUserGroupList.mockImplementation(() => Promise.resolve({ data: { data: { items: groups } } }))
  updateUserGroup.mockImplementation(async (data) => {
    groups = groups.map((group) => group.id === data.id ? {
      ...group,
      members: data.user_ids.map((id) => ({ user_id: id, username: ({ 65: 'admin', 66: 'alice', 77: 'bob' })[id] })),
      member_count: data.user_ids.length,
    } : group)
    return { data: { data: {} } }
  })
})

afterEach(async () => {
  wrapper?.unmount()
  Modal.destroyAll()
  await flushPromises()
  document.body.innerHTML = ''
  vi.restoreAllMocks()
})

async function loadPage() {
  wrapper = mount(UserGroups, {
    attachTo: document.body,
    global: { plugins: [Antd], stubs: { FontAwesomeIcon: true } },
  })
  await flushPromises()
}

async function clickRemove() {
  const tooltip = wrapper.findAllComponents(Tooltip).find((item) => item.props('title') === '移出本组')
  expect(tooltip.props('placement')).toBe('top')
  await tooltip.find('button').trigger('click')
  await flushPromises()
}

async function clickConfirmation(text) {
  const modal = document.body.querySelector('.ant-modal-confirm')
  Array.from(modal.querySelectorAll('button'))
    .find((button) => button.textContent.replace(/\s/g, '') === text).click()
  await flushPromises()
}

describe('用户组成员移出确认', () => {
  it('点击仅展示包含影响对象的居中确认框，取消不发送更新', async () => {
    await loadPage()
    const removeButton = wrapper.findAllComponents(Tooltip)
      .find((item) => item.props('title') === '移出本组').find('button')
    expect(removeButton.isVisible()).toBe(true)
    await removeButton.trigger('mouseenter')
    await new Promise((resolve) => setTimeout(resolve, 150))
    await flushPromises()
    expect(document.body.querySelector('.ant-tooltip-inner')?.textContent).toBe('移出本组')
    await clickRemove()
    const modal = document.body.querySelector('.ant-modal-confirm')
    expect(modal.closest('.ant-modal-wrap').classList.contains('ant-modal-centered')).toBe(true)
    for (const text of ['确认移出用户组成员', '用户组：值班组（#11）', '成员：admin（#65）', '不会删除平台用户']) {
      expect(modal.textContent).toContain(text)
    }
    expect(updateUserGroup).not.toHaveBeenCalled()
    await clickConfirmation('取消')
    expect(updateUserGroup).not.toHaveBeenCalled()
    expect(batchDeleteUserGroups).not.toHaveBeenCalled()
    expect(wrapper.vm.currentGroup.members.map((member) => member.user_id)).toEqual([65, 66])
    expect(getUserGroupList).toHaveBeenCalledTimes(1)
    expect(wrapper.vm.memberColumns.find((column) => column.key === 'operation')).toMatchObject({ fixed: 'right', width: 100 })
    expect(wrapper.findComponent({ name: 'ATable' }).props('scroll')).toEqual({ x: 640 })
  })

  it('确认后才更新保留成员列表，并刷新成员及组计数', async () => {
    const success = vi.spyOn(message, 'success').mockImplementation(() => {})
    await loadPage()
    await clickRemove()
    expect(updateUserGroup).not.toHaveBeenCalled()
    await clickConfirmation('确认移出')
    expect(updateUserGroup).toHaveBeenCalledTimes(1)
    expect(updateUserGroup).toHaveBeenCalledWith({
      id: 11, name: '值班组', remark: '告警通知', user_ids: [66],
    })
    expect(wrapper.vm.currentGroup.members.map((member) => member.username)).toEqual(['alice'])
    expect(wrapper.vm.currentGroup.member_count).toBe(1)
    expect(getUserGroupList).toHaveBeenCalledTimes(2)
    expect(success).toHaveBeenCalledWith('已将 admin 移出用户组 值班组')
    expect(batchDeleteUserGroups).not.toHaveBeenCalled()
  })

  it('确认期间切换组也只更新最初的目标组', async () => {
    await loadPage()
    await clickRemove()
    wrapper.vm.handleSelectGroup({ key: 22 })
    await flushPromises()
    await clickConfirmation('确认移出')
    expect(updateUserGroup).toHaveBeenCalledWith({
      id: 11, name: '值班组', remark: '告警通知', user_ids: [66],
    })
    expect(wrapper.vm.currentGroup.id).toBe(22)
    expect(wrapper.vm.currentGroup.members.map((member) => member.user_id)).toEqual([77])
    expect(wrapper.vm.groupList.find((group) => group.id === 11).member_count).toBe(1)
  })

  it('更新失败不显示成功、不改成员列表并恢复 loading 状态', async () => {
    const success = vi.spyOn(message, 'success').mockImplementation(() => {})
    const error = vi.spyOn(message, 'error').mockImplementation(() => {})
    updateUserGroup.mockRejectedValueOnce({ isAxiosError: true, message: '移出请求失败' })
    await loadPage()
    await clickRemove()
    await clickConfirmation('确认移出')
    expect(error).toHaveBeenCalledWith('移出请求失败')
    expect(success).not.toHaveBeenCalled()
    expect(wrapper.vm.currentGroup.members.map((member) => member.user_id)).toEqual([65, 66])
    expect(wrapper.vm.memberSaving).toBe(false)
    expect(getUserGroupList).toHaveBeenCalledTimes(1)
  })

  it('添加成员仍更新当前组，不触发移出确认', async () => {
    await loadPage()
    wrapper.vm.memberToAdd = 77
    await wrapper.vm.addMember()
    await flushPromises()
    expect(updateUserGroup).toHaveBeenCalledWith({
      id: 11, name: '值班组', remark: '告警通知', user_ids: [65, 66, 77],
    })
    expect(wrapper.vm.currentGroup.member_count).toBe(3)
    expect(wrapper.vm.memberToAdd).toBeNull()
    expect(document.body.querySelector('.ant-modal-confirm')).toBeNull()
  })
})
