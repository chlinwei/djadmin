from django.http import JsonResponse
from django.utils.deprecation import MiddlewareMixin
from jwt import ExpiredSignatureError, InvalidTokenError, PyJWTError
from rest_framework_jwt.settings import api_settings


class JwtAuthenticationMiddleware(MiddlewareMixin):

    def process_request(self, request):
        white_list = ["/auth/login"]  # 请求白名单
        path = request.path
        if path not in white_list and not path.startswith("/media"):
            print("要进行token验证")
            token = request.META.get('HTTP_AUTHORIZATION')
            print("token:", token)
            err_ret = {
                'code':301,
                'msg': ''
            }
            try:
                jwt_decode_handler = api_settings.JWT_DECODE_HANDLER
                print(token)
                jwt_decode_handler(token)
            except ExpiredSignatureError:
                err_ret.msg = 'Token过期，请重新登录！'
                return JsonResponse(err_ret)
            except InvalidTokenError:
                err_ret.msg = 'Token验证失败！'
                return JsonResponse(err_ret)
            except PyJWTError:
                err_ret.msg = 'Token验证异常！'
                return JsonResponse(err_ret)
        else:
            print("不需要token验证")
            return None
