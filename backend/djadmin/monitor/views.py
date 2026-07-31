from django_filters.rest_framework import DjangoFilterBackend
from django.db.models import Q
from rest_framework.decorators import action
from rest_framework.filters import OrderingFilter, SearchFilter
from rest_framework.mixins import CreateModelMixin, DestroyModelMixin, ListModelMixin, RetrieveModelMixin, UpdateModelMixin
from rest_framework.viewsets import GenericViewSet

import hashlib
import io
import os
import re
import urllib.error
import urllib.request

from django.core.files.base import ContentFile
from django.http import JsonResponse
from django.utils import timezone
from rest_framework.permissions import AllowAny

from djadmin.utils import Response_200, Response_error_str
from djadmin.utils import CustomPagination
from menu.permisssion import CustomMenuPermission

from .alert_history import ingest_alert_webhook_alerts
from .models import (
    AlertHistory,
    AlertMedia,
    AlertRoute,
    MonitorTarget,
    MonitorTargetInstallHistory,
    SoftwarePackage,
    build_node_exporter_official_url,
)
from .prometheus_api import (
    api_get,
    get_prometheus_base_url,
)
from .serializer import (
    AlertHistorySerializer,
    AlertMediaSerializer,
    AlertRouteSerializer,
    MonitorTargetInstallHistorySerializer,
    MonitorTargetSerializer,
    SoftwarePackageSerializer,
)

# 校验用户传入的目标版本号，防止拼接进下载 URL 时被注入路径穿越等非法字符
NODE_EXPORTER_VERSION_RE = re.compile(r'^\d+(\.\d+){1,3}$')# 官方 tarball 命名规则：node_exporter-<version>.<os>-<arch>.tar.gz，用于行内上传时解析版本/校验架构
NODE_EXPORTER_FILENAME_RE = re.compile(r'^node_exporter-([^.]+)\.([a-z0-9]+)-([a-z0-9]+)\.tar\.gz$', re.IGNORECASE)# 官方软件包体积上限（字节），避免异常响应把磁盘写满
MAX_OFFICIAL_PACKAGE_SIZE = 200 * 1024 * 1024


