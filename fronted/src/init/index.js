import { getPerms } from "@/api/menu"
import { getRoleCodes } from "@/api/role"
import { getCurrentUser } from "@/api/user"
import store from '@/store'

export default  function() {
    // 从localstorage加载user 
    loadUser()
    // 从localstorage加载perms
    loadPerms()
    // 从localstorage加载role_codes
    loadRole_codes()
}

function loadUser() {
    var user = getCurrentUser()
    store.commit("add_user",user)
}
function loadPerms() {
    var perms = getPerms()
    store.commit("add_perms",perms)
}
function loadRole_codes() {
    var role_codes = getRoleCodes()
    store.commit("add_role_codes",role_codes)
}
