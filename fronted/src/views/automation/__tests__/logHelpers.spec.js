import {
  buildHostScopedLogText,
  copyTextWithFallback,
  normalizeUnifiedLogAliases,
} from '../logHelpers'

describe('automation log helpers', () => {
  it('normalizes host alias in unified log lines', () => {
    const source = [
      '[2026-07-09 10:00:00][web01(10.0.0.1)][stdout] ok: [host_212] => {}',
      '[2026-07-09 10:00:01][db01(10.0.0.2)][stderr] fatal: [host_213]: FAILED!',
      'plain line should stay as is',
    ].join('\n')

    const normalized = normalizeUnifiedLogAliases(source)

    expect(normalized).toContain('[web01(10.0.0.1)][stdout] ok: [web01(10.0.0.1)] => {}')
    expect(normalized).toContain('[db01(10.0.0.2)][stderr] fatal: [db01(10.0.0.2)]: FAILED!')
    expect(normalized).toContain('plain line should stay as is')
  })

  it('extracts host scoped lines and replaces host alias in detailed log', () => {
    const source = [
      '[2026-07-09 10:00:00][web01(10.0.0.1)][stdout] ok: [host_212] => {}',
      '[2026-07-09 10:00:01][db01(10.0.0.2)][stdout] ok: [host_213] => {}',
      '[web01(10.0.0.1)][stderr] error from host_212',
      '[misc][stdout] should be ignored',
    ].join('\n')

    const result = buildHostScopedLogText(source, {
      host_name: 'web01',
      host_ip: '10.0.0.1',
    })

    expect(result).toContain('[2026-07-09 10:00:00][stdout] ok: [web01(10.0.0.1)] => {}')
    expect(result).toContain('[stderr] error from web01(10.0.0.1)')
    expect(result).not.toContain('db01(10.0.0.2)')
    expect(result).not.toContain('host_212')
  })

  it('falls back to execCommand copy when clipboard API fails', async () => {
    let appended = false
    let removed = false

    const textarea = {
      value: '',
      style: {},
      setAttribute: () => {},
      select: () => {},
      setSelectionRange: () => {},
    }

    const fakeDocument = {
      createElement: () => textarea,
      body: {
        appendChild: () => {
          appended = true
        },
        removeChild: () => {
          removed = true
        },
      },
      execCommand: (command) => command === 'copy',
    }

    const fakeNavigator = {
      clipboard: {
        writeText: async () => {
          throw new Error('denied')
        },
      },
    }

    const copied = await copyTextWithFallback('sample text', {
      navigator: fakeNavigator,
      document: fakeDocument,
    })

    expect(copied).toBe(true)
    expect(appended).toBe(true)
    expect(removed).toBe(true)
  })
})
