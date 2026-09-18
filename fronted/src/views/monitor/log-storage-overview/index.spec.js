import { flushPromises, mount } from '@vue/test-utils'
import Antd from 'ant-design-vue'
import { afterEach, describe, expect, it, vi } from 'vitest'

// 存储水位页的采集状态标注：状态属于**逻辑服务**（配置事实，来自服务行），
// 因此页面上必须能一眼看出"这条流所属的服务已停用 / 未开启采集"，
// 而不是让它们看起来跟正常采集的服务一样。
//
// 背景（2026-09-18）：服务停用后，它的流曾被识别成「未识别」孤儿；修掉识别之后，
// 页面仍然没有任何标记能区分"在采集"和"服务已停用、数据等 ILM 到期"。
vi.mock('@/api/monitor.js', () => ({
  getElasticsearchClusterList: vi.fn(() => Promise.resolve({
    data: { data: { results: [{ id: 2, name: '日志集群', index_prefix: 'autoadmin', enabled: true }] } },
  })),
  getLogStorageOverview: vi.fn(() => Promise.resolve({
    data: { code: 200, data: {
      cluster: { id: 2, index_prefix: 'autoadmin' },
      generated_at: '2026-09-18T12:00:00Z',
      allocation: [],
      alloc_error: '',
      dims: {
        projects: [{ id: 1, code: 'yilake', name: 'yilake' }],
        business_systems: [{ id: 19, code: 'tib', name: 'tib', project_id: 1 }],
        environments: [{ id: 10, code: 'poc', name: 'poc' }],
      },
      data_streams: [
        {
          name: 'autoadmin-yilake-tib-poc-nginx-wuhan-test',
          project: 'yilake', business_system: 'tib', environment: 'poc',
          service: 'nginx', tier: 'wuhan-test', recognized: true,
          // 服务已停用：采集已停止，数据保留至档位到期。
          service_enabled: false, service_collection_enabled: false,
          health: 'green', bytes: 1024, docs: 10, ilm_state: 'hot', backing_indices: [],
        },
        {
          name: 'autoadmin-yilake-tib-poc-tomcat-wuhan-test',
          project: 'yilake', business_system: 'tib', environment: 'poc',
          service: 'tomcat', tier: 'wuhan-test', recognized: true,
          // 服务启用但没开启日志采集。
          service_enabled: true, service_collection_enabled: false,
          health: 'green', bytes: 2048, docs: 20, ilm_state: 'hot', backing_indices: [],
        },
        {
          name: 'autoadmin-yilake-tib-poc-redis-wuhan-test',
          project: 'yilake', business_system: 'tib', environment: 'poc',
          service: 'redis', tier: 'wuhan-test', recognized: true,
          service_enabled: true, service_collection_enabled: true,
          health: 'green', bytes: 4096, docs: 40, ilm_state: 'hot', backing_indices: [],
        },
      ],
    } },
  })),
  getLogServiceUsage: vi.fn(() => Promise.resolve({ data: { data: { buckets: [] } } })),
}))

import LogStorageOverview from './index.vue'

// 树层级：root → 项目 → 业务系统 → 环境 → 逻辑服务。
function serviceNode(wrapper, serviceCode) {
  return wrapper.vm.treeData
    .flatMap((root) => root.children || [])
    .flatMap((project) => project.children || [])
    .flatMap((bizsys) => bizsys.children || [])
    .flatMap((env) => env.children || [])
    .find((node) => node.serviceCode === serviceCode)
}

describe('LogStorageOverview collect state', () => {
  afterEach(() => {
    document.body.innerHTML = ''
  })

  async function mountPage() {
    const wrapper = mount(LogStorageOverview, {
      attachTo: document.body,
      global: { plugins: [Antd] },
    })
    await flushPromises()
    return wrapper
  }

  it('marks a disabled service and a collection-off service, and leaves a collecting one unmarked', async () => {
    const wrapper = await mountPage()

    const disabled = serviceNode(wrapper, 'nginx')
    const collectionOff = serviceNode(wrapper, 'tomcat')
    const collecting = serviceNode(wrapper, 'redis')

    expect(disabled.collectState?.label).toBe('已停用')
    expect(disabled.title).toContain('已停用')
    expect(collectionOff.collectState?.label).toBe('未开启采集')
    // 正常采集的服务不标注——"在采集"不是这里要断言的事（数据态另算），但绝不能标成"已停用"。
    expect(collecting.collectState).toBeNull()
    expect(collecting.title).toBe('服务：redis')

    wrapper.unmount()
  })

  it('shows the collect state in the selected service panel', async () => {
    const wrapper = await mountPage()
    wrapper.vm.selectNode([], { node: serviceNode(wrapper, 'nginx') })
    await flushPromises()

    expect(document.body.textContent).toContain('采集状态')
    expect(document.body.textContent).toContain('已停用')
    // 明确表达"数据不会因为停用被清理"，避免运维误以为要手动清理。
    expect(wrapper.vm.selectedService.collectState.tooltip).toContain('不会自动清理')
    wrapper.unmount()
  })
})
