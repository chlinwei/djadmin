from django.urls import path,include
from rest_framework.routers import DefaultRouter
from .views import *
router = DefaultRouter()
router.register(r'credentials',CredentialManage,basename="credentials")
router.register(r'applications',ApplicationManage,basename="applications")

urlpatterns = [
    path('', include(router.urls)),
]

