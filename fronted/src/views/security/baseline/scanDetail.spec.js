import { flushPromises, mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import ScanDetail from './scanDetail.vue'
import requestUtil from '@/util/request'

vi.mock('vue-router', () => ({
  useRoute: () => ({ params: { id: '5' } }),
  useRouter: () => ({ push: vi.fn(), back: vi.fn() }),
}))
vi.mock('@/util/request', () => ({ default: { get: vi.fn() } }))

const slotStub = { template: '<div><slot /></div>' }
const descriptionStub = {
  props: ['label'],
  template: '<div :data-label="label"><slot /></div>',
}
const stubs = {
  'a-card': slotStub, 'a-spin': slotStub, 'a-descriptions': slotStub,
  'a-descriptions-item': descriptionStub, 'a-space': slotStub, 'a-button': true,
  'a-popconfirm': true, 'a-alert': true, 'a-progress': true, 'a-tooltip': true,
  'a-popover': true, 'a-table': true, 'a-tag': slotStub, 'a-input-search': true,
  'a-select': true, 'a-radio-group': true, 'a-radio-button': true, FontAwesomeIcon: true,
}

describe('扫描详情的项目与环境', () => {
  it.each([
    ['项目环境范围', { mount_type: 'environment', project_id: 2, project_name: '核心项目', environment_id: 3, environment_name: '生产' }, '核心项目', '生产'],
    ['项目范围', { mount_type: 'project', project_name: '核心项目', environments: [{ id: 1, name: 'test' }, { id: 2, name: 'prod' }] }, '核心项目', 'test、prod'],
    ['关联已删除', { mount_type: 'environment', project_id: 2, environment_id: 3 }, '项目 #2（已删除或不可用）', '环境 #3（已删除或不可用）'],
    ['未记录范围', { mount_type: 'environment' }, '-', '未记录环境'],
  ])('%s', async (_label, scope, project, environment) => {
    requestUtil.get.mockResolvedValueOnce({
      data: { data: { scan: { id: 5, baseline: '基线', status: 'success', ...scope }, targets: [], items: [] } },
    })
    const wrapper = mount(ScanDetail, {
      global: {
        stubs,
      },
    })
    await flushPromises()
    expect(requestUtil.get).toHaveBeenLastCalledWith('sys/security/scans/5/')
    expect(wrapper.find('[data-label="项目"]').text()).toBe(project)
    expect(wrapper.find('[data-label="环境"]').text()).toBe(environment)
    wrapper.unmount()
  })

  it('按环境同时过滤两张表，多环境主机不重复，切换重置分页和无效主机条件', async () => {
    const test = { id: 1, name: 'test' }
    const prod = { id: 2, name: 'prod' }
    const targets = [
      { host_id: 10, host_name: 'shared', host_ip: '10.0.0.1', environments: [test, prod] },
      { host_id: 11, host_name: 'test-only', host_ip: '10.0.0.2', environments: [test] },
      { host_id: 12, host_name: 'unknown', host_ip: '10.0.0.3', environments: [] },
    ]
    requestUtil.get.mockResolvedValueOnce({ data: { data: {
      scan: { id: 5, mount_type: 'project', environments: [test, prod], environment_source: 'current_assets' },
      targets, items: targets.map((target, i) => ({ ...target, status: i === 0 ? 'pass' : 'fail' })),
    } } })
    const wrapper = mount(ScanDetail, { global: { stubs } })
    await flushPromises()
    expect(wrapper.find('[data-label="环境"]').text()).toBe('test、prod')
    expect(wrapper.find('a-alert-stub').attributes('message')).toContain('非扫描时快照')
    expect(wrapper.vm.environmentOptions).toEqual([
      { value: 1, label: 'test' }, { value: 2, label: 'prod' }, { value: 'unknown', label: '未记录环境' },
    ])
    wrapper.vm.environmentFilter = 1
    await flushPromises()
    expect(wrapper.vm.filteredTargets.map((row) => row.host_id)).toEqual([10, 11])
    expect(wrapper.vm.filteredItems).toHaveLength(2)
    expect(wrapper.vm.failedCount).toBe(1)
    wrapper.vm.hostFilter = 11
    wrapper.vm.targetPagination.current = 2
    wrapper.vm.itemPagination.current = 2
    wrapper.vm.environmentFilter = 2
    await flushPromises()
    expect(wrapper.vm.hostFilter).toBeUndefined()
    expect(wrapper.vm.filteredTargets.map((row) => row.host_id)).toEqual([10])
    expect(wrapper.vm.filteredItems.map((row) => row.host_id)).toEqual([10])
    expect(wrapper.vm.targetPagination.current).toBe(1)
    expect(wrapper.vm.itemPagination.current).toBe(1)
    expect(wrapper.vm.targetPagination.total).toBe(1)
    wrapper.vm.environmentFilter = 'unknown'
    await flushPromises()
    expect(wrapper.vm.filteredTargets.map((row) => row.host_id)).toEqual([12])
    wrapper.vm.environmentFilter = undefined
    await flushPromises()
    expect(wrapper.vm.filteredTargets).toHaveLength(3)
    expect(wrapper.vm.filteredItems).toHaveLength(3)
    wrapper.unmount()
  })
})
