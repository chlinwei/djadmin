from django.urls import path,include
from .views import *
urlpatterns = [
    path('user',UserListOrCreateView.as_view(),name='listOrcreate'),
    path('test',TestView.as_view(),name='test'),
    path('login',LoginView.as_view(),name='login'),
    path('updateUserInfo',UpdateUserInfoView.as_view(),name='updateUserInfo'),
    path('updateUserPassword',UpdateUserPasswordView.as_view(),name='updateUserPassword'),
    path('changeAvatar',ChangeAvatarView.as_view(),name="ChangeAvatarView"),
    path('user/changeStatus',ChangeStatusView.as_view(),name="user/ChangeStatusView")
]