class MonitorViewSet(
    GenericViewSet,
    ListModelMixin,
    RetrieveModelMixin,
    CreateModelMixin,
    UpdateModelMixin,
    DestroyModelMixin,
):
    queryset = MonitorTarget.objects.select_related('host').all()
    serializer_class = MonitorTargetSerializer
    pagination_class = CustomPagination
    filter_backends = (OrderingFilter, DjangoFilterBackend, SearchFilter)
    filterset_fields = ['exporter_type', 'managed_enabled', 'install_status', 'last_scrape_status']
    search_fields = ['host__instance_name', 'host__ip', 'exporter_type']
    ordering_fields = ['id', 'create_time', 'update_time']
    lookup_field = 'id'
    permission_classes = [CustomMenuPermission]
    action_perms_map = {
        'list': 'monitor:view',
        'retrieve': 'monitor:view',
        'create': 'monitor:view',
        'partial_update': 'monitor:view',
        'perform_update': 'monitor:view',
        'destroy': 'monitor:view',
        'retry': 'monitor:view',
        'cancel_pending': 'monitor:view',
        'check_service_status': 'monitor:view',
        'start_service': 'monitor:view',
        'stop_service': 'monitor:view',
        'summary': 'monitor:view',
        'prometheus_targets': 'monitor:view',
        'prometheus_alerts': 'monitor:view',
        'prometheus_rules': 'monitor:view',
        'prometheus_overview': 'monitor:view',
        'prometheus_http_sd': None,
        'alert_webhook': None,
        'prometheus_tsdb_status': 'monitor:view',
        'prometheus_config': 'monitor:view',
        'prometheus_flags': 'monitor:view',
        'prometheus_query': 'monitor:view',
        'prometheus_query_range': 'monitor:view',
        'prometheus_proxy': 'monitor:view',
    }

    @staticmethod
    def _parse_port_value(raw_value):
        try:
            port = int(raw_value)
        except (TypeError, ValueError):
            return None
        if 1 <= port <= 65535:
            return port
        return None

    @classmethod
    def _resolve_target_scrape_port(cls, target):
        structured_port = cls._parse_port_value(getattr(target, 'scrape_port', None))
        if structured_port is not None:
            return structured_port

        labels = getattr(target, 'labels', None)
        if isinstance(labels, dict):
            for key in ('scrape_port', 'exporter_port', 'port'):
                if key in labels:
                    port = cls._parse_port_value(labels.get(key))
                    if port is not None:
                        return port

        # 未配置端口时不做猜测，避免不同 exporter 端口不一致导致误抓。
        return None

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

    def destroy(self, request, *args, **kwargs):
        """删除监控目标记录。加两个前置校验，避免删掉一个仍在被期望管理/卸载任务还没跑完的记录：
        1. managed_enabled 必须已经是 False——想删除必须先关闭（关闭会自动下发卸载），不允许直接删掉
           一个仍处于“期望被纳管”状态的目标，防止误删导致该主机的这项监控从此失踪却没人知道要不要重装。
        2. install_status 不能是 pending——卸载任务可能还在跑，此时删除记录会让用户无法再追踪这次卸载
           是否真的成功（纳管目标列表里再也看不到这一行了），等任务跑完出结果后再删除更安全。
        """
        instance = self.get_object()
        if instance.managed_enabled:
            return Response_error_str('该监控项仍处于纳管开启状态，请先关闭（会自动下发卸载）后再删除', code=400)
        if instance.install_status == MonitorTarget.InstallStatus.PENDING:
            return Response_error_str('卸载任务尚未结束，请等待完成后再删除', code=400)
        deleted_id = instance.id
        instance.delete()
        return Response_200(data={'id': deleted_id})

    @action(detail=True, methods=['post'], url_path='retry')
    def retry(self, request, id=None):
        """人工重试安装/卸载：任一失败后由用户手动重新下发同类型任务。"""
        target = self.get_object()
        host = target.host
        if host is None:
            return Response_error_str('监控目标未关联主机，无法重试', code=400)

        # 人工触发视为新一轮操作周期，重置历史重试计数。
        # 注意：不要在这里先把 install_status 写成 pending，否则 dispatch_* 会认为
        # “已有 pending 任务”直接短路返回，导致任务永远停在 pending。
        target.retry_count = 0
        target.install_message = '人工触发重试'
        target.save(update_fields=['retry_count', 'install_message', 'update_time'])

        from assets.views import dispatch_exporter_install_job, dispatch_exporter_uninstall_job

        if target.managed_enabled:
            dispatch_exporter_install_job(host, target, manual=True, sync_execute=True)
        else:
            dispatch_exporter_uninstall_job(host, target, manual=True, sync_execute=True)

        target.refresh_from_db()
        serializer = self.get_serializer(target)
        return Response_200(data=serializer.data)

    @action(detail=True, methods=['post'], url_path='cancel')
    def cancel_pending(self, request, id=None):
        """人工取消 pending 中的安装/卸载任务。

        说明：当前执行链路是“本地线程 + agent gRPC 同步等待”，没有远端强杀机制。
        这里的取消语义是：先把本地状态从 pending 解锁，写一条 cancelled 历史，
        允许用户立即重新下发；若后台线程稍后返回，会由执行链路上的取消检查避免反写覆盖。
        """
        target = self.get_object()

        latest_history = MonitorTargetInstallHistory.objects.filter(
            target_id=target.id,
        ).order_by('-id').first()
        latest_status = str(getattr(latest_history, 'status', '') or '').lower()

        # 与自动化任务记录保持一致：仅终态（success/failed/cancelled）不可取消；
        # pending/running 均允许取消。
        if latest_status not in {
            MonitorTargetInstallHistory.Status.PENDING,
            MonitorTargetInstallHistory.Status.RUNNING,
        }:
            if str(target.install_status or '').lower() != MonitorTarget.InstallStatus.PENDING:
                return Response_error_str('当前任务已结束，无需取消', code=400)

        cancel_message = '已人工取消 pending 任务'
        now_ts = timezone.now()

        if latest_history is not None:
            latest_history.status = MonitorTargetInstallHistory.Status.CANCELLED
            latest_history.summary_message = cancel_message
            latest_history.error_message_snapshot = cancel_message
            latest_history.end_time = now_ts
            if latest_history.start_time is not None:
                latest_history.duration_seconds = (now_ts - latest_history.start_time).total_seconds()
            latest_history.save(update_fields=[
                'status', 'summary_message', 'error_message_snapshot',
                'end_time', 'duration_seconds', 'update_time',
            ])

        target.install_status = MonitorTarget.InstallStatus.FAILED
        target.install_message = cancel_message
        target.save(update_fields=['install_status', 'install_message', 'update_time'])

        target.refresh_from_db()
        serializer = self.get_serializer(target)
        return Response_200(data=serializer.data)

    @action(detail=True, methods=['post'], url_path='check-service-status')
    def check_service_status(self, request, id=None):
        """按需查询一次 exporter 的 systemd 运行状态（sudo systemctl status）。

        走 assets.AgentJob + RabbitMQ 异步下发通道（非实时任务标准通道），立即返回 job_id，
        不等待结果、不写 MonitorTarget 任何字段——前端需自行轮询
        GET /api/agent/jobs/query?job_id=... 获取 status/exit_code/stdout/stderr。
        """
        target = self.get_object()
        host = target.host
        if host is None:
            return Response_error_str('监控目标未关联主机，无法查询运行状态', code=400)

        from assets.views import dispatch_check_exporter_status_job
        try:
            job = dispatch_check_exporter_status_job(host, target)
        except RuntimeError as exc:
            return Response_error_str(str(exc), code=400)

        return Response_200(data={'job_id': job.job_id})

    @action(detail=True, methods=['post'], url_path='start-service')
    def start_service(self, request, id=None):
        """下发一次"启动 exporter 服务"作业（sudo systemctl start），走 AgentJob + RabbitMQ 异步通道。"""
        target = self.get_object()
        host = target.host
        if host is None:
            return Response_error_str('监控目标未关联主机，无法启动服务', code=400)

        from assets.views import dispatch_start_exporter_job
        try:
            job = dispatch_start_exporter_job(host, target)
        except RuntimeError as exc:
            return Response_error_str(str(exc), code=400)

        return Response_200(data={'job_id': job.job_id})

    @action(detail=True, methods=['post'], url_path='stop-service')
    def stop_service(self, request, id=None):
        """下发一次"停止 exporter 服务"作业（sudo systemctl stop），走 AgentJob + RabbitMQ 异步通道。"""
        target = self.get_object()
        host = target.host
        if host is None:
            return Response_error_str('监控目标未关联主机，无法停止服务', code=400)

        from assets.views import dispatch_stop_exporter_job
        try:
            job = dispatch_stop_exporter_job(host, target)
        except RuntimeError as exc:
            return Response_error_str(str(exc), code=400)

        return Response_200(data={'job_id': job.job_id})

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
            # 注意：/api/v1/alerts 返回的 state 是顶层字段，不是嵌套在 status 对象里
            # （之前误照搬 Alertmanager v2 查询接口的结构，两者格式不一样，导致 state 永远读
            # 不到值、前端一直显示 unknown）。
            state = str(item.get('state') or '').lower()
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

    @action(detail=False, methods=['get'], url_path='prometheus/rules')
    def prometheus_rules(self, request):
        """只读展示 Prometheus 侧当前已生效的告警/记录规则（/api/v1/rules），
        平台不再本地维护规则模型，避免出现“djadmin 规则”和“Prometheus 实际生效规则”两份数据不一致。
        """
        response = api_get('/api/v1/rules')
        if not response.get('ok'):
            return Response_200(data={
                'status': 'error',
                'prometheus_base_url': get_prometheus_base_url(),
                'error': response.get('error') or 'query prometheus rules failed',
                'group_count': 0,
                'rule_count': 0,
                'groups': [],
            })

        data = response.get('data') or {}
        raw_groups = data.get('groups') if isinstance(data, dict) else []
        groups = []
        rule_count = 0
        for raw_group in (raw_groups or []):
            rows = []
            for raw_rule in (raw_group.get('rules') or []):
                rule_type = str(raw_rule.get('type') or '')
                labels = raw_rule.get('labels') if isinstance(raw_rule.get('labels'), dict) else {}
                annotations = raw_rule.get('annotations') if isinstance(raw_rule.get('annotations'), dict) else {}
                rows.append({
                    'type': rule_type,
                    'name': raw_rule.get('name') or '',
                    'query': raw_rule.get('query') or '',
                    'duration': raw_rule.get('duration'),
                    'keep_firing_for': raw_rule.get('keepFiringFor'),
                    'labels': labels,
                    'annotations': annotations,
                    'health': raw_rule.get('health') or '',
                    # state 仅 alerting 规则有意义（inactive/pending/firing），recording 规则该字段为空
                    'state': raw_rule.get('state') or '',
                    'last_error': raw_rule.get('lastError') or '',
                    'evaluation_time': raw_rule.get('evaluationTime'),
                    'last_evaluation': raw_rule.get('lastEvaluation') or '',
                    'active_alerts_count': len(raw_rule.get('alerts') or []) if rule_type == 'alerting' else None,
                })
                rule_count += 1
            groups.append({
                'name': raw_group.get('name') or '',
                'file': raw_group.get('file') or '',
                'interval': raw_group.get('interval'),
                'rules': rows,
            })

        return Response_200(data={
            'status': 'success',
            'prometheus_base_url': get_prometheus_base_url(),
            'group_count': len(groups),
            'rule_count': rule_count,
            'groups': groups,
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

    @action(detail=False, methods=['get'], url_path='prometheus/tsdb-status')
    def prometheus_tsdb_status(self, request):
        """查询 Prometheus TSDB 运行状态（/api/v1/status/tsdb）。"""
        response = api_get('/api/v1/status/tsdb')
        if not response.get('ok'):
            return Response_200(data={
                'status': 'error',
                'prometheus_base_url': get_prometheus_base_url(),
                'error': response.get('error') or 'query prometheus tsdb status failed',
                'result': {},
            })

        data = response.get('data') if isinstance(response.get('data'), dict) else {}
        return Response_200(data={
            'status': 'success',
            'prometheus_base_url': get_prometheus_base_url(),
            'result': data or {},
            'warnings': response.get('warnings') or [],
        })

    @action(detail=False, methods=['get'], url_path='prometheus/config')
    def prometheus_config(self, request):
        """查询 Prometheus 当前生效配置（/api/v1/status/config）。"""
        response = api_get('/api/v1/status/config')
        if not response.get('ok'):
            return Response_200(data={
                'status': 'error',
                'prometheus_base_url': get_prometheus_base_url(),
                'error': response.get('error') or 'query prometheus config failed',
                'result': {},
                'config_yaml': '',
            })

        data = response.get('data') if isinstance(response.get('data'), dict) else {}
        yaml_text = str(data.get('yaml') or '') if isinstance(data, dict) else ''
        return Response_200(data={
            'status': 'success',
            'prometheus_base_url': get_prometheus_base_url(),
            'result': data or {},
            'config_yaml': yaml_text,
            'warnings': response.get('warnings') or [],
        })

    @action(detail=False, methods=['get'], url_path='prometheus/flags')
    def prometheus_flags(self, request):
        """查询 Prometheus 启动参数（/api/v1/status/flags）。"""
        response = api_get('/api/v1/status/flags')
        if not response.get('ok'):
            return Response_200(data={
                'status': 'error',
                'prometheus_base_url': get_prometheus_base_url(),
                'error': response.get('error') or 'query prometheus flags failed',
                'result': {},
            })

        data = response.get('data') if isinstance(response.get('data'), dict) else {}
        return Response_200(data={
            'status': 'success',
            'prometheus_base_url': get_prometheus_base_url(),
            'result': data or {},
            'warnings': response.get('warnings') or [],
        })

    @action(detail=False, methods=['get'], url_path='prometheus/query')
    def prometheus_query(self, request):
        """PromQL 即时查询代理（后端同域转发，避免前端直连 Prometheus 的跨域与鉴权问题）。"""
        query = str(request.query_params.get('query') or '').strip()
        if not query:
            return Response_error_str('query 参数不能为空', code=400)

        query_time = str(request.query_params.get('time') or '').strip()
        timeout = str(request.query_params.get('timeout') or '').strip()
        params = {'query': query}
        if query_time:
            params['time'] = query_time
        if timeout:
            params['timeout'] = timeout

        response = api_get('/api/v1/query', params=params)
        if not response.get('ok'):
            return Response_200(data={
                'status': 'error',
                'error': response.get('error') or 'query prometheus failed',
                'error_type': response.get('errorType') or '',
                'result_type': '',
                'result': [],
            })

        data = response.get('data')
        data_dict = data if isinstance(data, dict) else {}
        return Response_200(data={
            'status': 'success',
            'result_type': data_dict.get('resultType') or '',
            'result': data_dict.get('result') if isinstance(data_dict.get('result'), list) else [],
            'warnings': response.get('warnings') or [],
        })

    @action(detail=False, methods=['get'], url_path='prometheus/query-range')
    def prometheus_query_range(self, request):
        """PromQL 区间查询代理，参数与 Prometheus /api/v1/query_range 对齐。"""
        query = str(request.query_params.get('query') or '').strip()
        start = str(request.query_params.get('start') or '').strip()
        end = str(request.query_params.get('end') or '').strip()
        step = str(request.query_params.get('step') or '').strip()
        timeout = str(request.query_params.get('timeout') or '').strip()

        if not query:
            return Response_error_str('query 参数不能为空', code=400)
        if not start or not end or not step:
            return Response_error_str('start/end/step 参数不能为空', code=400)

        params = {
            'query': query,
            'start': start,
            'end': end,
            'step': step,
        }
        if timeout:
            params['timeout'] = timeout

        response = api_get('/api/v1/query_range', params=params)
        if not response.get('ok'):
            return Response_200(data={
                'status': 'error',
                'error': response.get('error') or 'query prometheus range failed',
                'error_type': response.get('errorType') or '',
                'result_type': '',
                'result': [],
            })

        data = response.get('data')
        data_dict = data if isinstance(data, dict) else {}
        return Response_200(data={
            'status': 'success',
            'result_type': data_dict.get('resultType') or '',
            'result': data_dict.get('result') if isinstance(data_dict.get('result'), list) else [],
            'warnings': response.get('warnings') or [],
        })

    @action(detail=False, methods=['get'], url_path=r'prometheus/proxy/(?P<api_path>.+)')
    def prometheus_proxy(self, request, api_path=None):
        """Prometheus 只读代理：供 codemirror-promql 远程补全等能力走同域请求。

        安全约束：仅允许转发 /api/v1/*，避免该接口被用于访问 Prometheus 非查询类路径。
        """
        normalized_path = f"/{str(api_path or '').lstrip('/')}"
        if not normalized_path.startswith('/api/v1/'):
            return JsonResponse(
                {'status': 'error', 'errorType': 'bad_data', 'error': 'only /api/v1/* is allowed'},
                status=400,
            )

        params = {key: value for key, value in request.query_params.items()}
        response = api_get(normalized_path, params=params)
        payload_data = response.get('data')
        return JsonResponse({
            'status': 'success' if response.get('ok') else 'error',
            'data': payload_data if payload_data is not None else {},
            'errorType': response.get('errorType') or '',
            'error': response.get('error') or '',
            'warnings': response.get('warnings') or [],
        })

    @action(
        detail=False,
        methods=['get'],
        url_path='prometheus/http-sd',
        permission_classes=[AllowAny],
        authentication_classes=[],
    )
    def prometheus_http_sd(self, request):
        # 内网 Prometheus service discovery 专用：该路径已在 JwtAuthenticationMiddleware 里
        # 走全局 ApiToken 认证（与 dj-agent 同一套），此处不再做额外 token 校验。
        queryset = (
            self.get_queryset()
            .filter(managed_enabled=True, install_status=MonitorTarget.InstallStatus.SUCCESS)
            .select_related('host')
        )

        results = []
        for target in queryset:
            host = getattr(target, 'host', None)
            host_ip = str(getattr(host, 'ip', '') or '').strip()
            if host is None or host_ip == '':
                continue

            scrape_port = self._resolve_target_scrape_port(target)
            if scrape_port is None:
                continue
            target_address = f'{host_ip}:{scrape_port}'
            exporter_type = str(getattr(target, 'exporter_type', '') or '').strip()
            host_id = getattr(host, 'id', '')
            host_name = str(getattr(host, 'instance_name', '') or '').strip()

            results.append({
                'targets': [target_address],
                'labels': {
                    'job': exporter_type or 'exporter',
                    '__meta_dj_exporter_type': exporter_type,
                    '__meta_dj_host_id': str(host_id),
                    '__meta_dj_instance_name': host_name,
                },
            })

        return JsonResponse(results, safe=False)

    @action(
        detail=False,
        methods=['post'],
        url_path='alert-webhook/api/v2/alerts',
        permission_classes=[AllowAny],
        authentication_classes=[],
    )
    def alert_webhook(self, request):
        """backend 替代 Alertmanager 接收 Prometheus notifier 推送的告警（Alertmanager v2 协议）。

        该路径已在 JwtAuthenticationMiddleware 里走全局 ApiToken 认证（与 dj-agent 同一套，
        Prometheus 侧用 alerting.alertmanagers[].authorization.credentials 下发 Bearer token），
        此处不再做额外 token 校验。
        """
        payload = request.data
        alerts = payload if isinstance(payload, list) else []
        result = ingest_alert_webhook_alerts(alerts)
        return JsonResponse({'status': 'success', **result})


class SoftwarePackageViewSet(
    GenericViewSet,
    ListModelMixin,
    CreateModelMixin,
    RetrieveModelMixin,
    UpdateModelMixin,
    DestroyModelMixin,
):
    """本地软件仓库：上传/列表/删除待下发的二进制包（当前用于 node_exporter）。"""

    queryset = SoftwarePackage.objects.all()
    serializer_class = SoftwarePackageSerializer
    pagination_class = CustomPagination
    filter_backends = (OrderingFilter, DjangoFilterBackend, SearchFilter)
    filterset_fields = ['name', 'version', 'os', 'arch', 'enabled']
    search_fields = ['name', 'version']
    ordering_fields = ['id', 'create_time', 'update_time', 'version']
    lookup_field = 'id'
    permission_classes = [CustomMenuPermission]
    action_perms_map = {
        'list': 'monitor:view',
        'retrieve': 'monitor:view',
        'create': 'monitor:view',
        'partial_update': 'monitor:view',
        'perform_update': 'monitor:view',
        'destroy': 'monitor:view',
        'upload_file': 'monitor:view',
        'sync_from_official': 'monitor:view',
    }

    def list(self, request, *args, **kwargs):
        queryset = self.filter_queryset(self.get_queryset())
        page = self.paginate_queryset(queryset)
        serializer = self.get_serializer(page if page is not None else queryset, many=True)
        if page is not None:
            return self.get_paginated_response(serializer.data)
        return Response_200(data=serializer.data)

    def create(self, request, *args, **kwargs):
        serializer = self.get_serializer(data=request.data)
        serializer.is_valid(raise_exception=True)
        serializer.save()
        return Response_200(data=serializer.data)

    def partial_update(self, request, *args, **kwargs):
        # 目前仅用于编辑自定义生命周期脚本/环境变量/启动参数这几项元数据，
        # 文件本身（sha256/size_bytes）仍走行内“上传”接口，避免和该接口职责重叠。
        instance = self.get_object()
        serializer = self.get_serializer(instance, data=request.data, partial=True)
        serializer.is_valid(raise_exception=True)
        serializer.save()
        return Response_200(data=serializer.data)

    def destroy(self, request, *args, **kwargs):
        instance = self.get_object()
        # 删除记录同时删除 media 中的物理文件，避免残留
        file_field = getattr(instance, 'file', None)
        if file_field and file_field.name:
            file_field.delete(save=False)
        instance.delete()
        return Response_200(data={'deleted': True})


    @action(detail=True, methods=['post'], url_path='upload')
    def upload_file(self, request, *args, **kwargs):
        """行内“上传”：为当前记录（固定 os/arch）替换软件包文件，version 按文件名自动识别并更新，
        避免通过顶部全局上传按钮重复创建新记录（现已改为默认预置固定行，只支持在行内更新）。"""
        instance = self.get_object()
        upload = request.FILES.get('file')
        if not upload:
            return Response_error_str('请提供上传文件', code=400)

        filename = str(getattr(upload, 'name', '') or '')
        match = NODE_EXPORTER_FILENAME_RE.match(filename)
        if not match:
            return Response_error_str('文件名需符合 node_exporter-<version>.<os>-<arch>.tar.gz 命名规范', code=400)
        version, os_name, arch = match.group(1), match.group(2).lower(), match.group(3).lower()
        # 行内上传固定对应当前记录的 os/arch，防止误传到错误架构导致 agent 下发时与实际机器不匹配
        if os_name != instance.os or arch != instance.arch:
            return Response_error_str(
                f'文件架构（{os_name}-{arch}）与当前记录（{instance.os}-{instance.arch}）不一致', code=400,
            )

        conflict = SoftwarePackage.objects.filter(
            name=instance.name, version=version, os=instance.os, arch=instance.arch,
        ).exclude(pk=instance.pk).exists()
        if conflict:
            return Response_error_str(f'版本 {version} 已存在同架构记录，请先删除或更换版本', code=400)

        hasher = hashlib.sha256()
        for chunk in upload.chunks():
            hasher.update(chunk)
        upload.seek(0)

        tarball_name = f'node_exporter-{version}.{instance.os}-{instance.arch}.tar.gz'
        instance.version = version
        instance.file.save(tarball_name, upload, save=False)
        instance.sha256 = hasher.hexdigest()
        instance.size_bytes = int(getattr(upload, 'size', 0) or 0)
        instance.save(update_fields=['version', 'file', 'sha256', 'size_bytes', 'update_time'])
        return Response_200(data=self.get_serializer(instance).data)

    @action(detail=True, methods=['post'], url_path='sync-official')
    def sync_from_official(self, request, *args, **kwargs):
        """点击“自动更新”：按官方 GitHub release 命名规则拼接下载地址并下载覆盖当前包，
        同时更新 version/sha256/size_bytes。仅支持 node_exporter（当前唯一的本地仓库品类）。"""
        instance = self.get_object()
        if instance.name != 'node_exporter':
            return Response_error_str('当前仅支持 node_exporter 自动更新', code=400)

        target_version = str(request.data.get('version') or instance.version or '').strip().lstrip('v')
        if not NODE_EXPORTER_VERSION_RE.match(target_version):
            return Response_error_str('版本号格式不正确，应类似 1.8.2', code=400)
        # 目标版本若已被同名 os/arch 的其他记录占用，提前拦截，避免落库时触发唯一约束报错
        conflict = SoftwarePackage.objects.filter(
            name=instance.name, version=target_version, os=instance.os, arch=instance.arch,
        ).exclude(pk=instance.pk).exists()
        if conflict:
            return Response_error_str(f'版本 {target_version} 已存在同架构记录，请先删除或更换版本', code=400)

        url = build_node_exporter_official_url(target_version, instance.os, instance.arch)
        try:
            req = urllib.request.Request(url, headers={'User-Agent': 'djadmin-monitor-sync/1.0'})
            with urllib.request.urlopen(req, timeout=30) as resp:
                hasher = hashlib.sha256()
                buf = io.BytesIO()
                total = 0
                while True:
                    chunk = resp.read(1024 * 1024)
                    if not chunk:
                        break
                    total += len(chunk)
                    if total > MAX_OFFICIAL_PACKAGE_SIZE:
                        raise ValueError('软件包体积超出限制')
                    hasher.update(chunk)
                    buf.write(chunk)
        except (urllib.error.URLError, urllib.error.HTTPError, ValueError) as exc:
            return Response_error_str(f'下载官方软件包失败：{exc}', code=400)

        tarball_name = f'node_exporter-{target_version}.{instance.os}-{instance.arch}.tar.gz'
        instance.version = target_version
        # 先删除旧文件再写入新内容，OverwriteStorage 已保证同名覆盖，这里显式 save 更新文件字段
        instance.file.save(tarball_name, ContentFile(buf.getvalue()), save=False)
        instance.sha256 = hasher.hexdigest()
        instance.size_bytes = total
        instance.save(update_fields=['version', 'file', 'sha256', 'size_bytes', 'update_time'])
        return Response_200(data=self.get_serializer(instance).data)


class MonitorTargetInstallHistoryViewSet(
    GenericViewSet,
    ListModelMixin,
    RetrieveModelMixin,
):
    queryset = MonitorTargetInstallHistory.objects.select_related('target', 'host', 'automation_job').all()
    serializer_class = MonitorTargetInstallHistorySerializer
    pagination_class = CustomPagination
    filter_backends = (OrderingFilter, DjangoFilterBackend, SearchFilter)
    filterset_fields = ['target_id', 'action', 'trigger_type', 'status']
    search_fields = ['host_name_snapshot', 'host_ip_snapshot', 'exporter_type_snapshot', 'summary_message']
    ordering_fields = ['id', 'create_time', 'update_time', 'start_time', 'end_time']
    lookup_field = 'id'
    permission_classes = [CustomMenuPermission]
    action_perms_map = {
        'list': 'monitor:view',
        'retrieve': 'monitor:view',
    }

    def get_queryset(self):
        queryset = super().get_queryset()
        query_target_id = str(self.request.query_params.get('target_id') or '').strip()  # type: ignore[union-attr]
        query_job_id = str(self.request.query_params.get('automation_job_id') or '').strip()  # type: ignore[union-attr]
        query_keyword = str(self.request.query_params.get('keyword') or '').strip()  # type: ignore[union-attr]
        query_start = str(self.request.query_params.get('start_time') or '').strip()  # type: ignore[union-attr]
        query_end = str(self.request.query_params.get('end_time') or '').strip()  # type: ignore[union-attr]

        if query_target_id.isdigit():
            queryset = queryset.filter(target_id=int(query_target_id))
        if query_job_id.isdigit():
            value = int(query_job_id)
            queryset = queryset.filter(
                Q(automation_job_id_snapshot=value) | Q(automation_job_id=value)
            )
        if query_keyword:
            queryset = queryset.filter(
                Q(host_name_snapshot__icontains=query_keyword)
                | Q(host_ip_snapshot__icontains=query_keyword)
                | Q(exporter_type_snapshot__icontains=query_keyword)
                | Q(summary_message__icontains=query_keyword)
            )
        if query_start:
            queryset = queryset.filter(create_time__gte=query_start)
        if query_end:
            queryset = queryset.filter(create_time__lte=query_end)
        return queryset

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


class AlertHistoryViewSet(
    GenericViewSet,
    ListModelMixin,
    RetrieveModelMixin,
):
    """历史告警只读查询：数据来源于 alert_webhook 接收 + 每日对账兜底写入的 AlertHistory。"""

    queryset = AlertHistory.objects.all()
    serializer_class = AlertHistorySerializer
    pagination_class = CustomPagination
    filter_backends = (OrderingFilter, DjangoFilterBackend, SearchFilter)
    filterset_fields = ['state', 'severity']
    search_fields = ['alertname', 'instance']
    ordering_fields = ['id', 'started_at', 'resolved_at', 'create_time']
    lookup_field = 'id'
    permission_classes = [CustomMenuPermission]
    action_perms_map = {
        'list': 'monitor:view',
        'retrieve': 'monitor:view',
    }

    def get_queryset(self):
        queryset = super().get_queryset()
        query_keyword = str(self.request.query_params.get('keyword') or '').strip()  # type: ignore[union-attr]
        query_start = str(self.request.query_params.get('start_time') or '').strip()  # type: ignore[union-attr]
        query_end = str(self.request.query_params.get('end_time') or '').strip()  # type: ignore[union-attr]

        if query_keyword:
            queryset = queryset.filter(
                Q(alertname__icontains=query_keyword)
                | Q(instance__icontains=query_keyword)
            )
        if query_start:
            queryset = queryset.filter(started_at__gte=query_start)
        if query_end:
            queryset = queryset.filter(started_at__lte=query_end)
        return queryset

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


class AlertMediaViewSet(
    GenericViewSet,
    ListModelMixin,
    CreateModelMixin,
    RetrieveModelMixin,
    UpdateModelMixin,
    DestroyModelMixin,
):
    queryset = AlertMedia.objects.prefetch_related('users').all()
    serializer_class = AlertMediaSerializer
    pagination_class = CustomPagination
    filter_backends = (OrderingFilter, DjangoFilterBackend, SearchFilter)
    filterset_fields = ['media_type', 'enabled']
    search_fields = ['name']
    ordering_fields = ['id', 'name', 'create_time', 'update_time']
    lookup_field = 'id'
    permission_classes = [CustomMenuPermission]
    action_perms_map = {
        'list': 'monitor:view',
        'retrieve': 'monitor:view',
        'create': 'monitor:view',
        'partial_update': 'monitor:view',
        'update': 'monitor:view',
        'destroy': 'monitor:view',
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

    def update(self, request, *args, **kwargs):
        instance = self.get_object()
        serializer = self.get_serializer(instance, data=request.data)
        serializer.is_valid(raise_exception=True)
        serializer.save()
        return Response_200(data=serializer.data)

    def partial_update(self, request, *args, **kwargs):
        instance = self.get_object()
        serializer = self.get_serializer(instance, data=request.data, partial=True)
        serializer.is_valid(raise_exception=True)
        serializer.save()
        return Response_200(data=serializer.data)

    def destroy(self, request, *args, **kwargs):
        instance = self.get_object()
        instance.delete()
        return Response_200(data={'deleted': True})


class AlertRouteViewSet(
    GenericViewSet,
    ListModelMixin,
    CreateModelMixin,
    RetrieveModelMixin,
    UpdateModelMixin,
    DestroyModelMixin,
):
    queryset = AlertRoute.objects.prefetch_related('media').all()
    serializer_class = AlertRouteSerializer
    pagination_class = CustomPagination
    filter_backends = (OrderingFilter, DjangoFilterBackend, SearchFilter)
    filterset_fields = ['enabled', 'notify_on_firing', 'notify_on_resolved']
    search_fields = ['name']
    ordering_fields = ['id', 'name', 'create_time', 'update_time']
    lookup_field = 'id'
    permission_classes = [CustomMenuPermission]
    action_perms_map = {
        'list': 'monitor:view',
        'retrieve': 'monitor:view',
        'create': 'monitor:view',
        'partial_update': 'monitor:view',
        'update': 'monitor:view',
        'destroy': 'monitor:view',
    }

    def list(self, request, *args, **kwargs):
        queryset = self.filter_queryset(self.get_queryset())
        page = self.paginate_queryset(queryset)
        serializer = self.get_serializer(page if page is not None else queryset, many=True)
        if page is not None:
            return self.get_paginated_response(serializer.data)
        return Response_200(data=serializer.data)

    def retrieve(self, request, *args, **kwargs):
        return Response_200(data=self.get_serializer(self.get_object()).data)

    def create(self, request, *args, **kwargs):
        serializer = self.get_serializer(data=request.data)
        serializer.is_valid(raise_exception=True)
        serializer.save()
        return Response_200(data=serializer.data)

    def update(self, request, *args, **kwargs):
        instance = self.get_object()
        serializer = self.get_serializer(instance, data=request.data)
        serializer.is_valid(raise_exception=True)
        serializer.save()
        return Response_200(data=serializer.data)

    def partial_update(self, request, *args, **kwargs):
        instance = self.get_object()
        serializer = self.get_serializer(instance, data=request.data, partial=True)
        serializer.is_valid(raise_exception=True)
        serializer.save()
        return Response_200(data=serializer.data)

    def destroy(self, request, *args, **kwargs):
        instance = self.get_object()
        instance.delete()
        return Response_200(data={'deleted': True})


