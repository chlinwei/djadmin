from django_filters.rest_framework import DjangoFilterBackend
from django.db.models import Count, Q
from rest_framework.decorators import action
from rest_framework.filters import OrderingFilter, SearchFilter
from rest_framework.mixins import CreateModelMixin, DestroyModelMixin, ListModelMixin, RetrieveModelMixin, UpdateModelMixin
from rest_framework.viewsets import GenericViewSet

import hashlib
import io
import re
import urllib.error
import urllib.request
from pathlib import PurePath

from django.core.files.base import ContentFile
from django.core.mail import EmailMultiAlternatives, get_connection
from django.http import JsonResponse
from django.utils import timezone
from rest_framework.permissions import AllowAny
from rest_framework.request import Request
from rest_framework.response import Response

from djadmin.utils import Response_200, Response_error_str
from djadmin.utils import CustomPagination
from menu.permisssion import CustomMenuPermission
from assets.credential_crypto import decrypt_secret
from assets.grpc_transfer.registry import REGISTRY
from assets.models import Host, HostGroup

from .alert_history import (
    compute_alert_fingerprint,
    get_prometheus_alert_rule_groups,
    ingest_alert_webhook_alerts,
    resolve_alert_rule_group,
    resolve_alert_rule_details,
)
from .models import (
    AlertHistory,
    AlertMedia,
    AlertNotificationDelivery,
    AlertNotificationEvent,
    AlertRoute,
    LogCollectionTarget,
    LogProcessingRule,
    LogRetentionTier,
    MonitorTarget,
    MonitorTargetInstallHistory,
    OpenSearchCluster,
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
    LogCollectionTargetSerializer,
    LogProcessingRuleSerializer,
    LogRetentionTierSerializer,
    MonitorTargetInstallHistorySerializer,
    MonitorTargetSerializer,
    OpenSearchClusterSerializer,
    SoftwarePackageSerializer,
)
from .opensearch_client import OpenSearchClient, OpenSearchError
from .fluent_bit import build_host_fragments
from .log_collection_service import (
    LogCollectionApplyError,
    apply_host_log_config,
    control_fluent_bit_service,
    dispatch_fluent_bit_install,
    dispatch_fluent_bit_uninstall,
    read_instance_log_tail,
    refresh_target_status,
)
from .log_management import bootstrap_log_storage

# 校验用户传入的目标版本号，防止拼接进下载 URL 时被注入路径穿越等非法字符
NODE_EXPORTER_VERSION_RE = re.compile(r'^\d+(\.\d+){1,3}$')
# 官方软件包体积上限（字节），避免异常响应把磁盘写满
MAX_OFFICIAL_PACKAGE_SIZE = 200 * 1024 * 1024


def _annotate_alert_notification_summary(queryset):
    return queryset.annotate(
        notification_count=Count('notification_events', distinct=True),
        notification_delivery_count=Count('notification_events__deliveries', distinct=True),
        notification_failed_count=Count(
            'notification_events',
            filter=Q(notification_events__status=AlertNotificationEvent.Status.FAILED),
            distinct=True,
        ),
        notification_active_count=Count(
            'notification_events',
            filter=Q(notification_events__status__in=[
                AlertNotificationEvent.Status.PENDING,
                AlertNotificationEvent.Status.SENDING,
            ]),
            distinct=True,
        ),
    )


def _descendant_group_ids(root_id):
    """主机组按整棵子树展开，选父组时子组主机一并纳入视图。"""
    child_map = {}
    for group_id, parent_id in HostGroup.objects.values_list('id', 'parent_id'):
        child_map.setdefault(parent_id, []).append(group_id)
    result = []
    pending = [root_id]
    seen = set()
    while pending:
        current = pending.pop()
        if current in seen:
            continue
        seen.add(current)
        result.append(current)
        pending.extend(child_map.get(current, []))
    return result


def _build_host_group_tree(managed_host_ids):
    """左树数据：每个分组带「已纳管主机数 / 主机总数」，Exporter 与 Fluent Bit 共用。"""
    groups = list(HostGroup.objects.order_by('name', 'id').values('id', 'name', 'parent_id'))

    stats = {}
    for host_id, group_id in Host.objects.filter(is_deleted_in_cloud=False).values_list('id', 'group_id'):
        entry = stats.setdefault(group_id, {'host_count': 0, 'managed_count': 0})
        entry['host_count'] += 1
        if host_id in managed_host_ids:
            entry['managed_count'] += 1

    nodes = {
        item['id']: {
            **item,
            'host_count': stats.get(item['id'], {}).get('host_count', 0),
            'managed_count': stats.get(item['id'], {}).get('managed_count', 0),
            'children': [],
        }
        for item in groups
    }
    roots = []
    for node in nodes.values():
        parent = nodes.get(node['parent_id'])
        (parent['children'] if parent else roots).append(node)

    ungrouped = stats.get(None, {'host_count': 0, 'managed_count': 0})
    return {
        'groups': roots,
        'total_host_count': sum(item['host_count'] for item in stats.values()),
        'total_managed_count': len(managed_host_ids),
        'ungrouped_host_count': ungrouped['host_count'],
    }


def _filter_overview_hosts(request):
    """主机总览的公共筛选：分组子树 / 关键字，并返回 gRPC Registry 的在线 agent_id 集合。"""
    queryset = Host.objects.filter(is_deleted_in_cloud=False).select_related('group')

    group_id = str(request.query_params.get('group_id') or '').strip()
    if group_id.isdigit():
        queryset = queryset.filter(group_id__in=_descendant_group_ids(int(group_id)))

    keyword = str(request.query_params.get('search') or '').strip()
    if keyword:
        queryset = queryset.filter(Q(instance_name__icontains=keyword) | Q(ip__icontains=keyword))

    from assets.host_online import get_connected_agent_ids
    # Agent 在线状态以 gRPC Registry 为准，避免用陈旧 DB 值误开放安装按钮。
    return queryset.order_by('instance_name', 'id'), get_connected_agent_ids()


