import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import Layout from './index.vue'
import router from '@/router/index.js'
import store from '@/store/index.js'

const lifecycle = vi.hoisted(() => ({ unmounted: vi.fn() }))

vi.mock('@/layout/menu/index.vue', () => ({ default: { render: () => null } }))
vi.mock('@/layout/header/index.vue', () => ({ default: { render: () => null } }))
vi.mock('@/layout/footer/index.vue', () => ({ default: { render: () => null } }))
vi.mock('@/layout/tabs/index.vue', () => ({ default: { render: () => null } }))
vi.mock('@/store/index.js', async () => {
  const { reactive } = await import('vue')
  return { default: { state: reactive({ tabs: [] }) } }
})
vi.mock('@/router/index.js', async () => {
  const { defineComponent, h, onUnmounted, ref } = await import('vue')
  const { createMemoryHistory, createRouter } = await import('vue-router')
  const page = (name) => defineComponent({
    name,
    setup() {
      const text = ref('')
      onUnmounted(() => lifecycle.unmounted(name))
      return () => h('input', {
        value: text.value,
        onInput: (event) => { text.value = event.target.value },
      })
    },
  })
  return {
    default: createRouter({
      history: createMemoryHistory(),
      routes: [
        { path: '/first', alias: '/first-alias', name: 'view_first', component: page('view_first') },
        { path: '/second', name: 'view_second', component: page('view_second') },
      ],
    }),
    keepAliveNameOfRecord: (record) => record?.aliasOf?.name || record?.name || '',
  }
})

let wrapper

beforeEach(async () => {
  lifecycle.unmounted.mockClear()
  store.state.tabs = [{ key: '/first?filter=one' }, { key: '/second' }]
  await router.push('/first')
  wrapper = mount(Layout, {
    global: {
      plugins: [router],
      stubs: Object.fromEntries(
        ['ALayout', 'ALayoutSider', 'ALayoutHeader', 'ALayoutContent', 'ALayoutFooter']
          .map((name) => [name, { template: '<div><slot /></div>' }]),
      ),
    },
  })
  await flushPromises()
})

afterEach(() => wrapper?.unmount())

describe('layout page cache', () => {
  it('retains page state when equivalent tab lists and route changes produce new include arrays', async () => {
    const input = wrapper.get('input').element
    await wrapper.get('input').setValue('retained')
    store.state.tabs = [{ key: '/first?filter=two' }, { key: '/second', title: 'Updated' }]
    await flushPromises()
    await router.push('/second')
    await flushPromises()
    await router.push('/first')
    await flushPromises()
    expect(wrapper.get('input').element).toBe(input)
    expect(wrapper.get('input').element.value).toBe('retained')
    expect(lifecycle.unmounted).not.toHaveBeenCalled()
  })

  it('evicts a closed inactive tab without unmounting the current page', async () => {
    await wrapper.get('input').setValue('discarded')
    await router.push('/second')
    await flushPromises()
    store.state.tabs = [{ key: '/second' }]
    await flushPromises()
    expect(lifecycle.unmounted.mock.calls).toEqual([['view_first']])
    await router.push('/first')
    await flushPromises()
    expect(wrapper.get('input').element.value).toBe('')
  })

  it('keeps the current page until navigation completes after closing all tabs', async () => {
    await wrapper.get('input').setValue('current')
    store.state.tabs = []
    await flushPromises()
    expect(wrapper.get('input').element.value).toBe('current')
    expect(lifecycle.unmounted).not.toHaveBeenCalled()
    await router.push('/second')
    await flushPromises()
    expect(lifecycle.unmounted.mock.calls).toEqual([['view_first']])
  })

  it('resolves alias tab URLs with query and hash before deciding which pages to retain', async () => {
    await wrapper.get('input').setValue('alias retained')
    store.state.tabs = [{ key: '/first-alias?filter=one#details' }, { key: '/second' }]
    await router.push('/second')
    await flushPromises()
    await router.push('/first')
    await flushPromises()
    expect(wrapper.get('input').element.value).toBe('alias retained')
    expect(lifecycle.unmounted).not.toHaveBeenCalled()
  })
})