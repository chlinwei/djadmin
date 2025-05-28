from django.shortcuts import render
from rest_framework.views import APIView
from role.models import SysRole
from user.utils import getCurrentUser
from django.http import JsonResponse
from .models import SysRole
from .serializer import SysRoleSerializer
from rest_framework import generics
# Create your views here.

from djadmin.utils import CustomPagination
from djadmin.utils import Response_200


class currentUserRoleListView(APIView):
    # SysRole.objects.raw("select ")
    def get(self,request):
        # 获取当前用户id
        userInfo = getCurrentUser(request)
        #查询用户角色根据用户id
        raw_data = SysRole.objects.raw("select sr.id as id,sr.name as name,sr.code as code,sr.create_time  as create_time,sr.update_time as update_time ,sr.remark as remark  from sys_user_role sur   inner join sys_role sr ON sur.role_id = sr.id  WHERE sur.user_id  = %s",[userInfo['user_id']])
        roleList = SysRoleSerializer(raw_data,many=True).data
        return JsonResponse({
            'code':200,
            'data': {
                'roleList': roleList,
            },
            'msg': 'success'
        })
    


# 角色 列表，新增
class RoleListCreate(generics.ListCreateAPIView):
    queryset = SysRole.objects.all()
    serializer_class = SysRoleSerializer
    pagination_class = CustomPagination
# 角色 detail,
class RoleRetrieveUpdateDestroyAPIView(generics.RetrieveUpdateDestroyAPIView):
    queryset = SysRole.objects.all()
    serializer_class = SysRoleSerializer
    pagination_class = CustomPagination
    lookup_field = 'id'


#根据用户id获取用户包含的角色列表
class GetUserRolesByIdView(APIView):
    def get(self,request):
        # 获取当前用户id
        user_id = request.query_params.get('user_id')
        #查询用户角色根据用户id
        raw_data = SysRole.objects.raw("select sr.id as id,sr.name as name,sr.code as code,sr.create_time  as create_time,sr.update_time as update_time ,sr.remark as remark  from sys_user_role sur   inner join sys_role sr ON sur.role_id = sr.id  WHERE sur.user_id  = %s",[user_id])
        roleList = SysRoleSerializer(raw_data,many=True).data
        return Response_200(data={"roleList":roleList})