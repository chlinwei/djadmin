import { flushPromises, mount } from '@vue/test-utils'
import Antd from 'ant-design-vue'
import { afterEach, describe, expect, it, vi } from 'vitest'

vi.mock('@/api/assets/application/index.js', () => ({
  SaveOrCreateApplication: vi.fn(() => Promise.resolve({ data: { data: { id: 1 } } })),
  getApplicationById: vi.fn(),
}))

import { getApplicationById, SaveOrCreateApplication } from '@/api/assets/application/index.js'
import Dialog from './Dialog.vue'

let wrapper

afterEach(() => {
  wrapper?.unmount()
  vi.clearAllMocks()
  document.body.innerHTML = ''
})

describe('application categories', () => {
  it.each([
    ['cache', '缓存'],
    ['message_queue', '消息中间件'],
  ])('creates an application with category %s', async (category, label) => {
    wrapper = mount(Dialog, {
      props: { open: false, title: '新增应用', item_id: -1, appname: '应用' },
      global: { plugins: [Antd] },
      attachTo: document.body,
    })
    await wrapper.setProps({ open: true })
    await flushPromises()

    const select = wrapper.findComponent({ name: 'ASelect' })
    expect(select.props('options')).toEqual(expect.arrayContaining([
      { value: category, label },
      { value: 'middleware', label: '中间件' },
    ]))
    expect(select.props('value')).toBe('other')
    const inputs = wrapper.findAllComponents({ name: 'AInput' })
    inputs[0].vm.$emit('update:value', 'test-app')
    inputs[1].vm.$emit('update:value', 'test-app')
    select.vm.$emit('update:value', category)
    await flushPromises()
    wrapper.findComponent({ name: 'AModal' }).vm.$emit('ok')
    await flushPromises()

    expect(SaveOrCreateApplication).toHaveBeenCalledWith(expect.objectContaining({ category }))
  })

  it.each(['cache', 'message_queue'])('preserves category %s when editing', async (category) => {
    getApplicationById.mockResolvedValueOnce({
      data: { data: { id: 1, name: 'test-app', code: 'test-app', category } },
    })
    wrapper = mount(Dialog, {
      props: { open: false, title: '编辑应用', item_id: 1, appname: '应用' },
      global: { plugins: [Antd] },
      attachTo: document.body,
    })
    await wrapper.setProps({ open: true })
    await flushPromises()

    expect(wrapper.findComponent({ name: 'ASelect' }).props('value')).toBe(category)
    wrapper.findComponent({ name: 'AModal' }).vm.$emit('ok')
    await flushPromises()

    expect(SaveOrCreateApplication).toHaveBeenCalledWith(expect.objectContaining({ id: 1, category }))
  })
})
