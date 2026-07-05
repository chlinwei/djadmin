// JSDOM does not provide matchMedia; mock it for Ant Design Vue components.
if (!window.matchMedia) {
  window.matchMedia = function matchMedia() {
    return {
      matches: false,
      media: '',
      onchange: null,
      addListener: () => {},
      removeListener: () => {},
      addEventListener: () => {},
      removeEventListener: () => {},
      dispatchEvent: () => false,
    }
  }
}