def _build_fluent_bit_row(host, agent_online):
    """Fluent Bit 侧数据做成自包含子记录，前端行内动作可直接拿它当操作对象。"""
    target = getattr(host, 'log_collection_target', None)
    return {
        'id': target.id if target else None,
        'host_id': host.pk,
        'host_name': str(host.instance_name or ''),
        'host_ip': str(host.ip or ''),
        'host_agent_online': agent_online,
        'managed': target is not None,
        'agent_installed': bool(target.agent_installed) if target else False,
        'agent_version': str(target.agent_version or '') if target else '',
        'runtime_status': str(target.runtime_status or '') if target else '',
        'install_status': str(target.install_status or '') if target else '',
        'config_fingerprint': str(target.config_fingerprint or '') if target else '',
        'last_applied_time': target.last_applied_time if target else None,
        'last_error': str(target.last_error or '') if target else '',
    }


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
        'host_group_tree': 'monitor:view',
        'host_overview': 'monitor:view',
        'exporter_options': 'monitor:view',
        'batch_create': 'monitor:view',
    }

    @action(detail=False, methods=['get'], url_path='host-group-tree')
    def host_group_tree(self, request):
        """纳管目标页左树：Exporter 与 Fluent Bit 已合并为同一张主机表，纳管数按两者并集统计。"""
        managed_host_ids = set(MonitorTarget.objects.values_list('host_id', flat=True))
        managed_host_ids |= set(LogCollectionTarget.objects.values_list('host_id', flat=True))
        return Response_200(data=_build_host_group_tree(managed_host_ids))

    @action(detail=False, methods=['get'], url_path='exporter-options')
    def exporter_options(self, request):
        """可选 exporter 及其默认端口，来源是监控软件仓库中启用的 exporter 包。"""
        rows = (
            SoftwarePackage.objects
            .filter(package_type=SoftwarePackage.PackageType.EXPORTER, enabled=True)
            .values('name', 'default_port')
            .order_by('name')
        )
        seen = {}
        for row in rows:
            seen.setdefault(row['name'], row['default_port'])
        return Response_200(data=[{'name': name, 'default_port': port} for name, port in seen.items()])

    @action(detail=False, methods=['get'], url_path='host-overview')
    def host_overview(self, request):
        """纳管目标总表：一行一台主机，同时给出 Exporter 与 Fluent Bit 的纳管状态。

        指定 exporter_type 时把该 exporter 的字段摊平到行上，让行内直接具备可操作对象；
        Fluent Bit 侧字段统一收在 fluent_bit 子对象里，避免 id/install_status 与 exporter 撞名。
        """
        queryset, connected_agent_ids = _filter_overview_hosts(request)
        queryset = queryset.prefetch_related('monitor_targets').select_related('log_collection_target')

        exporter_type = str(request.query_params.get('exporter_type') or '').strip()
        exporter_managed = str(request.query_params.get('exporter_managed') or '').strip()
        fluent_bit_managed = str(request.query_params.get('fluent_bit_managed') or '').strip()
        legacy_managed = str(request.query_params.get('managed') or '').strip()

        # 精确的 Exporter 维度筛选
        if exporter_managed in ('true', 'false'):
            if exporter_type:
                condition = Q(monitor_targets__exporter_type=exporter_type)
            else:
                condition = Q(monitor_targets__isnull=False)
            queryset = queryset.filter(condition) if exporter_managed == 'true' else queryset.exclude(condition)
            queryset = queryset.distinct()
        elif legacy_managed in ('true', 'false'):
            if exporter_type:
                condition = Q(monitor_targets__exporter_type=exporter_type)
            else:
                condition = Q(monitor_targets__isnull=False) | Q(log_collection_target__isnull=False)
            queryset = queryset.filter(condition) if legacy_managed == 'true' else queryset.exclude(condition)
            queryset = queryset.distinct()

        # 精确的 Fluent Bit 维度独立筛选：未安装 = 既未纳管，或者纳管记录存在但 agent_installed 为 False
        if fluent_bit_managed in ('true', 'false'):
            if fluent_bit_managed == 'true':
                condition = Q(log_collection_target__isnull=False, log_collection_target__agent_installed=True)
            else:
                condition = Q(log_collection_target__isnull=True) | Q(log_collection_target__agent_installed=False)
            queryset = queryset.filter(condition)
            queryset = queryset.distinct()

        page = self.paginate_queryset(queryset)
        rows = page if page is not None else list(queryset)

        data = []
        for host in rows:
            targets = list(host.monitor_targets.all())  # type: ignore[attr-defined]
            if exporter_type:
                targets = [item for item in targets if item.exporter_type == exporter_type]
            agent_online = bool(host.agent_id and host.agent_id in connected_agent_ids)
            row = {
                'host_id': host.pk,
                'host_name': str(host.instance_name or ''),
                'host_ip': str(host.ip or ''),
                'group_id': host.group.pk if host.group else None,
                'group_name': host.group.name if host.group else '',
                'host_agent_online': agent_online,
                'managed': bool(targets),
                'exporters': [
                    {
                        'id': item.id,
                        'exporter_type': item.exporter_type,
                        'scrape_port': item.scrape_port,
                        'managed_enabled': item.managed_enabled,
                        'install_status': item.install_status,
                        'install_message': item.install_message,
                        'last_scrape_status': item.last_scrape_status,
                    }
                    for item in sorted(targets, key=lambda x: x.exporter_type)
                ],
                'fluent_bit': _build_fluent_bit_row(host, agent_online),
            }
            if exporter_type and targets:
                target = targets[0]
                row.update({
                    'id': target.id,
                    'exporter_type': target.exporter_type,
                    'scrape_port': target.scrape_port,
                    'managed_enabled': target.managed_enabled,
                    'install_status': target.install_status,
                    'install_message': target.install_message,
                    'last_scrape_status': target.last_scrape_status,
                })
            data.append(row)

        if page is None:
            return Response_200(data={'count': len(data), 'results': data})
        paginator = self.paginator
        return Response_200(data={
            'count': paginator.page.paginator.count,  # type: ignore[union-attr]
            'results': data,
            'pageNumber': paginator.page.number,  # type: ignore[union-attr]
            'pageSize': paginator.page_size,  # type: ignore[union-attr]
            'totalPages': paginator.page.paginator.num_pages,  # type: ignore[union-attr]
            'next': paginator.get_next_link(),  # type: ignore[union-attr]
            'previous': paginator.get_previous_link(),  # type: ignore[union-attr]
        })

    @action(detail=False, methods=['post'], url_path='batch-create')
    def batch_create(self, request):
        """批量纳管 exporter：同一主机的同一 exporter 已存在则跳过；install_now 时立刻下发安装。"""
        from assets.views import _enqueue_exporter_job

        raw_ids = request.data.get('host_ids')
        if not isinstance(raw_ids, list) or not raw_ids:
            return Response_error_str('host_ids 必须是非空数组', code=400)
        host_ids = [int(item) for item in raw_ids if str(item).isdigit()]
        if not host_ids:
            return Response_error_str('host_ids 只能包含正整数 ID', code=400)

        exporter_type = str(request.data.get('exporter_type') or '').strip()
        if not exporter_type:
            return Response_error_str('exporter_type 不能为空', code=400)
        package = SoftwarePackage.objects.filter(
            name=exporter_type, package_type=SoftwarePackage.PackageType.EXPORTER, enabled=True,
        ).first()
        if package is None:
            return Response_error_str(f'监控软件仓库中没有启用的 exporter：{exporter_type}', code=400)

        scrape_port = self._parse_port_value(request.data.get('scrape_port')) or package.default_port
        install_now = bool(request.data.get('install_now'))

        results = []
        for host in Host.objects.filter(id__in=host_ids, is_deleted_in_cloud=False):
            host_label = str(host.instance_name or host.ip or f'Host-{host.pk}')
            target, created = MonitorTarget.objects.get_or_create(
                host=host,
                exporter_type=exporter_type,
                defaults={'managed_enabled': True, 'scrape_port': scrape_port},
            )
            if not created:
                results.append({
                    'host_id': host.pk, 'host': host_label, 'ok': False,
                    'message': f'该主机已纳管 {exporter_type}，已跳过',
                })
                continue
            results.append({'host_id': host.pk, 'host': host_label, 'ok': True, 'message': ''})
            if install_now:
                target.retry_count = 0
                target.save(update_fields=['retry_count', 'update_time'])
                _enqueue_exporter_job('install', int(host.pk), int(target.pk), manual=True)

        success = sum(1 for item in results if item['ok'])
        return Response_200(data={
            'total': len(results),
            'success': success,
            'failed': len(results) - success,
            'results': results,
        })

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

        from assets.views import _enqueue_exporter_job

        action = 'install' if target.managed_enabled else 'uninstall'
        _enqueue_exporter_job(action, int(host.pk), int(target.pk), manual=True)

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

        通过 Agent gRPC 同步执行并直接返回最终 status/exit_code/stdout/stderr。
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

        return Response_200(data={
            'job_id': job.job_id,
            'status': job.status,
            'exit_code': job.exit_code,
            'stdout': job.stdout,
            'stderr': job.stderr,
            'error_message': job.error_message,
        })

    @action(detail=True, methods=['post'], url_path='start-service')
    def start_service(self, request, id=None):
        """通过 Agent gRPC 同步启动 exporter 服务。"""
        target = self.get_object()
        host = target.host
        if host is None:
            return Response_error_str('监控目标未关联主机，无法启动服务', code=400)

        from assets.views import dispatch_start_exporter_job
        try:
            job = dispatch_start_exporter_job(host, target)
        except RuntimeError as exc:
            return Response_error_str(str(exc), code=400)

        return Response_200(data={
            'job_id': job.job_id,
            'status': job.status,
            'exit_code': job.exit_code,
            'stdout': job.stdout,
            'stderr': job.stderr,
            'error_message': job.error_message,
        })

    @action(detail=True, methods=['post'], url_path='stop-service')
    def stop_service(self, request, id=None):
        """通过 Agent gRPC 同步停止 exporter 服务。"""
        target = self.get_object()
        host = target.host
        if host is None:
            return Response_error_str('监控目标未关联主机，无法停止服务', code=400)

        from assets.views import dispatch_stop_exporter_job
        try:
            job = dispatch_stop_exporter_job(host, target)
        except RuntimeError as exc:
            return Response_error_str(str(exc), code=400)

        return Response_200(data={
            'job_id': job.job_id,
            'status': job.status,
            'exit_code': job.exit_code,
            'stdout': job.stdout,
            'stderr': job.stderr,
            'error_message': job.error_message,
        })

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
        rule_group_indexes = get_prometheus_alert_rule_groups()
        firing_count = 0
        resolved_count = 0
        rows = []
        alert_items = list(alerts or [])
        fingerprints = []
        for item in alert_items:
            labels = item.get('labels') if isinstance(item.get('labels'), dict) else {}
            fingerprints.append(compute_alert_fingerprint(labels))

        histories_by_fingerprint_and_state = {}
        histories = _annotate_alert_notification_summary(
            AlertHistory.objects.filter(fingerprint__in=fingerprints)
        ).order_by('-id')
        for history in histories:
            key = (history.fingerprint, history.state)
            histories_by_fingerprint_and_state.setdefault(key, history)

        for item in alert_items:
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
            fingerprint = compute_alert_fingerprint(labels)
            alertname = labels.get('alertname') or ''
            # pending 尚未触发 Prometheus notifier，不能继承同 fingerprint 的历史 firing 记录。
            # firing/resolved 也必须匹配本地相同状态，避免新一轮 pending/firing 显示上一轮投递结果。
            history = (
                histories_by_fingerprint_and_state.get((fingerprint, state))
                if state in {AlertHistory.State.FIRING, AlertHistory.State.RESOLVED}
                else None
            )
            rows.append({
                'name': alertname,
                'rule_group': resolve_alert_rule_group(labels, alertname, rule_group_indexes),
                'rule_details': resolve_alert_rule_details(labels, alertname, rule_group_indexes),
                'severity': labels.get('severity') or '',
                'state': state or 'unknown',
                'instance': labels.get('instance') or '',
                'labels': labels,
                'summary': annotations.get('summary') or annotations.get('description') or '',
                'active_at': item.get('activeAt') or '',
                'value': item.get('value') or '',
                'history_id': history.id if history is not None else None,
                'notification_count': getattr(history, 'notification_count', 0) if history is not None else 0,
                'notification_delivery_count': getattr(history, 'notification_delivery_count', 0) if history is not None else 0,
                'notification_status': AlertHistorySerializer.get_notification_status(history) if history is not None else 'none',
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
    filterset_fields = ['package_type', 'name', 'version', 'os', 'arch', 'enabled']
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
        if not serializer.is_valid():
            return Response_error_str('软件包配置无效', code=400, data=serializer.errors)
        serializer.save()
        return Response_200(data=serializer.data)

    def partial_update(self, request, *args, **kwargs):
        # 目前仅用于编辑自定义生命周期脚本/环境变量/启动参数这几项元数据，
        # 文件本身（sha256/size_bytes）仍走行内“上传”接口，避免和该接口职责重叠。
        instance = self.get_object()
        serializer = self.get_serializer(instance, data=request.data, partial=True)
        if not serializer.is_valid():
            return Response_error_str('软件包配置无效', code=400, data=serializer.errors)
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
        """为当前软件包记录上传文件；平台、架构和版本以记录字段为准。"""
        instance = self.get_object()
        upload = request.FILES.get('file')
        if not upload:
            return Response_error_str('请提供上传文件', code=400)

        filename = PurePath(str(getattr(upload, 'name', '') or '')).name
        format_suffixes = {
            SoftwarePackage.PackageFormat.TAR_GZ: ('.tar.gz',),
            SoftwarePackage.PackageFormat.RPM: ('.rpm',),
            SoftwarePackage.PackageFormat.DEB: ('.deb',),
        }
        expected_suffixes = format_suffixes.get(instance.package_format, ())
        if not filename or not filename.lower().endswith(expected_suffixes):
            return Response_error_str(
                f'上传文件格式与当前记录（{instance.package_format}）不一致', code=400,
            )

        hasher = hashlib.sha256()
        for chunk in upload.chunks():
            hasher.update(chunk)
        upload.seek(0)

        instance.file.save(filename, upload, save=False)
        instance.sha256 = hasher.hexdigest()
        instance.size_bytes = int(getattr(upload, 'size', 0) or 0)
        instance.save(update_fields=['file', 'sha256', 'size_bytes', 'update_time'])
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
            platform_family=instance.platform_family, platform_major=instance.platform_major,
        ).exclude(pk=instance.pk).exists()
        if conflict:
            return Response_error_str(f'版本 {target_version} 已存在相同平台记录，请先删除或更换版本', code=400)

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
    queryset = MonitorTargetInstallHistory.objects.select_related('target', 'log_collection_target', 'host').all()
    serializer_class = MonitorTargetInstallHistorySerializer
    pagination_class = CustomPagination
    filter_backends = (OrderingFilter, DjangoFilterBackend, SearchFilter)
    filterset_fields = ['target_id', 'log_collection_target_id', 'action', 'trigger_type', 'status']
    search_fields = ['host_name_snapshot', 'host_ip_snapshot', 'exporter_type_snapshot', 'summary_message']
    ordering_fields = ['id', 'create_time', 'update_time', 'start_time', 'end_time']
    lookup_field = 'id'
    permission_classes = [CustomMenuPermission]
    action_perms_map = {
        'list': 'monitor:view',
        'retrieve': 'monitor:view',
        'cancel': 'monitor:view',
    }

    def get_queryset(self):
        queryset = super().get_queryset()
        query_target_id = str(self.request.query_params.get('target_id') or '').strip()  # type: ignore[union-attr]
        query_log_target_id = str(  # type: ignore[union-attr]
            self.request.query_params.get('log_collection_target_id') or ''
        ).strip()
        query_keyword = str(self.request.query_params.get('keyword') or '').strip()  # type: ignore[union-attr]
        query_start = str(self.request.query_params.get('start_time') or '').strip()  # type: ignore[union-attr]
        query_end = str(self.request.query_params.get('end_time') or '').strip()  # type: ignore[union-attr]

        if query_target_id.isdigit():
            queryset = queryset.filter(target_id=int(query_target_id))
        if query_log_target_id.isdigit():
            queryset = queryset.filter(log_collection_target_id=int(query_log_target_id))
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

    @action(detail=True, methods=['post'], url_path='cancel')
    def cancel(self, request, *args, **kwargs):
        from django.db import transaction

        from .models import MonitorTargetInstallHistory

        with transaction.atomic():
            history = self.get_queryset().select_for_update().get(pk=self.get_object().pk)
            if history.status not in {
                MonitorTargetInstallHistory.Status.PENDING,
                MonitorTargetInstallHistory.Status.RUNNING,
            }:
                return Response_error_str('当前任务已结束，不能取消')

            now = timezone.now()
            history.status = MonitorTargetInstallHistory.Status.CANCELLED
            history.summary_message = '任务已取消'
            history.error_message_snapshot = '任务已由用户取消'
            history.end_time = now
            if history.start_time is not None:
                history.duration_seconds = (now - history.start_time).total_seconds()
            history.save(update_fields=[
                'status', 'summary_message', 'error_message_snapshot',
                'end_time', 'duration_seconds', 'update_time',
            ])

            target = history.target or history.log_collection_target
            if target is None:
                return Response_error_str('任务未关联纳管目标，无法取消', code=400)
            target.install_status = target.InstallStatus.UNKNOWN
            target.install_message = '安装/卸载任务已取消'
            target.save(update_fields=['install_status', 'install_message', 'update_time'])

        return Response_200(data=self.get_serializer(history).data)


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
        'notification_status': 'monitor:view',
    }

    def get_queryset(self):
        queryset = _annotate_alert_notification_summary(
            super().get_queryset()
        ).order_by('-started_at', '-id')
        query_keyword = str(self.request.query_params.get('keyword') or '').strip()  # type: ignore[union-attr]
        query_start = str(self.request.query_params.get('start_time') or '').strip()  # type: ignore[union-attr]
        query_end = str(self.request.query_params.get('end_time') or '').strip()  # type: ignore[union-attr]
        query_notification_status = str(
            self.request.query_params.get('notification_status') or ''  # type: ignore[union-attr]
        ).strip()

        if query_keyword:
            queryset = queryset.filter(
                Q(alertname__icontains=query_keyword)
                | Q(instance__icontains=query_keyword)
            )
        if query_start:
            queryset = queryset.filter(started_at__gte=query_start)
        if query_end:
            queryset = queryset.filter(started_at__lte=query_end)
        if query_notification_status == 'none':
            queryset = queryset.filter(notification_count=0)
        elif query_notification_status == 'failed':
            queryset = queryset.filter(
                Q(notification_failed_count__gt=0)
                | Q(
                    notification_count__gt=0,
                    notification_active_count=0,
                    notification_delivery_count=0,
                )
            )
        elif query_notification_status == 'in_progress':
            queryset = queryset.filter(
                notification_failed_count=0,
                notification_active_count__gt=0,
            )
        elif query_notification_status == 'success':
            queryset = queryset.filter(
                notification_count__gt=0,
                notification_failed_count=0,
                notification_active_count=0,
                notification_delivery_count__gt=0,
            )
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

    @action(detail=True, methods=['get'], url_path='notification-status')
    def notification_status(self, request, *args, **kwargs):
        alert = self.get_object()
        events = (
            AlertNotificationEvent.objects.filter(alert=alert)
            .order_by('-create_time', '-id')
        )
        event_rows = []
        for event in events:
            deliveries = [
                {
                    'id': delivery.id,
                    'user_id': delivery.user_id,
                    'username': delivery.user.username if delivery.user is not None else '-',
                    'media_id': delivery.media_id,
                    'media_name': delivery.media.name if delivery.media is not None else '-',
                    'media_type': delivery.media.media_type if delivery.media is not None else '-',
                    'address': delivery.address,
                    'status': delivery.status,
                    'attempt_count': delivery.attempt_count,
                    'error_message': delivery.error_message,
                    'sent_at': delivery.sent_at,
                    'create_time': delivery.create_time,
                }
                for delivery in AlertNotificationDelivery.objects.filter(event=event).select_related('user', 'media')
            ]
            event_status = event.status
            event_error_message = event.error_message
            if not deliveries and event_status == AlertNotificationEvent.Status.SUCCESS:
                event_status = AlertNotificationEvent.Status.FAILED
                event_error_message = '没有投递明细，无法确认实际接收用户、媒介和地址'
            event_rows.append({
                'id': event.id,
                'event_type': event.event_type,
                'status': event_status,
                'attempt_count': event.attempt_count,
                'error_message': event_error_message,
                'sent_at': event.sent_at,
                'create_time': event.create_time,
                'deliveries': deliveries,
            })
        return Response_200(data={
            'alert_id': alert.id,
            'alertname': alert.alertname,
            'instance': alert.instance,
            'events': event_rows,
        })


