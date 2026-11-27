import { flushPromises, mount } from '@vue/test-utils'
import Antd from 'ant-design-vue'
import { afterEach, expect, it, vi } from 'vitest'
import InspectionPage from './index.vue'
import request from '@/util/request'
import store from '@/store'
import permission from '@/directives/permission'

vi.mock('@/store', () => ({ default: { state: { perms: [], role_codes: [], user: { timezone: 'Asia/Shanghai' } } } }))
vi.mock('@/util/request', () => ({ default: { get: vi.fn(), post: vi.fn(), patch: vi.fn() } }))

const group = {
  id: 2, name: '数据库检查', application: 8, application_name: '数据库', category: 'application',
  params: [{ name: 'SID' }], checks: [{ name: '配置', config: { input_files: [{ key: 'config', path: '/etc/example', parse: 'lines' }], policy: 'package baseline' } }],
}
const task = {
  id: 7, name: '测试巡检', inspection_name: '夜间检查', cron_expression: '0 1 * * *',
  target_name: '项目 A · 业务 B @ 测试 · 逻辑服务 C', groups: [
    { ...group, service_id: 9, mount_type: 'service', param_values: { SID: { value: 'testing' } } },
  ],
}
let wrapper
afterEach(() => { wrapper?.unmount(); document.body.innerHTML = ''; vi.clearAllMocks() })
async function loadPage(perms) {
  store.state.perms = perms
  request.get.mockImplementation(async url => ({ data: { data: url.endsWith('groups/2/') ? group :
    url.endsWith('tasks/7/') ? task : { results: [], count: 0 } } }))
  wrapper = mount(InspectionPage, {
    attachTo: document.body, global: { plugins: [Antd], directives: { permission }, stubs: { FontAwesomeIcon: true } },
  })
  await flushPromises()
}

it('仅查看巡检任务、组和调度：详情提供关联路径、参数、应用及检查配置', async () => {
  await loadPage(['inspection:view'])
  const listReads = request.get.mock.calls.map(([url]) => url)
  expect(listReads.sort()).toEqual(['sys/inspection/executions/', 'sys/inspection/groups/', 'sys/inspection/tasks/'])
  request.get.mockClear()
  await wrapper.vm.openTaskModal({ id: 7 })
  await flushPromises()
  const modal = document.body.querySelector('.ant-modal-content')
  expect(modal.textContent).toContain(task.target_name)
  expect(modal.textContent).toContain(group.name)
  expect([...modal.querySelectorAll('input')].some(input => input.value === 'testing' && input.disabled)).toBe(true)
  expect(modal.querySelector('.ant-modal-footer').textContent.replace(/\s/g, '')).toBe('关闭')
  await wrapper.vm.submitTask()
  await wrapper.vm.openGroupFromTask({ id: 2 })
  await flushPromises()
  expect(wrapper.vm.groupReadOnly).toBe(true)
  expect(wrapper.vm.groupForm.checks[0].config.input_files[0].path).toBe('/etc/example')
  expect(document.body.textContent).not.toContain('添加采集命令')
  expect(document.body.textContent).toContain('数据库')
  await wrapper.vm.submitGroup()
  await wrapper.vm.openScheduleModal({ id: 7 })
  expect(wrapper.vm.scheduleReadOnly).toBe(true)
  expect(wrapper.vm.scheduleForm.cron_expression).toBe('0 1 * * *')
  await wrapper.vm.submitSchedule()
  expect(request.get.mock.calls.map(([url]) => url)).toEqual(['sys/inspection/tasks/7/', 'sys/inspection/groups/2/', 'sys/inspection/tasks/7/'])
  expect(request.patch).not.toHaveBeenCalled()
  expect(request.post).not.toHaveBeenCalled()
})

it('有任务更新权限时可保存计划；详情显示计划已删除时不再打开', async () => {
  await loadPage(['inspection:view', 'inspection:tasks:update'])
  await wrapper.vm.openScheduleModal({ id: 7 })
  expect(wrapper.vm.scheduleReadOnly).toBe(false)
  wrapper.vm.scheduleForm.cron_expression = '0 2 * * *'
  await wrapper.vm.submitSchedule()
  expect(request.patch).toHaveBeenCalledWith('sys/inspection/tasks/7/', {
    id: 7, inspection_name: '夜间检查', cron_expression: '0 2 * * *',
  })
  request.get.mockResolvedValueOnce({ data: { data: { ...task, cron_expression: '' } } })
  await wrapper.vm.openScheduleModal(task)
  expect(wrapper.vm.scheduleModalOpen).toBe(false)
})
