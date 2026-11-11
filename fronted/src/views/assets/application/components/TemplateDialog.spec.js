import { flushPromises, mount } from '@vue/test-utils'
import { compileStyle, parse } from '@vue/compiler-sfc'
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

import { getApplicationDeploymentTemplate, getApplicationList, saveApplicationDeploymentTemplate } from '@/api/assets/application'
import { getLogCollectionFilterRules, getLogProcessingRules } from '@/api/monitor'
import TemplateDialog from './TemplateDialog.vue'
import templateSource from './TemplateDialog.vue?raw'

const wrappers = []
async function openDialog(props = {}) {
  const wrapper = mount(TemplateDialog, {
    props: { open: false, ...props },
    attachTo: document.body,
    global: {
      plugins: [Antd],
      stubs: {
        FontAwesomeIcon: true,
        AModal: {
          name: 'AModal',
          props: ['title', 'okText'],
          template: '<div><slot /><footer><slot name="footer" /></footer></div>',
        },
      },
    },
  })
  wrappers.push(wrapper)
  await wrapper.setProps({ open: true })
  await flushPromises()
  if (!props.readOnly) {
    Object.assign(wrapper.vm.form, {
      application: 1, name: 'Template', run_user: 'app', service_name: 'app',
    })
  }
  return wrapper
}

afterEach(() => {
  wrappers.splice(0).forEach((wrapper) => wrapper.unmount())
  document.body.innerHTML = ''
  vi.restoreAllMocks()
  vi.clearAllMocks()
})

