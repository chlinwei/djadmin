const USER_TIMEZONE_CHANGED_EVENT = 'user-timezone-changed'

export function emitUserTimezoneChanged(timezone) {
  if (!timezone) {
    return
  }
  window.dispatchEvent(new CustomEvent(USER_TIMEZONE_CHANGED_EVENT, {
    detail: { timezone },
  }))
}

export function listenUserTimezoneChanged(handler) {
  if (typeof handler !== 'function') {
    return () => {}
  }

  const wrapped = (event) => {
    const timezone = event?.detail?.timezone
    if (timezone) {
      handler(timezone)
    }
  }

  window.addEventListener(USER_TIMEZONE_CHANGED_EVENT, wrapped)
  return () => {
    window.removeEventListener(USER_TIMEZONE_CHANGED_EVENT, wrapped)
  }
}
