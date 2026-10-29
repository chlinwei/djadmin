import { shallowMount } from '@vue/test-utils'
import { nextTick } from 'vue'
import { afterEach, describe, expect, it, vi } from 'vitest'
import Avatar from './avatar.vue'

vi.mock('@/api/user', () => ({
  getCurrentUser: () => ({ id: 3, username: 'operator', timezone: 'America/New_York' }),
}))
vi.mock('@/api/sys/userTimezone', () => ({
  getCurrentUserInfo: () => Promise.resolve({ data: { data: { timezone: 'America/New_York' } } }),
}))
vi.mock('@/util/request', () => ({ getMediaUrl: value => value }))
vi.mock('@/router', () => ({ default: { replace: vi.fn() } }))
vi.mock('@/util/userTimezoneSync', () => ({ listenUserTimezoneChanged: () => vi.fn() }))

let wrapper
afterEach(() => {
  wrapper?.unmount()
  vi.useRealTimers()
})

describe('顶部用户时区标签', () => {
  it('updates the offset alongside the clock across a DST transition', async () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-03-08T06:59:59Z'))
    wrapper = shallowMount(Avatar, {
      global: { stubs: { 'a-dropdown': true, 'a-avatar': true, 'a-menu': true, 'a-menu-item': true, 'router-link': true, SvgIcon: true } },
    })
    await nextTick()
    await nextTick()
    expect(wrapper.find('.current-time-timezone').text()).toBe('时区: UTC-05:00 · America/New_York')
    expect(wrapper.find('.current-time-clock').text()).toBe('01:59:59')
    await vi.advanceTimersByTimeAsync(1000)
    expect(wrapper.find('.current-time-timezone').text()).toBe('时区: UTC-04:00 · America/New_York')
    expect(wrapper.find('.current-time-clock').text()).toBe('03:00:00')
  })
})
