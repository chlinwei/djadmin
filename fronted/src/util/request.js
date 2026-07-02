// 引入axios
import axios from 'axios';
import {getToken} from '@/api/user'
import router from '@/router';
import { message } from 'ant-design-vue';


let baseUrl="http://localhost:8000";
// 创建axios实例
const httpService = axios.create({
    // url前缀-'http:xxx.xxx'
    // baseURL: process.env.BASE_API, // 需自定义
    baseURL:baseUrl,
    // 请求超时时间（默认30秒，针对长时间运行的任务如主机信息采集）
    timeout: 30000 // 需自定义
});

//添加请求和响应拦截器
// 添加请求拦截器
httpService.interceptors.request.use(function (config) {
    // 在发送请求之前做些什么
    config.headers.AUTHORIZATION=getToken();
    console.log(config.headers.AUTHORIZATION)
    return config;
}, function (error) {
    // 对请求错误做些什么
    return Promise.reject(error);
});

// 添加响应拦截器
httpService.interceptors.response.use(function (response) {
    // 对响应数据做点什么
    if(response.data.code === 300) {
        message.error("账号或者密码输入错误")
        return Promise.reject(new Error(response.data.msg))
    }else if(response.data.code != 200 && response.data.code !== undefined){
        message.error(response.data.msg)
        console.log(response)
        return Promise.reject(new Error(response.data.msg))
    }
    return response;

}, function (error) {
    // 对响应错误做点什么
    // 不在拦截器中显示消息，让各个页面自己处理
    return Promise.reject(error);
});

/*网络请求部分*/

/*
 *  get请求
 *  url:请求地址
 *  params:参数
 * */
export function get(url, params = {}) {
    return new Promise((resolve, reject) => {
        httpService({
            url: url,
            method: 'get',
            params: params
        }).then(response => {
             if(response.data.code == 301) {
                message.error("登录过期，请重新登陆");
                router.push("/login");
            }else{
                resolve(response);
            }
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
export function post(url, params = {}, timeout = null) {
    return new Promise((resolve, reject) => {
        const config = {
            url: url,
            method: 'post',
            data: params
        }
        // 如果指定了自定义超时，则覆盖默认超时
        if (timeout) {
            config.timeout = timeout
        }
        httpService(config).then(response => {
           if(response.data.code == 301) {
                message.error("登录过期，请重新登陆");
                router.push("/login");
            }else{
                resolve(response);
            }
        }).catch(error => {
            console.log(error)
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
            console.log(error)
            reject(error);
        });
    });
}


/*
 *  文件上传
 *  url:请求地址
 *  params:参数
 * */
export function fileUpload(url, params = {}) {
    return new Promise((resolve, reject) => {
        httpService({
            url: url,
            method: 'post',
            data: params,
            headers: { 'Content-Type': 'multipart/form-data' }
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

export function getWebSocketBaseUrl() {
    const parsed = new URL(baseUrl, window.location.origin)
    const wsProtocol = parsed.protocol === 'https:' ? 'wss:' : 'ws:'
    return `${wsProtocol}//${parsed.host}`
}


export function patch(url, params = {}) {
    return new Promise((resolve, reject) => {
        httpService({
            url: url,
            method: 'patch',
            data: params
        }).then(response => {
            resolve(response);
        }).catch(error => {
            console.log(error)
            reject(error);
        });
    });
}

export function download(url, params = {}) {
    return new Promise((resolve, reject) => {
        httpService({
            url: url,
            method: 'get',
            params: params,
            responseType: 'blob'
        }).then(response => {
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
    getWebSocketBaseUrl,
    patch,
    download,
}