class AlertMediaViewSet(
    GenericViewSet,
    ListModelMixin,
    CreateModelMixin,
    RetrieveModelMixin,
    UpdateModelMixin,
    DestroyModelMixin,
):
    queryset = AlertMedia.objects.all()
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
        'test': 'monitor:view',
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

    @action(detail=True, methods=['post'], url_path='test')
    def test(self, request, *args, **kwargs):
        media = self.get_object()
        if media.media_type != AlertMedia.MediaType.EMAIL:
            return Response_error_str('当前仅支持测试 Email 媒介')

        recipients = request.data.get('recipients')
        if isinstance(recipients, str):
            recipients = [item.strip() for item in recipients.replace(';', ',').split(',') if item.strip()]
        if not isinstance(recipients, list) or not recipients:
            return Response_error_str('请至少填写一个收件人')

        subject = str(request.data.get('subject') or '').strip()
        message = str(request.data.get('message') or '')
        if not subject:
            return Response_error_str('请填写主题')
        if not message.strip():
            return Response_error_str('请填写消息')

        config = media.config or {}
        try:
            password = decrypt_secret(config.get('password'))
            connection = get_connection(
                backend='django.core.mail.backends.smtp.EmailBackend',
                fail_silently=False,
                host=config.get('smtpServer'),
                port=int(config.get('smtpPort')),
                username=config.get('username'),
                password=password,
                use_tls=bool(config.get('useTLS', config.get('provider') == 'gmail')),
                use_ssl=bool(config.get('useSSL', False)),
            )
            email = EmailMultiAlternatives(
                subject=subject,
                body=message,
                from_email=config.get('email'),
                to=[str(item).strip() for item in recipients if str(item).strip()],
                connection=connection,
            )
            if config.get('messageFormat', 'html') == 'html':
                email.attach_alternative(message, 'text/html')
            sent_count = email.send()
        except (OSError, TypeError, ValueError) as exc:
            return Response_error_str(f'测试邮件发送失败：{exc}')
        if sent_count != 1:
            return Response_error_str('测试邮件发送失败')
        return Response_200(data={'sent': True})

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




