import { expect, it, vi } from 'vitest'
import { batchUpload } from './index'
import request from '@/util/request'

vi.mock('@/util/request', () => ({ default: { fileUpload: vi.fn() } }))

it('uploads the CSV as the file part and preserves row-level failures', async () => {
  const file = new File(['username,port,auth_type\ntest,22,1'], 'demo.csv')
  const failure = { response: { data: { code: 400, data: { count: 0, results: [{ id: 2, ok: false }] } } } }
  request.fileUpload.mockRejectedValueOnce(failure)
  await expect(batchUpload(file)).rejects.toBe(failure)
  const [url, body] = request.fileUpload.mock.calls[0]
  expect(url).toBe('assets/credentials/batch-create/')
  expect(body.get('file')).toBe(file)
})
