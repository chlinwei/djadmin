from django.shortcuts import render

from rest_framework.generics import ListCreateAPIView

from django.db import connection

from django.views import View
from django.http import JsonResponse
# Create your views here.
from django_filters import rest_framework as filters
from rest_framework.views import APIView
from rest_framework.response import Response 
from .models import SysUser
from .serializer import SysUserSerializer
from .filters import SysUserFilter

from django.core.exceptions import ObjectDoesNotExist
from djadmin.utils import Response_200,Response_error



from datetime import datetime
#login
from rest_framework_jwt.settings import api_settings
from role.models import SysRole

from role.serializer import SysRoleSerializer


from menu.serializer import SysMenuDynamicListSerializer
from menu.models import SysMenu

from user.utils import getCurrentUser

from .serializer import SysUserRoleSerializer

import os


#缓存
from django.core.cache import cache

#错误常量
from djadmin.errordict import UserError

from djadmin import settings

from djadmin.utils import CustomPagination
class TestView(APIView):
    def get(self,request):
        return Response("hello,world")
    
from django.db.models import Prefetch
from .models import SysUserRole
class UserListOrCreateView(ListCreateAPIView):
    # queryset = SysUser.objects.all().order_by("-id")

    queryset = SysUser.objects.prefetch_related(
        Prefetch('sysuserrole_set', queryset=SysUserRole.objects.select_related('role'))
    ).order_by('-id')
    serializer_class = SysUserRoleSerializer
    filter_backends = (filters.DjangoFilterBackend,)
    filterset_class = SysUserFilter
    pagination_class = CustomPagination


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




class LoginView(APIView):
    def getMenuList(self,userId: int):
        menu_list = SysMenu.objects.raw("select sm.id,sm.name,sm.icon,sm.parent_id,sm.order_num,sm.path,sm.component,sm.menu_type,sm.perms,sm.create_time,sm.update_time,sm.remark from sys_menu sm where id in (select menu_id as id from sys_role_menu srm where srm.role_id in (select role_id from sys_user_role sur  where sur.user_id = %s))",[userId])
        menu_list_data = SysMenuDynamicListSerializer(menu_list)
        return menu_list_data.data

            
                

    #根据用户id查询menu(一级和二级菜单)
    def _getMenuList(self,userId: int):
        menu_list = []
        with connection.cursor() as cursor:
            cursor.execute("select sm.id,sm.name,sm.icon,sm.parent_id,sm.order_num,sm.path,sm.component,sm.menu_type,sm.perms,sm.create_time,sm.update_time,sm.remark from sys_menu sm where id in (select menu_id as id from sys_role_menu srm where srm.role_id in (select role_id from sys_user_role sur  where sur.user_id = %s))",[userId])
            
            for row in cursor.fetchall():
                menu = {
                    'id': row[0],
                    'name': row[1],
                    'icon': row[2],
                    'parent_id': row[3],
                    'order_num': row[4],
                    'path': row[5],
                    'component': row[6],
                    'menu_type': row[7],
                    'perms': row[8],
                    'create_time': row[9],
                    'update_time': row[10],
                    'remark': row[11]
                }
                menu_list.append(menu)
               
        menu2_list = []
        for menu_m in menu_list:
            for menu_c in menu_list:
                #寻找子节点
                if menu_c['parent_id'] == menu_m['id']:
                    if 'children' not in menu_m:
                        menu_m['children'] = []
                    menu_m['children'].append(menu_c)
                #判断父节点
            if menu_m['parent_id'] == 0:
                menu2_list.append(menu_m)
        return menu2_list
        
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
            'code':300,
            'data': None
        })
        else:
            # menu_list = self._getMenuList(user.id)
            menu_list = self.getMenuList(user.id)
            current_user = SysUserSerializer(user).data

            #缓存到cache中
            
            # self.test(user.id)
            return JsonResponse({
                'code':200,
                'data': {
                    'currentUser':current_user,
                    'token': token,
                    'menuList': menu_list
                },
            })
        


#修改基础信息
class UpdateUserInfoView(APIView):
    def post(self,request):
        phonenumber = request.data['phonenumber']
        email = request.data['email']
        user = getCurrentUser(request)
        user_id = user['user_id']
        db_user = SysUser.objects.get(id=user_id)
        db_user.phonenumber = phonenumber
        db_user.email = email
        db_user.update_time = datetime.now().date()
        db_user.save()
        return JsonResponse({
            'code':200,
            'data': {
                'user': SysUserSerializer(db_user).data
            },
            'msg':'success'
        })
    

# 修改用户密码
class UpdateUserPasswordView(APIView):
    def post(self,request):
        user = getCurrentUser(request)
        user_id = user['user_id']
        db_user = SysUser.objects.get(id=user_id)
        old_password = request.data['old_password']
        new_password = request.data['new_password']
        if old_password != db_user.password:
            return Response_error(UserError.update_password_error)
        else:
            db_user.password = new_password
            db_user.save()
            return Response_200()


class ChangeAvatarView(APIView):
    def post(self, request):
        file = request.FILES.get('avatar')
        print("file:", file)
        if file:
            file_name = file.name
            suffixName = file_name[file_name.rfind("."):]
            new_file_name = datetime.now().strftime('%Y%m%d%H%M%S') + suffixName
            file_path = os.path.join(str(settings.MEDIA_ROOT),"userAvatar",new_file_name)
            try:
                with open(file_path, 'wb') as f:
                    for chunk in file.chunks():
                        f.write(chunk)
                return Response_200(data={"new_file_name": new_file_name})
            except:
                return Response_error(UserError.change_avatar_error)
            



from .serializer import StatusSerializer
#修改用户的状态
class ChangeStatusView(APIView):
    def post(self,request):
        serializer = StatusSerializer(data=request.data)
        serializer.is_valid(raise_exception=True)  # 自动返回400错误
        if serializer.is_valid():
            try:
                user = SysUser.objects.get(id=serializer.validated_data['user_id'])
                print(user)
                print(serializer.validated_data)
                user.status = serializer.validated_data['status']
                user.save()
                return Response_200()
            except:
                return Response_error(UserError.change_status_error)