class OpenSearchClusterViewSet(
    GenericViewSet,
    ListModelMixin,
    CreateModelMixin,
    RetrieveModelMixin,
    UpdateModelMixin,
    DestroyModelMixin,
):
    """日志存储集群连接配置，供日志采集与查询复用。"""

    queryset = OpenSearchCluster.objects.all()
    serializer_class = OpenSearchClusterSerializer
    pagination_class = CustomPagination
    filter_backends = (OrderingFilter, DjangoFilterBackend, SearchFilter)
    filterset_fields = ['enabled', 'is_default']
    search_fields = ['name', 'hosts', 'remark']
    ordering_fields = ['id', 'name', 'create_time', 'update_time']
    lookup_field = 'id'
    permission_classes = [CustomMenuPermission]
    action_perms_map = {
        'list': 'monitor:view',
        'retrieve': 'monitor:view',
        'create': 'monitor:view',
        'update': 'monitor:view',
        'partial_update': 'monitor:view',
        'perform_update': 'monitor:view',
        'destroy': 'monitor:view',
        'test_connection': 'monitor:view',
        'bootstrap': 'monitor:view',
        'simulate_pipeline': 'monitor:view',
        'error_patterns': 'monitor:view',
        'new_errors': 'monitor:view',
        'error_spikes': 'monitor:view',
        'error_by_instance': 'monitor:view',
    }

    def list(self, request, *args, **kwargs):
        queryset = self.filter_queryset(self.get_queryset())
        page = self.paginate_queryset(queryset)
        serializer = self.get_serializer(page if page is not None else queryset, many=True)
        if page is None:
            return Response_200(data=serializer.data)
        paginator = self.paginator
        return Response_200(data={
            'count': paginator.page.paginator.count,
            'results': serializer.data,
            'pageNumber': paginator.page.number,
            'pageSize': paginator.page_size,
            'totalPages': paginator.page.paginator.num_pages,
            'next': paginator.get_next_link(),
            'previous': paginator.get_previous_link(),
        })

    def retrieve(self, request, *args, **kwargs):
        return Response_200(data=self.get_serializer(self.get_object()).data)

    def create(self, request, *args, **kwargs):
        # 日志存储为全局单例配置，只允许保留一个集群，防止前端绕过禁用按钮重复创建
        if OpenSearchCluster.objects.exists():
            return Response_error_str('仅支持配置一个日志存储集群，请编辑现有记录', code=400)
        serializer = self.get_serializer(data=request.data)
        serializer.is_valid(raise_exception=True)
        serializer.save()
        return Response_200(data=serializer.data)

    def update(self, request, *args, **kwargs):
        partial = kwargs.pop('partial', False)
        serializer = self.get_serializer(self.get_object(), data=request.data, partial=partial)
        serializer.is_valid(raise_exception=True)
        serializer.save()
        return Response_200(data=serializer.data)

    def partial_update(self, request, *args, **kwargs):
        kwargs['partial'] = True
        return self.update(request, *args, **kwargs)

    def destroy(self, request, *args, **kwargs):
        self.get_object().delete()
        return Response_200(data={'deleted': True})

    @action(detail=True, methods=['post'], url_path='test-connection')
    def test_connection(self, request, id=None):
        cluster = self.get_object()
        try:
            info = OpenSearchClient(cluster).ping()
        except OpenSearchError as exc:
            cluster.last_check_time = timezone.now()
            cluster.last_check_success = False
            cluster.last_check_message = str(exc)[:1000]
            cluster.save(update_fields=['last_check_time', 'last_check_success', 'last_check_message', 'update_time'])
            return Response_error_str(f'连接失败: {exc}', code=400)
        cluster.last_check_time = timezone.now()
        cluster.last_check_success = True
        cluster.last_check_message = (
            f"{info.get('distribution') or 'opensearch'} {info.get('version')} / "
            f"{info.get('cluster_name')} / {info.get('status')}"
        )
        cluster.save(update_fields=['last_check_time', 'last_check_success', 'last_check_message', 'update_time'])
        return Response_200(data=info)

    def _call_client(self, func, *args, **kwargs):
        """统一把 OpenSearchError 转为错误响应，避免每个 action 重复 try/except。"""
        try:
            return func(*args, **kwargs), None
        except OpenSearchError as exc:
            return None, Response_error_str(str(exc), code=400)


    @action(detail=True, methods=['post'], url_path='bootstrap')
    def bootstrap(self, request, id=None):
        """确保 index template 与 hot/std/cold 三条 ISM policy 存在（架构文档 §11 阶段 1）。"""
        result, error = self._call_client(bootstrap_log_storage, self.get_object())
        if error is not None:
            return error
        return Response_200(data=result)

    @action(detail=True, methods=['post'], url_path='pipeline-simulate')
    def simulate_pipeline(self, request, id=None):
        """解析规则调试（架构文档 §5.4，最高优先级）：

        支持两种模式：
        - 传 pipeline 体：用未保存的定义直接试跑（编辑页实时预览）
        - 传 name：对服务端已存在的 pipeline 试跑
        """
        docs = request.data.get('docs') or []
        if not isinstance(docs, list) or not docs:
            return Response_error_str('docs 必须是非空数组', code=400)
        client = OpenSearchClient(self.get_object())
        body = request.data.get('pipeline')
        if body is not None:
            if not isinstance(body, dict):
                return Response_error_str('pipeline 必须是对象', code=400)
            data, error = self._call_client(client.simulate_pipeline_body, body, docs)
        else:
            name = str(request.data.get('name') or '').strip()
            if not name:
                return Response_error_str('pipeline 与 name 至少提供一个', code=400)
            data, error = self._call_client(client.simulate_pipeline, name, docs)
        if error is not None:
            return error
        return Response_200(data=data)

    @staticmethod
    def _insight_params(request):
        index = str(request.query_params.get('index') or '').strip()
        if not index:
            return None, None, Response_error_str('缺少 index 参数', code=400)
        try:
            hours = min(max(int(request.query_params.get('hours', 1)), 1), 24 * 30)
        except (TypeError, ValueError):
            return None, None, Response_error_str('hours 必须是整数', code=400)
        return index, hours, None

    def _search(self, cluster, index, body):
        data, error = self._call_client(OpenSearchClient(cluster).search, index, body)
        if error is not None:
            return error
        return Response_200(data=data)

    @action(detail=True, methods=['get'], url_path='insight/error-patterns')
    def error_patterns(self, request, id=None):
        """自动错误清单：按 error_fingerprint 聚合，含样例与服务分布（§9.1）。"""
        index, hours, error = self._insight_params(request)
        if error is not None:
            return error
        body = {
            'size': 0,
            'query': {'bool': {'filter': [
                {'terms': {'log_level': ['ERROR', 'SEVERE', 'FATAL']}},
                {'range': {'@timestamp': {'gte': f'now-{hours}h'}}},
            ]}},
            'aggs': {
                'patterns': {
                    'terms': {'field': 'error_fingerprint', 'size': 50},
                    'aggs': {
                        'sample': {'top_hits': {
                            'size': 1,
                            # 归一化模板只是指纹的中间量、不落库，样例直接取该组里的一条真实错误。
                            '_source': ['error_type', 'error_message', 'root_cause_type', 'service', 'instance'],
                        }},
                        'services': {'terms': {'field': 'service'}},
                    },
                },
            },
        }
        return self._search(self.get_object(), index, body)

    @action(detail=True, methods=['get'], url_path='insight/new-errors')
    def new_errors(self, request, id=None):
        """新增错误识别：significant_terms 对比 7 天背景频率，无需阈值（§9.2）。"""
        index, hours, error = self._insight_params(request)
        if error is not None:
            return error
        body = {
            'size': 0,
            'query': {'range': {'@timestamp': {'gte': f'now-{hours}h'}}},
            'aggs': {
                'unusual_errors': {
                    'significant_terms': {
                        'field': 'error_fingerprint',
                        'size': 20,
                        'background_filter': {
                            'range': {'@timestamp': {'gte': 'now-7d', 'lt': f'now-{hours}h'}},
                        },
                    },
                },
            },
        }
        return self._search(self.get_object(), index, body)

    @action(detail=True, methods=['get'], url_path='insight/error-spikes')
    def error_spikes(self, request, id=None):
        """突增检测：terms 嵌套 date_histogram 取时序，由调用方比对最近桶与历史均值（§9.3）。"""
        index, hours, error = self._insight_params(request)
        if error is not None:
            return error
        assert hours is not None  # error 为空时 _insight_params 保证 hours 已解析为 int
        body = {
            'size': 0,
            'query': {'bool': {'filter': [
                {'terms': {'log_level': ['ERROR', 'SEVERE', 'FATAL']}},
                {'range': {'@timestamp': {'gte': f'now-{hours}h'}}},
            ]}},
            'aggs': {
                'by_error': {
                    'terms': {'field': 'error_fingerprint', 'size': 50},
                    'aggs': {
                        'trend': {'date_histogram': {
                            'field': '@timestamp',
                            'fixed_interval': f'{max(hours * 60 // 30, 1)}m',
                        }},
                    },
                },
            },
        }
        return self._search(self.get_object(), index, body)

    @action(detail=True, methods=['get'], url_path='insight/error-by-instance')
    def error_by_instance(self, request, id=None):
        """实例分布下钻：区分「代码缺陷」（均匀分布）与「单机环境问题」（§9.4）。"""
        index, hours, error = self._insight_params(request)
        if error is not None:
            return error
        body = {
            'size': 0,
            'query': {'bool': {'filter': [
                {'terms': {'log_level': ['ERROR', 'SEVERE', 'FATAL']}},
                {'range': {'@timestamp': {'gte': f'now-{hours}h'}}},
            ]}},
            'aggs': {
                'by_error': {
                    'terms': {'field': 'error_type', 'size': 10},
                    'aggs': {'by_instance': {'terms': {'field': 'instance'}}},
                },
            },
        }
        return self._search(self.get_object(), index, body)


