from django.shortcuts import render
from rest_framework.views import APIView
from role.models import SysRole
from user.utils import getCurrentUser
from django.http import JsonResponse
from .models import SysRole
from .serializer import SysRoleSerializer
from rest_framework import generics
# Create your views here.
from user.models import SysUserRole
from menu.models import SysRoleMenu
from djadmin.utils import CustomPagination
from djadmin.utils import Response_200,Response_error
from djadmin.errordict import RoleError
from django_filters import rest_framework as filters

from .filters import SysRoleFilter
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
    filter_backends = (filters.DjangoFilterBackend,)
    filterset_class = SysRoleFilter
# 角色 detail,update
class RoleRetrieveUpdateAPIView(generics.RetrieveUpdateAPIView):
    queryset = SysRole.objects.all()
    serializer_class = SysRoleSerializer
    pagination_class = CustomPagination
    lookup_field = 'id'

# 批量删除角色
class RoleBatchDeleteAPI(APIView):
    def delete(self, request):
        # 获取ID数组参数
        role_ids = request.data.get('role_ids', [])
        if not role_ids:
            return Response_error(error=RoleError.role_ids_empty)
        # 先查用户角色列表
        SysUserRole.objects.filter(role_id__in=role_ids).delete()
        # 再执行删除（此时deleted_count包含关联表）
        SysRoleMenu.objects.filter(role_id__in=role_ids).delete()
        #删除角色
        SysRole.objects.filter(id__in=role_ids).delete() 
        return Response_200()



#根据用户id获取用户包含的角色列表
class GetUserRolesByIdView(APIView):
    def get(self,request):
        # 获取当前用户id
        user_id = request.query_params.get('user_id')
        #查询用户角色根据用户id
        raw_data = SysRole.objects.raw("select sr.id as id,sr.name as name,sr.code as code,sr.create_time  as create_time,sr.update_time as update_time ,sr.remark as remark  from sys_user_role sur   inner join sys_role sr ON sur.role_id = sr.id  WHERE sur.user_id  = %s",[user_id])
        roleList = SysRoleSerializer(raw_data,many=True).data
        return Response_200(data={"roleList":roleList})
    