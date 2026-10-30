import request from '@/util/request'

const relations = 'assets/service-dependencies/'
const types = 'assets/service-dependency-types/'

export const getServiceDependencyTopology = (params) => request.get(`${relations}topology/`, params)
export const saveServiceDependency = (id, payload) => id
  ? request.put(`${relations}${id}/`, payload)
  : request.post(relations, payload)
export const deleteServiceDependencies = (ids) => request.post(`${relations}batch-delete/`, { ids })
export const getServiceDependencyTypes = (params) => request.get(types, params)
export const saveServiceDependencyType = (id, payload) => id
  ? request.put(`${types}${id}/`, payload)
  : request.post(types, payload)
export const deleteServiceDependencyTypes = (ids) => request.post(`${types}batch-delete/`, { ids })
