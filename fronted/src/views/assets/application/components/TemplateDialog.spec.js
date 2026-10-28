import { flushPromises, mount } from '@vue/test-utils'
import Antd, { message } from 'ant-design-vue'
import { afterEach, describe, expect, it, vi } from 'vitest'

vi.mock('@/api/assets/application', () => ({
  getApplicationList: vi.fn(async () => ({ data: { data: { results: [{ id: 1, name: 'App' }] } } })),
  getApplicationDeploymentTemplate: vi.fn(async () => ({ data: { data: {
    id: 2, application: 1, name: 'Template', control_type: 'systemd', service_name: 'app',
    run_user: 'app', ports: [{ name: 'HTTP', protocol: 'tcp', port: '${HTTP_PORT}' }],
  } } })),
  saveApplicationDeploymentTemplate: vi.fn(async () => ({ data: { data: { id: 2 } } })),
}))
vi.mock('@/api/monitor', () => ({
  getLogProcessingRules: vi.fn(async () => ({ data: { data: { results: [] } } })),
  getLogCollectionFilterRules: vi.fn(async () => ({ data: { data: { results: [] } } })),
}))

import { saveApplicationDeploymentTemplate } from '@/api/assets/application'
import TemplateDialog from './TemplateDialog.vue'

const wrappers = []
async function openDialog(props = {}) {
  const wrapper = mount(TemplateDialog, {
    props: { open: false, ...props },
    attachTo: document.body,
    global: {
      plugins: [Antd],
      stubs: {
        FontAwesomeIcon: true,
        AModal: { template: '<div><slot /></div>' },
      },
    },
  })
  wrappers.push(wrapper)
  await wrapper.setProps({ open: true })
  await flushPromises()
  Object.assign(wrapper.vm.form, {
    application: 1, name: 'Template', run_user: 'app', service_name: 'app',
  })
  return wrapper
}

afterEach(() => {
  wrappers.splice(0).forEach((wrapper) => wrapper.unmount())
  document.body.innerHTML = ''
  vi.restoreAllMocks()
  vi.clearAllMocks()
})

describe('模板端口表达式', () => {
  it.each([
    ['创建', {}, '${HTTP_PORT}'],
    ['编辑', { templateId: 2 }, '${HTTP_PORT}'],
    ['复制', { copyFromId: 2 }, '${HTTP_PORT}'],
    ['普通数字端口', {}, 8080],
  ])('%s保存保留端口原值', async (_name, props, port) => {
    const wrapper = await openDialog(props)
    if (props.templateId || props.copyFromId) {
      expect(wrapper.vm.form.ports[0].port).toBe('${HTTP_PORT}')
    }
    wrapper.vm.form.ports = [{ name: 'HTTP', protocol: 'tcp', port }]
    await wrapper.findAll('.ant-tabs-tab').find((tab) => tab.text() === '端口').trigger('click')
    await flushPromises()
    const input = wrapper.find('input[placeholder="端口或 ${PORT}"]')
    expect(input.exists()).toBe(true)
    await input.setValue(String(port))
    await wrapper.vm.saveTemplate()
    expect(saveApplicationDeploymentTemplate).toHaveBeenCalledWith(expect.objectContaining({
      ports: [expect.objectContaining({ port })],
    }))
  })

  it.each(['', '0', '65536', '${PORT', 'abc', '1.5'])('非法端口 %s 阻止保存而非变成 0', async (port) => {
    const error = vi.spyOn(message, 'error').mockImplementation(() => {})
    const wrapper = await openDialog()
    wrapper.vm.form.ports = [{ name: 'HTTP', protocol: 'tcp', port }]
    await wrapper.vm.saveTemplate()
    expect(saveApplicationDeploymentTemplate).not.toHaveBeenCalled()
    expect(error).toHaveBeenCalledWith(expect.stringContaining('端口必须为'))
  })
})
