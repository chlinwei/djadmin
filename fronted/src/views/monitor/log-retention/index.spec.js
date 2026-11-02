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

  it('view 用户可通过顶部提示的详情按钮查看全部配置，但没有编辑或保存控件', async () => {
    store.state.perms = ['monitor:view']
    const api = await import('@/api/monitor')
    const { openDeleteConfirm } = await import('@/util/deleteConfirm')
    const wrapper = await mountPage()
    try {
      expect(wrapper.findAll('[data-view-tier]')).toHaveLength(2)
      const tooltips = wrapper.findAllComponents({ name: 'ATooltip' })
      expect(tooltips.find((item) => item.props('title') === '编辑').find('button').exists()).toBe(false)
      expect(tooltips.find((item) => item.props('title') === '删除').find('button').exists()).toBe(false)
      const tooltip = tooltips.find((item) => item.props('title') === '查看详情')
      expect(tooltip.props('placement')).toBe('top')
      const entry = wrapper.findAll('[data-view-tier]')[0]
      expect(entry.classes()).toContain('ant-btn-link')
      expect(entry.classes()).toContain('detail-action')
      expect(entry.text()).toBe('详情')
      await entry.trigger('mouseenter')
      await new Promise((resolve) => setTimeout(resolve, 150))
      await flushPromises()
      expect(document.body.querySelector('.ant-tooltip-inner')?.textContent).toBe('查看详情')
      await entry.trigger('click')
      await flushPromises()

      expect(document.body.querySelector('.ant-modal-title').textContent).toBe('保留档位详情：标准')
      const dialog = document.body.querySelector('.ant-modal-content')
      const details = Object.fromEntries(Array.from(dialog.querySelectorAll('.ant-descriptions-item-label')).map((label) => [
        label.textContent, label.textContent === '状态'
          ? (label.nextElementSibling.querySelector('[role="switch"]').getAttribute('aria-checked') === 'true' ? '启用' : '停用')
          : label.nextElementSibling.textContent.trim(),
      ]))
      expect(details).toEqual({
        档位编码: 'std',
        档位名称: '标准',
        每天写入量: '5 GB/天',
        保留期: '30 天',
        预计占用: '150 GB',
        引用服务: '2',
        滚动最小时长: '1d',
        滚动最小主分片大小: '5gb',
        状态: '启用',
        默认档位: '是',
        备注: '标准日志\n保留三十天',
      })
      expect(dialog.querySelector('input, textarea, .ant-select')).toBeNull()
      expect(dialog.querySelector('.ant-switch').disabled).toBe(true)
      expect(dialog.querySelector('.ant-modal-footer')).toBeNull()
      expect(wrapper.vm.editorOpen).toBe(false)
      wrapper.vm.openEdit(wrapper.vm.tiers[0])
      wrapper.vm.openCreate()
      await wrapper.vm.submit()
      wrapper.vm.confirmDelete(wrapper.vm.tiers[0])
      expect(wrapper.vm.editorOpen).toBe(false)
      expect(api.saveLogRetentionTier).not.toHaveBeenCalled()
      expect(api.batchDeleteLogRetentionTiers).not.toHaveBeenCalled()
      expect(openDeleteConfirm).not.toHaveBeenCalled()
    } finally {
      wrapper.unmount()
    }
  })

  it('关闭详情后可查看另一档位，小时、零引用和空备注均正确显示', async () => {
    store.state.perms = ['monitor:view']
    const wrapper = await mountPage()
    try {
      await wrapper.findAll('[data-view-tier]')[0].trigger('click')
      await flushPromises()
      document.body.querySelector('.ant-modal-close').click()
      await flushPromises()
      expect(wrapper.vm.detailTier).toBeNull()
      await wrapper.findAll('[data-view-tier]')[1].trigger('click')
      await flushPromises()
      const dialog = document.body.querySelector('.ant-modal-content')
      expect(dialog.querySelector('.ant-modal-title').textContent).toBe('保留档位详情：短期')
      for (const text of ['12 小时', '2.5 GB', '30m']) expect(dialog.textContent).toContain(text)
      const details = Object.fromEntries(Array.from(dialog.querySelectorAll('.ant-descriptions-item-label')).map((label) => [
        label.textContent, label.nextElementSibling.textContent.trim(),
      ]))
      expect(details.引用服务).toBe('0')
      expect(details.默认档位).toBe('否')
      expect(details.备注).toBe('-')
    } finally {
      wrapper.unmount()
    }
  })

  it('管理员沿用详情权限，普通用户缺少查看权限时不能打开详情', async () => {
    store.state.perms = []
    const wrapper = await mountPage()
    try {
      expect(wrapper.find('[data-view-tier]').exists()).toBe(false)
      wrapper.vm.openDetails(wrapper.vm.tiers[0])
      expect(wrapper.vm.detailTier).toBeNull()
      store.state.role_codes = ['admin']
      await flushPromises()
      const initialForm = JSON.stringify(wrapper.vm.form)
      await wrapper.findAll('[data-view-tier]')[0].trigger('click')
      await flushPromises()
      expect(document.body.querySelector('.ant-descriptions')).not.toBeNull()
      expect(document.body.querySelector('.ant-modal-footer')).toBeNull()
      expect(JSON.stringify(wrapper.vm.form)).toBe(initialForm)
      const action = wrapper.vm.columns.find((column) => column.key === 'action')
      expect(action).toMatchObject({ fixed: 'right', width: 220 })
      expect(wrapper.findComponent({ name: 'ATable' }).props('scroll')).toEqual({ x: 1290 })
    } finally {
      wrapper.unmount()
    }
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
