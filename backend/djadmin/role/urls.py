from django.urls import path,include
from .views import *
from rest_framework.routers import DefaultRouter
router = DefaultRouter()
router.register(r'roles',RoleManage,basename="roles")
urlpatterns = [
    path('', include(router.urls)),
]