from role.models import SysRole
from user.utils import getCurrentUser
from django.http import JsonResponse
from .models import SysRole
from .serializer import SysRoleSerializer
# Create your views here.
from user.models import SysUserRole
from menu.models import SysRoleMenu
from djadmin.utils import CustomPagination
from djadmin.utils import Response_200,Response_error
from djadmin.errordict import RoleError
from django_filters import rest_framework as filters
from rest_framework.mixins import CreateModelMixin,UpdateModelMixin,RetrieveModelMixin
from rest_framework.mixins import ListModelMixin
from rest_framework.viewsets import GenericViewSet
from rest_framework.decorators import action
from .filters import SysRoleFilter
from rest_framework.filters import OrderingFilter,SearchFilter
from menu.permisssion import CustomMenuPermission
#detail,update,crate,list
class RoleManage(GenericViewSet,ListModelMixin,CreateModelMixin,RetrieveModelMixin,UpdateModelMixin):
    queryset = SysRole.objects.all()
    serializer_class = SysRoleSerializer
    pagination_class = CustomPagination
    # filterset_class = SysRoleFilter
    filter_backends = (OrderingFilter,filters.DjangoFilterBackend,SearchFilter)
    search_fields = ['name', 'code','remark'] 
    ordering_fields = [ 'name','create_time'] 
    lookup_field = 'id'
    permission_classes = [CustomMenuPermission]
    action_perms_map = {
        'list': 'system:roles:view',
        'batch-delete': 'system:roles:delete',
        'partial_update': 'system:roles:update',
        'perform_update': 'system:roles:update',
        'create': 'system:roles:create',
        'retrieve': 'system:roles:view',
        
    }
    # 获取当前用户所包含的角色
    @action(detail=False,methods=['get'],url_path='getCurrentUserRoleList')
    def getCurrentUserRoleList(self,request):
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
    # 批量删除角色
    @action(detail=False,methods=['delete'],url_path='batch-delete')
    def batchDelete(self,request):
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