class LogRetentionTierViewSet(
    GenericViewSet,
    ListModelMixin,
    CreateModelMixin,
    RetrieveModelMixin,
    UpdateModelMixin,
    DestroyModelMixin,
):
    """日志保留档位：决定 data stream 后缀与 ISM 滚动/删除策略。"""

    queryset = LogRetentionTier.objects.all()
    serializer_class = LogRetentionTierSerializer
    pagination_class = CustomPagination
    filter_backends = (OrderingFilter, DjangoFilterBackend, SearchFilter)
    filterset_fields = ['enabled', 'is_default']
    search_fields = ['code', 'name', 'remark']
    ordering_fields = ['id', 'code', 'retention_days', 'daily_size_gb']
    permission_classes = [CustomMenuPermission]
    action_perms_map = {
        'list': 'monitor:view',
        'retrieve': 'monitor:view',
        'create': 'monitor:view',
        'update': 'monitor:view',
        'partial_update': 'monitor:view',
        'destroy': 'monitor:view',
    }

    def list(self, request: Request, *args, **kwargs) -> Response:
        queryset = self.filter_queryset(self.get_queryset())
        page = self.paginate_queryset(queryset)
        serializer = self.get_serializer(page if page is not None else queryset, many=True)
        if page is None:
            return Response_200(data=serializer.data)
        paginator = self.paginator
        return Response_200(data={
            'count': paginator.page.paginator.count,
            'results': serializer.data,
            'pageNumber': paginator.page.number,
            'pageSize': paginator.page_size,
            'totalPages': paginator.page.paginator.num_pages,
            'next': paginator.get_next_link(),
            'previous': paginator.get_previous_link(),
        })

    def retrieve(self, request: Request, *args, **kwargs) -> Response:
        return Response_200(data=self.get_serializer(self.get_object()).data)

    def create(self, request: Request, *args, **kwargs) -> Response:
        serializer = self.get_serializer(data=request.data)
        serializer.is_valid(raise_exception=True)
        instance = serializer.save()
        self._sync_default(instance)
        self._apply_policies()
        return Response_200(data=self.get_serializer(instance).data)

    def update(self, request: Request, *args, **kwargs) -> Response:
        partial = kwargs.pop('partial', False)
        serializer = self.get_serializer(self.get_object(), data=request.data, partial=partial)
        serializer.is_valid(raise_exception=True)
        instance = serializer.save()
        self._sync_default(instance)
        self._apply_policies()
        return Response_200(data=self.get_serializer(instance).data)

    def partial_update(self, request: Request, *args, **kwargs) -> Response:
        kwargs['partial'] = True
        return self.update(request, *args, **kwargs)

    def destroy(self, request: Request, *args, **kwargs) -> Response:
        instance = self.get_object()
        if instance.services.exists():
            return Response_error_str('该档位仍被逻辑服务引用，不能删除', code=400)
        instance.delete()
        return Response_200(data={'deleted': True})

    @staticmethod
    def _sync_default(instance):
        """默认档位全局唯一：新的默认生效时取消其他档位的默认标记。"""
        if instance.is_default:
            LogRetentionTier.objects.exclude(pk=instance.pk).filter(is_default=True).update(is_default=False)

    @staticmethod
    def _apply_policies():
        """档位改动后立刻把 ISM policy 推到各启用集群，避免"页面改了但集群没生效"。"""
        for cluster in OpenSearchCluster.objects.filter(enabled=True):
            try:
                bootstrap_log_storage(cluster)
            except OpenSearchError:
                # 集群暂时不可达不应阻塞档位保存，下次 bootstrap 会补齐。
                continue


