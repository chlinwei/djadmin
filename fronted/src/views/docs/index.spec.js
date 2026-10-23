import { mount, flushPromises } from '@vue/test-utils'
import { describe, expect, it, vi, afterEach } from 'vitest'
import Antd, { message } from 'ant-design-vue'
import { documentLoaders } from '@/util/documentCenter'
import { renderDocumentDiagrams } from '@/util/documentDiagrams'
import DocumentCenter from './index.vue'

vi.mock('@/util/documentDiagrams', () => ({ renderDocumentDiagrams: vi.fn(async (html) => html) }))

const wrappers = []
const scrollIntoView = vi.fn()
Element.prototype.scrollIntoView = scrollIntoView

async function waitForText(wrapper, text) {
  await flushPromises()
  await vi.waitFor(() => expect(wrapper.find('article').text()).toContain(text))
}

async function mountPage() {
  const wrapper = mount(DocumentCenter, { global: { plugins: [Antd] } })
  wrappers.push(wrapper)
  await waitForText(wrapper, '按菜单阅读')
  return wrapper
}

afterEach(() => {
  wrappers.splice(0).forEach((wrapper) => wrapper.unmount())
  vi.restoreAllMocks()
  renderDocumentDiagrams.mockReset()
  renderDocumentDiagrams.mockImplementation(async (html) => html)
  scrollIntoView.mockClear()
})

describe('菜单模块阅读页', () => {
  it('首页展示模块链接，目录定位模块章节并提供本篇导航', async () => {
    const wrapper = await mountPage()
    expect(wrapper.find('article h1').text()).toBe('文档中心')
    const hosts = wrapper.findAll('.ant-tree-node-content-wrapper').find((node) => node.text() === '主机管理与 Agent')
    await hosts.trigger('click')
    await waitForText(wrapper, '安装 Agent')
    expect(wrapper.find('.document-path').text()).toBe('docs/architecture/modules/ASSETS.md')
    expect(scrollIntoView).toHaveBeenCalled()
    const toc = wrapper.findAll('.document-toc a').find((node) => node.text() === '凭据管理')
    await toc.trigger('click')
    expect(scrollIntoView).toHaveBeenCalledTimes(2)
    expect(renderDocumentDiagrams).toHaveBeenCalled()
  })

  it('三级目录搜索完整展开，认证模块突出关键点', async () => {
    const wrapper = await mountPage()
    await wrapper.find('input').setValue('日志采集')
    expect(wrapper.find('.ant-tree').text()).toContain('日志管理')
    expect(wrapper.find('.ant-tree').text()).toContain('日志采集')
    expect(wrapper.find('.ant-tree').text()).not.toContain('历史归档')
    await wrapper.find('article a[data-document="docs/architecture/modules/ACCESS.md"]').trigger('click')
    await waitForText(wrapper, '登录流程')
    expect(wrapper.find('article blockquote strong').text()).toContain('登录成功不等于')
    expect(wrapper.find('.document-toc').text()).toContain('每次请求怎样鉴权')
  })

  it('内部链接切换文档、显式锚点可定位', async () => {
    const wrapper = await mountPage()
    await wrapper.find('article a[data-document="docs/architecture/modules/LOGGING.md"]').trigger('click')
    await waitForText(wrapper, '日志中心')
    await wrapper.find('article a[data-anchor="service-tree"]').trigger('click')
    await waitForText(wrapper, '服务树管理')
    expect(wrapper.find('.document-path').text()).toContain('ASSETS.md')
    expect(scrollIntoView).toHaveBeenCalled()
  })

  it('加载失败显示错误，不保留旧正文', async () => {
    const wrapper = await mountPage()
    vi.spyOn(documentLoaders, 'docs/architecture/SQL_DESIGN.md').mockRejectedValue(new Error('chunk unavailable'))
    await wrapper.find('article a[data-document="docs/architecture/SQL_DESIGN.md"]').trigger('click')
    await flushPromises()
    expect(wrapper.find('.ant-alert-error').text()).toContain('chunk unavailable')
    expect(wrapper.find('article').text()).toBe('')
  })

  it('流程图错误明确展示，不能伪装成渲染成功', async () => {
    const wrapper = await mountPage()
    renderDocumentDiagrams.mockRejectedValueOnce(new Error('流程图语法错误'))
    await wrapper.find('article a[data-document="docs/architecture/modules/ACCESS.md"]').trigger('click')
    await flushPromises()
    await vi.waitFor(() => expect(wrapper.find('.ant-alert-error').text()).toContain('流程图语法错误'))
    expect(wrapper.find('article').text()).toBe('')
  })

  it('快速切换时旧文档的延迟加载不能覆盖新文档', async () => {
    const wrapper = await mountPage()
    let finish
    vi.spyOn(documentLoaders, 'docs/architecture/SQL_DESIGN.md').mockImplementation(() => new Promise((resolve) => { finish = resolve }))
    await wrapper.find('article a[data-document="docs/architecture/SQL_DESIGN.md"]').trigger('click')
    const auth = wrapper.findAll('.ant-tree-node-content-wrapper').find((node) => node.text() === '登录认证与权限')
    await auth.trigger('click')
    await waitForText(wrapper, '登录流程')
    const current = wrapper.find('article').text()
    finish('# old document')
    await flushPromises()
    expect(wrapper.find('article').text()).toBe(current)
    expect(wrapper.find('.document-path').text()).toContain('ACCESS.md')
  })

  it('加载过程中旧图表渲染结果不能覆盖新模块', async () => {
    const wrapper = await mountPage()
    let finish
    renderDocumentDiagrams.mockImplementationOnce(() => new Promise((resolve) => { finish = resolve }))
    await wrapper.find('article a[data-document="docs/architecture/modules/ACCESS.md"]').trigger('click')
    await vi.waitFor(() => expect(finish).toBeTypeOf('function'))
    const hosts = wrapper.findAll('.ant-tree-node-content-wrapper').find((node) => node.text() === '主机管理与 Agent')
    await hosts.trigger('click')
    await waitForText(wrapper, '安装 Agent')
    finish('<h1>old rendered diagram</h1>')
    await flushPromises()
    expect(wrapper.find('article').text()).not.toContain('old rendered diagram')
  })

  it('未发布文件明确提示，不提供源码或协作指令', async () => {
    const warn = vi.spyOn(message, 'warning').mockImplementation(() => {})
    const wrapper = await mountPage()
    await wrapper.find('article a[data-document="docs/overview/PROJECT_CONTEXT.md"]').trigger('click')
    await waitForText(wrapper, '项目上下文')
    await wrapper.find('article a[data-unavailable]').trigger('click')
    expect(warn).toHaveBeenCalledWith(expect.stringContaining('未发布的文件'))
  })
})
