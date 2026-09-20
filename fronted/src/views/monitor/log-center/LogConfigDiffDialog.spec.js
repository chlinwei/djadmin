import { flushPromises, mount } from '@vue/test-utils'
import Antd from 'ant-design-vue'
import { afterEach, describe, expect, it } from 'vitest'

import LogConfigDiffDialog from './LogConfigDiffDialog.vue'

// 「查看配置差异」弹窗：回答"这次下发会改什么"。
// 这里钉住三件事：默认只看本服务、读不到主机上的现状时不给差异结论、行级 diff 真的渲染出来。

function file(overrides = {}) {
  return {
    path: '/etc/filebeat/inputs.d/tomcat__order__15__a.yml',
    base_name: 'tomcat__order__15__a',
    status: 'changed',
    service_id: 15,
    service_code: 'order',
    expected: 'a\nnew\nc\n',
    applied: 'a\nold\nc\n',
    applied_truncated: false,
    read_error: '',
    ...overrides,
  }
}

function diffPayload(overrides = {}) {
  return {
    target_id: 101,
    host_id: 10,
    host_ip: '10.0.0.10',
    host_instance_name: 'node-a',
    state: 'drift',
    expected_fingerprint: 'expected-fingerprint-1234',
    applied_fingerprint_from_db: 'applied-fingerprint-5678',
    applied_fingerprint: 'applied-fingerprint-5678',
    service: { service_id: 15, expected_fingerprint: 'sub-expected', applied_fingerprint: 'sub-applied', pending: true },
    files: [
      file(),
      file({
        path: '/etc/filebeat/inputs.d/tomcat__pay__16__b.yml',
        base_name: 'tomcat__pay__16__b',
        status: 'removed',
        service_id: 16,
        service_code: 'pay',
        expected: null,
        applied: 'stale\n',
      }),
      file({
        path: '/etc/filebeat/inputs.d/tomcat__order__15__c.yml',
        base_name: 'tomcat__order__15__c',
        status: 'unchanged',
        expected: 'same\n',
        applied: 'same\n',
      }),
    ],
    summary: { added: 0, removed: 1, changed: 1, unchanged: 1, unread: 0 },
    read_error: '',
    ...overrides,
  }
}

function mountDialog(props = {}) {
  return mount(LogConfigDiffDialog, {
    props: {
      open: true,
      diff: diffPayload(),
      targetId: 101,
      serviceId: 15,
      hostLabel: 'node-a（10.0.0.10）',
      hostOptions: [{ label: 'node-a（10.0.0.10）（待下发）', value: 101 }],
      ...props,
    },
    attachTo: document.body,
    global: {
      plugins: [Antd],
      // 弹窗内容默认 teleport 到 body，断言就得去 document.body 里找；stub 掉 a-modal 让它就地渲染，
      // 这样 wrapper.find 能直接命中（与 LogFormatVerifyDialog.spec.js 同一做法）。
      stubs: { AModal: { template: '<div><slot /></div>' } },
    },
  })
}

describe('LogConfigDiffDialog', () => {
  afterEach(() => {
    document.body.innerHTML = ''
  })

  it('summarises what this delivery will change', () => {
    const wrapper = mountDialog()

    expect(wrapper.text()).toContain('新增 0')
    expect(wrapper.text()).toContain('修改 1')
    expect(wrapper.text()).toContain('删除 1')
    expect(wrapper.text()).toContain('未变 1')
    // 下发是全量替换：这一点必须写在明面上（否则"删除"会被读成"下发不动它"）。
    expect(wrapper.text()).toContain('完全替换')
    expect(wrapper.text()).toContain('本服务待下发')
    wrapper.unmount()
  })

  it('shows only this service by default and can show every fragment', async () => {
    const wrapper = mountDialog()

    const rows = () => wrapper.findAll('.ant-table-tbody tr.ant-table-row').map((row) => row.text())
    // 默认只看本服务（服务 15 的两个片段），同主机其他服务（服务 16）的折叠掉。
    expect(rows()).toHaveLength(2)
    expect(rows().join('|')).not.toContain('tomcat__pay__16__b')

    const checkbox = wrapper.find('.diff-toolbar input[type="checkbox"]')
    await checkbox.setValue(false)
    expect(rows()).toHaveLength(3)
    // 显示他服务片段时要有标记，否则用户会以为那也是本服务的配置。
    expect(rows().join('|')).toContain('他服务')
    wrapper.unmount()
  })

  it('renders a line diff when a fragment is expanded', async () => {
    const wrapper = mountDialog()

    await wrapper.find('.ant-table-tbody button').trigger('click')
    await flushPromises()

    const lines = wrapper.findAll('.diff-line').map((line) => `${line.classes().join(',')}|${line.text()}`)
    // 未变的行是上下文，改动的那一行 **先删后加**（统一 diff 惯例），并带两侧行号。
    expect(lines.some((line) => line.includes('diff-line--del') && line.includes('old'))).toBe(true)
    expect(lines.some((line) => line.includes('diff-line--add') && line.includes('new'))).toBe(true)
    expect(lines.some((line) => line.includes('diff-line--ctx'))).toBe(true)
    expect(wrapper.text()).toContain('+1 / -1')
    wrapper.unmount()
  })

  it('refuses to claim a diff when the host config cannot be read', () => {
    const wrapper = mountDialog({
      diff: diffPayload({
        read_error: '读不到主机上已下发的配置（主机 agent 可能离线）：dial timeout',
        state: 'unknown',
        // 读不到时后端只回期望侧：已下发侧为空。
        files: [file({ expected: 'a\nb\n', applied: null, status: 'added' })],
        summary: { added: 1, removed: 0, changed: 0, unchanged: 0, unread: 0 },
      }),
    })

    expect(wrapper.text()).toContain('读不到主机上已下发的配置')
    expect(wrapper.text()).toContain('dial timeout')
    // 明确它是"将要下发的内容"而不是差异结论。
    expect(wrapper.text()).toContain('将要下发')
    expect(wrapper.text()).not.toContain('这次下发不会改动任何片段')
    wrapper.unmount()
  })

  it('says when nothing will change', () => {
    const wrapper = mountDialog({
      diff: diffPayload({
        state: 'synced',
        files: [file({ status: 'unchanged', expected: 'same\n', applied: 'same\n' })],
        summary: { added: 0, removed: 0, changed: 0, unchanged: 1, unread: 0 },
      }),
    })

    expect(wrapper.text()).toContain('这次下发不会改动任何片段')
    wrapper.unmount()
  })

  it('switches host through the parent (the diff is per host)', async () => {
    const wrapper = mountDialog()

    wrapper.findComponent({ name: 'ASelect' }).vm.$emit('update:value', 202)
    await flushPromises()
    expect(wrapper.emitted('change-host')?.at(-1)).toEqual([202])
    // 重新比对由父组件拉数据（页面持有 applyState 那份主机清单与加载态）。
    wrapper.find('.diff-toolbar button').trigger('click')
    await flushPromises()
    expect(wrapper.emitted('reload')).toBeTruthy()
    wrapper.unmount()
  })
})
