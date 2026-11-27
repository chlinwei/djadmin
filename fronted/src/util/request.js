// 引入axios
import axios from 'axios';
import {getToken} from '@/api/user'
import router from '@/router';
import { message } from 'ant-design-vue';
import { batchFailureReason } from './batchFeedback'


const defaultBaseUrl = `${window.location.protocol}//${window.location.hostname}:9000`
const defaultTransferUrl = `${window.location.protocol}//${window.location.hostname}:9101`
let baseUrl = import.meta.env.VITE_API_BASE_URL || defaultBaseUrl;
let transferBaseUrl = import.meta.env.VITE_TRANSFER_BASE_URL || defaultTransferUrl;
// 创建axios实例
const httpService = axios.create({
    // url前缀-'http:xxx.xxx'
    // baseURL: process.env.BASE_API, // 需自定义
    baseURL:baseUrl,
    // 请求超时时间（默认30秒，针对长时间运行的任务如主机信息采集）
    timeout: 30000 // 需自定义
});

let redirectingToLogin = false

class BusinessError extends Error {
    constructor(response, fallbackMessage) {
        super(response.data.msg || fallbackMessage)
        this.name = 'BusinessError'
        // 批量操作可能已提交部分结果；调用方必须能读取完整响应并刷新界面。
        this.response = response
        this.code = response.data.code
    }
}

function clearAuthState() {
    localStorage.removeItem('token')
    localStorage.removeItem('currentUser')
}

function handleAuthExpired(msg) {
    clearAuthState()
    const currentRoute = router.currentRoute?.value
    if (redirectingToLogin || currentRoute?.path === '/login') {
        return
    }
    redirectingToLogin = true
    message.error(msg || '登录过期，请重新登陆')
    const currentPath = currentRoute?.fullPath || '/index'
    router.replace({
        path: '/login',
        query: { redirect: currentPath }
    }).finally(() => {
        redirectingToLogin = false
    })
}

function businessErrorMessage(payload) {
    const results = payload?.data?.results
    if (!Array.isArray(results) || (results.length === 1 && !results[0]?.detail)) return payload.msg
    const failures = results.filter((item) => item?.ok === false)
    if (!failures.length) return payload.msg
    const details = failures.map(batchFailureReason)
    return `${payload.msg}：${details.join('；')}`
}

// HTTP 非 2xx 也可能携带标准业务信封；同一错误只在一处解释，保留 data。
// 协议响应（Prometheus、下载等）不能只因包含一个同名字段而被当作业务错误。
function checkBusinessResponse(response) {
    const payload = response?.data
    if (!Number.isInteger(payload?.code) || typeof payload.msg !== 'string' ||
        !Object.hasOwn(payload, 'data') || payload.code === 200) return response

    if (payload.code === 301) {
        handleAuthExpired(payload.msg)
    } else if (!response.config?.suppressBusinessErrorMessage) {
        message.error(businessErrorMessage(payload))
    }
    throw new BusinessError(response, '请求失败')
}

//添加请求和响应拦截器
// 添加请求拦截器
httpService.interceptors.request.use(function (config) {
    // 在发送请求之前做些什么
    config.headers.AUTHORIZATION=getToken();
    return config;
}, function (error) {
    // 对请求错误做些什么
    return Promise.reject(error);
});

// 添加响应拦截器
httpService.interceptors.response.use(checkBusinessResponse, function (error) {
    checkBusinessResponse(error.response)
    // 没有业务错误信封时保留网关的认证处理；业务 403 不应清除有效会话。
    if (error?.response && [401, 403].includes(Number(error.response.status))) {
        handleAuthExpired('登录状态失效，请重新登陆')
    }
    if (!error.response && error.request && error.code !== 'ERR_CANCELED' &&
        ['post', 'put', 'patch', 'delete'].includes(error.config?.method)) {
        error.message = '未收到服务器响应，操作结果未知；请刷新核对后再决定是否重试'
    }
    return Promise.reject(error);
});

/*网络请求部分*/

/*
 *  get请求
 *  url:请求地址
 *  params:参数
 *  timeout:自定义超时时间(可选，毫秒)
 * */
