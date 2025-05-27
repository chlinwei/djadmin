from django.urls import path,include
from .views import *
urlpatterns = [
    path('userList',UserListView.as_view(),name='listOrcreate'),
    path('test',TestView.as_view(),name='test'),
    path('login',LoginView.as_view(),name='login'),
    path('updateUserInfo',UpdateUserInfoView.as_view(),name='updateUserInfo'),
    path('updateUserPassword',UpdateUserPasswordView.as_view(),name='updateUserPassword'),
    path('changeAvatar',ChangeAvatarView.as_view(),name="ChangeAvatarView"),
    path('user/changeStatus',ChangeStatusView.as_view(),name="user/ChangeStatusView"),
    
    path('users/',UserManageView.as_view(),name="user-create"),
    path('users/<int:id>/',UserManageView.as_view(),name="user-detail"),
    path('users/<int:id>/',UserManageView.as_view(),name="updateUser"),
    path('users/<int:id>/',UserManageView.as_view(),name="deleteUser"),
    # 检查用户是否存在
    path('users/CheckUsername/',CheckUsername.as_view(),name="CheckUsername"),

    #批量删除用户
    path('users/batch-delete/',UserBatchDeleteAPI.as_view(),name="user-batch-delete"),
    #重置密码
    path('users/resetPwd/',ResetPasswdView.as_view(),name="user-resetPwd"),
]
