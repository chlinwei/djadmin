from django.urls import include, path
from rest_framework.routers import DefaultRouter

from .views import MonitorViewSet

router = DefaultRouter()
router.register(r'targets', MonitorViewSet, basename='monitor-targets')

urlpatterns = [
    path('', include(router.urls)),
    path('summary/', MonitorViewSet.as_view({'get': 'summary'})),
]
