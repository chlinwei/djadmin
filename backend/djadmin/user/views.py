from django.shortcuts import render

from rest_framework.generics import ListCreateAPIView

from django.views import View
from django.http import JsonResponse,request
# Create your views here.
from django_filters import rest_framework as filters
from rest_framework.views import APIView
from rest_framework.response import Response 
from .models import SysUser
from .serializer import SysUserSerializer
from .filters import SysUserFilter

from django.core.exceptions import ObjectDoesNotExist



#login
from rest_framework_jwt.settings import api_settings




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




# class LoginView(View):
#     def post(self, request):
#         username = request.GET.get('username')
#         print(username)
#         password = request.GET.get('password')
#         print(password)
#         try:
#             user = SysUser.objects.get(username=username, password=password)
#             jwt_payload_handler = api_settings.JWT_PAYLOAD_HANDLER # 小写快捷键
#             jwt_encode_handler = api_settings.JWT_ENCODE_HANDLER
#             # 将用户对象传递进去，获取到该对象的属性值
#             payload = jwt_payload_handler(user)
#         # 将属性值编码成jwt格式的字符串
#             token = jwt_encode_handler(payload)
#         except Exception as e:
#             print(e)
#             return JsonResponse({'code': 500, 'info': '用户名或者密码错误！'})
#         return JsonResponse({'code': 200, 'token': token, 'info': '登录成功！'})

class LoginView(APIView):
    def post(self,request,format=None):
        username = request.POST.get("username")
        password = request.POST.get("password")


            
        try:
            user = SysUser.objects.get(username=username, password=password)
            jwt_payload_handler = api_settings.JWT_PAYLOAD_HANDLER
            jwt_encode_handler = api_settings.JWT_ENCODE_HANDLER
            payload = jwt_payload_handler(user)
            token = jwt_encode_handler(payload)
        except ObjectDoesNotExist as e:
            user = None
        if user == None:
            return JsonResponse({
            'code':200,
            'data': None
        })
        else:
            return JsonResponse({
                'code':200,
                'data': SysUserSerializer(user).data,
                'token': token,
            })