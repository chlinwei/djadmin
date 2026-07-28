import { onActivated, onDeactivated } from 'vue'

// 统一处理 keep-alive 页面切换时的轮询生命周期：
// 激活时恢复刷新，失活时暂停刷新，避免后台持续请求。
export function useKeepAliveRefreshLifecycle(startRefresh, stopRefresh) {
  onActivated(() => {
    if (typeof startRefresh === 'function') {
      startRefresh()
    }
  })

  onDeactivated(() => {
    if (typeof stopRefresh === 'function') {
      stopRefresh()
    }
  })
}
