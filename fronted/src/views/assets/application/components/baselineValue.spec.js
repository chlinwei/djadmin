import { describe, expect, it } from 'vitest'

import { formatActualCheckValue } from './baselineValue'

describe('formatActualCheckValue', () => {
  it('shows shell stdout instead of execution metadata', () => {
    const result = {
      check_type: 'shell',
      actual_value: {
        exit_code: 0,
        stdout: 'Apache Tomcat/9.0.35\n',
        stderr: '',
      },
    }

    expect(formatActualCheckValue(result)).toBe('Apache Tomcat/9.0.35')
  })
})