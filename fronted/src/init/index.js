import { getPerms } from "@/api/menu"
import { getCurrentUser } from "@/api/user"
import store from '@/store'

export default  function() {
    // 从localstorage加载user 
    loadUser()
    // 从localstorage加载perms
    loadPerms()
}

function loadUser() {
    var user = getCurrentUser()
    store.commit("add_user",user)
}
function loadPerms() {
    var perms = getPerms()
    store.commit("add_perms",perms)
}