from django_filters.rest_framework import DjangoFilterBackend
from rest_framework.decorators import action
from rest_framework.filters import OrderingFilter, SearchFilter
from rest_framework.mixins import CreateModelMixin, ListModelMixin, RetrieveModelMixin, UpdateModelMixin
from rest_framework.viewsets import GenericViewSet

from djadmin.utils import Response_200
from djadmin.utils import CustomPagination
from menu.permisssion import CustomMenuPermission

from .models import MonitorTarget
from .prometheus_api import api_get, get_prometheus_base_url
from .serializer import MonitorTargetSerializer


class MonitorViewSet(
    GenericViewSet,
    ListModelMixin,
    RetrieveModelMixin,
    CreateModelMixin,
    UpdateModelMixin,
):
    queryset = MonitorTarget.objects.select_related('host').all()
    serializer_class = MonitorTargetSerializer
    pagination_class = CustomPagination
    filter_backends = (OrderingFilter, DjangoFilterBackend, SearchFilter)
    filterset_fields = ['exporter_type', 'managed_enabled', 'install_status', 'last_scrape_status']
    search_fields = ['host__instance_name', 'host__ip', 'exporter_type']
    ordering_fields = ['id', 'create_time', 'update_time', 'exporter_port']
    lookup_field = 'id'
    permission_classes = [CustomMenuPermission]
    action_perms_map = {
        'list': 'monitor:view',
        'retrieve': 'monitor:view',
        'create': 'monitor:view',
        'partial_update': 'monitor:view',
        'perform_update': 'monitor:view',
        'summary': 'monitor:view',
        'prometheus_targets': 'monitor:view',
        'prometheus_alerts': 'monitor:view',
        'prometheus_overview': 'monitor:view',
    }

    def list(self, request, *args, **kwargs):
        queryset = self.filter_queryset(self.get_queryset())
        page = self.paginate_queryset(queryset)
        serializer = self.get_serializer(page if page is not None else queryset, many=True)
        if page is not None:
            return self.get_paginated_response(serializer.data)
        return Response_200(data=serializer.data)

    def retrieve(self, request, *args, **kwargs):
        serializer = self.get_serializer(self.get_object())
        return Response_200(data=serializer.data)

    def create(self, request, *args, **kwargs):
        serializer = self.get_serializer(data=request.data)
        serializer.is_valid(raise_exception=True)
        serializer.save()
        return Response_200(data=serializer.data)

    def partial_update(self, request, *args, **kwargs):
        instance = self.get_object()
        serializer = self.get_serializer(instance, data=request.data, partial=True)
        serializer.is_valid(raise_exception=True)
        serializer.save()
        return Response_200(data=serializer.data)

    @action(detail=False, methods=['get'], url_path='summary')
    def summary(self, request):
        queryset = self.get_queryset()
        total_targets = queryset.count()
        managed_enabled = queryset.filter(managed_enabled=True).count()
        install_success = queryset.filter(install_status=MonitorTarget.InstallStatus.SUCCESS).count()
        scrape_up = queryset.filter(last_scrape_status=MonitorTarget.ScrapeStatus.UP).count()
        return Response_200(data={
            'module': 'monitor',
            'name': '智能监控',
            'status': 'ready',
            'message': '智能监控模块已就绪，可在此扩展告警、巡检与AI分析能力。',
            'targets': {
                'total': total_targets,
                'managed_enabled': managed_enabled,
                'install_success': install_success,
                'scrape_up': scrape_up,
            },
        })

    @action(detail=False, methods=['get'], url_path='prometheus/targets')
    def prometheus_targets(self, request):
        response = api_get('/api/v1/targets', params={'state': 'active'})
        if not response.get('ok'):
            return Response_200(data={
                'status': 'error',
                'prometheus_base_url': get_prometheus_base_url(),
                'error': response.get('error') or 'query prometheus targets failed',
                'results': [],
            })

        data = response.get('data') or {}
        active_targets = data.get('activeTargets') if isinstance(data, dict) else []
        rows = []
        for item in (active_targets or []):
            labels = item.get('labels') if isinstance(item.get('labels'), dict) else {}
            rows.append({
                'scrape_pool': item.get('scrapePool') or '',
                'health': item.get('health') or 'unknown',
                'job': labels.get('job') or '',
                'instance': labels.get('instance') or '',
                'last_error': item.get('lastError') or '',
                'last_scrape': item.get('lastScrape') or '',
                'scrape_url': item.get('scrapeUrl') or '',
            })

        return Response_200(data={
            'status': 'success',
            'prometheus_base_url': get_prometheus_base_url(),
            'count': len(rows),
            'results': rows,
            'warnings': response.get('warnings') or [],
        })

    @action(detail=False, methods=['get'], url_path='prometheus/alerts')
    def prometheus_alerts(self, request):
        response = api_get('/api/v1/alerts')
        if not response.get('ok'):
            return Response_200(data={
                'status': 'error',
                'prometheus_base_url': get_prometheus_base_url(),
                'error': response.get('error') or 'query prometheus alerts failed',
                'results': [],
            })

        data = response.get('data') or {}
        alerts = data.get('alerts') if isinstance(data, dict) else []
        firing_count = 0
        resolved_count = 0
        rows = []
        for item in (alerts or []):
            status_obj = item.get('status') if isinstance(item.get('status'), dict) else {}
            state = str(status_obj.get('state') or '').lower()
            if state == 'firing':
                firing_count += 1
            elif state == 'resolved':
                resolved_count += 1
            labels = item.get('labels') if isinstance(item.get('labels'), dict) else {}
            annotations = item.get('annotations') if isinstance(item.get('annotations'), dict) else {}
            rows.append({
                'name': labels.get('alertname') or '',
                'severity': labels.get('severity') or '',
                'state': state or 'unknown',
                'instance': labels.get('instance') or '',
                'summary': annotations.get('summary') or annotations.get('description') or '',
                'active_at': item.get('activeAt') or '',
                'value': item.get('value') or '',
            })

        return Response_200(data={
            'status': 'success',
            'prometheus_base_url': get_prometheus_base_url(),
            'count': len(rows),
            'firing_count': firing_count,
            'resolved_count': resolved_count,
            'results': rows,
            'warnings': response.get('warnings') or [],
        })

    @action(detail=False, methods=['get'], url_path='prometheus/overview')
    def prometheus_overview(self, request):
        response = api_get('/api/v1/targets', params={'state': 'active'})
        if not response.get('ok'):
            return Response_200(data={
                'status': 'error',
                'prometheus_base_url': get_prometheus_base_url(),
                'error': response.get('error') or 'query prometheus overview failed',
            })

        data = response.get('data') or {}
        active_targets = data.get('activeTargets') if isinstance(data, dict) else []
        total_targets = len(active_targets or [])
        up_targets = sum(1 for item in (active_targets or []) if str(item.get('health') or '').lower() == 'up')
        down_targets = total_targets - up_targets

        return Response_200(data={
            'status': 'success',
            'prometheus_base_url': get_prometheus_base_url(),
            'targets': {
                'total': total_targets,
                'up': up_targets,
                'down': down_targets,
            },
            'warnings': response.get('warnings') or [],
        })
