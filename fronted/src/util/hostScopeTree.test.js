import { describe, expect, it } from 'vitest'

import { retainAvailableHostIds } from './hostScopeTree'

describe('retainAvailableHostIds', () => {
  it('removes stale host IDs before an inspection task is saved', () => {
    const selectedHostIds = [242, 212, 213]
    const hosts = [{ id: 212 }, { id: 213 }]

    expect(retainAvailableHostIds(selectedHostIds, hosts)).toEqual([212, 213])
  })

  it('normalizes numeric IDs and removes duplicates', () => {
    const selectedHostIds = ['212', 212, 0, null]
    const hosts = [{ id: 212 }]

    expect(retainAvailableHostIds(selectedHostIds, hosts)).toEqual([212])
  })
})