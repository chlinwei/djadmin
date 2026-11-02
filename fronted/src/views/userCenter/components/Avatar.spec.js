import { mount } from '@vue/test-utils'
import Antd, { Upload, message } from 'ant-design-vue'
import uploadRequest from 'ant-design-vue/es/vc-upload/request'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import Avatar from './Avatar.vue'
import { getCurrentUser, saveCurrentUser } from '@/api/user'

vi.mock('@/api/user', () => ({
  getToken: () => 'token',
  getCurrentUser: vi.fn(),
  saveCurrentUser: vi.fn(),
}))
vi.mock('@/util/request', () => ({
  getServerUrl: () => 'http://api.test',
  getMediaUrl: value => `http://api.test/media/${value}`,
}))

function mountAvatar() {
  return mount(Avatar, { global: { plugins: [Antd], stubs: { SvgIcon: true } } })
}

describe('数据库头像', () => {
  beforeEach(() => {
    vi.restoreAllMocks()
    vi.clearAllMocks()
    getCurrentUser.mockReturnValue({ id: 7, avatar: 'userAvatar/old.png' })
    vi.spyOn(message, 'error').mockImplementation(() => {})
    vi.spyOn(message, 'success').mockImplementation(() => {})
  })

  it('回显带目录的图片地址，并说明上传和最终存储限制', () => {
    const wrapper = mountAvatar()
    expect(wrapper.findComponent({ name: 'AAvatar' }).props('src')).toBe('http://api.test/media/userAvatar/old.png')
    expect(wrapper.text()).toContain('原图不超过 5 MiB，自动处理后存储不超过 1 MiB')
    const upload = wrapper.findComponent(Upload)
    expect(upload.props('beforeUpload')({ size: 5 * 1024 * 1024 })).toBe(true)
    expect(upload.props('beforeUpload')({ size: 5 * 1024 * 1024 + 1 })).toBe(Upload.LIST_IGNORE)
    expect(message.error).toHaveBeenCalledWith('头像原文件不得超过 5 MiB')
    wrapper.unmount()
  })

  it('成功后缓存新地址并更新预览，不缓存图片内容', async () => {
    const wrapper = mountAvatar()
    wrapper.findComponent(Upload).vm.$emit('change', {
      file: { status: 'done', response: {
        code: 200, data: { avatar: 'userAvatar/new.png', new_file_name: 'new.png' },
      } },
    })
    await wrapper.vm.$nextTick()
    expect(wrapper.findComponent({ name: 'AAvatar' }).props('src')).toBe('http://api.test/media/userAvatar/new.png')
    expect(saveCurrentUser).toHaveBeenCalledWith({ id: 7, avatar: 'userAvatar/new.png' })
    expect(message.success).toHaveBeenCalledWith('头像已更新')
    wrapper.unmount()
  })

  it('实际上传保留鉴权但不发送多余的 X-Requested-With 请求头', () => {
    const wrapper = mountAvatar()
    const upload = wrapper.findComponent(Upload)
    const xhr = {
      upload: {},
      open: vi.fn(),
      setRequestHeader: vi.fn(),
      send: vi.fn(),
    }
    vi.stubGlobal('XMLHttpRequest', vi.fn(function () { return xhr }))
    try {
      uploadRequest({
        action: upload.props('action'),
        method: 'post',
        filename: 'avatar',
        file: new File(['image'], 'photo.jpg', { type: 'image/jpeg' }),
        headers: upload.props('headers'),
      })
      expect(xhr.open).toHaveBeenCalledWith('post', 'http://api.test/user/changeAvatar', true)
      expect(xhr.setRequestHeader.mock.calls).toEqual([['Authorization', 'token']])
      expect(xhr.send).toHaveBeenCalledOnce()
      expect(xhr.send.mock.calls[0][0].get('avatar').name).toBe('photo.jpg')
    } finally {
      vi.unstubAllGlobals()
      wrapper.unmount()
    }
  })

  it.each([
    [{ code: 400, msg: '头像尺寸超限' }, '头像尺寸超限'],
    [{ code: 600, msg: '存储失败' }, '存储失败'],
    [{ code: 200, data: {} }, '头像上传响应异常'],
  ])('业务失败或非法响应保留旧头像并明确提示', async (response, expected) => {
    const wrapper = mountAvatar()
    wrapper.findComponent(Upload).vm.$emit('change', { file: { status: 'done', response } })
    await wrapper.vm.$nextTick()
    expect(message.error).toHaveBeenCalledWith(expected)
    expect(wrapper.findComponent({ name: 'AAlert' }).props('message')).toBe(expected)
    expect(saveCurrentUser).not.toHaveBeenCalled()
    expect(wrapper.findComponent({ name: 'AAvatar' }).props('src')).toBe('http://api.test/media/userAvatar/old.png')
    wrapper.unmount()
  })

  it.each([
    [{ status: 500 }, { code: 600, msg: '头像保存失败' }, '头像上传失败：头像保存失败（HTTP 500）'],
    [{ status: 401 }, { code: 301, msg: '登录已过期' }, '头像上传失败：登录已过期（HTTP 401）'],
    [{ status: 413 }, '<html>Request Entity Too Large</html>', '头像上传失败：上传请求体过大，被服务器或代理拒绝（HTTP 413）'],
    [{ status: 502 }, '<html>Bad Gateway</html>', '头像上传失败：服务器未返回可读的错误原因，请查看后端或代理日志（HTTP 502）'],
    [{ status: 0 }, undefined, '头像上传失败：未收到服务器响应，请检查网络、跨域配置或服务是否可用'],
    [{ type: 'error' }, undefined, '头像上传失败：未收到服务器响应，请检查网络、跨域配置或服务是否可用'],
  ])('HTTP 或网络失败显示具体原因并持久保留', async (error, response, expected) => {
    const wrapper = mountAvatar()
    wrapper.findComponent(Upload).vm.$emit('change', {
      file: { name: 'photo.jpg', status: 'error', error, response },
    })
    await wrapper.vm.$nextTick()
    expect(message.error).toHaveBeenCalledWith(expected)
    expect(wrapper.findComponent({ name: 'AAlert' }).props('message')).toBe(expected)
    expect(wrapper.text()).not.toContain('<html>')
    expect(saveCurrentUser).not.toHaveBeenCalled()
    expect(message.success).not.toHaveBeenCalled()
    expect(wrapper.findComponent({ name: 'AAvatar' }).props('src')).toBe('http://api.test/media/userAvatar/old.png')
    wrapper.unmount()
  })

  it('重新上传清除旧错误，成功后保持新头像和无错误状态', async () => {
    const wrapper = mountAvatar()
    const upload = wrapper.findComponent(Upload)
    upload.vm.$emit('change', {
      file: { status: 'error', error: { status: 500 }, response: { msg: '头像保存失败' } },
    })
    await wrapper.vm.$nextTick()
    expect(wrapper.findComponent({ name: 'AAlert' }).exists()).toBe(true)
    upload.vm.$emit('change', { file: { status: 'uploading' } })
    await wrapper.vm.$nextTick()
    expect(wrapper.findComponent({ name: 'AAlert' }).exists()).toBe(false)
    upload.vm.$emit('change', { file: {
      status: 'done', response: { code: 200, data: { avatar: 'userAvatar/new.png' } },
    } })
    await wrapper.vm.$nextTick()
    expect(wrapper.findComponent({ name: 'AAlert' }).exists()).toBe(false)
    expect(saveCurrentUser).toHaveBeenCalledWith({ id: 7, avatar: 'userAvatar/new.png' })
    wrapper.unmount()
  })
})
