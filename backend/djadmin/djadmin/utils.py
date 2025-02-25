

from django.http import JsonResponse
from .errordict import ErrorMixin

def Response_200(data=None,msg="success"):
        return JsonResponse({
            'code':200,
            'data': data,
            'msg':msg
        })

def Response_error(error:ErrorMixin,data=None):
        return JsonResponse({
            'code':error.code,
            'data': data,
            'msg':error.msg
        })



