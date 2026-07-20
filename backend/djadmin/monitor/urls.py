from django.urls import include, path
from rest_framework.routers import DefaultRouter

from .views import MonitorViewSet, SoftwarePackageViewSet

router = DefaultRouter()
router.register(r'targets', MonitorViewSet, basename='monitor-targets')
router.register(r'packages', SoftwarePackageViewSet, basename='monitor-packages')

urlpatterns = [
    path('', include(router.urls)),
    path('summary/', MonitorViewSet.as_view({'get': 'summary'})),
]
