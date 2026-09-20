import { flushPromises, mount } from '@vue/test-utils'
import Antd from 'ant-design-vue'
import { afterEach, describe, expect, it, vi } from 'vitest'

vi.mock('@/api/assets/application', () => ({
  verifyApplicationServiceLogFormat: vi.fn(() => Promise.resolve({ data: { data: {
    passed: true, missing_fields: [], format_verified_source: 'instance',
  } } })),
}))

import LogFormatVerifyDialog from './LogFormatVerifyDialog.vue'

// 共享的格式认证弹窗（架构文档 §4.8）：逻辑服务编辑弹窗与日志中心页都用它，
// 所以提交语义、失败展示、参数校验都在这里钉住。
function mountDialog(props = {}) {
  return mount(LogFormatVerifyDialog, {
    props: {
      open: true,
      serviceId: 20,
      target: { log_definition: 81, name: 'application.log' },
      deploymentOptions: [
        { label: 'tomcat-1（node-3）', value: 13 },
        { label: 'tomcat-2（node-4）', value: 14 },
      ],
      ...props,
    },
    attachTo: document.body,
    global: {
      plugins: [Antd],
      stubs: { AModal: { template: '<div><slot /></div>' } },
    },
  })
}

describe('LogFormatVerifyDialog', () => {
  afterEach(() => {
    document.body.innerHTML = ''
    vi.clearAllMocks()
  })

  it('submits an instance sample by default and emits verified on pass', async () => {
    const { verifyApplicationServiceLogFormat } = await import('@/api/assets/application')
    const wrapper = mountDialog()
    await flushPromises()

    // 默认依据是实例抽样（最贴近真实采集），并预选第一个候选实例。
    expect(wrapper.vm.form).toMatchObject({ source: 'instance', deployment_id: 13 })

    await wrapper.vm.submit()
    await flushPromises()

    expect(verifyApplicationServiceLogFormat).toHaveBeenCalledWith(20, {
      log_definition_id: 81, source: 'instance', all_deployments: false, deployment_id: 13,
    })
    expect(wrapper.emitted('verified')).toBeTruthy()
    expect(wrapper.emitted('update:open').at(-1)).toEqual([false])
    wrapper.unmount()
  })

  // 认证范围：全部实例时不需要选具体实例，deployment_id 传 0。
  it('submits all_deployments=true without a specific deployment', async () => {
    const { verifyApplicationServiceLogFormat } = await import('@/api/assets/application')
    const wrapper = mountDialog()
    await flushPromises()
    wrapper.vm.form.all_deployments = true
    await wrapper.vm.submit()
    await flushPromises()

    expect(verifyApplicationServiceLogFormat).toHaveBeenCalledWith(20, {
      log_definition_id: 81, source: 'instance', all_deployments: true, deployment_id: 0,
    })
    wrapper.unmount()
  })

  it('sends deployment_id=0 for sources that do not sample an instance', async () => {
    const { verifyApplicationServiceLogFormat } = await import('@/api/assets/application')
    const wrapper = mountDialog()
    wrapper.vm.form.source = 'sample_log'
    await wrapper.vm.submit()
    await flushPromises()

    expect(verifyApplicationServiceLogFormat).toHaveBeenCalledWith(20, {
      log_definition_id: 81, source: 'sample_log', all_deployments: false, deployment_id: 0,
    })
    wrapper.unmount()
  })

  // 校验不通过是业务结果（200 + passed=false），不是接口错误：弹窗留在原地列出缺哪些必备字段，
  // 用户可以换依据重试，而不是只看到一句"失败"。
  it('stays open with the missing required fields when the rule cannot produce them', async () => {
    const { verifyApplicationServiceLogFormat } = await import('@/api/assets/application')
    verifyApplicationServiceLogFormat.mockResolvedValueOnce({
      data: { data: { passed: false, missing_fields: ['log_level', 'error_fingerprint'], format_state: 'unverified' } },
    })
    const wrapper = mountDialog()
    await wrapper.vm.submit()
    await flushPromises()

    expect(wrapper.vm.result.missing_fields).toEqual(['log_level', 'error_fingerprint'])
    expect(document.body.textContent).toContain('error_fingerprint')
    // 不通过时不关闭弹窗、不发 verified。
    expect(wrapper.emitted('verified')).toBeFalsy()
    expect(wrapper.emitted('update:open')).toBeFalsy()
    wrapper.unmount()
  })

  it('warns instead of calling the API when instance sampling has no deployment', async () => {
    const { verifyApplicationServiceLogFormat } = await import('@/api/assets/application')
    const wrapper = mountDialog({ deploymentOptions: [] })
    await flushPromises()

    expect(wrapper.vm.form.deployment_id).toBeNull()
    await wrapper.vm.submit()
    await flushPromises()

    expect(verifyApplicationServiceLogFormat).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  // 重新打开（换一条日志）时必须清掉上一次的失败提示与依据选择，否则会把上一条的结论
  // 误当成这一条的。
  it('resets the previous result when reopened', async () => {
    const { verifyApplicationServiceLogFormat } = await import('@/api/assets/application')
    verifyApplicationServiceLogFormat.mockResolvedValueOnce({
      data: { data: { passed: false, missing_fields: ['log_level'] } },
    })
    const wrapper = mountDialog()
    await wrapper.vm.submit()
    await flushPromises()
    expect(wrapper.vm.result).toBeTruthy()

    await wrapper.setProps({ open: false })
    await wrapper.setProps({ open: true })
    await flushPromises()

    expect(wrapper.vm.result).toBeNull()
    expect(wrapper.vm.form).toMatchObject({ source: 'instance', deployment_id: 13 })
    wrapper.unmount()
  })
})
