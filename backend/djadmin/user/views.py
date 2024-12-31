from django.shortcuts import render

from rest_framework.generics import ListCreateAPIView

from django.db import connection

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
from role.models import SysRole

from role.serializer import SysRoleSerializer


from menu.serializer import SysMenuDynamicListSerializer
from menu.models import SysMenu




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
            # self.test(user.id)
            return JsonResponse({
                'code':200,
                'data': {
                    'currentUser':SysUserSerializer(user).data,
                    'token': token,
                    'menuList': menu_list
                },
            })