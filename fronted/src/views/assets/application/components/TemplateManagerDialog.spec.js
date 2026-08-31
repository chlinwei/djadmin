import { flushPromises, mount } from '@vue/test-utils'
import Antd from 'ant-design-vue'
import { afterEach, describe, expect, it, vi } from 'vitest'

vi.mock('@/api/assets/application', () => ({
  getApplicationDeploymentTemplateList: vi.fn(() => Promise.resolve({
    data: { data: { results: [
      { id: 2, name: 'tomcat-systemd', application: 1, application_name: 'Tomcat', control_type: 'systemd', run_user: 'app', app_home: '/srv/tomcat', enabled: true, service_count: 0 },
    ], count: 1 } },
  })),
  deleteApplicationDeploymentTemplate: vi.fn(() => Promise.resolve({ data: { data: null } })),
}))

vi.mock('@/util/deleteConfirm', () => ({
  openDeleteConfirm: vi.fn(),
}))

import * as applicationApi from '@/api/assets/application'
import { openDeleteConfirm } from '@/util/deleteConfirm'
import TemplateManagerDialog from './TemplateManagerDialog.vue'

const TemplateDialogStub = {
  name: 'TemplateDialog',
  props: ['open', 'templateId', 'copyFromId', 'initialApplicationId'],
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
        permission: () => {},
      },
    },
  })
}

describe('TemplateManagerDialog', () => {
  afterEach(() => {
    document.body.innerHTML = ''
    vi.clearAllMocks()
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

    await wrapper.vm.openCopy({ id: 2 })
    dialog = wrapper.findComponent(TemplateDialogStub)
    expect(dialog.props('templateId')).toBeNull()
    expect(dialog.props('copyFromId')).toBe(2)
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

    expect(applicationApi.deleteApplicationDeploymentTemplate).toHaveBeenCalledWith(2)
    expect(wrapper.emitted('changed')).toBeTruthy()
    wrapper.unmount()
  })
})
