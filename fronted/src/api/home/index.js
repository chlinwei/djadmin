import request from '@/util/request'

export function getHomeOverview() {
  return request.get('/sys/home/overview/', {}, 10000, { suppressBusinessErrorMessage: true })
}
