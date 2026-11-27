import { flushPromises, mount } from '@vue/test-utils'
import { ref } from 'vue'
import Antd, { Tooltip } from 'ant-design-vue'
import { describe, expect, it, vi } from 'vitest'
import CredentialPage from './index.vue'
import { checkPermission } from '@/directives/permission/permission'

vi.mock('vue-router', () => ({ useRoute: () => ({ query: {} }) }))
vi.mock('@/util/request', () => ({ default: {} }))
vi.mock('@/directives/permission/permission', () => ({ checkPermission: vi.fn() }))
vi.mock('vue-request', () => ({
  usePagination: () => ({ data: ref([{ id: 8, name: '生产 SSH' }]), current: ref(1), pageSize: ref(10), run: vi.fn() }),
}))
const dialog = { props: ['open', 'item_id', 'readOnly'], emits: ['update:open'], template: '<div />' }

describe('凭证列表入口', () => {
  it.each([true, false])('名称和唯一配置按钮打开相同表单，更新权限=%s', async editable => {
    checkPermission.mockImplementation(code => code === 'assets:credentials:view' || (editable && code === 'assets:credentials:update'))
    const wrapper = mount(CredentialPage, {
      global: { plugins: [Antd], directives: { permission: () => {} }, stubs: { Dialog: dialog, FontAwesomeIcon: true } },
    })
    try {
      const form = wrapper.findComponent(dialog)
      await wrapper.get('td a').trigger('click')
      expect(form.props()).toMatchObject({ item_id: 8, open: true, readOnly: !editable })
      form.vm.$emit('update:open', false)
      await flushPromises()
      const config = wrapper.findAllComponents(Tooltip).filter(t => ['编辑', '查看'].includes(t.props('title')))
      expect(config.map(t => t.props('title'))).toEqual([editable ? '编辑' : '查看'])
      await config[0].get('button').trigger('click')
      expect(form.props()).toMatchObject({ item_id: 8, open: true, readOnly: !editable })
    } finally { wrapper.unmount() }
  })
})
