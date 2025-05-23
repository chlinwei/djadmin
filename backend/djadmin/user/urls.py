from django.urls import path,include
from .views import UserListOrCreateView,TestView,LoginView,UpdateUserInfoView,UpdateUserPasswordView,ChangeAvatarView
urlpatterns = [
    path('user',UserListOrCreateView.as_view(),name='listOrcreate'),
    path('test',TestView.as_view(),name='test'),
    path('login',LoginView.as_view(),name='login'),
    path('updateUserInfo',UpdateUserInfoView.as_view(),name='updateUserInfo'),
    path('updateUserPassword',UpdateUserPasswordView.as_view(),name='updateUserPassword'),
    path('changeAvatar',ChangeAvatarView.as_view(),name="ChangeAvatarView")
]
