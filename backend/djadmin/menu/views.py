from django.shortcuts import render
from rest_framework.generics import ListCreateAPIView
from .models import SysMenu,SysRoleMenu
# Create your views here.

from rest_framework.views import APIView
from .models import SysMenu
from .models import SysRoleMenu
from .serializer import SysMenuDynamicListSerializer
from role.models import SysRole
from djadmin.utils import Response_200


# 获取权限树(所有权限)
class MenuTree(APIView):
    def get(self,request):        
        menu_list = SysMenu.objects.order_by("order_num")
        menu_list_data = SysMenuDynamicListSerializer(menu_list)
        return Response_200(menu_list_data.data)

        
        
        
        