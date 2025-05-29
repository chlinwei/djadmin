from django.urls import path
from .views import *
urlpatterns = [
    path('roles/getCurrentUserRoleList/',currentUserRoleListView.as_view(),name='getCurrentUserRoleListView'),
    path('roles/getUserRoleList/',GetUserRolesByIdView.as_view(),name='getUserRoleList'),
    # # 列表，新增
    path('roles/',RoleListCreate.as_view(),name='role-manage'),
    # retrive,udpate
    path('roles/<int:id>/',RoleRetrieveUpdateAPIView.as_view(),name='role-manage-detail'),
    # 批量删除角色
    path('roles/batch-delete/',RoleBatchDeleteAPI.as_view(),name='role-manage-batch-delete'),
]