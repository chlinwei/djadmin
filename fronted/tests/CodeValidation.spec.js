import { readFileSync } from 'node:fs'
import { describe, expect, it } from 'vitest'

describe.each([
  'ApplicationServiceDialog',
  'BusinessSystemDialog',
  'BusinessEnvironmentDialog',
  'ClusterProfileDialog',
  'ProjectDialog',
])('%s code validation', (component) => {
  const source = readFileSync(`src/views/assets/application/components/${component}.vue`, 'utf8')
  const match = source.match(/pattern:\s*\/([^/]+)\/,\s*message:\s*'编码/)
  const pattern = new RegExp(match[1])

  it.each(['QM1', 'imf-MQ_02', 'nkg', '123', 'A'])('accepts %s', (value) => {
    expect(pattern.test(value)).toBe(true)
  })

  it.each(['', '_QM1', '-QM1', 'QM 1', 'QM.1', '队列'])('rejects %s', (value) => {
    expect(pattern.test(value)).toBe(false)
  })
})
