// 同一失败项无论由拦截器还是页面展示，都要区分“目标已创建”和“安装已受理”。
export function batchFailureReason({ host, host_id, id, detail, message }) {
  const state = detail?.created && detail.install_accepted === false ? '目标已创建，安装未受理；' : ''
  const job = detail?.job_id ? `（作业 ${detail.job_id}）` : ''
  return `${host || `#${host_id ?? id}`}：${state}${message || '操作失败'}${job}`
}
