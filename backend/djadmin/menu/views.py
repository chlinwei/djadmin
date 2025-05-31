from django.shortcuts import render
from rest_framework.generics import ListCreateAPIView
from .models import SysMenu,SysRoleMenu
# Create your views here.

from rest_framework.views import APIView
from .models import SysMenu
from .models import SysRoleMenu
from .serializer import SysMenuDynamicListSerializer,SysMenuSerializer,SysMenuSerializer2
from role.models import SysRole
from djadmin.utils import Response_200,Response_error_str,Response_error
from djadmin.errordict import MenuError
from datetime import datetime


# 获取权限树(所有权限)
class MenuTree(APIView):
    def get(self,request):        
        menu_list = SysMenu.objects.order_by("order_num")
        menu_list_data = SysMenuDynamicListSerializer(menu_list)
        return Response_200(menu_list_data.data)

    
# 获取一个角色所包含的权限列表
class GetMenuListByRoleId(APIView):
    def get(self,request):
        role_id = request.query_params.get("role_id")
        menuList = SysRoleMenu.objects.filter(role_id=role_id).values("menu_id")
        menuIdList = [m['menu_id'] for m in menuList]
        return Response_200(menuIdList)
    

class GrantMenu(APIView):
    def post(self,request):
        role_id = request.data.get("role_id")
        menuIdList = request.data.get('menuIds')
        SysRoleMenu.objects.filter(role_id=role_id).delete()
         # 准备批量插入数据
        role_menus = [
            SysRoleMenu(role_id=role_id, menu_id=menuId)
            for menuId in menuIdList
        ]
        SysRoleMenu.objects.bulk_create(role_menus)
        return Response_error_str("保存菜单权限失败")


        
# 新建和保存menu
class SaveOrCreateMenuView(APIView):
    def post(self,request):
        serializer = SysMenuSerializer2(data=request.data)
        if serializer.is_valid():
            menu = SysMenu(**serializer.validated_data)
            if menu.id == -1:
                # 新增
                menu.create_time = datetime.now().date()
                SysMenu.objects.create(menu)
            else:
                #保存
                menu.update_time = datetime.now().date()
                SysMenu.objects.update(menu)
            return Response_200(menu)
        else:
            return Response_error(MenuError.menu_saveOrcreate_error)
        
# 根据菜单ID获取菜单
class GetMenuById(APIView):
    def get(self,request):
        id = request.query_params.get("id")
        menu = SysMenu.objects.get(id=id)
        serializer = SysMenuSerializer2(menu)
        return Response_200(serializer.data)

            