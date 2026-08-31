import { flushPromises, mount } from '@vue/test-utils'
import Antd from 'ant-design-vue'
import { describe, expect, it, vi } from 'vitest'

import LogStoragePage from './index.vue'

vi.mock('@/store', () => ({ default: { state: { user: { timezone: 'Asia/Shanghai' } } } }))

const CLUSTER = {
  id: 1,
  name: '日志集群',
  hosts: 'https://10.25.66.150:9200',
  index_prefix: 'logs',
  enabled: true,
  is_default: true,
  verify_tls: false,
  last_check_success: true,
  last_check_message: 'opensearch 2.19.0',
  last_check_time: '2026-08-30T10:00:00Z',
  storage_sync_status: 'success',
  storage_sync_error: '',
  storage_sync_time: '2026-08-30T10:01:00Z',
}

// 覆盖真实返回的形态：含正常层、漂移层，以及明细为空的层（会渲染 a-empty）。
const HEALTH = {
  status: 'drift',
  checked_at: '2026-08-30T11:00:00Z',
  layers: [
    {
      key: 'index_template',
      name: '索引模板',
      status: 'ok',
      summary: '17 项全部一致',
      items: [{ name: 'error_fingerprint', status: 'ok', detail: 'keyword' }],
    },
    {
      key: 'host_configs',
      name: '主机配置',
      status: 'drift',
      summary: '1/19 项需要处理',
      items: [{ name: '192.168.201.215', status: 'drift', detail: '配置已变更但未下发' }],
    },
    { key: 'data_flow', name: '数据写入', status: 'warn', summary: '最近 15 分钟没有新日志写入', items: [] },
  ],
}

vi.mock('@/api/monitor', () => ({
  getOpenSearchClusterList: vi.fn(() => Promise.resolve({ data: { data: { results: [CLUSTER], count: 1 } } })),
  getLogPipelineHealth: vi.fn(() => Promise.resolve({ data: { data: HEALTH } })),
  saveOpenSearchCluster: vi.fn(() => Promise.resolve({ data: { data: CLUSTER } })),
  deleteOpenSearchCluster: vi.fn(() => Promise.resolve({ data: { data: null } })),
  testOpenSearchCluster: vi.fn(() => Promise.resolve({ data: { data: {} } })),
}))

function mountPage() {
  return mount(LogStoragePage, {
    global: {
      plugins: [Antd],
      stubs: { FontAwesomeIcon: true },
    },
    attachTo: document.body,
  })
}

describe('日志存储页面', () => {
  it('加载集群列表后不会陷入渲染循环', async () => {
    const wrapper = mountPage()
    await flushPromises()
    expect(wrapper.text()).toContain('日志集群')
    wrapper.unmount()
  })

  it('体检结果渲染完整，且明细为空的层不会导致渲染异常', async () => {
    const wrapper = mountPage()
    await flushPromises()

    await wrapper.vm.loadHealth()
    await flushPromises()

    const text = wrapper.text()
    expect(text).toContain('主机配置')
    expect(text).toContain('配置已变更但未下发')
    // 明细为空的层走 a-empty 分支，历史上共享 VNode 会在这里炸掉。
    expect(text).toContain('无明细')
    wrapper.unmount()
  })

  it('默认只展开有问题的层', async () => {
    const wrapper = mountPage()
    await flushPromises()

    await wrapper.vm.loadHealth()
    await flushPromises()

    expect(wrapper.vm.openLayers).toEqual(['host_configs', 'data_flow'])
    wrapper.unmount()
  })
})
