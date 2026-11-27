import { flushPromises, mount } from '@vue/test-utils'
import Antd from 'ant-design-vue'
import { describe, expect, it, vi } from 'vitest'
import Dialog from './Dialog.vue'
import { getCredentailById, SaveOrCreateCredential } from '@/api/assets/credential/index'

vi.mock('@/api/assets/credential/index', () => ({ getCredentailById: vi.fn(), SaveOrCreateCredential: vi.fn() }))

describe('凭证配置', () => {
  it.each([[1, 'password'], [2, 'private_key']])('只读认证类型 %i 仅回显掩码，不能提交', async (auth_type, key) => {
    getCredentailById.mockResolvedValue({ data: { code: 200, data: { id: 8, name: 'SSH', auth_type, [key]: 'synthetic-test-value' } } })
    const wrapper = mount(Dialog, {
      props: { open: true, readOnly: true, item_id: 8, title: '查看凭证' },
      global: { plugins: [Antd], stubs: { AModal: { template: '<div><slot /><slot name="footer" /></div>' } } },
    })
    try {
      await flushPromises()
      const state = wrapper.vm.$.setupState
      expect(state.form[key]).toBe('******')
      expect(wrapper.findAll('input, textarea').map(field => field.element.value)).not.toContain('synthetic-test-value')
      expect(wrapper.findAll('button').map(button => button.text().replace(/\s/g, ''))).toEqual(['关闭'])
      await state.save()
      expect(SaveOrCreateCredential).not.toHaveBeenCalled()
      await wrapper.setProps({ readOnly: false })
      SaveOrCreateCredential.mockResolvedValue({ data: { code: 200 } })
      await state.save()
      expect(SaveOrCreateCredential).toHaveBeenCalledWith(expect.objectContaining({ id: 8, [key]: '******' }))
    } finally { wrapper.unmount(); vi.clearAllMocks() }
  })
})
