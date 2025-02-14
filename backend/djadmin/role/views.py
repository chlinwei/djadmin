from django.shortcuts import render
from rest_framework.views import APIView
from role.models import SysRole
from user.utils import getCurrentUser
from django.http import JsonResponse
from .models import SysRole
from .serializer import SysRoleSerializer
# Create your views here.



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