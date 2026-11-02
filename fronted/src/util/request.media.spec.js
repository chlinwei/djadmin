import { afterEach, describe, expect, it, vi } from 'vitest'

vi.mock('@/api/user', () => ({ getToken: () => 'test-token' }))
vi.mock('@/router', () => ({ default: {} }))

afterEach(() => {
  vi.unstubAllEnvs()
  vi.resetModules()
})

describe('backend media URLs', () => {
  it.each([
    ['http://10.25.66.150:9000', 'http://10.25.66.150:9000'],
    ['https://api.example.test/', 'https://api.example.test'],
    ['https://gateway.example.test/api/', 'https://gateway.example.test/api'],
  ])('resolves software packages against configured API base %s', async (baseUrl, expectedBase) => {
    vi.stubEnv('VITE_API_BASE_URL', baseUrl)
    vi.resetModules()
    const { getMediaUrl } = await import('./request')
    const path = 'monitor_packages/node_exporter/amd64/linux/node_exporter-1.8.2.linux-amd64.tar.gz'
    expect(getMediaUrl(`/media/${path}`)).toBe(`${expectedBase}/media/${path}`)
    expect(getMediaUrl(`media/${path}`)).toBe(`${expectedBase}/media/${path}`)
    expect(getMediaUrl(path)).toBe(`${expectedBase}/media/${path}`)
  })

  it('uses the backend port by default and preserves absolute download URLs', async () => {
    vi.stubEnv('VITE_API_BASE_URL', '')
    vi.resetModules()
    const { getMediaUrl } = await import('./request')
    expect(getMediaUrl('/media/package.tar.gz')).toBe(`${window.location.protocol}//${window.location.hostname}:9000/media/package.tar.gz`)
    expect(getMediaUrl('https://files.example.test/package.tar.gz')).toBe('https://files.example.test/package.tar.gz')
    expect(getMediaUrl('')).toBe('')
  })
})
