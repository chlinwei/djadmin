import { beforeEach, describe, expect, it } from 'vitest'
import store from './index'

beforeEach(() => {
  store.commit('close_all_tabs')
})

describe('service tree workspace tabs', () => {
  it.each(['/assets/service-tree/index', '/monitor/logging/center'])('updates the existing workspace tab %s and preserves other tabs', (path) => {
    store.commit('add_tab', { title: '工作台', key: path })
    store.commit('add_tab', { title: '其他页面', key: '/other' })
    const key = `${path}?application_service_id=21&panel=config`
    store.commit('add_tab', { title: '工作台', key })
    store.commit('add_tab', { title: '工作台', key: `${key}&node_title=test` })
    expect(store.state.tabs.map((tab) => tab.key)).toEqual([
      '/index', `${key}&node_title=test`, '/other',
    ])
    expect(store.state.activeKey).toBe(`${key}&node_title=test`)
  })

  it('preserves distinct query tabs on other pages', () => {
    store.commit('add_tab', { title: '其他页面', key: '/other?id=1' })
    store.commit('add_tab', { title: '其他页面', key: '/other?id=2' })
    expect(store.state.tabs.map((tab) => tab.key)).toEqual(['/index', '/other?id=1', '/other?id=2'])
  })
})
