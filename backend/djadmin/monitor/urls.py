from django.urls import include, path
from rest_framework.routers import DefaultRouter

from .views import (
    AlertRuleDeployHistoryViewSet,
    AlertRuleGroupViewSet,
    AlertRuleViewSet,
    MonitorTargetInstallHistoryViewSet,
    MonitorViewSet,
    SoftwarePackageViewSet,
)

router = DefaultRouter()
router.register(r'targets', MonitorViewSet, basename='monitor-targets')
router.register(r'packages', SoftwarePackageViewSet, basename='monitor-packages')
router.register(r'install-histories', MonitorTargetInstallHistoryViewSet, basename='monitor-install-histories')
router.register(r'alert-rules', AlertRuleViewSet, basename='monitor-alert-rules')
router.register(r'alert-rule-groups', AlertRuleGroupViewSet, basename='monitor-alert-rule-groups')
router.register(r'alert-rule-deploy-histories', AlertRuleDeployHistoryViewSet, basename='monitor-alert-rule-deploy-histories')

urlpatterns = [
    path('', include(router.urls)),
    path('summary/', MonitorViewSet.as_view({'get': 'summary'})),
]
