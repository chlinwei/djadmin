import { compile } from '@vue/compiler-dom'
import * as Vue from 'vue'
import { afterEach, describe, expect, it } from 'vitest'

let app
let root
let target

afterEach(() => {
  app?.unmount()
  root?.remove()
  target?.remove()
})

describe('Teleport runtime regression', () => {
  it('patches nested slot content after a reactive update and HMR rerender', async () => {
    root = document.createElement('div')
    target = document.createElement('div')
    document.body.append(root, target)
    const { code } = compile('<Teleport :to="target"><div :class="phase"><section><slot /></section></div></Teleport>', { prefixIdentifiers: true })
    const Popup = Vue.defineComponent({
      setup() {
        const phase = Vue.ref('initial')
        Vue.onMounted(() => { phase.value = 'ready' })
        return { phase, target }
      },
      render: new Function('Vue', code)(Vue),
    })
    const hmrId = 'teleport-slot-regression'
    const Host = {
      __hmrId: hmrId,
      render: () => Vue.h(Popup, null, { default: () => Vue.h('span', 'before update') }),
    }
    // Vue 3.5.13 在普通更新后丢失嵌套静态节点引用，随后的 HMR 会向 null 写 __vnode。
    const hmr = globalThis.__VUE_HMR_RUNTIME__
    hmr.createRecord(hmrId, Host)
    app = Vue.createApp(Host)
    app.mount(root)
    await Vue.nextTick()
    expect(target.querySelector('.ready section')?.textContent).toBe('before update')

    hmr.rerender(hmrId, () => Vue.h(Popup, null, { default: () => Vue.h('span', 'after update') }))
    await Vue.nextTick()
    expect(target.querySelector('.ready section')?.textContent).toBe('after update')
  })
})