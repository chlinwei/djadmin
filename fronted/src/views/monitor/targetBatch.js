import { h } from 'vue'
import { Modal, message } from 'ant-design-vue'
import { batchFailureReason } from '@/util/batchFeedback'

// 业务失败可能已有目标落库或作业入队；保留逐项响应供页面刷新列表、接续作业进度。
export async function reportTargetBatch(label, request) {
  let response
  try {
    response = await request
  } catch (error) {
    if (!Array.isArray(error?.response?.data?.data?.results)) throw error
    response = error.response
  }
  const data = response?.data?.data
  if (!Array.isArray(data?.results) || !data.results.length) throw new Error('缺少逐台操作结果，请刷新列表')
  const failed = data.results.filter((item) => item.ok !== true)
  const count = data.results.length - failed.length
  if (!failed.length) {
    message.success(`${label}成功：${count} 台`)
    return data
  }
  const reasons = failed.map(batchFailureReason)
  const summary = `${label}：成功 ${count} 台，失败 ${failed.length} 台`
  if (reasons.length > 3) {
    Modal.warning({
      title: summary,
      content: h('ul', { style: { maxHeight: '40vh', overflowY: 'auto' } }, reasons.map((text) => h('li', text))),
    })
  } else {
    message[count ? 'warning' : 'error'](`${summary}。${reasons.join('；')}`)
  }
  return data
}
