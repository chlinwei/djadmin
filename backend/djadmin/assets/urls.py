from django.urls import path,include
from rest_framework.routers import DefaultRouter
from .views import *
router = DefaultRouter()
router.register(r'credentials',CredentialManage,basename="credentials")
router.register(r'applications',ApplicationManage,basename="applications")
router.register(r'business-systems', BusinessSystemManage, basename="business-systems")
router.register(r'cluster-profiles', ClusterProfileManage, basename="cluster-profiles")
router.register(r'application-services', ApplicationServiceManage, basename="application-services")
router.register(r'application-versions', ApplicationVersionManage, basename="application-versions")
router.register(r'application-deployment-templates', ApplicationDeploymentTemplateManage, basename="application-deployment-templates")
router.register(r'application-deployments', ApplicationDeploymentManage, basename="application-deployments")
router.register(r'host-groups',HostGroupManage,basename="host-groups")
router.register(r'hosts',HostManage,basename="hosts")

urlpatterns = [
    path('', include(router.urls)),
]

