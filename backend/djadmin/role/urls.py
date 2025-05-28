from django.urls import path
from .views import *
urlpatterns = [
    path('roles/getCurrentUserRoleList/',currentUserRoleListView.as_view(),name='getCurrentUserRoleListView'),
    path('roles/getUserRoleList/',GetUserRolesByIdView.as_view(),name='getUserRoleList'),
    # # 列表，新增
    path('roles/',RoleListCreate.as_view(),name='role-manage'),
    # retrive,udpate,delete
    path('roles/<int:id>/',RoleRetrieveUpdateDestroyAPIView.as_view(),name='role-manage-detail'),
]