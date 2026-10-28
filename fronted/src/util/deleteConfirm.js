import { h } from 'vue'
import { Modal, message } from 'ant-design-vue'

export function assertBatchDeleteSucceeded(response, ids) {
  const results = response?.data?.data?.results
  // HTTP 成功只表示批删请求已处理，每个目标都必须有明确的成功结果。
  const failures = ids.flatMap((id) => {
    const result = Array.isArray(results) ? results.find((item) => item?.id === id) : undefined
    return result?.ok === true ? [] : [`#${id}: ${result?.message || '删除结果异常，请刷新列表后重试'}`]
  })
  if (failures.length) {
    const reason = failures.join('；')
    message.error(reason)
    throw new Error(reason)
  }
}

function normalizeDeleteItems(items) {
  if (!Array.isArray(items)) {
    return []
  }
  return items
    .map((item) => String(item || '').trim())
    .filter((item) => item.length > 0)
}

function buildDeleteContent(summary, items) {
  const normalizedItems = normalizeDeleteItems(items)
  const contentNodes = []

  if (summary) {
    contentNodes.push(
      h(
        'div',
        {
          style: {
            marginBottom: normalizedItems.length ? '10px' : '0',
            color: 'rgba(0,0,0,0.88)',
            lineHeight: '22px',
          },
        },
        summary,
      ),
    )
  }

  if (normalizedItems.length) {
    contentNodes.push(
      h(
        'div',
        {
          style: {
            maxHeight: '260px',
            overflowY: 'auto',
            border: '1px solid #f0f0f0',
            borderRadius: '6px',
            padding: '8px 10px',
            background: '#fafafa',
          },
        },
        normalizedItems.map((item) =>
          h(
            'div',
            {
              style: {
                lineHeight: '22px',
                wordBreak: 'break-all',
                color: 'rgba(0,0,0,0.88)',
              },
            },
            `- ${item}`,
          ),
        ),
      ),
    )
  }

  return h('div', {}, contentNodes)
}

export function openDeleteConfirm(options = {}) {
  const {
    title = '确认删除',
    okText = '确认删除',
    cancelText = '取消',
    summary = '',
    items = [],
    onConfirm,
  } = options

  return new Promise((resolve) => {
    Modal.confirm({
      title,
      centered: true,
      okText,
      cancelText,
      okButtonProps: { danger: true },
      content: buildDeleteContent(summary, items),
      async onOk() {
        if (typeof onConfirm !== 'function') {
          resolve(true)
          return
        }
        try {
          await onConfirm()
          resolve(true)
        } catch (error) {
          // antd 会将 onOk 的拒绝原样抛出而不接管，不兜底就会变成全局未捕获拒绝。
          // 业务码错误已由请求拦截器提示，这里只补 HTTP 错误，避免重复弹窗。
          if (error?.isAxiosError) {
            message.error(error?.message || '删除失败')
          }
          resolve(false)
        }
      },
      onCancel() {
        resolve(false)
      },
    })
  })
}
