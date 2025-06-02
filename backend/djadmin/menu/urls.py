from django.urls import path,include
from rest_framework.routers import DefaultRouter
from .views import *
router = DefaultRouter()
router.register(r'menus',MenuManage,basename="menus")

urlpatterns = [
    path('', include(router.urls)),
]

