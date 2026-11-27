import request from '@/util/request'

const base = 'monitor/maintenance-windows/'
export const listWindows = (params) => request.get(base, params)
export const windowDetail = (id) => request.get(`${base}detail/`, { id })
export const previewWindow = (matchers) => request.get(`${base}preview/`, { matchers: JSON.stringify(matchers) })
export const saveWindow = (body) => request.post(`${base}${body.id ? 'update' : 'create'}/`, body)
export const cancelWindow = (id) => request.post(`${base}cancel/`, { id })
export const endWindow = (id) => request.post(`${base}end/`, { id })
export const deleteWindows = (ids) => request.post(`${base}batch-delete/`, { ids })
export const getSourceAlert = (id) => request.get(`monitor/alert-histories/${id}/`)
