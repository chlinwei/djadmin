import { flushPromises, mount } from '@vue/test-utils'
import { defineComponent, h, KeepAlive, ref } from 'vue'
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
  it('keeps an opened select attached to its modal after switching cached pages', async () => {
    const visible = ref(true)
    const open = ref(false)
    const page = defineComponent({
      setup: () => () => h('section', [h(Dialog, {
        open: open.value,
        title: '新增应用',
        item_id: -1,
        appname: '应用',
        'onUpdate:open': (value) => { open.value = value },
      })]),
    })
    const host = defineComponent({
      setup: () => () => h(KeepAlive, null, {
        default: () => visible.value ? h(page) : h('div'),
      }),
    })
    wrapper = mount(host, {
      global: { plugins: [Antd], stubs: { transition: false, 'transition-group': false } },
      attachTo: document.body,
    })
    for (let cycle = 0; cycle < 3; cycle += 1) {
      open.value = true
      await flushPromises()
      const select = document.body.querySelector('.ant-select-selector')
      expect(select).not.toBeNull()
      select.dispatchEvent(new MouseEvent('mousedown', { bubbles: true }))
      await flushPromises()
      await new Promise((resolve) => setTimeout(resolve, 50))
      const dropdown = document.body.querySelector('.ant-select-dropdown')
      expect(dropdown).not.toBeNull()
      expect(dropdown.closest('.ant-modal-wrap')).toBe(select.closest('.ant-modal-wrap'))
      const option = dropdown.querySelector('[title="缓存"]')
      expect(option).not.toBeNull()
      option.click()
      await flushPromises()
      expect(wrapper.findComponent(Dialog).findComponent({ name: 'ASelect' }).props('value')).toBe('cache')
      select.dispatchEvent(new MouseEvent('mousedown', { bubbles: true }))
      await flushPromises()
      visible.value = false
      await flushPromises()
      open.value = false
      await flushPromises()
      visible.value = true
      await flushPromises()
    }
  })

  it('validates and saves after closing and reopening the retained form', async () => {
    wrapper = mount(Dialog, {
      props: { open: false, title: '新增应用', item_id: -1, appname: '应用' },
      global: { plugins: [Antd] },
      attachTo: document.body,
    })
    await wrapper.setProps({ open: true })
    await flushPromises()
    const modal = wrapper.findComponent({ name: 'AModal' })
    await wrapper.setProps({ open: false })
    modal.vm.$emit('afterClose')
    await flushPromises()
    await wrapper.setProps({ open: true })
    await flushPromises()
    const inputs = wrapper.findAllComponents({ name: 'AInput' })
    inputs[0].vm.$emit('update:value', 'reopened-app')
    inputs[1].vm.$emit('update:value', 'reopened-app')
    await flushPromises()
    modal.vm.$emit('afterClose')
    await flushPromises()
    expect(inputs[0].props('value')).toBe('reopened-app')
    modal.vm.$emit('ok')
    await flushPromises()
    expect(SaveOrCreateApplication).toHaveBeenCalledWith(expect.objectContaining({ name: 'reopened-app' }))
  })

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
