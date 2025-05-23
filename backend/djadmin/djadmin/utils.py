

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




from rest_framework.pagination import PageNumberPagination


# 自定义分页类（可选）
class CustomPagination(PageNumberPagination):
    page_size_query_param = 'size'  # 允许客户端动态设置分页大小
    max_page_size = 30  # 最大允许每页记录数:ml-citation{ref="3,6" data="citationList"}

