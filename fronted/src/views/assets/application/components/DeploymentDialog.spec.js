import { flushPromises, mount } from '@vue/test-utils'
import Antd from 'ant-design-vue'
import { afterEach, describe, expect, it, vi } from 'vitest'

vi.mock('@/api/assets/host', () => ({
  getHostList: vi.fn(() => Promise.resolve({
    data: { data: { results: [{ id: 136, instance_name: 'mysql136', ip: '10.25.66.136' }], totalPages: 1 } },
  })),
}))

vi.mock('@/api/assets/application', () => ({
  getApplicationDeployment: vi.fn(() => Promise.resolve({
    data: { data: { id: 1, host: 136, host_name: 'mysql136', instance_name: '', enabled: true } },
  })),
  getApplicationDeploymentTemplateList: vi.fn(() => Promise.resolve({
    data: { data: { results: [], totalPages: 1 } },
  })),
  getApplicationVersionList: vi.fn(() => Promise.resolve({
    data: { data: { results: [], totalPages: 1 } },
  })),
  saveApplicationDeployment: vi.fn(),
}))

import DeploymentDialog from './DeploymentDialog.vue'

describe('DeploymentDialog', () => {
  afterEach(() => {
    document.body.innerHTML = ''
  })

  it('defaults an empty deployment instance name to the selected host name', async () => {
    const wrapper = mount(DeploymentDialog, {
      props: { open: false, deploymentId: 1 },
      attachTo: document.body,
      global: {
        plugins: [Antd],
        stubs: { AModal: { template: '<div><slot /></div>' } },
      },
    })

    await wrapper.setProps({ open: true })
    await flushPromises()

    const instanceNameInput = document.body.querySelector('input[placeholder="例如 tomcat-order-prod"]')
    expect(instanceNameInput.value).toBe('mysql136')
    wrapper.unmount()
  })

  it('fills the instance name when a host is picked in edit mode', async () => {
    const wrapper = mount(DeploymentDialog, {
      props: { open: false, deploymentId: 2 },
      attachTo: document.body,
      global: {
        plugins: [Antd],
        stubs: { AModal: { template: '<div><slot /></div>' } },
      },
    })

    await wrapper.setProps({ open: true })
    await flushPromises()

    wrapper.vm.form.host = null
    await flushPromises()
    wrapper.vm.form.host = 136
    await flushPromises()

    expect(wrapper.vm.form.instance_name).toBe('mysql136')
    wrapper.unmount()
  })

  it('no longer asks for version and template because the service owns them', async () => {
    const wrapper = mount(DeploymentDialog, {
      props: { open: false, applicationServiceId: 4 },
      attachTo: document.body,
      global: {
        plugins: [Antd],
        stubs: { AModal: { template: '<div><slot /></div>' } },
      },
    })

    await wrapper.setProps({ open: true })
    await flushPromises()

    expect(document.body.textContent).not.toContain('应用版本')
    expect(document.body.textContent).not.toContain('部署模板')
    wrapper.unmount()
  })
})
