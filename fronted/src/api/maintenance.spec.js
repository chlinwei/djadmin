import { it, expect, vi } from 'vitest'
import request from '@/util/request'
import { listWindows, windowDetail, previewWindow, saveWindow, cancelWindow, endWindow } from './maintenance'

vi.mock('@/util/request', () => ({ default: { get:vi.fn(), post:vi.fn() } }))
it('查询、预览只使用 GET URL 参数；写入与提前结束均为显式动作', () => {
  const conditions = [{ type:'label', label:'team', operator:'=', value:'ops' }]
  listWindows({ page:2, page_size:10 }); windowDetail(7); previewWindow(conditions)
  expect(request.get.mock.calls).toEqual([
    ['monitor/maintenance-windows/', { page:2, page_size:10 }],
    ['monitor/maintenance-windows/detail/', { id:7 }],
    ['monitor/maintenance-windows/preview/', { matchers:JSON.stringify(conditions) }],
  ])
  expect(request.post).not.toHaveBeenCalled()
  saveWindow({ title:'发布' }); saveWindow({ id:7 }); cancelWindow(7); endWindow(8)
  expect(request.post.mock.calls).toEqual([
    ['monitor/maintenance-windows/create/', { title:'发布' }], ['monitor/maintenance-windows/update/', { id:7 }],
    ['monitor/maintenance-windows/cancel/', { id:7 }], ['monitor/maintenance-windows/end/', { id:8 }],
  ])
})
