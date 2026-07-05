import { emitUserTimezoneChanged, listenUserTimezoneChanged } from '../userTimezoneSync'

describe('userTimezoneSync', () => {
  it('triggers listener with emitted timezone', () => {
    const handler = vi.fn()
    const stop = listenUserTimezoneChanged(handler)

    emitUserTimezoneChanged('Asia/Shanghai')

    expect(handler).toHaveBeenCalledTimes(1)
    expect(handler).toHaveBeenCalledWith('Asia/Shanghai')

    stop()
  })

  it('does not emit event when timezone is empty', () => {
    const handler = vi.fn()
    const stop = listenUserTimezoneChanged(handler)

    emitUserTimezoneChanged('')

    expect(handler).not.toHaveBeenCalled()
    stop()
  })

  it('returns noop stop function when handler is invalid', () => {
    const stop = listenUserTimezoneChanged(null)

    expect(typeof stop).toBe('function')
    expect(() => stop()).not.toThrow()
  })

  it('removes listener after stop is called', () => {
    const handler = vi.fn()
    const stop = listenUserTimezoneChanged(handler)

    stop()
    emitUserTimezoneChanged('Asia/Tokyo')

    expect(handler).not.toHaveBeenCalled()
  })
})
