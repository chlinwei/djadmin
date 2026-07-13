import { h } from 'vue'
import { Modal } from 'ant-design-vue'

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
        await onConfirm()
        resolve(true)
      },
      onCancel() {
        resolve(false)
      },
    })
  })
}
