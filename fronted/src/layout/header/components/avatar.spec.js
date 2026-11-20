import { mount, shallowMount } from '@vue/test-utils'
import { nextTick } from 'vue'
import Antd, { Avatar as AntAvatar, Upload, message } from 'ant-design-vue'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import Avatar from './avatar.vue'
import AvatarUploader from '@/views/userCenter/components/Avatar.vue'
import { saveCurrentUser } from '@/api/user'
import store from '@/store'
import router from '@/router'

vi.mock('@/api/sys/userTimezone', () => ({
  getCurrentUserInfo: () => Promise.resolve({ data: { data: { timezone: 'America/New_York' } } }),
}))
vi.mock('@/util/request', () => ({
  default: {},
  getMediaUrl: value => `/media/${value}`,
  getServerUrl: () => 'http://api.test',
}))
vi.mock('@/router', () => ({ default: { replace: vi.fn() } }))
vi.mock('@/util/userTimezoneSync', () => ({ listenUserTimezoneChanged: () => vi.fn() }))

let wrapper
const initialUser = {
  id: 3, username: 'operator', timezone: 'America/New_York', avatar: 'userAvatar/old.png',
}
beforeEach(() => {
  localStorage.setItem('currentUser', JSON.stringify(initialUser))
  store.state.user = undefined
})
afterEach(() => {
  wrapper?.unmount()
  store.state.user = undefined
  localStorage.clear()
  vi.useRealTimers()
  vi.restoreAllMocks()
})

describe('顶部头像实时同步', () => {
  function mountProfile() {
    wrapper = mount({
      components: { HeaderAvatar: Avatar, AvatarUploader },
      template: '<div><HeaderAvatar /><AvatarUploader /></div>',
    }, {
      global: { plugins: [Antd], stubs: { 'router-link': true, SvgIcon: true } },
    })
    return {
      header: wrapper.findComponent(Avatar),
      uploader: wrapper.findComponent(AvatarUploader),
    }
  }

  it('无共享状态时从缓存回显，上传成功后无需重新挂载即同步顶栏、预览及缓存', async () => {
    vi.spyOn(message, 'success').mockImplementation(() => {})
    const { header, uploader } = mountProfile()
    const headerInstance = header.vm
    expect(header.findComponent(AntAvatar).props('src')).toBe('/media/userAvatar/old.png')
    uploader.findComponent(Upload).vm.$emit('change', {
      file: { status: 'done', response: {
        code: 200, data: { avatar: 'userAvatar/new.png' },
      } },
    })
    await nextTick()
    expect(header.vm).toBe(headerInstance)
    expect(header.findComponent(AntAvatar).props('src')).toBe('/media/userAvatar/new.png')
    expect(uploader.findComponent(AntAvatar).props('src')).toBe('/media/userAvatar/new.png')
    expect(store.state.user.avatar).toBe('userAvatar/new.png')
    expect(JSON.parse(localStorage.getItem('currentUser')).avatar).toBe('userAvatar/new.png')
    expect(header.text()).toContain('operator')
  })

  it('优先展示共享状态，并响应连续的用户信息更新', async () => {
    store.commit('add_user', { ...initialUser, avatar: 'userAvatar/current.png' })
    const { header } = mountProfile()
    expect(header.findComponent(AntAvatar).props('src')).toBe('/media/userAvatar/current.png')
    for (const avatar of ['userAvatar/first.png', 'userAvatar/second.png']) {
      saveCurrentUser({ ...initialUser, avatar })
      await nextTick()
      expect(header.findComponent(AntAvatar).props('src')).toBe(`/media/${avatar}`)
    }
  })

  it('上传失败保留顶栏旧头像与用户信息，不产生成功状态', async () => {
    vi.spyOn(message, 'error').mockImplementation(() => {})
    const success = vi.spyOn(message, 'success').mockImplementation(() => {})
    const { header, uploader } = mountProfile()
    uploader.findComponent(Upload).vm.$emit('change', {
      file: { status: 'error', error: { status: 500 }, response: { msg: '头像保存失败' } },
    })
    await nextTick()
    expect(header.findComponent(AntAvatar).props('src')).toBe('/media/userAvatar/old.png')
    expect(JSON.parse(localStorage.getItem('currentUser'))).toEqual(initialUser)
    expect(store.state.user).toBeUndefined()
    expect(success).not.toHaveBeenCalled()
  })
})

describe('顶部用户时区标签', () => {
  it('只显示时区名称，时钟仍按夏令时切换', async () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-03-08T06:59:59Z'))
    wrapper = shallowMount(Avatar, {
      global: { stubs: { 'a-dropdown': true, 'a-avatar': true, 'a-menu': true, 'a-menu-item': true, 'a-menu-divider': true, 'router-link': true, SvgIcon: true } },
    })

    await nextTick()
    await nextTick()
    expect(wrapper.find('.current-time-timezone').text()).toBe('时区: America/New_York')
    expect(wrapper.find('.current-time-clock').text()).toBe('01:59:59')
    await vi.advanceTimersByTimeAsync(1000)
    expect(wrapper.find('.current-time-timezone').text()).toBe('时区: America/New_York')
    expect(wrapper.find('.current-time-clock').text()).toBe('03:00:00')
  })
})

it('移入菜单或点击菜单留白不会误关闭，点击退出才关闭并清理会话', async () => {
  vi.useFakeTimers()
  sessionStorage.setItem('test-session', 'active')
  wrapper = mount(Avatar, {
    attachTo: document.body,
    global: {
      plugins: [Antd],
      stubs: { RouterLink: { template: '<a><slot /></a>' }, SvgIcon: true },
    },
  })
  await wrapper.get('.ant-dropdown-link').trigger('mouseenter')
  await vi.advanceTimersByTimeAsync(200)
  await nextTick()
  const menu = document.querySelector('.header-user-menu')
  expect(menu?.textContent).toContain('个人中心')

  const popup = menu.closest('.ant-dropdown')
  await wrapper.get('.ant-dropdown-link').trigger('mouseleave', { relatedTarget: document.body })
  await vi.advanceTimersByTimeAsync(150)
  expect(wrapper.vm.userMenuOpen).toBe(true)
  popup.dispatchEvent(new MouseEvent('mouseenter', { relatedTarget: document.body }))
  await vi.advanceTimersByTimeAsync(300)
  menu.click()
  await nextTick()
  expect(wrapper.vm.userMenuOpen).toBe(true)

  const logoutItem = menu.querySelector('.ant-dropdown-menu-item-danger')
  expect(logoutItem?.textContent).toContain('安全退出')
  logoutItem.click()
  await nextTick()
  expect(wrapper.vm.userMenuOpen).toBe(false)
  expect(sessionStorage.getItem('test-session')).toBeNull()
  expect(localStorage.getItem('currentUser')).toBeNull()
  expect(router.replace).toHaveBeenCalledWith('/login')
})
