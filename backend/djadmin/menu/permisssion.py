from rest_framework import permissions
from user.utils import getCurrentUser
from djadmin.errordict import DjadminException,CommonError
class CustomMenuPermission(permissions.BasePermission):
    
    
    def has_permission(self, request, view):
        print("view action")
        print(view.action)
        userInfo = getCurrentUser(request)
        action = view.action
        if userInfo['username'] == "admin":
            return True
        perm_code = view.action_perms_map.get(action)
            
        # 从用户角色关联的菜单中获取权限标识
        user_perms = userInfo['perms']
        if perm_code == None:
            # 说明不需要权限
            return True
        if  perm_code not in user_perms:
            message = "当前操作需要" + perm_code +"权限"
            raise DjadminException(CommonError.NO_PERMISSION,extra_msg=message)
        else:
            return True