describe('部署模板只读详情', () => {
  it('只读输入框和下拉框使用深色文字、浅色背景，同时保持禁用', async () => {
    const wrapper = await openDialog({ templateId: 2, readOnly: true })
    const form = wrapper.find('.template-readonly')
    const scopeId = Object.keys(form.attributes()).find((name) => name.startsWith('data-v-'))
    const { descriptor } = parse(templateSource)
    const style = document.createElement('style')
    style.textContent = compileStyle({ source: descriptor.styles[0].content, filename: 'TemplateDialog.vue', id: scopeId, scoped: true }).code
    document.head.appendChild(style)
    try {
      for (const selector of ['input.ant-input', '.ant-select-selector']) {
        const element = form.find(selector).element
        const computed = window.getComputedStyle(element)
        expect(computed.color).toBe('rgb(38, 38, 38)')
        expect(computed.backgroundColor).toBe('rgb(250, 250, 250)')
        expect(computed.opacity).toBe('1')
      }
      expect(form.find('input.ant-input').attributes('disabled')).toBeDefined()
    } finally {
      style.remove()
    }
    await wrapper.setProps({ readOnly: false })
    expect(wrapper.find('.template-readonly').exists()).toBe(false)
  })

  it('加载完整模板且不请求其他模块，全部页签禁止修改并隐藏增删入口', async () => {
    getApplicationDeploymentTemplate.mockResolvedValueOnce({ data: { data: {
      id: 2, application: 1, application_name: 'Tomcat', name: 'Readonly template',
      control_type: 'command', run_user: 'app', app_home: '/srv/app', remark: '模板备注',
      macro_definitions: [{ name: 'HTTP_PORT', value: '8080', description: 'HTTP 端口' }],
      control_actions: [
        { action: 'start', command: './start.sh' }, { action: 'stop', command: './stop.sh' },
        { action: 'status', command: './status.sh' },
      ],
      ports: [{ name: 'HTTP', protocol: 'tcp', port: '${HTTP_PORT}' }],
      paths: [{ name: 'Logs', path_type: 'log', path: '${APP_HOME}/logs' }],
      config_files: [{ name: 'Config', path: '${APP_HOME}/app.yaml', file_format: 'yaml' }],
      logs: [{ name: 'App log', path_pattern: '${APP_HOME}/logs/*.log', processing_rule: 7, filter_include_rule: 8, filter_exclude_rule: 9 }],
    } } })
    const wrapper = await openDialog({ templateId: 2, readOnly: true })
    expect(getApplicationDeploymentTemplate).toHaveBeenCalledWith(2)
    expect(getApplicationList).not.toHaveBeenCalled()
    expect(getLogProcessingRules).not.toHaveBeenCalled()
    expect(getLogCollectionFilterRules).not.toHaveBeenCalled()
    expect(wrapper.findComponent({ name: 'AModal' }).props('title')).toContain('查看部署模板')
    expect(wrapper.find('footer').text().replace(/\s/g, '')).toBe('关闭')
    expect(wrapper.vm.commandValues).toMatchObject({ start: './start.sh', stop: './stop.sh', status: './status.sh' })
    expect(wrapper.text()).toContain('Tomcat')
    expect(wrapper.findAll('textarea').some((input) => input.element.value === '模板备注')).toBe(true)
    expect(wrapper.vm.macroColumns.some((column) => column.key === 'action')).toBe(false)
    for (const tabName of ['运行配置', '端口', '路径与文件']) {
      await wrapper.findAll('.ant-tabs-tab').find((tab) => tab.text() === tabName).trigger('click')
      await flushPromises()
      expect(wrapper.findAll('input, textarea, .ant-switch').every((input) => input.attributes('disabled') !== undefined)).toBe(true)
      expect(wrapper.text()).not.toMatch(/新增宏|新增端口|新增路径|新增配置文件|新增日志|移除/)
    }
    const values = wrapper.findAll('input').map((input) => input.element.value)
    expect(values).toEqual(expect.arrayContaining([
      '${HTTP_PORT}', '${APP_HOME}/logs', '${APP_HOME}/app.yaml', '${APP_HOME}/logs/*.log',
      '处理规则 ID: 7', '保留规则 ID: 8', '丢弃规则 ID: 9',
    ]))
    const warning = vi.spyOn(message, 'warning').mockImplementation(() => {})
    await wrapper.vm.saveTemplate()
    expect(warning).toHaveBeenCalledWith('只读模式不能保存部署模板')
    expect(saveApplicationDeploymentTemplate).not.toHaveBeenCalled()
    expect(wrapper.emitted('saved')).toBeUndefined()
    await wrapper.find('footer button').trigger('click')
    expect(wrapper.emitted('update:open')).toEqual([[false]])
  })

  it.each([
    ['systemd', { service_name: 'tomcat', systemd_scope: 'user' }, 'tomcat'],
    ['docker', { docker_config: { container_name: 'tomcat-container' } }, 'tomcat-container'],
    ['docker_compose', { compose_config: { project_name: 'tomcat-project' } }, 'tomcat-project'],
    ['external_ha', { ha_resource_name: 'tomcat-resource', control_actions: [{ action: 'status', command: 'ha status' }] }, 'tomcat-resource'],
  ])('只读展示 %s 配置', async (control_type, fields, value) => {
    getApplicationDeploymentTemplate.mockResolvedValueOnce({ data: { data: {
      id: 2, application: 1, application_name: 'Tomcat', name: 'Template', control_type, ...fields,
    } } })
    const wrapper = await openDialog({ templateId: 2, readOnly: true })
    expect(wrapper.findAll('input').some((input) => input.element.value === value)).toBe(true)
    expect(wrapper.findAll('input, textarea').every((input) => input.attributes('disabled') !== undefined)).toBe(true)
    if (control_type === 'systemd') {
      expect(wrapper.findAll('.ant-segmented input').every((input) => input.attributes('disabled') !== undefined)).toBe(true)
    }
  })
})

