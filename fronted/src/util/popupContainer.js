export function resolvePopupContainerByContext(triggerNode, options = {}) {
  const {
    modalSelector = '.ant-modal-wrap',
    drawerSelector = '.ant-drawer-content-wrapper',
    fallback = document.body,
  } = options

  if (triggerNode && typeof triggerNode.closest === 'function') {
    if (modalSelector) {
      const modalWrap = triggerNode.closest(modalSelector)
      if (modalWrap && document.body.contains(modalWrap)) {
        return modalWrap
      }
    }

    if (drawerSelector) {
      const drawerWrap = triggerNode.closest(drawerSelector)
      if (drawerWrap && document.body.contains(drawerWrap)) {
        return drawerWrap
      }
    }
  }

  return fallback
}