export function get(url, params = {}, timeout = null, options = {}) {
    return new Promise((resolve, reject) => {
        const config = {
            url: url,
            method: 'get',
            params: params,
            // 状态轮询由调用方展示失败；切换为 GET 后仍保留原来的错误提示策略。
            suppressBusinessErrorMessage: Boolean(options.suppressBusinessErrorMessage),
        }
        // 如果指定了自定义超时，则覆盖默认超时（如按需展开日志路径要逐台调 agent，较慢）。
        if (timeout) {
            config.timeout = timeout
        }
        httpService(config).then(response => {
            resolve(response);
        }).catch(error => {
            reject(error);
        });
    });
}

/*
 *  post请求
 *  url:请求地址
 *  params:参数
 *  timeout:自定义超时时间(可选，毫秒)
 * */
export function post(url, params = {}, timeout = null, options = {}) {
    return new Promise((resolve, reject) => {
        const config = {
            url: url,
            method: 'post',
            data: params,
            // 后台轮询静默记录状态失败，用户主动操作仍使用全局错误提示。
            suppressBusinessErrorMessage: Boolean(options.suppressBusinessErrorMessage),
        }
        // 如果指定了自定义超时，则覆盖默认超时
        if (timeout) {
            config.timeout = timeout
        }
        httpService(config).then(response => {
            resolve(response);
        }).catch(error => {
            reject(error);
        });
    });
}

/*
 *  delete请求
 *  url:请求地址
 *  params:参数
 * */
export function del(url, params = {}) {
    return new Promise((resolve, reject) => {
        httpService({
            url: url,
            method: 'delete',
            data: params
        }).then(response => {
            resolve(response);
        }).catch(error => {
            reject(error);
        });
    });
}


/*
 *  文件上传
 *  url:请求地址
 *  params:参数
 * */
export function fileUpload(url, params = {}, options = {}) {
    return new Promise((resolve, reject) => {
        const config = {
            url: url,
            method: 'post',
            data: params,
            headers: { 'Content-Type': 'multipart/form-data' }
        }
        if (options?.timeout !== undefined && options?.timeout !== null) {
            config.timeout = options.timeout
        }
        if (options?.signal) {
            config.signal = options.signal
        }
        if (options?.onUploadProgress) {
            config.onUploadProgress = options.onUploadProgress
        }
        httpService({
            ...config
        }).then(response => {
            resolve(response);
        }).catch(error => {
            reject(error);
        });
    });
}

export function getServerUrl(){
    return baseUrl;
}

// 把后端返回的头像/媒体文件名拼成可直接给 <img> 用的地址。
// 后端静态目录是 /media（autoadmin/media），avatar 可能只存文件名或 /media/... 相对路径。
export function getMediaUrl(file) {
    if (!file) {
        return '';
    }
    const value = String(file);
    if (/^https?:\/\//i.test(value)) {
        return value;
    }
    const relative = value.replace(/^\/?media\//, '').replace(/^\/+/, '');
    return `${baseUrl.replace(/\/+$/, '')}/media/${relative}`;
}

export function getWebSocketBaseUrl() {
    const parsed = new URL(baseUrl, window.location.origin)
    const wsProtocol = parsed.protocol === 'https:' ? 'wss:' : 'ws:'
    return `${wsProtocol}//${parsed.host}`
}

export function getTransferServerUrl() {
    return transferBaseUrl
}


export function patch(url, params = {}, options = {}) {
    return new Promise((resolve, reject) => {
        httpService({
            url: url,
            method: 'patch',
            data: params,
            // 编辑请求与新增请求同样允许调用方统一展示业务错误，避免两层重复提示。
            suppressBusinessErrorMessage: Boolean(options.suppressBusinessErrorMessage),
        }).then(response => {
            resolve(response);
        }).catch(error => {
            reject(error);
        });
    });
}

// 完整替换接口仍经过相同的鉴权与业务错误拦截链，不另建请求实例。
export function put(url, params = {}) {
    return httpService({ url, method: 'put', data: params })
}

export function download(url, params = {}, timeout = null) {
    return new Promise((resolve, reject) => {
        const config = {
            url: url,
            method: 'get',
            params: params,
            responseType: 'blob'
        }
        if (timeout !== null && timeout !== undefined) {
            config.timeout = timeout
        }
        httpService(config).then(response => {
            resolve(response)
        }).catch(error => {
            reject(error)
        })
    })
}

export default {
    get,
    post,
    del,
    fileUpload,
    getServerUrl,
    getMediaUrl,
    getWebSocketBaseUrl,
    getTransferServerUrl,
    patch,
    put,
    download,
}
