from enum import Enum
class ErrorMixin:
    """错误码基础类，封装公共属性"""
    def __init__(self, code, msg):
        self.code = code
        self.msg = msg

    def __str__(self):
        return f"[{self.code}] {self.msg}"
    

class CommonError(ErrorMixin,Enum):
    NO_PERMISSION = (4001,"没有权限")
class MenuError(ErrorMixin,Enum):
    menu_saveOrcreate_error = (3001,"菜单创建失败")
    
class RoleError(ErrorMixin,Enum):
    role_ids_empty = (2001,"roleid数组为空错误")

class UserError(ErrorMixin, Enum):
    """认证相关错误（继承顺序必须Mixin在前）"""
    update_password_error = (1001, "旧密码错误，更新密码失败")
    change_avatar_error = (1002, "头像上传失败")
    change_status_error = (1003,"修改状态失败")
    user_ids_empty = (1004,"用户id数组为空错误")
    user_not_exists = (1005,"用户不存在错误")

class AssetsError(ErrorMixin,Enum):
    FILE_NOT_ENDSWITH_CSV = (5001,"文件后缀必须是CSV")
    BATCH_UPLOAD_ERROR = (5002,"批量导入失败")




class ServerError(ErrorMixin, Enum):    
    """服务器相关错误"""
    INTERNAL_ERROR = (2001, "服务器内部错误")


class DjadminException(Exception):
    def __init__(self, error_code: ErrorMixin, extra_msg=None):
        self.code = error_code.code
        self.message = f"{error_code.msg}({extra_msg})" if extra_msg else error_code.message




