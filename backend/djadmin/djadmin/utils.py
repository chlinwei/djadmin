

from django.http import JsonResponse
from .errordict import ErrorMixin
from djadmin.errordict import DjadminException



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
        

def Response_error_str(msg,code=400,data=None):
        return JsonResponse({
            'code':code,
            'data': data,
            'msg':msg
        })
def Response_djerror(djerror: DjadminException,data=None):
        return JsonResponse({
            'code':djerror.code,
            'data': data,
            'msg':djerror.message
        })


from rest_framework.pagination import PageNumberPagination
from rest_framework.response import Response


# 自定义分页类（可选）
class CustomPagination(PageNumberPagination):
    page_size = 10  # 默认每页记录数
    page_size_query_param = 'page_size'  # 前端传递的参数名
    max_page_size = 30  # 最大允许每页记录数
    
    def get_paginated_response(self, data):
        return Response({
            'code': 200,
            'msg': 'success',
            'data': {
                'results': data,  # 实际的数据列表
                'count': self.page.paginator.count,  # 总数
                'next': self.get_next_link(),  # 下一页链接
                'previous': self.get_previous_link(),  # 上一页链接
                'pageSize': self.page_size,  # 每页大小
                'pageNumber': self.page.number,  # 当前页号
                'totalPages': self.page.paginator.num_pages  # 总页数
            }
        })


