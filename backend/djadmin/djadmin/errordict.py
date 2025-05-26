from enum import Enum

class ErrorMixin:
    """错误码基础类，封装公共属性"""
    def __init__(self, code, msg):
        self.code = code
        self.msg = msg

    def __str__(self):
        return f"[{self.code}] {self.msg}"

class UserError(ErrorMixin, Enum):
    """认证相关错误（继承顺序必须Mixin在前）"""
    update_password_error = (1001, "旧密码错误，更新密码失败")
    change_avatar_error = (1002, "头像上传失败")
    change_status_error = (1003,"修改状态失败")
    user_ids_empty = (1004,"用户id数组为空错误")


class ServerError(ErrorMixin, Enum):
    """服务器相关错误"""
    INTERNAL_ERROR = (2001, "服务器内部错误")



