import { checkPermission } from "./permission"
export  default {
    // 仅通过 display 控制显隐，避免直接删除节点导致虚拟 DOM 与真实 DOM 不一致
    mounted(el, binding) {
        applyPermissionVisibility(el, binding)
    },
    updated(el, binding) {
        applyPermissionVisibility(el, binding)
    }
}

function applyPermissionVisibility(el, binding) {
    const { value } = binding
    const hasPermission = checkPermission(value)
    el.style.display = hasPermission ? '' : 'none'
}