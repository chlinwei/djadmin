
from rest_framework_jwt.settings import api_settings
#获取缓存信息
def getCurrentUser(request) -> dict:
    # 获取token
    token = request.META.get('HTTP_AUTHORIZATION')
    if(token):
        jwt_decode_handler = api_settings.JWT_DECODE_HANDLER
        userInfo = jwt_decode_handler(token)  # type: ignore
        # {'user_id': 65, 'username': 'admin', 'exp': 1749092928, 'email': 'fdsfafs@qq.com', 'perms': [...]}
        return userInfo
    return {}
    
    