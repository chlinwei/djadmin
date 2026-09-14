import requestUtil from '@/util/request'

const prefix = 'sys/user-groups/'

export function getUserGroupList() {
  return requestUtil.get(prefix)
}

export function createUserGroup(data) {
  return requestUtil.post(prefix + 'create/', data)
}

export function updateUserGroup(data) {
  return requestUtil.post(prefix + 'update/', data)
}

export function batchDeleteUserGroups(ids) {
  return requestUtil.post(prefix + 'batch-delete/', { ids })
}