describe('模板命令行控制', () => {
  it.each([{}, { templateId: 2 }, { copyFromId: 2 }])('启动和停止命令留空时仍可保存 %j', async (props) => {
    const wrapper = await openDialog(props)
    wrapper.vm.form.control_type = 'command'
    Object.assign(wrapper.vm.commandValues, { start: '', stop: '  ', status: './status.sh' })
    await flushPromises()
    const labels = wrapper.findAll('.command-grid .ant-form-item-label').map((label) => label.text())
    expect(labels).toContain('启动命令')
    expect(labels).toContain('停止命令')
    expect(labels).toContain('状态命令（必填）')
    await wrapper.vm.saveTemplate()
    expect(saveApplicationDeploymentTemplate).toHaveBeenCalledWith(expect.objectContaining({
      id: props.templateId,
      control_actions: [{ action: 'status', command: './status.sh', timeout_seconds: 60, success_exit_codes: [0] }],
    }))
    expect(wrapper.emitted('saved')).toHaveLength(1)
  })

  it.each([
    ['start'],
    ['stop'],
    ['start', 'stop'],
  ])('保留已填写的可选命令 %j', async (...actions) => {
    const wrapper = await openDialog()
    wrapper.vm.form.control_type = 'command'
    wrapper.vm.commandValues.status = './status.sh'
    for (const action of actions) wrapper.vm.commandValues[action] = `./${action}.sh`
    await wrapper.vm.saveTemplate()
    expect(saveApplicationDeploymentTemplate).toHaveBeenCalledWith(expect.objectContaining({
      control_actions: [...actions, 'status'].map((action) => ({
        action, command: `./${action}.sh`, timeout_seconds: 60, success_exit_codes: [0],
      })),
    }))
  })

  it.each(['', '  '])('状态命令为空 %j 时仍阻止保存', async (status) => {
    const error = vi.spyOn(message, 'error').mockImplementation(() => {})
    const wrapper = await openDialog()
    wrapper.vm.form.control_type = 'command'
    Object.assign(wrapper.vm.commandValues, { start: './start.sh', stop: './stop.sh', status })
    await wrapper.vm.saveTemplate()
    expect(error).toHaveBeenCalledWith('请填写状态命令')
    expect(saveApplicationDeploymentTemplate).not.toHaveBeenCalled()
  })
})

