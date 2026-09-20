import { flushPromises, mount } from '@vue/test-utils'
import Antd from 'ant-design-vue'
import { afterEach, describe, expect, it, vi } from 'vitest'

vi.mock('@/api/assets/application', () => ({
  previewApplicationServiceLogGlob: vi.fn(),
}))

import LogGlobPreview from './LogGlobPreview.vue'

// 路径通配的按需展开：只有点了「展开文件」才调接口，结果按实例分组列出真实文件。
function mountPreview(props = {}) {
  return mount(LogGlobPreview, {
    props: { serviceId: 20, logDefinitionId: 24, ...props },
    attachTo: document.body,
    global: { plugins: [Antd] },
  })
}

describe('LogGlobPreview', () => {
  afterEach(() => {
    document.body.innerHTML = ''
    vi.clearAllMocks()
  })

  it('点击展开才调接口，并按实例分组列出匹配文件', async () => {
    const { previewApplicationServiceLogGlob } = await import('@/api/assets/application')
    previewApplicationServiceLogGlob.mockResolvedValue({ data: { data: {
      path_pattern: '/home/esb/data/logs/*/log_error.log',
      instances: [
        {
          host_instance_name: 'node-1', deployment_instance_name: 'tomcat-1',
          pattern: '/home/esb/data/logs/*/log_error.log',
          matches: ['/home/esb/data/logs/app1/log_error.log', '/home/esb/data/logs/app2/log_error.log'],
        },
        {
          host_instance_name: 'node-2', deployment_instance_name: 'tomcat-2',
          pattern: '/home/esb/data/logs/*/log_error.log',
          matches: [],
          error: '读取远端日志文件失败（主机 agent 可能离线）：agent offline',
        },
      ],
    } } })

    const wrapper = mountPreview()
    expect(previewApplicationServiceLogGlob).not.toHaveBeenCalled()

    await wrapper.find('button').trigger('click')
    await flushPromises()

    expect(previewApplicationServiceLogGlob).toHaveBeenCalledWith(20, 24)
    const text = wrapper.text()
    expect(text).toContain('tomcat-1')
    expect(text).toContain('/home/esb/data/logs/app1/log_error.log')
    expect(text).toContain('/home/esb/data/logs/app2/log_error.log')
    // 一台主机失败不影响另一台的清单：错误只出现在它自己那一项。
    expect(text).toContain('agent offline')
    wrapper.unmount()
  })

  it('接口失败给出提示且不进入已展开态', async () => {
    const { previewApplicationServiceLogGlob } = await import('@/api/assets/application')
    previewApplicationServiceLogGlob.mockRejectedValue({ response: { data: { msg: 'boom' } } })

    const wrapper = mountPreview()
    await wrapper.find('button').trigger('click')
    await flushPromises()

    expect(wrapper.vm.loaded).toBe(false)
    expect(wrapper.text()).toContain('展开文件')
    wrapper.unmount()
  })

  it('缺少服务或日志定义时不发请求', async () => {
    const { previewApplicationServiceLogGlob } = await import('@/api/assets/application')
    const wrapper = mountPreview({ serviceId: null })
    await wrapper.find('button').trigger('click')
    await flushPromises()
    expect(previewApplicationServiceLogGlob).not.toHaveBeenCalled()
    wrapper.unmount()
  })
})
