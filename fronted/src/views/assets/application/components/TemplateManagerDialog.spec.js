import { flushPromises, mount } from '@vue/test-utils'
import Antd from 'ant-design-vue'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import store from '@/store'
import permission from '@/directives/permission/index.js'

vi.mock('@/api/assets/application', () => ({
  getApplicationDeploymentTemplateList: vi.fn(() => Promise.resolve({
    data: { data: { results: [
      { id: 2, name: 'tomcat-systemd', application: 1, application_name: 'Tomcat', control_type: 'systemd', run_user: 'app', app_home: '/srv/tomcat', enabled: true, service_count: 0 },
    ], count: 1 } },
  })),
  batchDeleteApplicationDeploymentTemplates: vi.fn(() => Promise.resolve({ data: { data: null } })),
}))

vi.mock('@/util/deleteConfirm', () => ({
  openDeleteConfirm: vi.fn(),
}))

import * as applicationApi from '@/api/assets/application'
import { openDeleteConfirm } from '@/util/deleteConfirm'
import TemplateManagerDialog from './TemplateManagerDialog.vue'

const TemplateDialogStub = {
  name: 'TemplateDialog',
  props: ['open', 'templateId', 'copyFromId', 'initialApplicationId', 'readOnly'],
  template: '<div class="template-dialog-stub" />',
}

function mountDialog(props = {}) {
  return mount(TemplateManagerDialog, {
    props: { open: false, application: { id: 1, name: 'Tomcat' }, ...props },
    attachTo: document.body,
    global: {
      plugins: [Antd],
      stubs: {
        TemplateDialog: TemplateDialogStub,
        FontAwesomeIcon: true,
      },
      directives: {
        permission,
      },
    },
  })
}

describe('TemplateManagerDialog', () => {
  const originalRoles = store.state.role_codes
  const originalPerms = store.state.perms
  beforeEach(() => {
    store.state.role_codes = ['admin']
    store.state.perms = []
  })
  afterEach(() => {
    store.state.role_codes = originalRoles
    store.state.perms = originalPerms
    document.body.innerHTML = ''
    vi.clearAllMocks()
  })

  it('仅应用查看权限显示可点击的查看按钮、顶部提示，且不显示写操作', async () => {
    store.state.role_codes = ['readonly']
    store.state.perms = ['assets:applications:view']
    const wrapper = mountDialog({ open: true })
    await flushPromises()

    const view = document.body.querySelector('[data-view-template]')
    expect(view).not.toBeNull()
    expect(view.classList.contains('ant-btn-link')).toBe(true)
    expect(view.classList.contains('detail-action')).toBe(true)
    expect(view.textContent.trim()).toBe('详情')
    const tooltip = wrapper.findAllComponents({ name: 'ATooltip' })
      .find((item) => item.props('title') === '查看详情')
    expect(tooltip.props('placement')).toBe('top')
    view.dispatchEvent(new MouseEvent('mouseenter', { bubbles: true }))
    await new Promise((resolve) => setTimeout(resolve, 150))
    await flushPromises()
    expect(document.body.querySelector('.ant-tooltip-inner')?.textContent).toBe('查看详情')
    expect([...document.body.querySelectorAll('tbody button')].filter((button) => button.style.display !== 'none')).toEqual([view])
    expect(document.body.querySelector('.template-manager-toolbar button').style.display).toBe('none')
    view.click()
    await flushPromises()
    const dialog = wrapper.findComponent(TemplateDialogStub)
    expect(dialog.props()).toMatchObject({ open: true, templateId: 2, copyFromId: null, readOnly: true })
    expect(wrapper.vm.columns.find((column) => column.key === 'action')).toMatchObject({ fixed: 'right', width: 220 })
    expect(wrapper.findComponent({ name: 'ATable' }).props('scroll')).toEqual({ x: 1200 })
    wrapper.unmount()
  })

  it('loads and shows the templates for the given application', async () => {
    const wrapper = mountDialog()
    await wrapper.setProps({ open: true })
    await flushPromises()

    expect(applicationApi.getApplicationDeploymentTemplateList).toHaveBeenCalledWith(
      expect.objectContaining({ application: 1 }),
    )
    expect(wrapper.vm.templates).toEqual([
      expect.objectContaining({ id: 2, name: 'tomcat-systemd' }),
    ])
    wrapper.unmount()
  })

  it('新增模板会打开 TemplateDialog 且不带 templateId/copyFromId，并预填所属应用', async () => {
    const wrapper = mountDialog()
    await wrapper.setProps({ open: true })
    await flushPromises()

    await wrapper.vm.openCreate()
    const dialog = wrapper.findComponent(TemplateDialogStub)
    expect(dialog.props('open')).toBe(true)
    expect(dialog.props('templateId')).toBeNull()
    expect(dialog.props('copyFromId')).toBeNull()
    expect(dialog.props('initialApplicationId')).toBe(1)
    expect(dialog.props('readOnly')).toBe(false)
    wrapper.unmount()
  })

  it('编辑/复制会分别带上对应的 templateId/copyFromId', async () => {
    const wrapper = mountDialog()
    await wrapper.setProps({ open: true })
    await flushPromises()

    await wrapper.vm.openEdit({ id: 2 })
    let dialog = wrapper.findComponent(TemplateDialogStub)
    expect(dialog.props('templateId')).toBe(2)
    expect(dialog.props('copyFromId')).toBeNull()
    expect(dialog.props('readOnly')).toBe(false)

    await wrapper.vm.openCopy({ id: 2 })
    dialog = wrapper.findComponent(TemplateDialogStub)
    expect(dialog.props('templateId')).toBeNull()
    expect(dialog.props('copyFromId')).toBe(2)
    expect(dialog.props('readOnly')).toBe(false)
    wrapper.unmount()
  })

  it('从查看切换到编辑、复制或新增不会残留只读状态', async () => {
    const wrapper = mountDialog({ open: true })
    await flushPromises()
    for (const action of ['openEdit', 'openCopy', 'openCreate']) {
      await wrapper.vm.openView({ id: 2 })
      expect(wrapper.findComponent(TemplateDialogStub).props('readOnly')).toBe(true)
      await wrapper.vm[action]({ id: 2 })
      expect(wrapper.findComponent(TemplateDialogStub).props('readOnly')).toBe(false)
    }
    wrapper.unmount()
  })
  it('保存后刷新列表并向外抛 changed', async () => {
    const wrapper = mountDialog()
    await wrapper.setProps({ open: true })
    await flushPromises()

    const dialog = wrapper.findComponent(TemplateDialogStub)
    await dialog.vm.$emit('saved')
    await flushPromises()

    expect(applicationApi.getApplicationDeploymentTemplateList).toHaveBeenCalledTimes(2)
    expect(wrapper.emitted('changed')).toBeTruthy()
    wrapper.unmount()
  })

  it('删除会调用删除确认，确认后请求删除接口、刷新列表并抛 changed', async () => {
    const wrapper = mountDialog()
    await wrapper.setProps({ open: true })
    await flushPromises()

    await wrapper.vm.confirmDelete({ id: 2, name: 'tomcat-systemd' })

    expect(openDeleteConfirm).toHaveBeenCalledWith(expect.objectContaining({
      title: '确认删除部署模板',
      items: ['Tomcat / tomcat-systemd'],
    }))

    const { onConfirm } = openDeleteConfirm.mock.calls.at(-1)[0]
    await onConfirm()

    expect(applicationApi.batchDeleteApplicationDeploymentTemplates).toHaveBeenCalledWith([2])
    expect(wrapper.emitted('changed')).toBeTruthy()
    wrapper.unmount()
  })
})
