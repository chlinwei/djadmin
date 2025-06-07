import { checkPermission } from "./permission"
export  default {
    // 支持remove,hidden
    mounted(el, binding) {
    const { value, modifiers } = binding
    console.log(typeof(value))
    if (!checkPermission(value)) {
    //   没有权限
    if(modifiers.remove) {
        console.log("remove")
        el.parentElement?.removeChild(el)
    }else{
        console.log("none")
        el.style.display = 'none'
    }
      
    }
  }
}