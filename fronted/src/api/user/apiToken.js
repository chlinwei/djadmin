import requestUtil from '@/util/request'

const prefix = 'sys/usercenter/'

export function getApiTokenList() {
  return requestUtil.get(prefix + 'apiTokens/')
}

export function createApiToken(payload = {}) {
  return requestUtil.post(prefix + 'createApiToken/', payload)
}

export function rotateApiToken(tokenId) {
  return requestUtil.post(prefix + 'rotateApiToken/', {
    id: tokenId,
  })
}

export function disableApiToken(tokenId) {
  return requestUtil.post(prefix + 'disableApiToken/', {
    id: tokenId,
  })
}

export function deleteApiToken(tokenId) {
  return requestUtil.post(prefix + 'deleteApiToken/', {
    id: tokenId,
  })
}
