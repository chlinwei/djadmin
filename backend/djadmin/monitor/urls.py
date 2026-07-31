from django.urls import include, path, re_path
from rest_framework.routers import DefaultRouter

from .views import (
    AlertHistoryViewSet,
    AlertMediaViewSet,
    AlertRouteViewSet,
    MonitorTargetInstallHistoryViewSet,
    MonitorViewSet,
    SoftwarePackageViewSet,
)

router = DefaultRouter()
router.register(r'targets', MonitorViewSet, basename='monitor-targets')
router.register(r'packages', SoftwarePackageViewSet, basename='monitor-packages')
router.register(r'install-histories', MonitorTargetInstallHistoryViewSet, basename='monitor-install-histories')
router.register(r'alert-histories', AlertHistoryViewSet, basename='monitor-alert-histories')
router.register(r'media', AlertMediaViewSet, basename='monitor-alert-media')
router.register(r'alert-routes', AlertRouteViewSet, basename='monitor-alert-routes')

urlpatterns = [
    # Prometheus 只读代理：codemirror-promql 会请求 <proxy>/api/v1/*（无结尾斜杠）。
    # DRF 路由默认强制结尾斜杠，导致无斜杠请求 404；跨域时浏览器会把无 CORS 头的
    # 404 误报为 “CORS 错误”。这里显式注册正则路由，保证有/无斜杠都能命中。
    re_path(
        r'^targets/prometheus/proxy/(?P<api_path>.+)$',
        MonitorViewSet.as_view({'get': 'prometheus_proxy', 'post': 'prometheus_proxy'}),
    ),
    path('', include(router.urls)),
    path('summary/', MonitorViewSet.as_view({'get': 'summary'})),
]
