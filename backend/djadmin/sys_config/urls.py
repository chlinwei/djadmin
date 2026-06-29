from django.urls import path, include
from rest_framework.routers import DefaultRouter
from sys_config import views

router = DefaultRouter()
router.register(r'configs', views.SysConfigViewSet, basename='sys-config')

urlpatterns = [
    path('', include(router.urls)),
]
