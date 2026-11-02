import { readFileSync } from 'node:fs'
import { URL as NodeURL } from 'node:url'
import { h } from 'vue'
import { parse } from 'vue/compiler-sfc'
import { mount, flushPromises } from '@vue/test-utils'
import { Button } from 'ant-design-vue'
import { FontAwesomeIcon } from '@fortawesome/vue-fontawesome'
import { faEye } from '@fortawesome/free-solid-svg-icons'
import { afterEach, describe, expect, it } from 'vitest'

const theme = readFileSync(new NodeURL('./button-theme.css', import.meta.url), 'utf8')

const pages = [
  ['assets/application/components/TemplateManagerDialog.vue', 1],
  ['automation/inventory/index.vue', 1],
  ['monitor/log-retention/index.vue', 1],
  ['security/baseline/index.vue', 2],
  ['assets/host/index.vue', 2],
  ['audit/operationLog/index.vue', 1],
  ['audit/webssh/index.vue', 1],
  ['sys/scheduler/index.vue', 1],
  ['monitor/log-center/LogQueryPanel.vue', 2],
]

function elements(node, parent = null) {
  return [
    ...(node.tag ? [{ node, parent }] : []),
    ...(node.children || []).flatMap((child) => elements(child, node)),
  ]
}

function attribute(node, name) {
  return node.props.find((prop) => prop.name === name)?.value?.content
}

describe('统一详情按钮', () => {
  let wrapper
  let style
  afterEach(() => {
    wrapper?.unmount()
    style?.remove()
    wrapper = null
    style = null
  })

  it.each(pages)('%s 的全部 %i 个入口使用同一图标、文字、样式与顶部提示', (file, count) => {
    const source = readFileSync(new NodeURL(`../../views/${file}`, import.meta.url), 'utf8')
    const { descriptor, errors } = parse(source)
    expect(errors).toEqual([])
    const actions = elements(descriptor.template.ast).filter(({ node }) => (
      node.tag === 'a-button' && attribute(node, 'class')?.split(/\s+/).includes('detail-action')
    ))
    expect(actions).toHaveLength(count)
    expect(source).not.toContain('fa-circle-info')
    for (const { node, parent } of actions) {
      expect(attribute(node, 'type')).toBe('link')
      expect(attribute(node, 'size')).toBe('small')
      const icon = node.children.find((child) => child.tag === 'FontAwesomeIcon')
      expect(icon.props.find((prop) => prop.arg?.content === 'icon').exp.content).toBe("['fas', 'eye']")
      const label = node.children.find((child) => child.tag === 'span')
      expect(label.children.some((child) => child.content === '详情')).toBe(true)
      expect(parent.tag).toBe('a-tooltip')
      expect(attribute(parent, 'placement')).toBe('top')
      expect(attribute(parent, 'title')).toBe(file.includes('LogQueryPanel') ? '详细日志' : '查看详情')
    }
  })

  it('公共样式为图文保留空间，只有可用详情按钮显示蓝色和悬停背景', async () => {
    const detail = (disabled) => h(Button, { type: 'link', size: 'small', class: 'detail-action', disabled }, {
      default: () => [h(FontAwesomeIcon, { icon: faEye }), h('span', '详情')],
    })
    wrapper = mount({ render: () => h('div', [detail(false), detail(true), h(Button, {}, () => '编辑')]) }, {
      attachTo: document.body,
    })
    await flushPromises()
    style = document.createElement('style')
    style.textContent = theme
    document.head.appendChild(style)
    const [active, disabled, edit] = wrapper.findAll('button')
    const computed = getComputedStyle(active.element)
    expect(computed.color).toBe('rgb(22, 119, 255)')
    expect(computed.minWidth).toBe('72px')
    expect(computed.height).toBe('28px')
    expect(computed.gap).toBe('6px')
    expect(getComputedStyle(active.get('svg').element).marginRight).toBe('0px')
    expect(getComputedStyle(disabled.element).color).not.toBe(computed.color)
    expect(edit.classes()).not.toContain('detail-action')
    const feedback = Array.from(style.sheet.cssRules).find((rule) => (
      rule.selectorText?.includes('.detail-action:not(:disabled):hover')
    ))
    expect(feedback.selectorText).toContain('.ant-btn.ant-btn-link.detail-action')
    expect(feedback.selectorText).toContain('.detail-action:not(:disabled):focus-visible')
    expect(feedback.style.getPropertyValue('background')).toBe('#e6f4ff')
    expect(feedback.style.getPropertyValue('transform')).toBe('none')
  })
})
