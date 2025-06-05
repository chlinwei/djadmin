import checkPermission from './permission'
export default {
  inserted(el, binding) {
    if (!checkPermission(binding.value)) {
      el.parentNode?.removeChild(el)
    }
  }
}