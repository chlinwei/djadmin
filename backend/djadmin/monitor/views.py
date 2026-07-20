from django_filters.rest_framework import DjangoFilterBackend
from rest_framework.decorators import action
from rest_framework.filters import OrderingFilter, SearchFilter
from rest_framework.mixins import CreateModelMixin, DestroyModelMixin, ListModelMixin, RetrieveModelMixin, UpdateModelMixin
from rest_framework.viewsets import GenericViewSet

import hashlib
import io
import re
import urllib.error
import urllib.request

from django.core.files.base import ContentFile

from djadmin.utils import Response_200, Response_error_str
from djadmin.utils import CustomPagination
from menu.permisssion import CustomMenuPermission

from .models import MonitorTarget, SoftwarePackage, build_node_exporter_official_url
from .prometheus_api import api_get, get_prometheus_base_url
from .serializer import MonitorTargetSerializer, SoftwarePackageSerializer

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
        'retry': 'monitor:view',
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

    @action(detail=True, methods=['post'], url_path='retry')
    def retry(self, request, id=None):
        """人工重试安装/卸载：自动重试耗尽（或任一失败）后，由用户手动重新下发同类型任务。"""
        target = self.get_object()
        host = target.host
        if host is None:
            return Response_error_str('监控目标未关联主机，无法重试', code=400)

        # 人工触发视为新一轮操作周期，重置自动重试计数
        target.retry_count = 0
        target.install_status = MonitorTarget.InstallStatus.PENDING
        target.install_message = '人工触发重试'
        target.save(update_fields=['retry_count', 'install_status', 'install_message', 'update_time'])

        from assets.views import dispatch_exporter_install_job, dispatch_exporter_uninstall_job

        if target.managed_enabled:
            dispatch_exporter_install_job(host, target, manual=True)
        else:
            dispatch_exporter_uninstall_job(host, target, manual=True)

        target.refresh_from_db()
        serializer = self.get_serializer(target)
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

    def retrieve(self, request, *args, **kwargs):
        return Response_200(data=self.get_serializer(self.get_object()).data)

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
