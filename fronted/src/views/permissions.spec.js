import { flushPromises, mount } from '@vue/test-utils'
import Antd from 'ant-design-vue'
import { computed, nextTick, ref } from 'vue'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import store from '@/store'
import permission from '@/directives/permission/index.js'
import request from '@/util/request'
import Scheduler from './sys/scheduler/index.vue'
import SysConfig from './sys/sysconfig/index.vue'
import Media from './monitor/media/index.vue'
import Baseline from './security/baseline/index.vue'
import Footer from '@/layout/footer/index.vue'
import FilePanel from './assets/host/webssh/components/WebsshFilePanel/index.vue'
import { createWebsshDataController } from './assets/host/webssh/controllers/websshDataController'
import { createWebsshLifecycleController } from './assets/host/webssh/controllers/websshLifecycleController'
import { createWebsshInteractionController } from './assets/host/webssh/controllers/websshInteractionController'

vi.mock('vue-router', () => ({ useRouter: () => ({ push: vi.fn() }), useRoute: () => ({ query: {} }) }))
vi.mock('@/util/keepAliveRefresh', () => ({ useKeepAliveRefreshLifecycle: vi.fn() }))
vi.mock('@/util/request', () => ({
  default: { get: vi.fn(), post: vi.fn(), patch: vi.fn(), del: vi.fn(), fileUpload: vi.fn() },
}))

const task = { id: 1, name: '测试任务', enabled: true, supported: true, code: 'test', cron_expression: '* * * * *' }
const config = { id: 2, key: 'test.config', name: '测试配置', value: '10', default_value: '5', is_readonly: false }
const media = { id: 3, name: '测试媒介', type: 'email', config: {} }
const response = (data) => ({ data: { code: 200, data } })
let wrapper
const originalGetComputedStyle = window.getComputedStyle.bind(window)
window.getComputedStyle = (element) => originalGetComputedStyle(element)

beforeEach(() => {
  vi.clearAllMocks()
  // 使用生产 Vuex store 与生产 checkPermission；测试的权限来自非 admin 用户的实际权限数组。
  store.state.role_codes = ['operator']
  store.state.perms = []
  request.get.mockImplementation((url) => {
    if (url === 'sys/menus/getMenuTree/') return Promise.resolve(response([]))
    const rows = url === 'sys/scheduler/tasks/' ? [task]
      : url === 'sys/configs/' ? [config]
        : url === 'monitor/media/' ? [media] : []
    return Promise.resolve(response({ results: rows, count: rows.length, totalPages: 1 }))
  })
  request.post.mockResolvedValue(response({}))
  request.patch.mockResolvedValue(response({}))
})

afterEach(() => {
  wrapper?.unmount()
  wrapper = null
  document.body.innerHTML = ''
})

async function render(component, perms, props = {}) {
  store.state.perms = perms
  wrapper = mount(component, {
    props,
    attachTo: document.body,
    global: {
      plugins: [Antd],
      directives: { permission },
      stubs: { FontAwesomeIcon: true },
    },
  })
  await flushPromises()
  return wrapper
}

function buttons(text) {
  return wrapper.findAll('button').filter((button) => button.text().replace(/\s/g, '').includes(text)
    && !button.element.closest('[style*="display: none"]'))
}