describe('模板端口表达式', () => {
  it.each([{}, { templateId: 2 }, { copyFromId: 2 }])('模板保存保留固定和通配排除目录 %j', async (props) => {
    const wrapper = await openDialog(props)
    wrapper.vm.form.logs = [{ id: 8, name: 'error', path_pattern: '${LOG_DIR}/*/error.log', exclude_directories: ['${LOG_DIR}/backup'] }]
    await wrapper.findAll('.ant-tabs-tab').find((tab) => tab.text() === '路径与文件').trigger('click')
    await flushPromises()
    const exclusions = wrapper.get('.log-exclude-directories textarea')
    expect(exclusions.element.value).toBe('${LOG_DIR}/backup')
    await exclusions.setValue('${LOG_DIR}/backup\n')
    expect(exclusions.element.value).toBe('${LOG_DIR}/backup\n')
    await exclusions.setValue('${LOG_DIR}/backup\n  ${LOG_DIR}/test-*  \n\n${LOG_DIR}/node[12]/archive')
    await wrapper.vm.saveTemplate()
    expect(saveApplicationDeploymentTemplate).toHaveBeenCalledWith(expect.objectContaining({
      logs: [expect.objectContaining({
        exclude_directories: ['${LOG_DIR}/backup', '${LOG_DIR}/test-*', '${LOG_DIR}/node[12]/archive'],
      })],
    }))
    expect(saveApplicationDeploymentTemplate.mock.calls[0][0].logs[0].id).toBe(props.templateId ? 8 : undefined)
  })

  it('只读模板回显排除目录且禁止修改', async () => {
    getApplicationDeploymentTemplate.mockResolvedValueOnce({ data: { data: {
      id: 2, application: 1, name: 'Template', control_type: 'systemd', service_name: 'app',
      logs: [{ name: 'error', path_pattern: '/logs/*/error.log', exclude_directories: ['/logs/backup', '/logs/test-*'] }],
    } } })
    const wrapper = await openDialog({ templateId: 2, readOnly: true })
    await wrapper.findAll('.ant-tabs-tab').find((tab) => tab.text() === '路径与文件').trigger('click')
    await flushPromises()
    const exclusions = wrapper.get('.log-exclude-directories textarea')
    expect(exclusions.element.value).toBe('/logs/backup\n/logs/test-*')
    expect(exclusions.attributes('disabled')).toBeDefined()
  })

  it.each([
    ['空白行', [{ name: 'ORACLE_PDB', value: 'aodb' }, { name: '', value: '', description: '' }], '第 2 行宏名必须'],
    ['非法宏名', [{ name: 'ORACLE-PDB', value: 'aodb' }], '第 1 行宏名必须'],
    ['重复宏名', [{ name: 'ORACLE_PDB', value: 'aodb' }, { name: 'ORACLE_PDB', value: 'other' }], '第 2 行宏名 ORACLE_PDB 重复'],
    ['值含换行', [{ name: 'ORACLE_PDB', value: 'aodb\nother' }], '第 1 行宏 ORACLE_PDB 的值不能包含换行'],
  ])('复制模板时%s提示具体原因且不发送请求', async (_label, definitions, expected) => {
    const error = vi.spyOn(message, 'error').mockImplementation(() => {})
    const wrapper = await openDialog({ copyFromId: 2 })
    wrapper.vm.macroDefinitions = definitions
    await wrapper.vm.saveTemplate()
    expect(error).toHaveBeenCalledWith(expect.stringContaining(expected))
    expect(saveApplicationDeploymentTemplate).not.toHaveBeenCalled()
  })

  it('复制模板删除空白宏行后可以重试保存，允许宏值为空且不提交行标识', async () => {
    vi.spyOn(message, 'error').mockImplementation(() => {})
    const wrapper = await openDialog({ copyFromId: 2 })
    wrapper.vm.macroDefinitions = [
      { _uid: 1, name: 'ORACLE_PDB', value: '', description: 'PDB name' },
      { _uid: 2, name: '', value: '', description: '' },
    ]
    await wrapper.vm.saveTemplate()
    expect(saveApplicationDeploymentTemplate).not.toHaveBeenCalled()
    wrapper.vm.macroDefinitions.splice(1, 1)
    await wrapper.vm.saveTemplate()
    expect(saveApplicationDeploymentTemplate).toHaveBeenCalledWith(expect.objectContaining({
      id: undefined,
      macro_definitions: [{ name: 'ORACLE_PDB', value: '', description: 'PDB name' }],
    }))
  })

  it('校验和请求期间重复保存只发送一次请求、提示一次', async () => {
    const success = vi.spyOn(message, 'success').mockImplementation(() => {})
    const wrapper = await openDialog({ templateId: 2 })
    let finishRequest
    saveApplicationDeploymentTemplate.mockImplementationOnce(() => new Promise((resolve) => {
      finishRequest = resolve
    }))
    const firstSave = wrapper.vm.saveTemplate()
    await Promise.all([wrapper.vm.saveTemplate(), wrapper.vm.saveTemplate()])
    await flushPromises()
    expect(saveApplicationDeploymentTemplate).toHaveBeenCalledTimes(1)
    await wrapper.vm.saveTemplate()
    expect(saveApplicationDeploymentTemplate).toHaveBeenCalledTimes(1)
    finishRequest({ data: { data: { id: 2 } } })
    await firstSave
    expect(success).toHaveBeenCalledTimes(1)
    expect(wrapper.emitted('saved')).toHaveLength(1)
  })

  it('校验失败和请求失败后均可重新保存', async () => {
    vi.spyOn(message, 'error').mockImplementation(() => {})
    vi.spyOn(message, 'success').mockImplementation(() => {})
    const wrapper = await openDialog({ templateId: 2 })
    wrapper.vm.form.ports = [{ port: '0' }]
    await wrapper.vm.saveTemplate()
    expect(saveApplicationDeploymentTemplate).not.toHaveBeenCalled()
    wrapper.vm.form.ports = []
    saveApplicationDeploymentTemplate.mockRejectedValueOnce(new Error('保存失败'))
    await wrapper.vm.saveTemplate()
    await wrapper.vm.saveTemplate()
    expect(saveApplicationDeploymentTemplate).toHaveBeenCalledTimes(2)
    expect(wrapper.emitted('saved')).toHaveLength(1)
  })

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
