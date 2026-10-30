import { describe, it, expect, vi, beforeEach } from 'vitest'
import { shallowMount, flushPromises } from '@vue/test-utils'
import { message } from 'ant-design-vue'
import Dialog from './index.vue'
import { getHostGroupById, saveOrCreateHostGroup } from '@/api/assets/hostgroup/index.js'

vi.mock('@/api/assets/hostgroup/index.js', () => ({
  getHostGroupById: vi.fn(),
  saveOrCreateHostGroup: vi.fn(() => Promise.resolve({ data: { code: 200 } })),
}))

const mountDialog = () => shallowMount(Dialog, {
  props: { open: false, title: '分组', item_id: -1 },
  global: { stubs: { 'a-modal': { template: '<div><slot /></div>' } } },
})

describe('单层主机分组表单', () => {
  beforeEach(() => vi.clearAllMocks())

  it('新增只提交名称与备注，不包含父级字段', async () => {
    const wrapper = mountDialog()
    await wrapper.setProps({ open: true })
    const state = wrapper.vm.$.setupState
    state.form.name = 'production'
    state.form.remark = 'core hosts'
    state.formRef = { validate: vi.fn(() => Promise.resolve()) }
    state.handleOk()
    await flushPromises()
    expect(saveOrCreateHostGroup).toHaveBeenCalledWith({ id: -1, name: 'production', remark: 'core hosts' })
    expect(wrapper.text()).not.toContain('上级分组')
    expect(wrapper.emitted('initList')).toHaveLength(1)
    wrapper.unmount()
  })

  it('编辑保留分组 ID，获取失败明确提示并关闭表单', async () => {
    const error = vi.spyOn(message, 'error').mockImplementation(() => {})
    getHostGroupById.mockResolvedValueOnce({ data: { code: 200, data: { id: 8, name: 'testing', remark: 'test hosts' } } })
    const wrapper = mountDialog()
    await wrapper.setProps({ item_id: 8, open: true })
    await flushPromises()
    const state = wrapper.vm.$.setupState
    expect(state.form).toEqual({ id: 8, name: 'testing', remark: 'test hosts' })
    state.formRef = { validate: vi.fn(() => Promise.resolve()) }
    state.handleOk()
    await flushPromises()
    expect(saveOrCreateHostGroup).toHaveBeenCalledWith({ id: 8, name: 'testing', remark: 'test hosts' })
    await wrapper.setProps({ open: false })
    getHostGroupById.mockRejectedValueOnce(new Error('详情加载失败'))
    await wrapper.setProps({ item_id: 9, open: true })
    await flushPromises()
    expect(error).toHaveBeenCalledWith('详情加载失败')
    expect(wrapper.emitted('update:open').at(-1)).toEqual([false])
    error.mockRestore()
    wrapper.unmount()
  })
})
