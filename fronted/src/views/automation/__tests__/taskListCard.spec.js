import { flushPromises, mount } from '@vue/test-utils'
import Antd, { Table, Tooltip } from 'ant-design-vue'
import { afterEach, describe, expect, it, vi } from 'vitest'
import TaskListCard from '../automationtask/components/TaskListCard/index.vue'
import { TASK_COLUMNS } from '../utils/automationTaskHelpers'

const task = {
  id: 7,
  name: `任务-${'长名称'.repeat(60)}`,
  inventory_name: `Inventory-${'production-'.repeat(40)}`,
  template_name: `[Playbook] ${'fluent-uninstall-'.repeat(40)}`,
  execution_timeout_seconds: 600,
  enabled: true,
  env_vars: {},
}

function mountCard(tasks = [task], canEditTask = true) {
  return mount(TaskListCard, {
    attachTo: document.body,
    props: {
      taskColumns: TASK_COLUMNS,
      tasks,
      taskPagination: { current: 1, pageSize: 10, total: tasks.length },
      canEditTask,
      formatEnvVarCell: () => '{}',
      formatEnvVarCellFullText: () => '{}',
      formatUpdateTime: () => '-',
    },
    global: {
      plugins: [Antd],
      stubs: { FontAwesomeIcon: true },
      directives: { permission: () => {} },
    },
  })
}

function cell(wrapper, key) {
  const index = TASK_COLUMNS.findIndex((column) => column.key === key)
  return wrapper.get('tr[data-row-key="7"]').findAll('td')[index]
}

describe('自动化任务列表名称列', () => {
  afterEach(() => {
    vi.useRealTimers()
    vi.restoreAllMocks()
  })

  it('keeps explicit widths, fixed layout, horizontal scrolling and right-fixed actions', () => {
    const wrapper = mountCard()
    try {
      const table = wrapper.findComponent(Table)
      expect(table.props('tableLayout')).toBe('fixed')
      expect(table.props('scroll').x).toBe(1700)
      expect(TASK_COLUMNS.find((column) => column.key === 'action')).toMatchObject({ fixed: 'right', width: 320 })
      for (const [key, width] of [['name', 160], ['inventory_name', 180], ['template_name', 140]]) {
        expect(TASK_COLUMNS.find((column) => column.key === key)).toMatchObject({ width, ellipsis: { showTitle: false } })
        expect(cell(wrapper, key).attributes('title')).toBeUndefined()
      }
      expect(cell(wrapper, 'execution_timeout_seconds').text()).toBe('600')
    } finally {
      wrapper.unmount()
    }
  })

  it.each([
    ['name', 'edit'],
    ['inventory_name', 'goto-inventory'],
    ['template_name', 'goto-template'],
  ])('constrains %s, reveals the full name above and preserves click navigation', async (key, event) => {
    vi.useFakeTimers()
    const wrapper = mountCard()
    try {
      await flushPromises()
      const nameCell = cell(wrapper, key)
      const link = nameCell.get('button.task-label-link')
      const label = link.get('.task-label-text')
      expect(label.text()).toBe(task[key])
      const tooltip = nameCell.findComponent(Tooltip)
      expect(tooltip.props()).toMatchObject({ title: task[key], placement: 'top' })
      await link.trigger('mouseenter')
      await vi.advanceTimersByTimeAsync(200)
      expect([...document.body.querySelectorAll('.ant-tooltip-inner')].some((node) => node.textContent === task[key])).toBe(true)
      await link.trigger('click')
      expect(wrapper.emitted(event)).toEqual([[task]])
      expect(cell(wrapper, 'execution_timeout_seconds').text()).toBe('600')
    } finally {
      wrapper.unmount()
    }
  })

  it('retains a full-name tooltip without making a read-only task name editable', async () => {
    const wrapper = mountCard([task], false)
    try {
      await flushPromises()
      const nameCell = cell(wrapper, 'name')
      expect(nameCell.find('button').exists()).toBe(false)
      expect(nameCell.get('.task-label-text').text()).toBe(task.name)
      expect(nameCell.findComponent(Tooltip).props()).toMatchObject({ title: task.name, placement: 'top' })
      expect(wrapper.emitted('edit')).toBeUndefined()
    } finally {
      wrapper.unmount()
    }
  })

  it.each([
    { ...task, name: 'uptime', inventory_name: 'test', template_name: '[Playbook] test' },
    { ...task, name: '', inventory_name: '', template_name: '' },
  ])('preserves short names and empty display values', async (record) => {
    const wrapper = mountCard([record])
    try {
      await flushPromises()
      for (const key of ['name', 'inventory_name', 'template_name']) {
        expect(cell(wrapper, key).get('.task-label-text').text()).toBe(record[key] || '-')
        expect(cell(wrapper, key).findComponent(Tooltip).props('title')).toBe(record[key] || '-')
      }
    } finally {
      wrapper.unmount()
    }
  })
})
