from django.shortcuts import render

from rest_framework.generics import ListCreateAPIView

from django.views import View
from django.http import JsonResponse
# Create your views here.
from django_filters import rest_framework as filters
from rest_framework.views import APIView
from rest_framework.response import Response 
from .models import SysUser
from .serializer import SysUserSerializer
from .filters import SysUserFilter

class TestView(APIView):
    def get(self,request):
        return Response("hello,world")
    

class UserListOrCreateView(ListCreateAPIView):
    queryset = SysUser.objects.all()
    serializer_class = SysUserSerializer
    filter_backends = (filters.DjangoFilterBackend,)
    filterset_class = SysUserFilter


class TestView(View):
    def get(self, request):
        token = request.META.get('HTTP_AUTHORIZATION')
        print("token:", token)
        if token != None and token != '':
            userList_obj = SysUser.objects.all()
            print(userList_obj, type(userList_obj))
            userList_dict = userList_obj.values() # 转存字典
            print(userList_dict, type(userList_dict))
            userList = list(userList_dict) # 把外层的容器转为List
            print(userList, type(userList))
            return JsonResponse({'code': 200, 'info': '测试！', 'data':
            userList})
        else:
            return JsonResponse({'code': 401, 'info': '没有'})
