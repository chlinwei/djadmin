from django.urls import path,include
from rest_framework.routers import DefaultRouter
from .views import *
router = DefaultRouter()
router.register(r'usercenter',UserCenterManage,basename="usercenter")
router.register(r'users',UserManage,basename="users")
urlpatterns = [
    path('login',LoginView.as_view(),name='login'),
    path('', include(router.urls)),
    path('changeAvatar',ChangeAvatarView.as_view(),name="ChangeAvatarView"),

]
