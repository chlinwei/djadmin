import { flushPromises, mount } from '@vue/test-utils'
import Antd from 'ant-design-vue'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { queryPrometheusInstant } from '@/api/monitor'
import Explore from './index.vue'

vi.mock('@/api/monitor', () => ({
  queryPrometheusInstant: vi.fn(),
  queryPrometheusRange: vi.fn(),
}))
vi.mock('@/store', () => ({ default: { state: { user: { timezone: 'Asia/Shanghai' } } } }))
vi.mock('@/util/request', () => ({ getServerUrl: () => '' }))
vi.mock('./components/PromQLEditor.vue', () => ({
  default: { props: ['modelValue', 'completionRemoteUrl'], template: '<div />' },
}))
vi.mock('./components/PrometheusChart.vue', () => ({
  default: { template: '<div />' },
}))

describe('Explore result pagination', () => {
  let wrapper

  afterEach(() => {
    wrapper?.unmount()
    vi.restoreAllMocks()
  })

  it('renders without undefined pagination and paginates query results locally', async () => {
    const warn = vi.spyOn(console, 'warn')
    queryPrometheusInstant.mockResolvedValue({
      data: { data: {
        result_type: 'vector',
        result: Array.from({ length: 25 }, (_, index) => ({
          metric: { instance: `host-${index}` },
          value: [1700000000, String(index)],
        })),
      } },
    })
    wrapper = mount(Explore, {
      global: { plugins: [Antd] },
    })
    await flushPromises()

    const table = wrapper.findComponent({ name: 'ATable' })
    expect(table.props('pagination')).toMatchObject({ current: 1, pageSize: 10, total: 0 })
    expect(warn.mock.calls.some((args) => args.some((arg) =>
      String(arg).includes('was accessed during render but is not defined'),
    ))).toBe(false)

    wrapper.vm.queryText = 'up'
    await wrapper.vm.runQuery()
    await flushPromises()
    expect(table.props('pagination').total).toBe(25)
    expect(wrapper.findAll('tr.ant-table-row')).toHaveLength(10)

    await wrapper.find('.ant-pagination-next button').trigger('click')
    await flushPromises()
    expect(table.props('pagination').current).toBe(2)
    expect(queryPrometheusInstant).toHaveBeenCalledTimes(1)
    expect(wrapper.find('tr.ant-table-row').text()).toContain('instance=host-10')

    table.vm.$emit('change', { current: 2, pageSize: 20 })
    await flushPromises()
    expect(wrapper.findAll('tr.ant-table-row')).toHaveLength(5)

    await wrapper.vm.runQuery()
    await flushPromises()
    expect(table.props('pagination')).toMatchObject({ current: 1, pageSize: 20, total: 25 })
    expect(wrapper.findAll('tr.ant-table-row')).toHaveLength(20)
  })
})
