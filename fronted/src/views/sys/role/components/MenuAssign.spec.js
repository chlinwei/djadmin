import { flushPromises, mount } from '@vue/test-utils'
import Antd, { message } from 'ant-design-vue'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import MenuAssign from './MenuAssign.vue'
import { getMenuTree, getMenuListByRoleId, grantMenu } from '@/api/menu'

vi.mock('@/api/menu', () => ({
  getMenuTree: vi.fn(),
  getMenuListByRoleId: vi.fn(),
  grantMenu: vi.fn(),
}))

const permissions = [1, 2].map((group) => ({
  id: group, name: `模块 ${group}`,
  children: Array.from({ length: 60 }, (_, index) => ({
    id: group * 100 + index, name: `权限 ${group * 100 + index}`,
  })),
}))
let wrapper

beforeEach(() => {
  vi.clearAllMocks()
  getMenuTree.mockResolvedValue({ data: { data: permissions } })
  getMenuListByRoleId.mockResolvedValue({ data: { data: [1, 101, 2] } })
  grantMenu.mockResolvedValue({ data: { code: 200 } })
  vi.spyOn(message, 'success').mockImplementation(() => {})
})

afterEach(() => {
  wrapper?.unmount()
  document.body.innerHTML = ''
  vi.restoreAllMocks()
})

async function openDialog() {
  wrapper = mount(MenuAssign, {
    props: { open: false, role_id: 9, title: '权限分配-只读角色' },
    attachTo: document.body,
    global: { plugins: [Antd] },
  })
  await wrapper.setProps({ open: true })
  await flushPromises()
}

describe('权限分配弹窗布局', () => {
  it('加宽但限制窄屏宽度，长权限树内部滚动且标题与保存取消位于滚动区外', async () => {
    await openDialog()
    const modal = wrapper.findComponent({ name: 'AModal' })
    expect(modal.props('width')).toBe(720)
    const dialog = document.querySelector('.ant-modal')
    expect(dialog.style.top).toBe('24px')
    expect(dialog.style.maxWidth).toBe('calc(100vw - 32px)')
    const scroll = document.querySelector('.permission-tree-scroll')
    expect(scroll.style.maxHeight).toBe('calc(100dvh - 200px)')
    expect(scroll.style.overflowY).toBe('auto')
    expect(scroll.querySelectorAll('.ant-tree-title')).toHaveLength(122)
    const title = document.querySelector('.ant-modal-title')
    const footer = document.querySelector('.ant-modal-footer')
    expect(title.textContent).toBe('权限分配-只读角色')
    expect(scroll.contains(title)).toBe(false)
    expect(scroll.contains(footer)).toBe(false)
    expect([...footer.querySelectorAll('button')].map((button) => button.textContent.replace(/\s/g, '')))
      .toEqual(['取消', '保存'])
    expect(grantMenu).not.toHaveBeenCalled()
  })

  it('保留全部展开和父子独立勾选，保存仍提交当前角色与精确勾选项', async () => {
    await openDialog()
    const tree = wrapper.findComponent({ name: 'ATree' })
    expect(tree.props('defaultExpandAll')).toBe(true)
    expect(tree.props('checkStrictly')).toBe(true)
    expect(tree.props('checkedKeys')).toEqual([1, 101, 2])
    const node = [...document.querySelectorAll('.ant-tree-treenode')]
      .find((item) => item.querySelector('.ant-tree-title')?.textContent === '权限 201')
    node.querySelector('.ant-tree-checkbox').click()
    await flushPromises()
    expect(tree.props('checkedKeys')).toEqual([1, 101, 2, 201])
    document.querySelector('.ant-modal-footer .ant-btn-primary').click()
    await flushPromises()
    expect(grantMenu).toHaveBeenCalledWith(9, [1, 101, 2, 201])
    expect(wrapper.emitted('initList')).toHaveLength(1)
    expect(wrapper.emitted('update:menuAssignVisible')).toEqual([[false]])
  })

  it('取消不提交权限，再次打开其他角色时重新加载对应勾选项', async () => {
    await openDialog()
    document.querySelector('.ant-modal-footer button').click()
    await flushPromises()
    expect(grantMenu).not.toHaveBeenCalled()
    expect(wrapper.emitted('update:menuAssignVisible')).toEqual([[false]])
    await wrapper.setProps({ open: false })
    getMenuListByRoleId.mockResolvedValueOnce({ data: { data: [2, 201] } })
    await wrapper.setProps({ open: true, role_id: 10, title: '权限分配-测试角色' })
    await flushPromises()
    expect(getMenuListByRoleId).toHaveBeenLastCalledWith(10)
    expect(wrapper.findComponent({ name: 'ATree' }).props('checkedKeys')).toEqual([2, 201])
    expect(document.querySelector('.ant-modal-title').textContent).toBe('权限分配-测试角色')
  })
})