class LogProcessingRuleViewSet(
    GenericViewSet,
    ListModelMixin,
    CreateModelMixin,
    RetrieveModelMixin,
    UpdateModelMixin,
    DestroyModelMixin,
):
    """统一管理 Fluent Bit 前处理配置和 OpenSearch Pipeline。"""

    queryset = LogProcessingRule.objects.select_related('cluster', 'application').all()
    serializer_class = LogProcessingRuleSerializer
    pagination_class = CustomPagination
    filter_backends = (OrderingFilter, DjangoFilterBackend, SearchFilter)
    filterset_fields = ['cluster', 'application', 'input_format', 'multiline_enabled']
    search_fields = ['name', 'description']
    ordering_fields = ['id', 'name', 'create_time', 'update_time']
    permission_classes = [CustomMenuPermission]
    action_perms_map = {
        'list': 'monitor:view',
        'retrieve': 'monitor:view',
        'create': 'monitor:view',
        'update': 'monitor:view',
        'partial_update': 'monitor:view',
        'destroy': 'monitor:view',
    }

    def list(self, request: Request, *args, **kwargs) -> Response:
        queryset = self.filter_queryset(self.get_queryset())
        page = self.paginate_queryset(queryset)
        serializer = self.get_serializer(page if page is not None else queryset, many=True)
        if page is None:
            return Response_200(data=serializer.data)
        paginator = self.paginator
        return Response_200(data={
            'count': paginator.page.paginator.count,
            'results': serializer.data,
            'pageNumber': paginator.page.number,
            'pageSize': paginator.page_size,
            'totalPages': paginator.page.paginator.num_pages,
            'next': paginator.get_next_link(),
            'previous': paginator.get_previous_link(),
        })

    def retrieve(self, request: Request, *args, **kwargs) -> Response:
        return Response_200(data=self.get_serializer(self.get_object()).data)

    def create(self, request: Request, *args, **kwargs) -> Response:
        serializer = self.get_serializer(data=request.data)
        serializer.is_valid(raise_exception=True)
        data = serializer.validated_data
        try:
            OpenSearchClient(data['cluster']).put_pipeline(data['name'], data['pipeline_body'])
        except OpenSearchError as exc:
            return Response_error_str(f'发布 Pipeline 失败: {exc}', code=400)
        serializer.save()
        return Response_200(data=serializer.data)

    def update(self, request: Request, *args, **kwargs) -> Response:
        partial = kwargs.pop('partial', False)
        instance = self.get_object()
        serializer = self.get_serializer(instance, data=request.data, partial=partial)
        serializer.is_valid(raise_exception=True)
        cluster = serializer.validated_data.get('cluster', instance.cluster)
        pipeline_body = serializer.validated_data.get('pipeline_body', instance.pipeline_body)
        try:
            OpenSearchClient(cluster).put_pipeline(instance.name, pipeline_body)
        except OpenSearchError as exc:
            return Response_error_str(f'发布 Pipeline 失败: {exc}', code=400)
        serializer.save()
        return Response_200(data=serializer.data)

    def partial_update(self, request: Request, *args, **kwargs) -> Response:
        kwargs['partial'] = True
        return self.update(request, *args, **kwargs)

    def destroy(self, request: Request, *args, **kwargs) -> Response:
        instance = self.get_object()
        if instance.log_definitions.exists():
            return Response_error_str('规则仍被日志定义引用，不能删除', code=400)
        try:
            OpenSearchClient(instance.cluster).delete_pipeline(instance.name)
        except OpenSearchError as exc:
            return Response_error_str(f'删除 Pipeline 失败: {exc}', code=400)
        instance.delete()
        return Response_200(data={'deleted': True})


