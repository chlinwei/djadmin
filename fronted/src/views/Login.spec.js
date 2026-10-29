import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import Antd from 'ant-design-vue'
import Cookies from 'js-cookie'
import request from '@/util/request'
import { clearRemeberMe, getRemeberMeInfo, saveCurrentUser, setRemeberMe, updateUserInfo } from '@/api/user'
import { emitUserTimezoneChanged } from '@/util/userTimezoneSync'
import Login from './Login.vue'

const mocks = vi.hoisted(() => ({
    replace: vi.fn().mockResolvedValue(undefined),
    response: { code: 301, msg: 'Token expired', data: null },
}))

vi.mock('axios', async (importOriginal) => {
    const actual = await importOriginal()
    return {
        ...actual,
        default: {
            ...actual.default,
            create: (config) => actual.default.create({
                ...config,
                adapter: async (requestConfig) => ({
                    data: mocks.response,
                    status: 200,
                    statusText: 'OK',
                    headers: {},
                    config: requestConfig,
                }),
            }),
        },
    }
})
vi.mock('@/router', () => ({
    default: {
        currentRoute: { value: { path: '/index', fullPath: '/index' } },
        replace: mocks.replace,
    },
    addDynamicRoutes: vi.fn(),
}))
vi.mock('vue-router', async (importOriginal) => ({
    ...await importOriginal(),
    useRoute: () => ({ query: {} }),
}))
vi.mock('@/store', () => ({ default: { commit: vi.fn() } }))

let wrapper
const rememberedUser = { username: 'test-viewer', password: 'test-password', remember: true }

beforeEach(() => {
    vi.clearAllMocks()
    mocks.replace.mockResolvedValue(undefined)
    mocks.response = { code: 301, msg: 'Token expired', data: null }
    localStorage.clear()
    clearRemeberMe()
})

afterEach(() => {
    wrapper?.unmount()
    wrapper = undefined
    vi.restoreAllMocks()
    clearRemeberMe()
    localStorage.clear()
})

function mountLogin() {
    wrapper = mount(Login, {
        global: { plugins: [Antd], stubs: { SvgIcon: true } },
    })
    return wrapper
}

describe('remembered login', () => {
    it('saves remembered credentials when the login form is submitted', async () => {
        mocks.response = {
            code: 200,
            msg: 'success',
            data: {
                token: 'test-token',
                currentUser: { username: rememberedUser.username },
                menuList: [],
                role_codes: [],
            },
        }
        mountLogin()
        await wrapper.find('input[type="text"]').setValue(rememberedUser.username)
        await wrapper.find('input[type="password"]').setValue(rememberedUser.password)
        await wrapper.find('input[type="checkbox"]').setValue(true)
        await wrapper.find('form').trigger('submit')
        await flushPromises()

        expect(getRemeberMeInfo()).toEqual(rememberedUser)
        expect(localStorage.getItem('token')).toBe('test-token')
    })

    it('encrypts the saved password and restores both login fields', () => {
        setRemeberMe({ ...rememberedUser })
        expect(Cookies.get('user')).not.toContain(rememberedUser.password)
        expect(getRemeberMeInfo()).toEqual(rememberedUser)
        mountLogin()
        expect(wrapper.find('input[type="text"]').element.value).toBe(rememberedUser.username)
        expect(wrapper.find('input[type="password"]').element.value).toBe(rememberedUser.password)
        expect(wrapper.find('input[type="checkbox"]').element.checked).toBe(true)
    })

    it('keeps remembered credentials when an expired token redirects to login', async () => {
        setRemeberMe({ ...rememberedUser })
        const savedCookie = Cookies.get('user')
        localStorage.setItem('token', 'expired-token')
        localStorage.setItem('currentUser', JSON.stringify({ username: rememberedUser.username }))

        await expect(request.get('/sys/users/current/')).rejects.toThrow('Token expired')
        await flushPromises()

        expect(localStorage.getItem('token')).toBeNull()
        expect(localStorage.getItem('currentUser')).toBeNull()
        expect(Cookies.get('user')).toBe(savedCookie)
        expect(mocks.replace).toHaveBeenCalledWith({ path: '/login', query: { redirect: '/index' } })
        mountLogin()
        expect(wrapper.find('input[type="text"]').element.value).toBe(rememberedUser.username)
        expect(wrapper.find('input[type="password"]').element.value).toBe(rememberedUser.password)
    })

    it('preserves remembered credentials across a timezone save followed by token expiration', async () => {
        setRemeberMe({ ...rememberedUser })
        const savedCookie = Cookies.get('user')
        const profile = { id: 3, username: rememberedUser.username, phonenumber: '256', timezone: 'Asia/Hong_Kong' }
        mocks.response = { code: 200, msg: 'success', data: { user: profile } }

        const response = await updateUserInfo({ phonenumber: profile.phonenumber, timezone: profile.timezone })
        saveCurrentUser(response.data.data.user)
        emitUserTimezoneChanged(response.data.data.user.timezone)
        expect(Cookies.get('user')).toBe(savedCookie)

        mocks.response = { code: 301, msg: 'Token expired', data: null }
        await expect(request.get('/sys/users/current/')).rejects.toThrow('Token expired')
        await flushPromises()
        expect(localStorage.getItem('currentUser')).toBeNull()
        expect(Cookies.get('user')).toBe(savedCookie)
        mountLogin()
        expect(wrapper.find('input[type="text"]').element.value).toBe(rememberedUser.username)
        expect(wrapper.find('input[type="password"]').element.value).toBe(rememberedUser.password)
        expect(wrapper.find('input[type="checkbox"]').element.checked).toBe(true)
    })
})
