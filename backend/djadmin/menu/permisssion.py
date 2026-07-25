from rest_framework import permissions
from user.utils import getCurrentUser
from djadmin.errordict import DjadminException,CommonError
class CustomMenuPermission(permissions.BasePermission):
    
    
    def has_permission(self, request, view):
        userInfo = getCurrentUser(request)
        action = view.action
        perm_code = view.action_perms_map.get(action)

        # action_perms_map 显式配置为 None 的动作表示无需菜单权限（如 Prometheus http_sd）。
        if perm_code is None:
            return True

        if userInfo.get('username') == "admin":
            return True
            
        # 从用户角色关联的菜单中获取权限标识
        user_perms = userInfo.get('perms') or []
        if  perm_code not in user_perms:
            message = "当前操作需要" + perm_code +"权限"
            raise DjadminException(CommonError.NO_PERMISSION,extra_msg=message)
        else:
            return True
