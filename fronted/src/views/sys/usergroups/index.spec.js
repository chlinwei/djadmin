import { flushPromises, mount } from '@vue/test-utils'
import Antd, { Modal, Tooltip, message } from 'ant-design-vue'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import UserGroups from './index.vue'
import { batchDeleteUserGroups, createUserGroup, getUserGroupList, updateUserGroup } from '@/api/userGroup'
import { getUserList } from '@/api/user/index.js'
import store from '@/store'

vi.mock('@/store', async () => {
  const { reactive } = await import('vue')
  return { default: { state: reactive({ user: {}, perms: [], role_codes: [] }) } }
})

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
  store.state.role_codes = ['operator']
  store.state.perms = [
    'system:usergroups:view', 'system:usergroups:create', 'system:usergroups:update',
    'system:usergroups:delete', 'system:users:view',
  ]
  createUserGroup.mockResolvedValue({ data: { data: { id: 33 } } })
  batchDeleteUserGroups.mockResolvedValue({ data: { data: { results: [{ id: 11, ok: true }] } } })
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

describe('用户组操作权限', () => {
  it.each(['readonly', 'operator'])('%s 仅查看时不显示写入口，也不能通过处理函数提交', async (role) => {
    store.state.perms = ['system:usergroups:view']
    store.state.role_codes = [role]
    const warning = vi.spyOn(message, 'warning').mockImplementation(() => {})
    await loadPage()
    expect(wrapper.findAll('button')).toHaveLength(0)
    expect(wrapper.find('.member-add-row').exists()).toBe(false)
    expect(wrapper.findAll('thead th').map((header) => header.text())).toEqual(['用户'])
    expect(wrapper.text()).toContain('admin')
    expect(getUserList).not.toHaveBeenCalled()
    wrapper.vm.openCreateModal()
    wrapper.vm.openEditModal(groups[0])
    expect(wrapper.vm.modalVisible).toBe(false)
    wrapper.vm.groupForm = { name: '不应写入', remark: '' }
    await wrapper.vm.saveGroup()
    wrapper.vm.editingId = 11
    await wrapper.vm.saveGroup()
    wrapper.vm.memberToAdd = 77
    await wrapper.vm.addMember()
    await wrapper.vm.removeMember(groups[0].members[0])
    await wrapper.vm.saveMembers(groups[0], [])
    await wrapper.vm.handleDelete(groups[0])
    expect(document.body.querySelector('.ant-modal-confirm')).toBeNull()
    expect(createUserGroup).not.toHaveBeenCalled()
    expect(updateUserGroup).not.toHaveBeenCalled()
    expect(batchDeleteUserGroups).not.toHaveBeenCalled()
    expect(warning).toHaveBeenCalled()
    wrapper.vm.handleSelectGroup({ key: 22 })
    await flushPromises()
    expect(wrapper.vm.currentGroup.name).toBe('研发组')
    expect(wrapper.find('tbody').text()).toContain('bob')
  })

  it.each([
    [['system:usergroups:create'], ['operator'], true, false, false, false],
    [['system:users:view'], ['operator'], false, false, false, false],
    [['system:usergroups:update'], ['operator'], false, true, false, false],
    [['system:usergroups:update', 'system:users:view'], ['operator'], false, true, false, true],
    [['system:usergroups:delete'], ['operator'], false, false, true, false],
    [[], ['admin'], true, true, true, true],
  ])('动作权限独立生效：%j / %j', async (perms, roles, create, update, remove, add) => {
    store.state.perms = ['system:usergroups:view', ...perms]
    store.state.role_codes = roles
    await loadPage()
    const buttons = wrapper.findAll('button').map((button) => button.text().replace(/\s/g, ''))
    expect(buttons.includes('新增')).toBe(create)
    expect(buttons.includes('编辑组信息')).toBe(update)
    expect(buttons.includes('删除组')).toBe(remove)
    expect(buttons.includes('移出')).toBe(update)
    expect(wrapper.find('.member-add-row').exists()).toBe(add)
    expect(getUserList.mock.calls.length > 0).toBe(add)
    expect(wrapper.vm.memberColumns.some((column) => column.key === 'operation')).toBe(update)
    expect(wrapper.findAll('thead th').map((header) => header.text())).toEqual(update ? ['用户', '操作'] : ['用户'])
    if (update) {
      expect(wrapper.vm.memberColumns.at(-1)).toMatchObject({ fixed: 'right', width: 100 })
      expect(wrapper.findComponent({ name: 'ATable' }).props('scroll')).toEqual({ x: 640 })
    }
  })

  it.each(['移出', '删除组'])('%s 确认期间撤权后不提交请求、不显示成功', async (action) => {
    const warning = vi.spyOn(message, 'warning').mockImplementation(() => {})
    const success = vi.spyOn(message, 'success').mockImplementation(() => {})
    await loadPage()
    if (action === '移出') {
      await clickRemove()
    } else {
      await wrapper.findAll('button').find((button) => button.text() === '删除组').trigger('click')
      await flushPromises()
    }
    expect(document.body.querySelector('.ant-modal-confirm')).not.toBeNull()
    store.state.perms = ['system:usergroups:view']
    await flushPromises()
    await clickConfirmation(action === '移出' ? '确认移出' : '确认删除')
    expect(warning).toHaveBeenCalled()
    expect(updateUserGroup).not.toHaveBeenCalled()
    expect(batchDeleteUserGroups).not.toHaveBeenCalled()
    expect(success).not.toHaveBeenCalled()
    expect(wrapper.vm.memberColumns.some((column) => column.key === 'operation')).toBe(false)
  })

  it('撤权后关闭编辑框，并丢弃迟到的用户候选结果', async () => {
    let resolveUsers
    getUserList.mockImplementationOnce(() => new Promise((resolve) => { resolveUsers = resolve }))
    await loadPage()
    wrapper.vm.openEditModal(groups[0])
    expect(wrapper.vm.modalVisible).toBe(true)
    store.state.perms = ['system:usergroups:view']
    await flushPromises()
    expect(wrapper.vm.modalVisible).toBe(false)
    resolveUsers({ data: { data: { results: [{ id: 77, username: 'bob' }], totalPages: 1 } } })
    await flushPromises()
    expect(wrapper.vm.userList).toEqual([])
    expect(wrapper.find('.member-add-row').exists()).toBe(false)
  })

  it('有对应权限时仍能新增、编辑和确认删除用户组', async () => {
    await loadPage()
    wrapper.vm.openCreateModal()
    wrapper.vm.groupForm = { name: '新组', remark: '新备注' }
    await wrapper.vm.saveGroup()
    expect(createUserGroup).toHaveBeenCalledWith({ name: '新组', remark: '新备注', user_ids: [] })
    wrapper.vm.openEditModal(wrapper.vm.currentGroup)
    wrapper.vm.groupForm.remark = '已更新'
    await wrapper.vm.saveGroup()
    expect(updateUserGroup).toHaveBeenCalledWith({
      id: 11, name: '值班组', remark: '已更新', user_ids: [65, 66],
    })
    await wrapper.findAll('button').find((button) => button.text() === '删除组').trigger('click')
    await flushPromises()
    expect(batchDeleteUserGroups).not.toHaveBeenCalled()
    await clickConfirmation('确认删除')
    expect(batchDeleteUserGroups).toHaveBeenCalledWith([11])
  })
})

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
