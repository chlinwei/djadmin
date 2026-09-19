import { flushPromises, mount } from '@vue/test-utils'
import Antd from 'ant-design-vue'
import { afterEach, describe, expect, it, vi } from 'vitest'

// 回归用例（2026-09-19 现场）：菜单保存失败，根因是**数值字段被当成字符串提交**——
// 「显示顺序」当时绑的是文本框，用户一改，`order_num` 就成了 `"6"`；后端 `menuRequest` 按
// `int32` 绑定，`ShouldBindJSON` 让整个请求失败（"请求参数错误"），前端只弹一个数字 `400`、
// 服务端日志只留一句 `<nil>`，两边都指不到"哪个字段错了"。
//
// 这里钉住前端这一半：提交前必须把 parent_id / order_num / location 归一成数字；
// 失败时必须把后端的 msg 显示出来（而不是拿 code 当消息）。
vi.mock('@/api/menu/index.js', () => ({
  getMenuById: vi.fn(() => Promise.resolve({
    data: { data: {
      id: 169,
      name: '日志中心',
      icon: 'search',
      parent_id: 160,
      order_num: 5,
      path: '/monitor/logging/center',
      component: 'monitor/log-center/index',
      menu_type: 'C',
      perms: 'monitor:view',
      is_expanded: false,
      remark: '服务树 + 日志配置/查询/本服务水位',
      location: 104,
      create_time: '2026-09-19',
      update_time: '2026-09-19',
    } },
  })),
  saveOrCreateMenu: vi.fn(() => Promise.resolve({ data: { code: 200, msg: 'success', data: null } })),
}))

import Dialog from './Dialog.vue'
import { getMenuById, saveOrCreateMenu } from '@/api/menu/index.js'
import { message } from 'ant-design-vue'

function mountDialog(props = {}) {
  return mount(Dialog, {
    props: { open: false, item_id: 169, title: '编辑-日志中心', treeData: [], ...props },
    attachTo: document.body,
    global: {
      plugins: [Antd],
      stubs: {
        AModal: { template: '<div><slot /></div>' },
        // App 里这个图标组件是全局注册的，单测里 stub 掉即可（否则只刷警告）。
        FontAwesomeIcon: true,
      },
    },
  })
}

describe('菜单编辑弹窗', () => {
  afterEach(() => {
    document.body.innerHTML = ''
    vi.clearAllMocks()
  })

  it('提交前把数值字段归一成数字（文本框给出的字符串也不许原样发出去）', async () => {
    const wrapper = mountDialog()
    await wrapper.setProps({ open: true })
    await flushPromises()
    expect(getMenuById).toHaveBeenCalledWith(169)

    // 模拟"显示顺序"被输入框改成了字符串（现场形态）。
    wrapper.vm.form.order_num = '6'
    wrapper.vm.form.parent_id = '160'
    wrapper.vm.form.location = '104'
    await wrapper.vm.handleOk()
    await flushPromises()

    expect(saveOrCreateMenu).toHaveBeenCalledTimes(1)
    const payload = saveOrCreateMenu.mock.calls[0][0]
    expect(payload.order_num).toBe(6)
    expect(payload.parent_id).toBe(160)
    expect(payload.location).toBe(104)
    // 其余字段原样带走（后端按整行合并，缺字段会被当成"不改"）。
    expect(payload.name).toBe('日志中心')
    expect(payload.id).toBe(169)
    wrapper.unmount()
  })

  it('数值字段留空时提交 null（= 不改动），不硬塞 0', async () => {
    const wrapper = mountDialog()
    await wrapper.setProps({ open: true })
    await flushPromises()

    wrapper.vm.form.order_num = ''
    await wrapper.vm.handleOk()
    await flushPromises()

    const payload = saveOrCreateMenu.mock.calls[0][0]
    expect(payload.order_num).toBeNull()
    expect(payload.parent_id).toBe(160)
    wrapper.unmount()
  })

  it('保存失败时显示后端的 msg，而不是把 code 当消息', async () => {
    const errorSpy = vi.spyOn(message, 'error').mockImplementation(() => {})
    saveOrCreateMenu.mockResolvedValueOnce({
      data: { code: 400, msg: '请求参数错误：json: cannot unmarshal string into Go struct field menuRequest.order_num of type int32', data: null },
    })
    const wrapper = mountDialog()
    await wrapper.setProps({ open: true })
    await flushPromises()

    await wrapper.vm.handleOk()
    await flushPromises()

    expect(errorSpy).toHaveBeenCalledTimes(1)
    const toast = errorSpy.mock.calls[0][0]
    // 必须是后端的 msg（以前取信封的第一个键 = code，弹出来就是个"400"）。
    expect(toast).toContain('请求参数错误')
    expect(toast).toContain('order_num')
    expect(toast).not.toBe('400')
    errorSpy.mockRestore()
    wrapper.unmount()
  })
})
