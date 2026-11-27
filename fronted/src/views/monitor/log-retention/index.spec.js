import { flushPromises, mount } from '@vue/test-utils'
import Antd from 'ant-design-vue'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import store from '@/store'

beforeEach(() => {
  store.state.role_codes = ['operator']
  store.state.perms = ['monitor:view', 'monitor:create', 'monitor:update', 'monitor:delete']
})

// 保留档位的「值 + 单位」（迁移 000045）：这一页是唯一能改保留期的地方，所以接线必须钉住——
//  1. 列表按单位显示（12 小时 / 30 天），不再一律"天"；
//  2. 表单提交 `retention_value` + `retention_unit`，**不带**老的 `retention_days`
//     （后端已按新列校验，带了会 400）；
//  3. 容量反推（预计占用）按单位折算——小时档位不折算会虚高 24 倍。
vi.mock('@/api/monitor', () => ({
  getLogRetentionTiers: vi.fn(() => Promise.resolve({ data: { data: { results: [
    { id: 1, code: 'std', name: '标准', daily_size_gb: 5, retention_value: 30, retention_unit: 'd', rollover_min_index_age: '1d', rollover_min_primary_shard_size: '5gb', estimated_total_gb: 150, enabled: true, is_default: true, service_count: 2, remark: '标准日志\n保留三十天' },
    { id: 2, code: 'short', name: '短期', daily_size_gb: 5, retention_value: 12, retention_unit: 'h', rollover_min_index_age: '30m', rollover_min_primary_shard_size: '5gb', estimated_total_gb: 2.5, enabled: true, is_default: false, service_count: 0 },
  ] } } })),
  saveLogRetentionTier: vi.fn(() => Promise.resolve({ data: { data: { id: 1 } } })),
  batchDeleteLogRetentionTiers: vi.fn(),
}))
vi.mock('@/util/deleteConfirm', () => ({ openDeleteConfirm: vi.fn(() => Promise.resolve(false)) }))

// antd Table 会调带伪元素的 getComputedStyle，jsdom 不认识（噪声，不影响断言）。
const originalGetComputedStyle = window.getComputedStyle.bind(window)
window.getComputedStyle = (element) => originalGetComputedStyle(element)

import LogRetention from './index.vue'

async function mountPage() {
  const wrapper = mount(LogRetention, {
    attachTo: document.body,
    global: { plugins: [Antd], stubs: { FontAwesomeIcon: true } },
  })
  await flushPromises()
  return wrapper
}

describe('日志保留档位：保留期的值 + 单位', () => {
  afterEach(() => {
    document.body.innerHTML = ''
    vi.clearAllMocks()
  })

  it('renders each tier with its own unit', async () => {
    const wrapper = await mountPage()

    expect(document.body.textContent).toContain('30 天')
    expect(document.body.textContent).toContain('12 小时')
    wrapper.unmount()
  })

  it('只读使用同一张表单：保留小时与单位，禁改、禁存，诊断字段仍可见', async () => {
    store.state.perms = ['monitor:view']
    const api = await import('@/api/monitor')
    const wrapper = await mountPage()
    try {
      const entry = wrapper.findAll('[data-view-tier]')[1]
      expect(wrapper.findAllComponents({ name: 'ATooltip' }).find(t => t.props('title') === '编辑').find('button').exists()).toBe(false)
      await entry.trigger('click')
      await flushPromises()
      const dialog = document.querySelector('.ant-modal-content')
      expect(dialog.querySelector('.ant-modal-title').textContent).toBe('查看保留档位：short')
      expect(dialog.querySelector('.ant-select-disabled').textContent).toContain('小时')
      expect([...dialog.querySelectorAll('input,textarea,button[role="switch"]')].every(el => el.disabled)).toBe(true)
      expect([...dialog.querySelectorAll('input')].some(el => el.value === '12')).toBe(true)
      expect(dialog.textContent).toContain('5gb')
      expect(dialog.textContent).toContain('引用服务')
      expect(dialog.querySelector('.ant-modal-footer').textContent.replace(/\s/g, '')).toBe('关闭')
      await wrapper.vm.submit()
      expect(api.saveLogRetentionTier).not.toHaveBeenCalled()
    } finally { wrapper.unmount() }
  })

  it('编辑与查看入口互斥，无查看权不能打开记录', async () => {
    const wrapper = await mountPage()
    try {
      expect(wrapper.find('[data-view-tier]').exists()).toBe(false)
      store.state.perms = ['monitor:update']
      await flushPromises()
      expect(wrapper.findAllComponents({ name: 'ATooltip' }).find(t => t.props('title') === '编辑').find('button').exists()).toBe(false)
      wrapper.vm.openEdit(wrapper.vm.tiers[0])
      expect(wrapper.vm.editorOpen).toBe(false)
    } finally { wrapper.unmount() }
  })

  it('edits a tier with value + unit and never sends the legacy days field', async () => {
    const { saveLogRetentionTier } = await import('@/api/monitor')
    const wrapper = await mountPage()

    // 打开"短期"档位的编辑：值/单位要按记录回填（单位缺省时按天）。
    wrapper.vm.openEdit(wrapper.vm.tiers[1])
    await flushPromises()
    expect(wrapper.vm.form.retention_value).toBe(12)
    expect(wrapper.vm.form.retention_unit).toBe('h')

    // 容量反推按小时折算：5GB/天 × 12 小时 = 2.5GB（不折算会是 60GB）。
    expect(wrapper.vm.estimatedTotal).toBe(2.5)
    const select = wrapper.findComponent({ name: 'ASelect' })
    const trigger = document.body.querySelector('.ant-modal-content .ant-select')
    expect(select.props('getPopupContainer')(trigger)).toBe(trigger.closest('.ant-modal-wrap'))

    await wrapper.vm.submit()
    await flushPromises()

    const payload = saveLogRetentionTier.mock.calls.at(-1)[0]
    expect(payload.retention_value).toBe(12)
    expect(payload.retention_unit).toBe('h')
    // 老字段必须不再出现：后端已按新列校验，留着它只会让人以为"改单位没生效"。
    expect('retention_days' in payload).toBe(false)
    wrapper.unmount()
  })

  it('limits the value by unit and falls back to days for missing units', async () => {
    const wrapper = await mountPage()

    wrapper.vm.openCreate()
    await flushPromises()
    // 新建默认：天 + 30（与存量档位语义一致）。
    expect(wrapper.vm.form.retention_unit).toBe('d')
    expect(wrapper.vm.form.retention_value).toBe(30)
    expect(wrapper.vm.retentionValueMax).toBe(3650)

    // 切到小时：上限跟着放大到 3650×24，容量反推同步按小时折算。
    wrapper.vm.form.retention_unit = 'h'
    wrapper.vm.form.retention_value = 24
    wrapper.vm.form.daily_size_gb = 10
    await flushPromises()
    expect(wrapper.vm.retentionValueMax).toBe(3650 * 24)
    expect(wrapper.vm.estimatedTotal).toBe(10)

    // 记录里没有单位（历史数据）时按天回填。
    wrapper.vm.openEdit({ id: 9, code: 'legacy', name: '旧档位', daily_size_gb: 1, retention_value: 7 })
    await flushPromises()
    expect(wrapper.vm.form.retention_unit).toBe('d')
    wrapper.unmount()
  })
})
