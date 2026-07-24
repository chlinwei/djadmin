from django.urls import include, path
from rest_framework.routers import DefaultRouter

from .views import MonitorTargetInstallHistoryViewSet, MonitorViewSet, SoftwarePackageViewSet

router = DefaultRouter()
router.register(r'targets', MonitorViewSet, basename='monitor-targets')
router.register(r'packages', SoftwarePackageViewSet, basename='monitor-packages')
router.register(r'install-histories', MonitorTargetInstallHistoryViewSet, basename='monitor-install-histories')

urlpatterns = [
    path('', include(router.urls)),
    path('summary/', MonitorViewSet.as_view({'get': 'summary'})),
]
