import { afterEach, describe, expect, it, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import Antd from 'ant-design-vue'
import dayjs from 'dayjs'
import Page from './index.vue'
import { buildMatchers, buildWindow, initForm, newScope } from './form'
import AssetSelector from './AssetSelector.vue'
import { getHostById, getHostList } from '@/api/assets/host'
import { buildAlertScopeTreeData } from '@/util/alertScope'
import { checkPermission } from '@/directives/permission/permission'
import { listWindows, previewWindow, saveWindow, windowDetail, deleteWindows } from '@/api/maintenance'

vi.mock('vue-router', async () => ({
  ...await vi.importActual('vue-router'),
  useRoute: () => ({ path:'/monitor/notification/maintenance', fullPath:'/monitor/notification/maintenance', query:{} }),
  useRouter: () => ({ replace:vi.fn() }),
}))
vi.mock('@/directives/permission/permission', () => ({ checkPermission:vi.fn() }))
vi.mock('@/api/assets/host', () => ({ getHostById:vi.fn(), getHostList:vi.fn() }))
vi.mock('@/api/maintenance', () => ({
  listWindows:vi.fn(), previewWindow:vi.fn(), saveWindow:vi.fn(), getSourceAlert:vi.fn(),
  windowDetail:vi.fn(), cancelWindow:vi.fn(), endWindow:vi.fn(), deleteWindows:vi.fn(),
}))
const pathPermissions = ['monitor:view', 'monitor:maintenance:view']
const mountPage = () => mount(Page, { attachTo:document.body, global:{ plugins:[Antd], stubs:{ FontAwesomeIcon:true } } })
function clickModalButton(text) {
  const button = [...document.querySelectorAll('.ant-modal button')].find(b => b.textContent.replace(/\s/g, '') === text)
  expect(button).toBeDefined()
  button.click()
}
let wrapper
afterEach(() => { wrapper?.unmount(); wrapper = null; document.body.innerHTML = ''; vi.clearAllMocks() })

describe('维护窗口', () => {
  it('只提交所选分支，跨业务的同名环境可区分并回显', () => {
    const data = buildAlertScopeTreeData({ projects:[{ id:1, name:'A' }, { id:2, name:'B' }],
      systems:[{ id:3, project:1, name:'支付' }, { id:4, project:2, name:'账务' }],
      services:[3, 4].map(id => ({ id, business_system:id, environment:9, environment_name:'prod' })),
    }, { businessScopedEnvironments:true })
    const env = data[1].children[0].children[0]
    expect(env.value).not.toBe(data[0].children[0].children[0].value)
    expect(env.pathTitle).toBe('B / 账务 / prod')
    const f = initForm({}, 'UTC')
    f.scopes.push({ ...newScope(), values:[data[0].value, env.value] })
    const matchers = buildMatchers(f)
    expect(matchers).toEqual([{ type:'tree', node_type:'project', id:1 }, { type:'tree', node_type:'environment', id:9, business_id:4 }])
    expect(initForm({ matchers }, 'UTC').scopes[0].values).toEqual(f.scopes[0].values)
  })
  it('预填已确认主机与告警名，不复制临时标签；拒绝不支持的运算符', () => {
    const f = initForm({}, 'UTC', { alertname:'CPU', host_id:8, pod:'old' })
    expect(buildMatchers(f)).toEqual([{ type:'host', id:8 }, { type:'label', label:'alertname', operator:'=', value:'CPU' }])
    f.scopes = []; f.labels[0].operator = '!='
    expect(() => buildMatchers(f)).toThrow('仅支持等于或包含')
  })
  it('主机、服务树和第二行主机编辑时分别回显，空行不可预览', () => {
    const f = initForm({}, 'UTC')
    f.scopes = [{ ...newScope('host'), values:[7, 8] }, { ...newScope(), values:['project:1'] }, { ...newScope('host'), values:[9] }]
    const matchers = buildMatchers(f)
    expect(initForm({ matchers }, 'UTC').scopes.map(s => [s.kind, s.values])).toEqual([['host',[7,8]],['tree',['project:1']],['host',[9]]])
    f.scopes.push(newScope())
    expect(() => buildMatchers(f)).toThrow('第 4 行')
  })
  it('搜索与预填主机按主机名(IP)展示，选择值仍为 ID', async () => {
    getHostById.mockResolvedValue({ data:{ data:{ id:230, instance_name:'oracle', ip:'192.0.2.1' } } })
    getHostList.mockResolvedValue({ data:{ data:{ results:[{ id:229, instance_name:'oracle', ip:'192.0.2.2' }] } } })
    wrapper = mount(AssetSelector, { props:{ modelValue:[230] }, global:{ plugins:[Antd] } })
    await flushPromises()
    const select = wrapper.getComponent({ name:'ASelect' })
    select.vm.$emit('search', 'cdm'); await flushPromises()
    expect(select.props('options')).toEqual([{ value:230, label:'oracle(192.0.2.1)' }, { value:229, label:'oracle(192.0.2.2)' }])
  })
  it('按操作者时区提交明确时段，编辑时保留范围', () => {
    const row = { id:3, title:'支付系统', reason:'发布', starts_at:'2099-03-01T14:00:00Z', ends_at:'2099-03-01T15:00:00Z',
      matchers:[{ type:'tree', node_type:'project', id:4 }, { type:'tree', node_type:'project', id:7 }] }
    const f = initForm(row, 'Asia/Shanghai')
    expect(f.start.format('HH:mm')).toBe('22:00')
    expect(buildWindow(f, 'Asia/Shanghai')).toMatchObject({ starts_at:'2099-03-01T14:00:00.000Z', ends_at:'2099-03-01T15:00:00.000Z', matchers:row.matchers })
    f.end = dayjs('2099-03-01 21:00:00')
    expect(() => buildWindow(f, 'Asia/Shanghai')).toThrow('结束时间')
  })
  it('新增时同时展示两个时间框，不能用保存时刻代替未填写的开始时间', async () => {
    listWindows.mockResolvedValue({ data:{ data:{ count:0, results:[] } } })
    checkPermission.mockImplementation(p => [...pathPermissions, 'monitor:maintenance:create'].includes(p))
    wrapper = mountPage(); await flushPromises()
    await wrapper.findAll('button').find(b => b.text().includes('新增')).trigger('click'); await flushPromises()
    const modal = document.querySelector('.ant-modal')
    expect(modal.querySelector('input[placeholder="开始时间"]')).not.toBeNull()
    expect(modal.querySelector('input[placeholder="结束时间"]')).not.toBeNull()
    const form = initForm({ title:'发布', reason:'升级' }, 'UTC', { host_id:7 })
    form.end = dayjs('2099-01-01 23:59:59')
    expect(() => buildWindow(form, 'UTC')).toThrow('开始和结束时间')
  })
  it('只读可查看但不可写；生效中可编辑但开始时间锁定', async () => {
    const record = { id:1, title:'生产', reason:'发布', matchers:[], starts_at:'2099-03-01T14:00:00Z', ends_at:'2099-03-01T15:00:00Z' }
    listWindows.mockResolvedValue({ data:{ data:{ count:2, results:[{ ...record, status:'scheduled' },{ ...record,id:2,status:'active' }] } } })
    checkPermission.mockImplementation(p => pathPermissions.includes(p))
    wrapper = mountPage(); await flushPromises()
    expect(wrapper.text()).not.toContain('新增')
    expect(wrapper.find('[aria-label="编辑"]').exists()).toBe(false)
    expect(wrapper.find('[aria-label="删除"]').exists()).toBe(false)
    windowDetail.mockResolvedValue({ data:{ data:{ ...record, status:'active',
      matchers:[{ type:'host', id:7 }, { type:'tree', node_type:'service', id:9 }, { type:'label', label:'team', operator:'contains', value:'ops' }],
      scope_names:{ 'host:7':'db(192.0.2.7)', 'service:9':'A / 支付 / prod / API' } } } })
    await wrapper.get('[aria-label="查看"]').trigger('click'); await flushPromises()
    expect(windowDetail).toHaveBeenCalledWith(1)
    const modal = document.querySelector('.ant-modal')
    expect(modal.textContent).toContain('db(192.0.2.7)')
    expect(modal.textContent).not.toContain('保存')
    expect(modal.textContent).toContain('A / 支付 / prod / API')
    expect(modal.querySelector('textarea').value).toBe('发布')
    expect(modal.querySelector('input[placeholder="标签值"]').value).toBe('ops')
    expect(modal.querySelector('input[placeholder="开始时间"]').disabled).toBe(true)
    expect(modal.querySelectorAll('input:not([readonly]):not([disabled]),textarea:not([readonly])')).toHaveLength(0)
    expect(modal.querySelectorAll('.scope-row .ant-select-disabled')).toHaveLength(4)
    expect(getHostById).not.toHaveBeenCalled()
    expect(getHostList).not.toHaveBeenCalled()
    wrapper.unmount()
    checkPermission.mockImplementation(p => [...pathPermissions, 'monitor:maintenance:update'].includes(p))
    wrapper = mountPage(); await flushPromises()
    expect(wrapper.findAll('[aria-label="编辑"]')).toHaveLength(2)
    expect(wrapper.find('[aria-label="查看"]').exists()).toBe(false)
    windowDetail.mockResolvedValue({ data:{ data:{ ...record, id:2, status:'active' } } })
    await wrapper.findAll('[aria-label="编辑"]')[1].trigger('click'); await flushPromises()
    expect(document.querySelector('input[placeholder="开始时间"]').disabled).toBe(true)
    expect(document.body.textContent).toContain('编辑维护窗口')
    clickModalButton('预览影响范围')
    await flushPromises()
    expect(previewWindow).not.toHaveBeenCalled()
    expect(document.body.textContent).toContain('请至少选择一个资产或添加 Label 条件')
  })
  it('有编辑权限但窗口已结束，点击名称仍进入只读模式', async () => {
    const record = { id:3, title:'已完成升级', reason:'升级', status:'expired', matchers:[], starts_at:'2026-01-01T00:00:00Z', ends_at:'2026-01-02T00:00:00Z' }
    listWindows.mockResolvedValue({ data:{ data:{ count:1, results:[record] } } })
    windowDetail.mockResolvedValue({ data:{ data:record } })
    checkPermission.mockImplementation(p => [...pathPermissions, 'monitor:maintenance:update'].includes(p))
    wrapper = mountPage(); await flushPromises()
    expect(wrapper.find('[aria-label="编辑"]').exists()).toBe(false)
    expect(wrapper.find('[aria-label="查看"]').exists()).toBe(true)
    await wrapper.get('tbody a').trigger('click'); await flushPromises()
    expect(document.querySelector('.ant-modal').textContent).toContain('查看维护窗口')
    expect(document.querySelector('.ant-modal').textContent).not.toContain('添加范围')
  })
  it('预览零命中不创建记录，确认保存时才写入', async () => {
    const matchers = [{ type:'label', label:'team', operator:'contains', value:'ops' }]
    const record = { id:4, status:'scheduled', title:'升级', reason:'变更', starts_at:'2099-01-01T01:00:00Z', ends_at:'2099-01-01T02:00:00Z', matchers }
    listWindows.mockResolvedValue({ data:{ data:{ count:1, results:[record] } } })
    previewWindow.mockResolvedValue({ data:{ data:{ count:0, results:[] } } })
    windowDetail.mockResolvedValue({ data:{ data:record } })
    checkPermission.mockImplementation(p => [...pathPermissions, 'monitor:maintenance:update'].includes(p))
    wrapper = mountPage(); await flushPromises()
    expect(wrapper.text()).not.toContain('匹配条件')
    expect(wrapper.text()).not.toContain('team 包含 ops')
    await wrapper.get('[aria-label="编辑"]').trigger('click'); await flushPromises()
    expect(document.querySelector('.condition .ant-select-selection-item').textContent).toBe('包含')
    clickModalButton('预览影响范围'); await flushPromises()
    expect(previewWindow).toHaveBeenCalledWith(matchers)
    expect(document.body.textContent).toContain('当前命中 0 条告警')
    expect(saveWindow).not.toHaveBeenCalled()
    clickModalButton('保存'); await flushPromises()
    expect(saveWindow).toHaveBeenCalledWith(expect.objectContaining({ id:4, matchers, starts_at:'2099-01-01T01:00:00.000Z' }))
  })
  it('生效窗口删除前列出名称，确认后才批删并刷新', async () => {
    const record = { id:8, title:'系统升级', status:'active', matchers:[], starts_at:'2026-01-01T00:00:00Z', ends_at:'2099-01-01T00:00:00Z' }
    listWindows.mockResolvedValue({ data:{ data:{ count:1, results:[record] } } })
    deleteWindows.mockResolvedValue({ data:{ data:{ results:[{ id:8, ok:true }] } } })
    checkPermission.mockImplementation(p => [...pathPermissions, 'monitor:maintenance:delete'].includes(p))
    wrapper = mountPage(); await flushPromises()
    await wrapper.get('[aria-label="删除"]').trigger('click'); await flushPromises()
    expect(document.querySelector('.ant-modal-confirm').textContent).toContain('系统升级')
    expect(deleteWindows).not.toHaveBeenCalled()
    clickModalButton('确认删除'); await flushPromises()
    expect(deleteWindows).toHaveBeenCalledWith([8])
    expect(listWindows).toHaveBeenCalledTimes(2)
  })
})