describe('分离读取、修改与执行权限', () => {
  it('scheduler view-only 可看任务历史，但不能编辑、切换或运行，也不会请求系统配置', async () => {
    await render(Scheduler, ['system:scheduler:view'])
    expect(wrapper.text()).toContain(task.name)
    expect(wrapper.findAll('.ant-switch').every((button) => button.attributes('disabled') !== undefined)).toBe(true)
    expect(buttons('立即执行')).toHaveLength(0)
    expect(wrapper.findAll('tbody button.ant-btn-primary')).toHaveLength(0)
    wrapper.vm.openEditModal(task)
    wrapper.vm.toggleEnable(task, false)
    wrapper.vm.submitTask()
    wrapper.vm.runNow(task)
    expect(wrapper.vm.editVisible).toBe(false)
    expect(request.post).not.toHaveBeenCalled()
    expect(request.patch).not.toHaveBeenCalled()
    expect(request.get.mock.calls.some(([url]) => url.includes('sys/configs/by-key/'))).toBe(false)
  })

  it('scheduler update 可编辑和启停但不包含立即运行权限', async () => {
    await render(Scheduler, ['system:scheduler:view', 'system:scheduler:update'])
    wrapper.vm.openEditModal(task)
    expect(wrapper.vm.editVisible).toBe(true)
    expect(buttons('立即执行')).toHaveLength(0)
    wrapper.vm.toggleEnable(task, false)
    await flushPromises()
    expect(request.post).toHaveBeenCalledWith('sys/scheduler/tasks/1/disable/')
  })

  it('scheduler execute 可运行，但不能修改 Cron 或启停', async () => {
    await render(Scheduler, ['system:scheduler:view', 'system:scheduler:execute'])
    expect(buttons('立即执行')).toHaveLength(1)
    expect(buttons('立即执行')[0].attributes('disabled')).toBeUndefined()
    wrapper.vm.openEditModal(task)
    expect(wrapper.vm.editVisible).toBe(false)
    await buttons('立即执行')[0].trigger('click')
    await flushPromises()
    expect(request.post).toHaveBeenCalledWith('sys/scheduler/tasks/1/run_now/', {})
  })

  it('config list 可查看，但编辑和恢复必须独立授权', async () => {
    await render(SysConfig, ['sys:config:list'])
    expect(wrapper.text()).toContain(config.key)
    expect(buttons('编辑')).toHaveLength(0)
    expect(buttons('重置默认')).toHaveLength(0)
    wrapper.vm.openEdit(config)
    wrapper.vm.handleResetDefault(config)
    wrapper.vm.saveEdit()
    expect(request.patch).not.toHaveBeenCalled()
    expect(request.post).not.toHaveBeenCalled()
    store.state.perms.push('sys:config:update')
    await nextTick()
    expect(buttons('编辑')).toHaveLength(1)
    expect(buttons('重置默认')).toHaveLength(1)
    await buttons('编辑')[0].trigger('click')
    expect(wrapper.vm.editVisible).toBe(true)
  })

  it('footer 无配置权限直接显示默认标题，不发送 by-key 请求', async () => {
    await render(Footer, [])
    expect(wrapper.text()).toBe('Djadmin')
    expect(request.get).not.toHaveBeenCalled()
  })

  it.each([
    [[], false, false, false, false],
    [['monitor:create'], true, false, false, false],
    [['monitor:update'], false, true, false, false],
    [['monitor:delete'], false, false, true, false],
    [['monitor:execute'], false, false, false, true],
  ])('monitor 媒介按动作授权，不从 view 推导写权限：%j', async (actions, create, update, remove, execute) => {
    await render(Media, ['monitor:view', ...actions])
    expect(wrapper.text()).toContain(media.name)
    expect(buttons('新增').length > 0).toBe(create)
    expect(buttons('编辑').length > 0).toBe(update)
    expect(buttons('删除').length > 0).toBe(remove)
    expect(buttons('测试').length > 0).toBe(execute)
    expect(request.post).not.toHaveBeenCalled()
    expect(request.patch).not.toHaveBeenCalled()
    if (!create) {
      wrapper.vm.openCreateModal()
      expect(wrapper.vm.createModalVisible).toBe(false)
    }
    if (!execute) {
      wrapper.vm.openTestModal(media)
      expect(wrapper.vm.testModalVisible).toBe(false)
    }
  })

  it('Exporter 安装/卸载分别要求 create/delete 与 execute 的交集', async () => {
    const { default: Monitor } = await import('./monitor/index.vue')
    await render(Monitor, ['monitor:view'])
    expect(wrapper.vm.canCreateTarget).toBe(false)
    expect(wrapper.vm.canDeleteTarget).toBe(false)
    wrapper.vm.openExporterCreateModal()
    wrapper.vm.openExporterBatchDeleteConfirm()
    expect(request.post).not.toHaveBeenCalled()
    expect(request.patch).not.toHaveBeenCalled()
    expect(request.get.mock.calls.some(([url]) => url.includes('sys/configs/by-key/'))).toBe(false)
    store.state.perms.push('monitor:create', 'monitor:delete')
    expect(wrapper.vm.canCreateTarget).toBe(false)
    expect(wrapper.vm.canDeleteTarget).toBe(false)
    store.state.perms.push('monitor:execute')
    expect(wrapper.vm.canCreateTarget).toBe(true)
    expect(wrapper.vm.canDeleteTarget).toBe(true)
  })

  it('日志采集纳管/删除不能用 ANY 权限；只读挂载不提交作业', async () => {
    const { default: Collectors } = await import('./monitor/log-collectors/index.vue')
    await render(Collectors, ['monitor:view', 'monitor:log_collect:view'])
    expect(wrapper.vm.canCreate).toBe(false)
    expect(wrapper.vm.canDelete).toBe(false)
    expect(buttons('装（')).toHaveLength(0)
    expect(buttons('启动')).toHaveLength(0)
    await wrapper.vm.handleBatchCreate()
    await wrapper.vm.handleStartService({ id: 1 })
    expect(request.post).not.toHaveBeenCalled()
    store.state.perms.push('monitor:log_collect:create', 'monitor:log_collect:delete')
    expect(wrapper.vm.canCreate).toBe(false)
    expect(wrapper.vm.canDelete).toBe(false)
    store.state.perms.push('monitor:log_collect:execute')
    expect(wrapper.vm.canCreate).toBe(true)
    expect(wrapper.vm.canDelete).toBe(true)
    await nextTick()
    expect(buttons('启动')).toHaveLength(1)
  })

  it('baseline view 不能通过行内编辑、排序或弹窗保存绕过 manage', async () => {
    await render(Baseline, ['baseline:view'])
    wrapper.vm.openBaselineModal({ id: 1 })
    wrapper.vm.openCategoryModal()
    wrapper.vm.openItemEditModal()
    await wrapper.vm.moveItem({ id: 1 }, 'up')
    await wrapper.vm.submitBaseline()
    await wrapper.vm.submitCategory()
    await wrapper.vm.confirmItemEdit()
    expect(wrapper.vm.baselineModalOpen).toBe(false)
    expect(wrapper.vm.itemEditModalOpen).toBe(false)
    expect(request.post).not.toHaveBeenCalled()
    expect(request.patch).not.toHaveBeenCalled()
    expect(buttons('新增')).toHaveLength(0)
  })

  it('直接进入 WebSSH 的 view-only 用户不会连接或启动轮询', async () => {
    store.state.perms = ['assets:hosts:view']
    const connect = vi.fn()
    const lifecycle = createWebsshLifecycleController({
      actions: { connectWebSsh: connect }, route: {}, state: {}, apis: {}, refs: {},
    })
    await lifecycle.handleMounted()
    expect(connect).not.toHaveBeenCalled()
  })

  it('WebSSH execute 不隐含文件写权限，浏览/下载保留，拖拽上传和菜单写动作均受 update 门禁', async () => {
    await render(FilePanel, ['assets:hosts:execute'], {
      fileOperationsEnabled: true,
      setFilePanelRef: () => {},
      setFileTableWrapRef: () => {},
      onBindFileRowEvents: () => ({}),
      onFormatFileSize: () => '',
      onFormatFileMtime: () => '',
    })
    expect(buttons('刷新')).toHaveLength(1)
    expect(buttons('上传文件')).toHaveLength(0)
    expect(buttons('新建目录')).toHaveLength(0)
    const data = createWebsshDataController({
      computed, refs: { fileContextMenuRecord: ref({ path: '/test.log' }) },
    })
    expect(data.fileContextMenuActions.value.map(({ key }) => key)).toEqual(['download', 'copy-dir-path'])
    const upload = vi.fn()
    const interaction = createWebsshInteractionController({ uploadRawFileToPath: upload })
    await interaction.handleDirectoryDrop({ dataTransfer: { files: [{ name: 'test' }] } }, '/')
    expect(upload).not.toHaveBeenCalled()
    store.state.perms.push('assets:hosts:update')
    await nextTick()
    expect(buttons('上传文件')).toHaveLength(1)
    expect(buttons('新建目录')).toHaveLength(1)
    expect(data.fileContextMenuActions.value.map(({ key }) => key)).toContain('rename')
  })
})
