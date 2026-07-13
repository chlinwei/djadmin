const VALID_ALERT_TYPES = new Set(['success', 'info', 'warning', 'error'])

export const websshTerminalPanelProps = {
    messageText: { type: String, default: '' },
    messageType: { type: String, default: 'info' },
    setTerminalRef: { type: Function, required: true },
}

export const createWebsshTerminalPanelController = () => {
    const shouldShowMessage = (text) => String(text || '').trim().length > 0

    const resolveAlertType = (type) => {
        const normalizedType = String(type || '').trim().toLowerCase()
        return VALID_ALERT_TYPES.has(normalizedType) ? normalizedType : 'info'
    }

    return {
        shouldShowMessage,
        resolveAlertType,
    }
}
