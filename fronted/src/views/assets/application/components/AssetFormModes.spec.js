import { flushPromises, mount } from '@vue/test-utils'
import Antd from 'ant-design-vue'
import { afterEach, describe, expect, it, vi } from 'vitest'
import store from '@/store'
import permission from '@/directives/permission'
import Projects from '../../projects/index.vue'
import Environments from '../../environments/index.vue'
import ProjectDialog from './ProjectDialog.vue'
import BusinessEnvironmentDialog from './BusinessEnvironmentDialog.vue'
import BusinessSystemDialog from './BusinessSystemDialog.vue'
import AssetFormAction from './AssetFormAction.vue'
import ClusterProfileDialog from './ClusterProfileDialog.vue'
import VersionDialog from './VersionDialog.vue'
import * as api from '@/api/assets/application'
import { openDeleteConfirm } from '@/util/deleteConfirm'

vi.mock('@/api/assets/application', () => {
  const detail = { id: 3, name: '生产', code: 'prod', enabled: true }
  const list = () => ({ data: { data: { results: [detail] } } })
  const get = () => ({ data: { data: detail } })
  return {
    getProjectList: vi.fn(list), getBusinessEnvironmentList: vi.fn(list),
    getProject: vi.fn(get), getBusinessEnvironment: vi.fn(get),
    saveProject: vi.fn(get), saveBusinessEnvironment: vi.fn(get),
    getBusinessSystem: vi.fn(() => ({ data: { data: { ...detail, project: 8, project_name: '华北项目' } } })),
    saveBusinessSystem: vi.fn(get),
    getClusterProfile: vi.fn(() => ({ data: { data: { ...detail, profile_type: 'builtin', application: 9, application_name: 'Redis' } } })),
    getApplicationList: vi.fn(list), saveClusterProfile: vi.fn(get),
    getApplicationVersionList: vi.fn(list), saveApplicationVersion: vi.fn(get), batchDeleteApplicationVersions: vi.fn(),
  }
})
vi.mock('@/util/deleteConfirm', () => ({ openDeleteConfirm: vi.fn() }))

const global = { plugins: [Antd], directives: { permission }, stubs: {
  FontAwesomeIcon: true, AModal: { props: ['open'], template: '<div v-if="open"><slot /><footer><slot name="footer" /></footer></div>' },
} }
const roles = store.state.role_codes
const perms = store.state.perms
afterEach(() => {
  store.state.role_codes = roles
  store.state.perms = perms
  vi.clearAllMocks()
})
function grant(...actions) {
  store.state.role_codes = []
  store.state.perms = actions.map(action => `assets:applications:${action}`)
}

it('缺少查看权限时更新权限不产生配置入口', () => {
  grant('update')
  const action = mount(AssetFormAction, { global })
  try { expect(action.find('button').exists()).toBe(false) } finally { action.unmount() }
})

describe.each([
  ['项目', Projects, ProjectDialog, api.saveProject],
  ['环境', Environments, BusinessEnvironmentDialog, api.saveBusinessEnvironment],
])('%s配置', (_, Page, Dialog, save) => {
  it.each([false, true])('查看和编辑互斥并打开同一组控件（可编辑=%s）', async editable => {
    grant('view', ...(editable ? ['update'] : []))
    const page = mount(Page, { global })
    try {
      await flushPromises()
      const actions = page.findAllComponents({ name: 'ATooltip' }).filter(t => ['编辑', '查看'].includes(t.props('title')))
      expect(actions.map(t => t.props('title'))).toEqual([editable ? '编辑' : '查看'])
      await actions[0].find('button').trigger('click')
      await flushPromises()
      const dialog = page.findComponent(Dialog)
      expect(dialog.props('readOnly')).toBe(!editable)
      expect(dialog.find('input').element.value).toBe('生产')
      expect(dialog.find('input').element.disabled).toBe(!editable)
      if (!editable) {
        expect(dialog.find('footer').text().replace(/\s/g, '')).toBe('关闭')
        await dialog.vm.submit()
        expect(save).not.toHaveBeenCalled()
      }
    } finally { page.unmount() }
  })
})

it('详情中的内置模型强制只读，不查询候选、不允许提交', async () => {
  grant('view', 'update')
  const dialog = mount(ClusterProfileDialog, { props: { open: true, profileId: 3 }, global })
  try {
    await flushPromises()
    expect(dialog.text()).toContain('Redis')
    expect(dialog.find('footer').text().replace(/\s/g, '')).toBe('关闭')
    expect(dialog.find('input').element.disabled).toBe(true)
    await dialog.vm.submit()
    expect(api.saveClusterProfile).not.toHaveBeenCalled()
    expect(api.getApplicationList).not.toHaveBeenCalled()
  } finally { dialog.unmount() }
})

it('业务系统查看表单从详情回显项目，不需要应用列表权限', async () => {
  grant()
  store.state.perms = ['assets:service-tree:view']
  const dialog = mount(BusinessSystemDialog, { props: { open: true, systemId: 3, readOnly: true }, global })
  try {
    await flushPromises()
    expect(dialog.find('.ant-select-disabled .ant-select-selection-item').text()).toBe('华北项目')
    await dialog.vm.submit()
    expect(api.getProjectList).not.toHaveBeenCalled()
    expect(api.saveBusinessSystem).not.toHaveBeenCalled()
  } finally { dialog.unmount() }
})

it.each([['仅查看', ['view'], false], ['显式只读', ['view', 'create', 'update', 'delete'], true]])('%s版本列表不提供任何写动作', async (_, actions, readOnly) => {
  grant(...actions)
  const dialog = mount(VersionDialog, { props: { open: true, application: { id: 9 }, readOnly }, global })
  try {
    await flushPromises()
    expect(dialog.find('.version-form').exists()).toBe(false)
    expect(dialog.find('.delBtn').exists()).toBe(false)
    expect(dialog.find('button[role=switch]').element.disabled).toBe(true)
    dialog.vm.versionForm.version = '1.0'
    await dialog.vm.saveVersion()
    dialog.vm.confirmDelete({ id: 3 })
    expect(api.saveApplicationVersion).not.toHaveBeenCalled()
    expect(openDeleteConfirm).not.toHaveBeenCalled()
  } finally { dialog.unmount() }
})