class LogCollectionTargetViewSet(
    GenericViewSet,
    ListModelMixin,
    CreateModelMixin,
    RetrieveModelMixin,
    DestroyModelMixin,
):
    """主机级日志采集状态（架构文档 §7）。

    render-config 只读预览；apply 经 dj-agent 实际写入 Fluent Bit 片段并热重载（阶段 5）。
    """

    queryset = LogCollectionTarget.objects.select_related('host').all()
    serializer_class = LogCollectionTargetSerializer
    pagination_class = CustomPagination
    filter_backends = (OrderingFilter, DjangoFilterBackend, SearchFilter)
    filterset_fields = ['agent_installed', 'runtime_status']
    search_fields = ['host__instance_name', 'host__ip']
    ordering_fields = ['id', 'last_applied_time', 'update_time']
    lookup_field = 'id'
    permission_classes = [CustomMenuPermission]
    action_perms_map = {
        'list': 'monitor:view',
        'create': 'monitor:view',
        'retrieve': 'monitor:view',
        'render_config': 'monitor:view',
        'apply': 'monitor:view',
        'check_status': 'monitor:view',
        'log_tail': 'monitor:view',
        'retry': 'monitor:view',
        'start_service': 'monitor:view',
        'stop_service': 'monitor:view',
        'cancel_pending': 'monitor:view',
        'destroy': 'monitor:view',
        'batch_retry': 'monitor:view',
        'batch_start_service': 'monitor:view',
        'batch_stop_service': 'monitor:view',
        'batch_apply': 'monitor:view',
        'batch_delete': 'monitor:view',
        'batch_create': 'monitor:view',
    }

    @action(detail=False, methods=['post'], url_path='batch-create')
    def batch_create(self, request):
        """批量纳管：已纳管主机自动跳过；install_now=true 时创建后立即下发安装。"""
        raw_ids = request.data.get('host_ids')
        if not isinstance(raw_ids, list) or not raw_ids:
            return Response_error_str('host_ids 必须是非空数组', code=400)
        host_ids = [int(item) for item in raw_ids if str(item).isdigit()]
        if not host_ids:
            return Response_error_str('host_ids 只能包含正整数 ID', code=400)

        install_now = bool(request.data.get('install_now'))
        hosts = list(Host.objects.filter(id__in=host_ids, is_deleted_in_cloud=False))
        managed_host_ids = set(
            LogCollectionTarget.objects.filter(host_id__in=host_ids).values_list('host_id', flat=True)
        )

        results = []
        for host in hosts:
            host_label = str(host.instance_name or host.ip or f'Host-{host.pk}')
            if host.pk in managed_host_ids:
                results.append({'host_id': host.pk, 'host': host_label, 'ok': False, 'message': '该主机已纳管，已跳过'})
                continue
            target = LogCollectionTarget.objects.create(host=host)
            if not install_now:
                results.append({'host_id': host.pk, 'host': host_label, 'ok': True, 'message': ''})
                continue
            try:
                dispatch_fluent_bit_install(target, manual=True)
            except LogCollectionApplyError as exc:
                target.install_status = LogCollectionTarget.InstallStatus.FAILED
                target.install_message = str(exc)
                target.save(update_fields=['install_status', 'install_message', 'update_time'])
                results.append({'host_id': host.pk, 'host': host_label, 'ok': False, 'message': str(exc)})
            else:
                results.append({'host_id': host.pk, 'host': host_label, 'ok': True, 'message': ''})

        success = sum(1 for item in results if item['ok'])
        return Response_200(data={
            'total': len(results),
            'success': success,
            'failed': len(results) - success,
            'results': results,
        })

    def _run_batch(self, request, handler):
        """逐台执行并汇总结果：单台失败不影响其余主机，前端据此展示部分失败。"""
        raw_ids = request.data.get('ids')
        if not isinstance(raw_ids, list) or not raw_ids:
            return Response_error_str('ids 必须是非空数组', code=400)
        ids = [int(item) for item in raw_ids if str(item).isdigit()]
        if not ids:
            return Response_error_str('ids 只能包含正整数 ID', code=400)
        targets = list(self.get_queryset().filter(id__in=ids))
        if not targets:
            return Response_error_str('未找到可操作的纳管目标', code=400)

        results = []
        for target in targets:
            host_label = str(getattr(target.host, 'instance_name', '') or getattr(target.host, 'ip', '') or f'Host-{target.host_id}')
            try:
                handler(target)
            except (LogCollectionApplyError, ValueError) as exc:
                results.append({'id': target.id, 'host': host_label, 'ok': False, 'message': str(exc)})
            else:
                results.append({'id': target.id, 'host': host_label, 'ok': True, 'message': ''})

        success = sum(1 for item in results if item['ok'])
        return Response_200(data={
            'total': len(results),
            'success': success,
            'failed': len(results) - success,
            'results': results,
        })

    def get_queryset(self):
        from assets.host_online import sync_host_online_status_to_db

        # gRPC Registry 是 Agent 在线状态的唯一实时来源；同步后再序列化，避免陈旧 DB 状态误开放安装按钮。
        sync_host_online_status_to_db()
        return super().get_queryset()

    def list(self, request, *args, **kwargs):
        queryset = self.filter_queryset(self.get_queryset())
        page = self.paginate_queryset(queryset)
        serializer = self.get_serializer(page if page is not None else queryset, many=True)
        if page is None:
            return Response_200(data=serializer.data)
        paginator = self.paginator
        return Response_200(data={
            'count': paginator.page.paginator.count,
            'results': serializer.data,
            'pageNumber': paginator.page.number,
            'pageSize': paginator.page_size,
            'totalPages': paginator.page.paginator.num_pages,
            'next': paginator.get_next_link(),
            'previous': paginator.get_previous_link(),
        })

    def retrieve(self, request, *args, **kwargs):
        return Response_200(data=self.get_serializer(self.get_object()).data)

    def create(self, request, *args, **kwargs):
        serializer = self.get_serializer(data=request.data)
        if not serializer.is_valid():
            return Response_error_str('Fluent Bit 纳管目标配置无效', code=400, data=serializer.errors)
        serializer.save()
        return Response_200(data=serializer.data)

    @action(detail=True, methods=['post'], url_path='retry')
    def retry(self, request: Request, id=None) -> Response:
        """使用当前 Host 精确匹配的本地 RPM/DEB 重新安装 Fluent Bit。"""
        target = self.get_object()
        target.retry_count = 0
        target.install_message = '人工触发重新安装'
        target.save(update_fields=['retry_count', 'install_message', 'update_time'])
        try:
            dispatch_fluent_bit_install(target, manual=True)
        except LogCollectionApplyError as exc:
            target.install_status = LogCollectionTarget.InstallStatus.FAILED
            target.install_message = str(exc)
            target.save(update_fields=['install_status', 'install_message', 'update_time'])
            return Response_error_str(str(exc), code=400)
        target.refresh_from_db()
        return Response_200(data=self.get_serializer(target).data)

    @action(detail=True, methods=['post'], url_path='start-service')
    def start_service(self, request: Request, id=None) -> Response:
        target = self.get_object()
        if not target.agent_installed:
            return Response_error_str('Fluent Bit 尚未安装，无法启动服务', code=400)
        try:
            result = control_fluent_bit_service(target, 'start')
        except LogCollectionApplyError as exc:
            return Response_error_str(str(exc), code=400)
        return Response_200(data=result)

    @action(detail=True, methods=['post'], url_path='stop-service')
    def stop_service(self, request: Request, id=None) -> Response:
        target = self.get_object()
        if not target.agent_installed:
            return Response_error_str('Fluent Bit 尚未安装，无法停止服务', code=400)
        try:
            result = control_fluent_bit_service(target, 'stop')
        except LogCollectionApplyError as exc:
            return Response_error_str(str(exc), code=400)
        return Response_200(data=result)

    @action(detail=True, methods=['post'], url_path='cancel')
    def cancel_pending(self, request: Request, id=None) -> Response:
        """取消最新的 Fluent Bit 安装/卸载任务，执行器完成后不会覆盖取消状态。"""
        target = self.get_object()
        history = MonitorTargetInstallHistory.objects.filter(
            log_collection_target_id=target.id,
        ).order_by('-id').first()
        history_status = str(getattr(history, 'status', '') or '').lower()
        if history_status not in {
            MonitorTargetInstallHistory.Status.PENDING,
            MonitorTargetInstallHistory.Status.RUNNING,
        }:
            return Response_error_str('当前任务已结束，无需取消', code=400)

        now = timezone.now()
        history.status = MonitorTargetInstallHistory.Status.CANCELLED
        history.summary_message = '任务已取消'
        history.error_message_snapshot = '任务已由用户取消'
        history.end_time = now
        if history.start_time is not None:
            history.duration_seconds = (now - history.start_time).total_seconds()
        history.save(update_fields=[
            'status', 'summary_message', 'error_message_snapshot',
            'end_time', 'duration_seconds', 'update_time',
        ])
        target.install_status = LogCollectionTarget.InstallStatus.FAILED
        target.install_message = '安装/卸载任务已取消'
        target.save(update_fields=['install_status', 'install_message', 'update_time'])
        return Response_200(data=self.get_serializer(target).data)

    def destroy(self, request: Request, *args, **kwargs) -> Response:
        """已安装目标先下发卸载，由 worker 在卸载成功后删除纳管记录；卸载失败则保留记录和日志。"""
        target = self.get_object()
        if target.install_status == LogCollectionTarget.InstallStatus.PENDING:
            return Response_error_str('安装/卸载任务尚未结束，请先等待或取消任务', code=400)
        target_id = target.id
        if not target.agent_installed:
            target.delete()
            return Response_200(data={'id': target_id, 'pending_uninstall': False})
        try:
            dispatch_fluent_bit_uninstall(target, manual=True, delete_after_success=True)
        except LogCollectionApplyError as exc:
            return Response_error_str(str(exc), code=400)
        return Response_200(data={'id': target_id, 'pending_uninstall': True})

    @action(detail=True, methods=['get'], url_path='render-config')
    def render_config(self, request, id=None):
        """预览将为该主机生成的 Fluent Bit 片段与配置指纹，不写入主机。"""
        target = self.get_object()
        cluster = OpenSearchCluster.objects.filter(enabled=True).order_by('-is_default', 'id').first()
        if cluster is None:
            return Response_error_str('尚未配置日志存储集群', code=400)
        try:
            fragments = build_host_fragments(target.host, cluster)
        except ValueError as exc:
            return Response_error_str(str(exc), code=400)
        # 指纹一致表示配置无变化，下发时可直接跳过（§8.4）。
        fragments['up_to_date'] = fragments['fingerprint'] == target.config_fingerprint
        return Response_200(data=fragments)

    @action(detail=True, methods=['post'], url_path='apply')
    def apply(self, request, id=None):
        """实际下发配置到主机并触发热重载（§8.4）。指纹一致时跳过，返回 skipped=true。"""
        target = self.get_object()
        try:
            result = apply_host_log_config(target)
        except LogCollectionApplyError as exc:
            return Response_error_str(str(exc), code=400)
        return Response_200(data=result)

    @action(detail=True, methods=['post'], url_path='check-status')
    def check_status(self, request, id=None):
        """查询主机上 fluent-bit 的 systemd 状态并回写 agent_installed/runtime_status。"""
        target = self.get_object()
        try:
            result = refresh_target_status(target)
        except LogCollectionApplyError as exc:
            return Response_error_str(str(exc), code=400)
        return Response_200(data=result)

    @action(detail=True, methods=['get'], url_path='log-tail')
    def log_tail(self, request, id=None):
        """读取该主机上指定实例日志的最近 N 行，供解析调试拿真实样例（§5.4 闭环）。"""
        target = self.get_object()
        instance_name = str(request.query_params.get('instance_name') or '').strip()
        log_name = str(request.query_params.get('log_name') or '').strip()
        if not instance_name or not log_name:
            return Response_error_str('instance_name 与 log_name 均必填', code=400)
        try:
            lines = int(request.query_params.get('lines', 100))
        except (TypeError, ValueError):
            return Response_error_str('lines 必须是整数', code=400)
        try:
            result = read_instance_log_tail(target, instance_name, log_name, lines=lines)
        except LogCollectionApplyError as exc:
            return Response_error_str(str(exc), code=400)
        return Response_200(data=result)

    @action(detail=False, methods=['post'], url_path='batch-retry')
    def batch_retry(self, request: Request):
        """批量安装/重新安装 Fluent Bit。"""
        def handler(target):
            target.retry_count = 0
            target.install_message = '人工触发批量安装'
            target.save(update_fields=['retry_count', 'install_message', 'update_time'])
            try:
                dispatch_fluent_bit_install(target, manual=True)
            except LogCollectionApplyError:
                target.install_status = LogCollectionTarget.InstallStatus.FAILED
                target.save(update_fields=['install_status', 'update_time'])
                raise

        return self._run_batch(request, handler)

    @action(detail=False, methods=['post'], url_path='batch-start-service')
    def batch_start_service(self, request: Request):
        def handler(target):
            if not target.agent_installed:
                raise LogCollectionApplyError('Fluent Bit 尚未安装，无法启动服务')
            control_fluent_bit_service(target, 'start')

        return self._run_batch(request, handler)

    @action(detail=False, methods=['post'], url_path='batch-stop-service')
    def batch_stop_service(self, request: Request):
        def handler(target):
            if not target.agent_installed:
                raise LogCollectionApplyError('Fluent Bit 尚未安装，无法停止服务')
            control_fluent_bit_service(target, 'stop')

        return self._run_batch(request, handler)

    @action(detail=False, methods=['post'], url_path='batch-apply')
    def batch_apply(self, request: Request):
        def handler(target):
            apply_host_log_config(target)

        return self._run_batch(request, handler)

    @action(detail=False, methods=['post'], url_path='batch-delete')
    def batch_delete(self, request: Request):
        """批量删除：已安装的先下发卸载，卸载成功后由 worker 删除记录，避免主机上残留进程失联。"""
        def handler(target):
            if target.install_status == LogCollectionTarget.InstallStatus.PENDING:
                raise LogCollectionApplyError('安装/卸载任务尚未结束，请先等待或取消任务')
            if target.agent_installed:
                dispatch_fluent_bit_uninstall(target, manual=True, delete_after_success=True)
                return
            target.delete()

        return self._run_batch(request, handler)
