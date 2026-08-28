import warnings
import time
import uuid
import asyncio
import json
import importlib
import os
import logging
import threading
from pathlib import Path
from typing import Any, TYPE_CHECKING, cast
from copy import deepcopy
from types import SimpleNamespace
from datetime import timedelta
from concurrent.futures import ThreadPoolExecutor, as_completed
from asgiref.sync import async_to_sync

from cryptography.utils import CryptographyDeprecationWarning
from djadmin.utils import Response_200, Response_error_str
from rest_framework.mixins import CreateModelMixin,DestroyModelMixin,UpdateModelMixin,RetrieveModelMixin,ListModelMixin
from rest_framework.request import Request
from rest_framework.response import Response
from rest_framework.viewsets import GenericViewSet
from rest_framework.decorators import action, api_view
from .models import *
from .serializer import *
from .application_variables import ApplicationVariableError, resolve_application_variables, resolve_macro_variables
from djadmin.utils import CustomPagination
from rest_framework.filters import OrderingFilter,SearchFilter
from django_filters import rest_framework as drf_filters
from django_filters.rest_framework  import DjangoFilterBackend
from djadmin.errordict import DjadminException,AssetsError
from io import TextIOWrapper
import csv
from menu.permisssion import CustomMenuPermission
from django.db.models import Prefetch, Count
from django.db.models.deletion import ProtectedError
from django.db import IntegrityError, transaction, connections, close_old_connections, DatabaseError
from django.core.exceptions import ObjectDoesNotExist
from django.conf import settings
from django.http import JsonResponse
from django.utils import timezone
from .credential_crypto import decrypt_secret
from .webssh_runtime import WebSSHRuntimeRegistry
from .webssh_host_mixin import WebSSHHostMixin
from .host_info import refresh_host_info
from .grpc_transfer.client import AgentChannelClient, AgentGrpcTransferError
from .grpc_transfer.registry import REGISTRY
from .agent_install_service import _refresh_automation_job, run_agent_install_job

# 日志记录器
logger = logging.getLogger(__name__)


# 自动下发主机采集任务的最小间隔（秒）。
AGENT_AUTO_COLLECT_MIN_INTERVAL_SECONDS = 300


def _fetch_agent_runtime_status(host):
    agent_id = str(getattr(host, 'agent_id', '') or '').strip()
    if not agent_id:
        raise RuntimeError('主机未绑定 agent 实例，无法获取状态')

    try:
        client = AgentChannelClient(agent_id, timeout=20)
        result = client.execute_automation(
            job_id=f'agent-runtime-{uuid.uuid4().hex[:12]}',
            params={},
            timeout_seconds=20,
            task_type='custom',
            action='get_agent_runtime_status',
        )
    except AgentGrpcTransferError as exc:
        raise RuntimeError(str(exc)) from exc
    except Exception as exc:
        raise RuntimeError(f'通过 gRPC 获取 agent 状态失败: {exc}') from exc

    status = str(result.get('status') or '').strip().lower()
    if status != 'success':
        error_message = str(result.get('error_message') or '').strip()
        raise RuntimeError(error_message or f'agent 返回状态异常: {status or "unknown"}')

    data = result.get('result_data')
    if not isinstance(data, dict):
        raise RuntimeError('agent gRPC 状态响应缺少 data')
    return data


def _resolve_target_agent_ids(target_type, target_value, fallback_agent_id):
    """按目标类型解析要下发的 agent 列表。"""
    normalized_target_type = str(target_type or 'single').strip().lower()
    normalized_target_value = str(target_value or '').strip()
    normalized_agent_id = str(fallback_agent_id or '').strip()

    if normalized_target_type == 'single':
        final_agent_id = normalized_target_value or normalized_agent_id
        if final_agent_id == '':
            raise RuntimeError('agent_id不能为空')
        return [final_agent_id]

    if normalized_target_type == 'group':
        if normalized_target_value == '':
            raise RuntimeError('target_type=group 时 target_value不能为空')
        rows = Host.objects.filter(group__name=normalized_target_value).exclude(
            agent_id__isnull=True,
        ).exclude(agent_id='').values_list('agent_id', flat=True)
        return list(dict.fromkeys([str(item).strip() for item in rows if str(item).strip()]))

    if normalized_target_type == 'all':
        rows = Host.objects.exclude(agent_id__isnull=True).exclude(
            agent_id='',
        ).values_list('agent_id', flat=True)
        return list(dict.fromkeys([str(item).strip() for item in rows if str(item).strip()]))

    raise RuntimeError('target_type仅支持single/group/all')


def _emit_agent_job_event(job, event_type, payload):
    """写入 Salt 风格作业事件，便于后续接入事件总线。"""
    if job is None:
        return
    job_id = str(getattr(job, 'job_id', '') or '').strip()
    agent_id = str(getattr(job, 'agent_id', '') or '').strip()
    if event_type == 'new':
        tag = f'salt/job/{job_id}/new'
    elif event_type == 'ret':
        safe_agent_id = agent_id or 'unknown'
        tag = f'salt/job/{job_id}/ret/{safe_agent_id}'
    else:
        tag = f'salt/job/{job_id}/{event_type}'

    AgentJobEvent.objects.create(
        tag=tag,
        job_id=job_id,
        agent_id=agent_id,
        event_type=event_type,
        payload=payload if isinstance(payload, dict) else {},
    )

def _dispatch_agent_job_via_grpc(job):
    """通过 agent 主动拨入的 gRPC 通道同步执行通用 AgentJob，并回写最终结果。"""
    if job is None:
        return

    job_id = str(getattr(job, 'job_id', '') or '').strip()
    agent_id = str(getattr(job, 'agent_id', '') or '').strip()
    if not job_id or not agent_id:
        return

    current_job = job
    current_job.status = AgentJob.JobStatus.RUNNING
    current_job.picked_at = timezone.now()
    current_job.save(update_fields=['status', 'picked_at', 'update_time'])
    _emit_agent_job_event(current_job, 'run', {'job_id': job_id, 'agent_id': agent_id})

    try:
        client = AgentChannelClient(agent_id, timeout=20)
        result = client.execute_automation(
            job_id=job_id,
            params=current_job.params if isinstance(current_job.params, dict) else {},
            timeout_seconds=int(current_job.timeout_seconds or 30),
            task_type=str(current_job.job_type or 'custom'),
            action=str(current_job.action or ''),
        )
        status = str(result.get('status') or AgentJob.JobStatus.FAILED).lower()
        if status not in {
            AgentJob.JobStatus.SUCCESS,
            AgentJob.JobStatus.FAILED,
            AgentJob.JobStatus.CANCELED,
            AgentJob.JobStatus.TIMEOUT,
        }:
            status = AgentJob.JobStatus.FAILED
        current_job.status = status
        current_job.result_data = result.get('result_data') if isinstance(result.get('result_data'), dict) else {}
        current_job.error_message = str(result.get('error_message') or '')
        current_job.stdout = str(result.get('stdout') or '')
        current_job.stderr = str(result.get('stderr') or '')
        exit_code = result.get('exit_code')
        current_job.exit_code = int(exit_code) if exit_code is not None else -1
    except Exception as exc:
        current_job.status = AgentJob.JobStatus.FAILED
        current_job.error_message = str(exc)
    current_job.finished_at = timezone.now()
    current_job.save(update_fields=[
        'status', 'result_data', 'error_message', 'stdout', 'stderr',
        'exit_code', 'finished_at', 'update_time',
    ])
    _emit_agent_job_event(current_job, 'ret', {
        'job_id': job_id,
        'agent_id': agent_id,
        'status': current_job.status,
    })
    return current_job

def _normalize_agent_job_params(payload):
    raw_params = payload.get('params') if isinstance(payload, dict) else {}
    params = dict(raw_params) if isinstance(raw_params, dict) else {}
    for key in ('command', 'args', 'work_dir', 'env'):
        if key in payload and key not in params:
            params[key] = payload[key]
    return params


class CredentialManage(GenericViewSet,CreateModelMixin,UpdateModelMixin,RetrieveModelMixin,ListModelMixin,DestroyModelMixin):
    # 临时凭证有关联的 WebSSHTempCredential 记录，排除在凭证管理列表之外
    queryset = Credential.objects.filter(temp_credential_info__isnull=True)
    serializer_class = CredentialSerializer
    pagination_class = CustomPagination
    filter_backends = (OrderingFilter,DjangoFilterBackend,SearchFilter)
    search_fields = ['name', 'remark'] 
    filterset_fields = ['id', 'auth_type']
    ordering_fields = [ 'name','create_time'] 
    lookup_field = 'id'
    permission_classes = [CustomMenuPermission]
    action_perms_map = {
        # view
        'list': 'assets:credentials:view',
        'retrieve': 'assets:credentials:view',
        # delete
        'destroy': 'assets:credentials:delete',
        'batch-delete': 'assets:credentials:delete',
        # update
        'partial_update': 'assets:credentials:update',
        'perform_update': 'assets:credentials:update',
        # create
        'create': 'assets:credentials:create',
        'batch-create': 'assets:credentials:create',
    }

    def list(self, request, *args, **kwargs):
        queryset = self.filter_queryset(self.get_queryset())
        page = self.paginate_queryset(queryset)
        serializer = self.get_serializer(page if page is not None else queryset, many=True)
        data = serializer.data
        if page is not None:
            paginator = self.paginator
            return Response_200(data={
                'count': paginator.page.paginator.count,
                'results': data,
                'pageNumber': paginator.page.number,
                'pageSize': paginator.page_size,
                'totalPages': paginator.page.paginator.num_pages,
                'next': paginator.get_next_link(),
                'previous': paginator.get_previous_link(),
            })
        return Response_200(data=data)

    def retrieve(self, request, *args, **kwargs):
        instance = self.get_object()
        serializer = self.get_serializer(instance)
        return Response_200(data=serializer.data)

    def create(self, request, *args, **kwargs):
        # 检测临时凭证标志（前端传入 is_temporary=true），剔除后单独处理
        is_temporary = bool(request.data.get('is_temporary', False))
        data = {k: v for k, v in request.data.items() if k != 'is_temporary'}
        serializer = self.get_serializer(data=data)
        serializer.is_valid(raise_exception=True)
        instance = serializer.save()
        # 临时凭证：创建关联记录，后续 WebSSH 会话关闭时自动删除
        if is_temporary:
            from .models import WebSSHTempCredential
            WebSSHTempCredential.objects.create(credential=instance)
        return Response_200(data=serializer.data)

    def _force_close_webssh_sessions_for_credential(self, credential_id):
        # 仅影响“把该凭证作为默认凭证”的主机，避免误踢无关会话。
        host_ids = list(
            HostCredential.objects.filter(
                credential_id=credential_id,
                is_default=True,
            ).values_list('host_id', flat=True)
        )
        if not host_ids:
            return 0
        return async_to_sync(WebSSHRuntimeRegistry.close_active_sessions_for_hosts)(
            host_ids,
            message='SSH 凭证已变更，连接已关闭',
            close_code=4411,
        )

    @staticmethod
    def _get_credential_connection_signature(credential):
        # 连接签名用于判断“会话连接参数”是否发生变化。
        decrypted_password = decrypt_secret(credential.password)
        return (
            str(credential.username or ''),
            str(decrypted_password or ''),
            str(credential.private_key or ''),
            int(credential.port or 22),
            int(credential.auth_type or 0),
        )

    def update(self, request, *args, **kwargs):
        instance = self.get_object()
        # 先比较变更前后签名，只有连接关键字段变化才断开会话。
        previous_signature = self._get_credential_connection_signature(instance)
        serializer = self.get_serializer(instance, data=request.data, partial=False)
        serializer.is_valid(raise_exception=True)
        serializer.save()
        instance.refresh_from_db()
        current_signature = self._get_credential_connection_signature(instance)
        if previous_signature != current_signature:
            self._force_close_webssh_sessions_for_credential(instance.id)
        return Response_200(data=serializer.data)

    def partial_update(self, request, *args, **kwargs):
        instance = self.get_object()
        previous_signature = self._get_credential_connection_signature(instance)
        serializer = self.get_serializer(instance, data=request.data, partial=True)
        serializer.is_valid(raise_exception=True)
        serializer.save()
        instance.refresh_from_db()
        current_signature = self._get_credential_connection_signature(instance)
        if previous_signature != current_signature:
            self._force_close_webssh_sessions_for_credential(instance.id)
        return Response_200(data=serializer.data)

    def destroy(self, request: Request, *args, **kwargs) -> Response:
        instance = self.get_object()
        deleted_id = instance.id
        self.perform_destroy(instance)
        return Response_200(data={"id": deleted_id})

    # 批量删除credential
    @action(detail=False,methods=['delete'],url_path='batch-delete')
    def batchDelete(self,request):
        # 获取ID数组参数
        ids = request.data.get('ids', [])
        # 先查用户角色列表
        Credential.objects.filter(id__in=ids).delete()
        return Response_200()
    # 批量导入credential
    @action(detail=False,methods=['post'],url_path='batch-create')
    def batchCreate(self,request):
        file = request.FILES.get('file')
        if not file.name.endswith('csv'):
            raise DjadminException(AssetsError.FILE_NOT_ENDSWITH_CSV)
        decoded_file = TextIOWrapper(file.file, encoding='utf-8')
        reader = csv.DictReader(decoded_file)
        input_datas = []
        for row in reader:
            input_datas.append(row)
        serializer = self.get_serializer(data=input_datas, many=True)
        if serializer.is_valid():
            serializer.save()
            return Response_200(serializer.data)
        raise DjadminException(AssetsError.BATCH_UPLOAD_ERROR,serializer.errors)
        

# Mixin 仅提供响应封装，运行时不入 MRO实体基类；仅在类型检查时挂到 GenericViewSet 以获得 self.* 提示。
if TYPE_CHECKING:
    _ApplicationResponseBase = GenericViewSet
else:
    _ApplicationResponseBase = object


class ApplicationResponseMixin(_ApplicationResponseBase):
    """统一应用类 ViewSet 的 {code,msg,data} 响应与分页结构，避免每个类重复实现。"""

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


class ApplicationManage(ApplicationResponseMixin, GenericViewSet,CreateModelMixin,UpdateModelMixin,RetrieveModelMixin,ListModelMixin):
    queryset = Application.objects.prefetch_related('versions').annotate(
        version_count=Count('versions', distinct=True),
        deployment_template_count=Count('deployment_templates', distinct=True),
        deployment_count=Count('services__deployments', distinct=True),
    ).order_by('-id')
    serializer_class = ApplicationSerializer
    pagination_class = CustomPagination
    filter_backends = (OrderingFilter,DjangoFilterBackend,SearchFilter)
    search_fields = ['name', 'remark'] 
    ordering_fields = [ 'name','create_time'] 
    lookup_field = 'id'
    permission_classes = [CustomMenuPermission]
    action_perms_map = {
        # view
        'list': 'assets:applications:view',
        'retrieve': 'assets:applications:view',
        # delete
        'batch-delete': 'assets:applications:delete',
        # update
        'partial_update': 'assets:applications:update',
        'perform_update': 'assets:applications:update',
        # create
        'create': 'assets:applications:create',
    }

    # 批量删除Application
    @action(detail=False,methods=['delete'],url_path='batch-delete')
    def batchDelete(self,request):
        # 获取ID数组参数
        ids = request.data.get('ids', [])
        # 先查用户角色列表
        try:
            Application.objects.filter(id__in=ids).delete()
        except ProtectedError:
            return Response_error_str('应用仍有部署实例，请先删除相关部署实例', code=400)
        return Response_200()


class ApplicationAssetManageBase(ApplicationResponseMixin, GenericViewSet, CreateModelMixin, DestroyModelMixin, UpdateModelMixin, RetrieveModelMixin, ListModelMixin):
    pagination_class = CustomPagination
    filter_backends = (OrderingFilter, DjangoFilterBackend, SearchFilter)
    lookup_field = 'id'
    permission_classes = [CustomMenuPermission]
    action_perms_map = {
        'list': 'assets:applications:view',
        'retrieve': 'assets:applications:view',
        'create': 'assets:applications:create',
        'destroy': 'assets:applications:delete',
        'update': 'assets:applications:update',
        'partial_update': 'assets:applications:update',
        'perform_update': 'assets:applications:update',
    }

    def destroy(self, request, *args, **kwargs):
        try:
            self.get_object().delete()
        except ProtectedError:
            return Response_error_str('记录仍被部署实例引用，无法删除', code=400)
        return Response_200()


class ApplicationVersionManage(ApplicationAssetManageBase):
    queryset = ApplicationVersion.objects.select_related('application').all()
    serializer_class = ApplicationVersionSerializer
    search_fields = ['version', 'application__name', 'remark']
    ordering_fields = ['version', 'create_time', 'update_time']
    filterset_fields = ['application', 'enabled']


class BusinessSystemManage(ApplicationAssetManageBase):
    queryset = BusinessSystem.objects.select_related('project').annotate(
        deployment_count=Count('services__deployments', distinct=True),
        environment_count=Count('services__environment', distinct=True),
    ).all()
    serializer_class = BusinessSystemSerializer
    search_fields = ['name', 'code', 'owner', 'project__name', 'remark']
    ordering_fields = ['name', 'code', 'create_time', 'update_time']
    filterset_fields = ['enabled', 'project']


class ProjectManage(ApplicationAssetManageBase):
    queryset = Project.objects.prefetch_related('business_systems').all()
    serializer_class = ProjectSerializer
    search_fields = ['name', 'code', 'owner', 'business_systems__name', 'remark']
    ordering_fields = ['name', 'code', 'create_time', 'update_time']
    filterset_fields = ['enabled', 'business_systems']


class BusinessEnvironmentManage(ApplicationAssetManageBase):
    queryset = BusinessEnvironment.objects.annotate(
        service_count=Count('services', distinct=True),
        deployment_count=Count('services__deployments', distinct=True),
    ).all()
    serializer_class = BusinessEnvironmentSerializer
    search_fields = ['name', 'code', 'owner', 'remark']
    ordering_fields = ['name', 'code', 'order', 'create_time', 'update_time']
    filterset_fields = ['enabled']


class ClusterProfileManage(ApplicationAssetManageBase):
    queryset = ClusterProfile.objects.select_related('application').annotate(service_count=Count('services')).all()
    serializer_class = ClusterProfileSerializer
    search_fields = ['name', 'code', 'application__name', 'remark']
    ordering_fields = ['name', 'code', 'create_time', 'update_time']
    filterset_fields = ['application', 'profile_type', 'cluster_type', 'enabled']

    def destroy(self, request, *args, **kwargs):
        if self.get_object().profile_type == ClusterProfile.ProfileType.BUILTIN:
            return Response_error_str('内置集群模型由系统维护，不能删除', code=400)
        return super().destroy(request, *args, **kwargs)


class ApplicationServiceFilter(drf_filters.FilterSet):
    business_system = drf_filters.NumberFilter(field_name='business_system')

    class Meta:
        model = ApplicationService
        fields = [
            'business_system', 'application', 'environment', 'topology_type',
            'cluster_profile', 'enabled',
        ]


class ApplicationServiceManage(ApplicationAssetManageBase):
    queryset = ApplicationService.objects.select_related(
        'business_system', 'environment', 'application', 'cluster_profile',
    ).annotate(deployment_count=Count('deployments', distinct=True)).all()
    serializer_class = ApplicationServiceSerializer
    search_fields = ['name', 'code', 'business_system__name', 'environment__name', 'application__name', 'access_address', 'remark']
    ordering_fields = ['name', 'code', 'create_time', 'update_time']
    filterset_class = ApplicationServiceFilter
    action_perms_map = {
        **ApplicationAssetManageBase.action_perms_map,
        'check_baseline': 'assets:applications:update',
        'refresh_runtime_status': 'assets:applications:update',
    }

    def create(self, request: Request, *args, **kwargs) -> Response:
        request_data = cast(dict[str, Any], request.data)
        data = {key: value for key, value in request_data.items() if key != 'draft'}
        serializer = self.get_serializer(data=data)
        serializer.is_valid(raise_exception=True)
        serializer.save()
        return Response_200(data=serializer.data)

    def retrieve(self, request: Request, *args, **kwargs) -> Response:
        service = self.get_object()
        profile = service.cluster_profile
        if profile and profile.cluster_type == ClusterProfile.ClusterType.HA and service.access_address.strip():
            deployments = list(service.deployments.select_related('host').all())
            roles = {
                deployment.id: (
                    ApplicationDeployment.HaRole.PRIMARY
                    if str(deployment.host.ip or '').strip() == service.access_address.strip()
                    else ApplicationDeployment.HaRole.STANDBY
                )
                for deployment in deployments
            }
            for deployment in deployments:
                role = roles[deployment.id]
                if deployment.ha_role != role:
                    deployment.ha_role = role
                    deployment.save(update_fields=['ha_role', 'update_time'])
        return Response_200(data=self.get_serializer(service).data)

    @action(detail=True, methods=['post'], url_path='refresh-runtime-status')
    def refresh_runtime_status(self, request: Request, *args, **kwargs) -> Response:
        """逐个查询该服务全部启用实例的运行状态；控制命令来自服务绑定的部署模板。"""
        service = self.get_object()
        deployments = list(ApplicationDeployment.objects.filter(
            service_links__service=service,
            service_links__enabled=True,
            enabled=True,
        ).select_related('host').distinct())
        if not deployments:
            return Response_error_str('该逻辑服务没有启用的部署实例', code=400)

        def check(deployment):
            error_message = ''
            try:
                _, error_message = _run_deployment_control(deployment, 'status')
            except Exception as error:  # 单个实例失败不能中断整体查询
                error_message = str(error)
            finally:
                # 线程池复用线程会保留 Django 线程级连接，用完主动关闭避免连接泄漏。
                close_old_connections()
            return {
                'deployment': deployment.pk,
                'instance_name': deployment.instance_name,
                'host_name': deployment.host.instance_name,
                'runtime_status': deployment.runtime_status,
                'last_status_check_time': deployment.last_status_check_time,
                'error': error_message,
            }

        with ThreadPoolExecutor(max_workers=min(8, len(deployments))) as executor:
            results = list(executor.map(check, deployments))

        statuses = [item['runtime_status'] for item in results]
        return Response_200(data={
            'service': service.pk,
            'results': results,
            'summary': {
                'total': len(results),
                'running': statuses.count(ApplicationDeployment.RuntimeStatus.RUNNING),
                'stopped': statuses.count(ApplicationDeployment.RuntimeStatus.STOPPED),
                'error': statuses.count(ApplicationDeployment.RuntimeStatus.ERROR),
            },
        })

    @action(detail=True, methods=['post'], url_path='check-baseline')
    def check_baseline(self, request: Request, *args, **kwargs) -> Response:
        service = self.get_object()
        if service.topology_type != ApplicationService.TopologyType.CLUSTER:
            return Response_error_str('只有实际集群可以执行集群基线检查', code=400)

        deployments = list(ApplicationDeployment.objects.filter(
            service_links__service=service,
        ).select_related('host'))
        links_by_deployment = {
            link.deployment_id: link
            for link in ApplicationServiceDeployment.objects.filter(service=service)
        }
        results = []

        def add_result(key: str, name: str, passed: bool, expected, actual) -> None:
            results.append({
                'key': key,
                'name': name,
                'status': 'pass' if passed else 'fail',
                'expected': expected,
                'actual': actual,
            })

        is_ha = service.cluster_profile.cluster_type == ClusterProfile.ClusterType.HA
        minimum_member_count = 2 if is_ha else 1
        add_result(
            'members', '集群成员', len(deployments) >= minimum_member_count,
            f'至少 {minimum_member_count} 个成员', len(deployments),
        )
        consistent_members = [
            deployment.id for deployment in deployments
            if deployment.application_version.application_id == service.application_id
            and deployment.deployment_template.application_id == service.application_id
        ]
        add_result(
            'application_consistency',
            '成员应用一致性',
            len(consistent_members) == len(deployments),
            service.application.name,
            f'{len(consistent_members)}/{len(deployments)} 个成员一致',
        )

        if is_ha:
            add_result(
                'vip', 'VIP 配置',
                bool(service.access_address.strip()),
                'VIP 地址非空', service.access_address or '未配置',
            )
        passed_count = sum(result['status'] == 'pass' for result in results)
        pass_rate = round(passed_count * 100 / len(results), 2)
        service.health_status = (
            ApplicationService.HealthStatus.HEALTHY
            if passed_count == len(results)
            else ApplicationService.HealthStatus.UNHEALTHY
        )
        service.baseline_pass_rate = pass_rate
        service.last_check_time = timezone.now()
        service.save(update_fields=['health_status', 'baseline_pass_rate', 'last_check_time', 'update_time'])
        return Response_200(data={
            'health_status': service.health_status,
            'baseline_pass_rate': pass_rate,
            'last_check_time': service.last_check_time,
            'results': results,
        })


class ApplicationDeploymentTemplateManage(ApplicationAssetManageBase):
    queryset = ApplicationDeploymentTemplate.objects.select_related('application').prefetch_related(
        'ports', 'paths', 'config_files', 'logs', 'control_actions',
    )
    serializer_class = ApplicationDeploymentTemplateSerializer
    search_fields = ['name', 'application__name', 'service_name', 'app_home', 'run_user', 'remark']
    ordering_fields = ['name', 'create_time', 'update_time']
    filterset_fields = ['application', 'control_type', 'enabled']


def _get_optional_related(instance, field_name):
    try:
        return getattr(instance, field_name)
    except ObjectDoesNotExist:
        return None


def _run_deployment_control(deployment, control_action):
    """执行部署实例控制动作并落库运行状态，返回 (completed_job, error_message)。"""
    agent_id = str(deployment.host.agent_id or '').strip()
    if agent_id == '':
        return None, '部署主机未绑定 Agent 实例，无法执行应用控制'

    params = _build_application_baseline_params(deployment)
    params['control_action'] = control_action
    action_config = params.get('control_actions', {}).get(control_action, {})
    timeout_seconds = int(action_config.get('timeout_seconds') or 30) if isinstance(action_config, dict) else 30
    job = AgentJob.objects.create(
        job_id=f'app-control-{uuid.uuid4().hex[:16]}',
        agent_id=agent_id,
        host=deployment.host,
        job_type='custom',
        action='control_application',
        params=params,
        timeout_seconds=timeout_seconds,
        status=AgentJob.JobStatus.QUEUED,
    )
    _emit_agent_job_event(job, 'new', {
        'jid': job.job_id,
        'tgt': job.agent_id,
        'tgt_type': 'agent_id',
        'fun': job.action,
        'arg': job.params,
        'minions': [job.agent_id],
    })
    completed_job = _dispatch_agent_job_via_grpc(job)
    if completed_job is None or completed_job.status != AgentJob.JobStatus.SUCCESS:
        error_message = (
            getattr(completed_job, 'error_message', '')
            or getattr(completed_job, 'stderr', '')
            or '应用控制执行失败'
        )
        if control_action == 'status':
            deployment.runtime_status = ApplicationDeployment.RuntimeStatus.ERROR
            deployment.runtime_status_output = str(error_message)[:4000]
            deployment.last_status_check_time = timezone.now()
            if deployment.deployment_template.control_type == ApplicationDeploymentTemplate.ControlType.EXTERNAL_HA:
                deployment.ha_role = ApplicationDeployment.HaRole.UNKNOWN
            deployment.save(update_fields=[
                'runtime_status', 'runtime_status_output', 'last_status_check_time', 'ha_role', 'update_time',
            ])
        return completed_job, str(error_message)

    if control_action == 'status':
        deployment.runtime_status = (
            ApplicationDeployment.RuntimeStatus.RUNNING
            if completed_job.exit_code == 0
            else ApplicationDeployment.RuntimeStatus.STOPPED
        )
        deployment.runtime_status_output = str(completed_job.stdout or completed_job.stderr or '')[:4000]
        deployment.last_status_check_time = timezone.now()
        deployment.save(update_fields=[
            'runtime_status', 'runtime_status_output', 'last_status_check_time', 'ha_role', 'update_time',
        ])
    else:
        # 启停命令成功仅代表动作已受理，旧状态必须失效并由后续状态检查重新确认。
        deployment.runtime_status = ApplicationDeployment.RuntimeStatus.UNKNOWN
        deployment.runtime_status_output = ''
        deployment.last_status_check_time = None
        deployment.save(update_fields=[
            'runtime_status', 'runtime_status_output', 'last_status_check_time', 'update_time',
        ])
    return completed_job, ''


def _build_application_baseline_params(deployment):
    deployment_template = deployment.deployment_template
    run_user = str(deployment_template.run_user or '').strip()
    app_home = resolve_application_variables(
        deployment_template.app_home,
        app_home=deployment_template.app_home,
        run_user=run_user,
    )

    def resolve_template_value(value):
        macro_values = {
            item['name']: item.get('value', '')
            for item in (deployment_template.macro_definitions or [])
            if isinstance(item, dict) and item.get('name')
        }
        macro_values.update(deployment.service.macro_values if deployment.service else {})
        return resolve_macro_variables(
            resolve_application_variables(value, app_home=app_home, run_user=run_user),
            definitions=deployment_template.macro_definitions or [],
            values=macro_values,
        )

    ports = [
        {
            'name': port.name,
            'protocol': port.protocol,
            'port': port.port,
        }
        for port in deployment_template.ports.filter(check_enabled=True)
    ]
    logs = [
        {
            'name': log.name,
            'path_pattern': resolve_template_value(log.path_pattern),
        }
        for log in deployment_template.logs.all()
    ]
    paths = [
        {
            'name': path.name,
            'path': resolve_template_value(path.path),
            'expected_owner': resolve_template_value(path.expected_owner),
            'expected_group': resolve_template_value(path.expected_group),
            'expected_mode': path.expected_mode,
        }
        for path in deployment_template.paths.filter(check_enabled=True)
    ]
    if app_home and not any(item['path'] == app_home for item in paths):
        paths.append({
            'name': 'App Home',
            'path': app_home,
            'expected_owner': run_user,
            'expected_group': deployment_template.run_group,
            'expected_mode': '',
        })
    for config_file in deployment_template.config_files.filter(baseline_enabled=True):
        config_path = resolve_template_value(config_file.path)
        if not any(item['path'] == config_path for item in paths):
            paths.append({
                'name': f'配置文件: {config_file.name}',
                'path': config_path,
                'expected_owner': '',
                'expected_group': '',
                'expected_mode': '',
            })

    params = {
        'control_type': deployment_template.control_type,
        'app_home': app_home,
        'work_directory': resolve_template_value(deployment_template.work_directory),
        'service_name': deployment_template.service_name,
        'systemd_scope': deployment_template.systemd_scope,
        'run_user': run_user,
        'ports': ports,
        'logs': logs,
        'paths': paths,
        'control_actions': {
            action.action: {
                'command': resolve_template_value(action.command),
                'timeout_seconds': action.timeout_seconds,
                'success_exit_codes': action.success_exit_codes,
            }
            for action in deployment_template.control_actions.all()
        },
    }
    docker_config = _get_optional_related(deployment_template, 'docker_config')
    if docker_config is not None:
        params['docker_config'] = {
            'container_name': docker_config.container_name,
            'docker_host': docker_config.docker_host,
            'expected_image': docker_config.expected_image,
            'expected_image_tag': docker_config.expected_image_tag,
        }
    compose_config = _get_optional_related(deployment_template, 'compose_config')
    if compose_config is not None:
        params['compose_config'] = {
            'project_name': compose_config.project_name,
            'service_name': compose_config.service_name,
            'compose_file_path': resolve_template_value(compose_config.compose_file_path),
            'working_directory': resolve_template_value(compose_config.working_directory),
            'env_file': resolve_template_value(compose_config.env_file),
            'expected_image': compose_config.expected_image,
            'expected_image_tag': compose_config.expected_image_tag,
        }
    return params


def _run_application_baseline_check(execution_id, job_id):
    close_old_connections()
    try:
        execution = ApplicationBaselineExecution.objects.select_related('deployment').get(id=execution_id)
        job = AgentJob.objects.get(job_id=job_id)
        execution.status = ApplicationBaselineExecution.Status.RUNNING
        execution.start_time = timezone.now()
        execution.save(update_fields=['status', 'start_time', 'update_time'])

        completed_job = _dispatch_agent_job_via_grpc(job)
        if completed_job is None:
            raise RuntimeError('Agent 基线检查未返回任务结果')
        if completed_job.status != AgentJob.JobStatus.SUCCESS:
            raise RuntimeError(completed_job.error_message or completed_job.stderr or 'Agent 基线检查执行失败')

        result_data = completed_job.result_data if isinstance(completed_job.result_data, dict) else {}
        raw_checks = result_data.get('checks')
        checks = raw_checks if isinstance(raw_checks, list) else []
        valid_statuses = {item[0] for item in ApplicationBaselineResult.Status.choices}
        result_rows = []
        for item in checks:
            if not isinstance(item, dict):
                continue
            status_value = str(item.get('status') or 'error')
            if status_value not in valid_statuses:
                status_value = ApplicationBaselineResult.Status.ERROR
            result_rows.append(ApplicationBaselineResult(
                execution=execution,
                check_key=str(item.get('key') or ''),
                check_type=str(item.get('type') or ''),
                name=str(item.get('name') or ''),
                status=status_value,
                expected_value=item.get('expected'),
                actual_value=item.get('actual'),
                message=str(item.get('message') or ''),
            ))

        evaluated_rows = [row for row in result_rows if row.status != ApplicationBaselineResult.Status.SKIPPED]
        passed_count = sum(1 for row in evaluated_rows if row.status == ApplicationBaselineResult.Status.PASS)
        pass_rate = round((passed_count / len(evaluated_rows)) * 100, 2) if evaluated_rows else 100.0
        passed = bool(result_data.get('passed'))
        now = timezone.now()
        with transaction.atomic():
            ApplicationBaselineResult.objects.bulk_create(result_rows)
            execution.status = ApplicationBaselineExecution.Status.COMPLETED
            execution.passed = passed
            execution.total_count = len(result_rows)
            execution.passed_count = passed_count
            execution.summary = result_data
            execution.end_time = now
            execution.save(update_fields=[
                'status', 'passed', 'total_count', 'passed_count', 'summary', 'end_time', 'update_time',
            ])
            deployment = execution.deployment
            deployment.health_status = (
                ApplicationDeployment.HealthStatus.HEALTHY
                if passed else ApplicationDeployment.HealthStatus.UNHEALTHY
            )
            deployment.baseline_pass_rate = pass_rate
            deployment.last_check_time = now
            deployment.save(update_fields=['health_status', 'baseline_pass_rate', 'last_check_time', 'update_time'])
    except Exception as exc:
        now = timezone.now()
        ApplicationBaselineExecution.objects.filter(id=execution_id).update(
            status=ApplicationBaselineExecution.Status.FAILED,
            passed=False,
            error_message=str(exc),
            end_time=now,
            update_time=now,
        )
        ApplicationDeployment.objects.filter(baseline_executions__id=execution_id).update(
            health_status=ApplicationDeployment.HealthStatus.ERROR,
            last_check_time=now,
            update_time=now,
        )
    finally:
        close_old_connections()


class ApplicationDeploymentFilter(drf_filters.FilterSet):
    # 保持既有查询参数名不变，实际过滤路径改为经由环境实体到业务系统。
    application_service__business_system = drf_filters.NumberFilter(
        field_name='application_services__business_system',
    )
    application_service = drf_filters.NumberFilter(field_name='application_services')
    application_service__environment = drf_filters.NumberFilter(field_name='application_services__environment')
    # 版本与模板已收收到逻辑服务，过滤同样经由服务关联。
    application_version = drf_filters.NumberFilter(field_name='application_services__application_version')
    application_version__application = drf_filters.NumberFilter(field_name='application_services__application')
    deployment_template = drf_filters.NumberFilter(field_name='application_services__deployment_template')

    class Meta:
        model = ApplicationDeployment
        fields = [
            'id', 'application_version', 'application_version__application', 'deployment_template',
            'application_service', 'application_service__business_system',
            'application_service__environment', 'host', 'enabled',
        ]


class ApplicationDeploymentManage(ApplicationAssetManageBase):
    queryset = ApplicationDeployment.objects.select_related(
        'host',
    ).prefetch_related(
        'application_services__application_version__application',
        'application_services__deployment_template__ports',
        'application_services__business_system',
        'application_services__cluster_profile',
    )
    serializer_class = ApplicationDeploymentSerializer
    search_fields = [
        'instance_name', 'application_services__application__name',
        'application_services__application_version__version', 'host__instance_name', 'host__ip',
    ]
    ordering_fields = ['instance_name', 'create_time', 'update_time']
    filterset_class = ApplicationDeploymentFilter
    action_perms_map = {
        **ApplicationAssetManageBase.action_perms_map,
        'control': 'assets:applications:update',
        'check_baseline': 'assets:applications:update',
        'baseline_history': 'assets:applications:view',
    }

    @action(detail=True, methods=['post'], url_path='control')
    def control(self, request, *args, **kwargs):
        deployment = self.get_object()
        control_action = str(request.data.get('action') or '').strip().lower()
        if control_action not in {'start', 'stop', 'status'}:
            return Response_error_str('应用控制动作必须为 start、stop 或 status', code=400)

        completed_job, error_message = _run_deployment_control(deployment, control_action)
        if error_message:
            return Response_error_str(error_message, code=400)
        return Response_200(data={
            'job_id': completed_job.job_id,
            'action': control_action,
            'status': completed_job.status,
            'output': completed_job.stdout,
            'exit_code': completed_job.exit_code,
            'runtime_status': deployment.runtime_status,
            'last_status_check_time': deployment.last_status_check_time,
            'result_data': completed_job.result_data,
        })

    @action(detail=True, methods=['post'], url_path='check-baseline')
    def check_baseline(self, request, *args, **kwargs):
        deployment = self.get_object()
        agent_id = str(deployment.host.agent_id or '').strip()
        if agent_id == '':
            return Response_error_str('部署主机未绑定 Agent 实例，无法执行检查', code=400)
        if ApplicationBaselineExecution.objects.filter(
            deployment=deployment,
            status__in=[ApplicationBaselineExecution.Status.QUEUED, ApplicationBaselineExecution.Status.RUNNING],
        ).exists():
            return Response_error_str('该部署实例已有检查任务正在执行', code=400)

        job_id = f'app-baseline-{uuid.uuid4().hex[:16]}'
        params = _build_application_baseline_params(deployment)
        with transaction.atomic():
            job = AgentJob.objects.create(
                job_id=job_id,
                agent_id=agent_id,
                host=deployment.host,
                job_type='custom',
                action='check_application_baseline',
                params=params,
                timeout_seconds=30,
                status=AgentJob.JobStatus.QUEUED,
            )
            user = getattr(request, 'user', None)
            execution = ApplicationBaselineExecution.objects.create(
                deployment=deployment,
                agent_job=job,
                requested_user_id=getattr(user, 'id', None),
                requested_username=str(getattr(user, 'username', '') or ''),
            )
            deployment.health_status = ApplicationDeployment.HealthStatus.CHECKING
            deployment.save(update_fields=['health_status', 'update_time'])
            _emit_agent_job_event(job, 'new', {
                'jid': job.job_id,
                'tgt': job.agent_id,
                'tgt_type': 'agent_id',
                'fun': job.action,
                'arg': job.params,
                'minions': [job.agent_id],
            })

        thread = threading.Thread(
            target=_run_application_baseline_check,
            args=(execution.id, job.job_id),
            daemon=True,
            name=f'application-baseline-{execution.id}',
        )
        thread.start()
        return Response_200(data={'execution_id': execution.id, 'job_id': job.job_id, 'status': execution.status})

    @action(detail=True, methods=['get'], url_path='baseline-history')
    def baseline_history(self, request, *args, **kwargs):
        deployment = self.get_object()
        executions = deployment.baseline_executions.select_related(
            'agent_job', 'deployment__host',
        ).prefetch_related('results')[:20]
        serializer = ApplicationBaselineExecutionSerializer(executions, many=True)
        return Response_200(data=serializer.data)


class HostGroupManage(GenericViewSet,CreateModelMixin,DestroyModelMixin,UpdateModelMixin,RetrieveModelMixin,ListModelMixin):
    queryset = HostGroup.objects.all()
    serializer_class = HostGroupSerializer
    pagination_class = CustomPagination
    filter_backends = (OrderingFilter, DjangoFilterBackend, SearchFilter)
    search_fields = ['name', 'remark']
    ordering_fields = ['name', 'create_time']
    lookup_field = 'id'
    permission_classes = [CustomMenuPermission]
    action_perms_map = {
        'list': 'assets:hostgroups:view',
        'retrieve': 'assets:hostgroups:view',
        'create': 'assets:hostgroups:create',
        'destroy': 'assets:hostgroups:delete',
        'partial_update': 'assets:hostgroups:update',
        'perform_update': 'assets:hostgroups:update',
        'batch-delete': 'assets:hostgroups:delete',
        'tree': 'assets:hostgroups:view',
    }

    def get_queryset(self):
        if self.action in ['list', 'retrieve', 'tree']:
            return HostGroup.objects.select_related('parent').order_by('id')
        return HostGroup.objects.all()

    def _build_tree(self, group_data):
        nodes = {}
        roots = []

        for item in group_data:
            item['children'] = []
            nodes[item['id']] = item

        for item in group_data:
            parent_id = item.get('parent')
            if parent_id:
                parent = nodes.get(parent_id)
                if parent:
                    parent['children'].append(item)
                else:
                    roots.append(item)
            else:
                roots.append(item)

        return roots

    def _get_group_and_subgroups(self, group_id):
        """递归获取分组及其所有子分组 ID。"""
        group_ids = [int(group_id)]
        children = HostGroup.objects.filter(parent_id=group_id).values_list('id', flat=True)
        for child_id in children:
            group_ids.extend(self._get_group_and_subgroups(int(child_id)))
        return group_ids

    def list(self, request, *args, **kwargs):
        queryset = self.filter_queryset(self.get_queryset())
        page = self.paginate_queryset(queryset)
        serializer = self.get_serializer(page if page is not None else queryset, many=True)
        data = serializer.data
        if page is not None:
            paginator = self.paginator
            return Response_200(data={
                'count': paginator.page.paginator.count,
                'results': data,
            })
        return Response_200(data=data)

    def retrieve(self, request, *args, **kwargs):
        instance = self.get_object()
        serializer = self.get_serializer(instance)
        return Response_200(data=serializer.data)

    def create(self, request, *args, **kwargs):
        serializer = self.get_serializer(data=request.data)
        serializer.is_valid(raise_exception=True)
        serializer.save()
        return Response_200(data=serializer.data)

    def update(self, request, *args, **kwargs):
        instance = self.get_object()
        serializer = self.get_serializer(instance, data=request.data, partial=False)
        serializer.is_valid(raise_exception=True)
        serializer.save()
        return Response_200(data=serializer.data)

    def partial_update(self, request, *args, **kwargs):
        instance = self.get_object()
        serializer = self.get_serializer(instance, data=request.data, partial=True)
        serializer.is_valid(raise_exception=True)
        serializer.save()
        return Response_200(data=serializer.data)

    @action(detail=False, methods=['get'], url_path='tree')
    def tree(self, request):
        queryset = self.filter_queryset(self.get_queryset())
        serializer = self.get_serializer(queryset, many=True)
        data = serializer.data
        tree_data = self._build_tree(data)
        return Response_200(data=tree_data)

    def destroy(self, request, *args, **kwargs):
        instance = self.get_object()
        from automation.models import AutomationInventory

        target_group_ids = self._get_group_and_subgroups(instance.id)
        target_group_id_set = set(target_group_ids)

        referenced = []
        for inventory in AutomationInventory.objects.only('id', 'name', 'selected_group_ids'):
            group_ids = [int(item) for item in (inventory.selected_group_ids or []) if str(item).isdigit()]
            if target_group_id_set.intersection(set(group_ids)):
                referenced.append(inventory)

        if referenced:
            names = '、'.join(item.name for item in referenced[:5])
            suffix = '' if len(referenced) <= 5 else f' 等{len(referenced)}个'
            return Response_error_str(
                f'该主机组已被 Inventory 引用，不能删除。受影响 Inventory: {names}{suffix}',
                code=400,
            )

        deleted_id = instance.id
        # 删除分组前先删除该分组树下所有主机，避免 Host.group=SET_NULL 导致主机残留。
        Host.objects.filter(group_id__in=target_group_ids).delete()
        HostGroup.objects.filter(id__in=target_group_ids).delete()
        return Response_200(data={"id": deleted_id})

    @action(detail=False, methods=['delete'], url_path='batch-delete')
    def batchDelete(self, request):
        ids = request.data.get('ids', [])
        if not ids:
            return Response_200(data=[])

        from automation.models import AutomationInventory

        normalized_ids = [int(item) for item in ids if str(item).isdigit()]
        delete_group_ids = set()
        for group_id in normalized_ids:
            delete_group_ids.update(self._get_group_and_subgroups(group_id))

        referenced_pairs = []
        for inventory in AutomationInventory.objects.only('id', 'name', 'selected_group_ids'):
            group_ids = [int(item) for item in (inventory.selected_group_ids or []) if str(item).isdigit()]
            hit_ids = sorted(set(group_ids).intersection(delete_group_ids))
            if hit_ids:
                referenced_pairs.append((inventory.name, hit_ids))

        if referenced_pairs:
            sample = '；'.join(f"{name} -> {','.join(str(i) for i in group_ids)}" for name, group_ids in referenced_pairs[:3])
            suffix = '' if len(referenced_pairs) <= 3 else f' 等{len(referenced_pairs)}个 Inventory'
            return Response_error_str(
                f'批量删除中包含被 Inventory 引用的主机组，操作已阻止：{sample}{suffix}',
                code=400,
            )

        # 批量删除时，统一删除目标分组树下主机与分组。
        Host.objects.filter(group_id__in=list(delete_group_ids)).delete()
        deleted_count, _ = HostGroup.objects.filter(id__in=list(delete_group_ids)).delete()
        return Response_200(data={"deleted_count": deleted_count})


def _build_single_host_snapshot(host):
    return [{
        'host_id': int(getattr(host, 'id', 0) or 0),
        'host_name': str(getattr(host, 'instance_name', '') or ''),
        'host_ip': str(getattr(host, 'ip', '') or ''),
    }]


def _split_monitor_output_snapshots(output_text):
    """把 execute_job_via_agent_grpc 的统一输出拆分为 stdout/stderr/error 三类快照。"""
    text = str(output_text or '')
    if not text.strip():
        return {'stdout': '', 'stderr': '', 'error': ''}

    host_header = re.compile(r'^=+\s*Agent Host\s*#\d+\s*\(([^)]*)\).*?=+\s*$')
    stdout_parts = []
    stderr_parts = []
    error_parts = []

    current_host = '-'
    section = 'stdout'
    section_lines = []

    def _flush_section():
        nonlocal section_lines
        payload = '\n'.join(section_lines).strip()
        section_lines = []
        if not payload:
            return
        block = f'[{current_host}]\n{payload}'
        if section == 'stdout':
            stdout_parts.append(block)
        elif section == 'stderr':
            stderr_parts.append(block)
        else:
            error_parts.append(block)

    for raw_line in text.splitlines():
        line = str(raw_line or '')
        matched = host_header.match(line.strip())
        if matched:
            _flush_section()
            host_value = str(matched.group(1) or '').strip()
            current_host = host_value or '-'
            section = 'stdout'
            continue

        marker = line.strip().lower()
        if marker == '[stderr]':
            _flush_section()
            section = 'stderr'
            continue
        if marker == '[error]':
            _flush_section()
            section = 'error'
            continue

        section_lines.append(line)

    _flush_section()
    return {
        'stdout': '\n\n'.join(stdout_parts),
        'stderr': '\n\n'.join(stderr_parts),
        'error': '\n\n'.join(error_parts),
    }


def _resolve_monitor_timeout_seconds():
    """监控安装/卸载执行超时秒数（默认 600 秒，可在 settings 覆盖）。"""
    raw_value = getattr(settings, 'MONITOR_EXECUTION_TIMEOUT_SECONDS', 600)
    try:
        timeout_seconds = int(raw_value)
    except (TypeError, ValueError):
        timeout_seconds = 600
    return max(timeout_seconds, 30)


def _resolve_monitor_pending_stale_seconds(timeout_seconds):
    """pending 判定为“卡死”的阈值：执行超时 + 90 秒缓冲。"""
    raw_value = getattr(settings, 'MONITOR_PENDING_STALE_SECONDS', None)
    if raw_value is None:
        return int(timeout_seconds) + 90
    try:
        stale_seconds = int(raw_value)
    except (TypeError, ValueError):
        stale_seconds = int(timeout_seconds) + 90
    return max(stale_seconds, int(timeout_seconds))


def _expire_stale_monitor_pending(target, action, stale_seconds):
    """若最新 pending/running 历史已超时，自动转 failed 并允许本次重新下发。"""
    from monitor.models import MonitorTargetInstallHistory

    latest_history = MonitorTargetInstallHistory.objects.filter(
        target_id=target.id,
        action=action,
        status__in=[
            MonitorTargetInstallHistory.Status.PENDING,
            MonitorTargetInstallHistory.Status.RUNNING,
        ],
    ).order_by('-id').first()

    if latest_history is None:
        return False

    reference_time = latest_history.start_time or latest_history.create_time
    if reference_time is None:
        return False

    elapsed_seconds = (timezone.now() - reference_time).total_seconds()
    if elapsed_seconds < float(stale_seconds):
        return False

    timeout_message = f'执行超时（超过 {int(stale_seconds)} 秒），已标记失败，请重新下发'
    latest_history.status = MonitorTargetInstallHistory.Status.FAILED
    latest_history.summary_message = timeout_message
    latest_history.error_message_snapshot = timeout_message
    latest_history.end_time = timezone.now()
    start_ts = latest_history.start_time
    end_ts = latest_history.end_time
    if start_ts is not None and end_ts is not None:
        latest_history.duration_seconds = (end_ts - start_ts).total_seconds()
    latest_history.save(update_fields=[
        'status', 'summary_message', 'error_message_snapshot', 'end_time', 'duration_seconds', 'update_time',
    ])

    target.install_status = target.InstallStatus.FAILED
    target.install_message = timeout_message
    target.save(update_fields=['install_status', 'install_message', 'update_time'])
    return True


def _run_monitor_playbook_and_update_history(*, target, host, history, template_content, extra_vars, work_directory, timeout_seconds):
    """通过 backend Ansible 执行监控安装/卸载并回写历史。"""
    from automation.executor_playbook import execute_playbook_job
    from monitor.models import MonitorTargetInstallHistory

    close_old_connections()
    history_id = int(getattr(history, 'id', 0) or 0)
    target_id = int(getattr(target, 'id', 0) or 0)

    def _safe_history_save(fields):
        """记录不存在时静默退出，避免执行结果回写影响主流程。"""
        if history_id <= 0:
            return False
        try:
            history.save(update_fields=fields)
            return True
        except DatabaseError:
            logger.debug('Skip monitor history save: history row disappeared (id=%s)', history_id)
            return False

    def _safe_target_save(fields):
        if target_id <= 0:
            return False
        try:
            target.save(update_fields=fields)
            return True
        except DatabaseError:
            logger.debug('Skip monitor target save: target row disappeared (id=%s)', target_id)
            return False

    def _is_history_cancelled():
        if history_id <= 0:
            return False
        current_status = MonitorTargetInstallHistory.objects.filter(id=history_id).values_list('status', flat=True).first()
        return current_status == MonitorTargetInstallHistory.Status.CANCELLED

    started_at = timezone.now()
    if _is_history_cancelled():
        logger.info('Monitor history already cancelled before execution start, skip run (history_id=%s)', history_id)
        return

    history.status = MonitorTargetInstallHistory.Status.RUNNING
    history.start_time = started_at
    if not _safe_history_save(['status', 'start_time', 'update_time']):
        return

    try:
        playbook_request = SimpleNamespace(
            inventory_snapshot={'hosts': _build_single_host_snapshot(host)},
            template_content_snapshot=str(template_content or ''),
            extra_vars=extra_vars if isinstance(extra_vars, dict) else {},
            run_as_user_snapshot='root',
            run_as_group_snapshot='root',
            task=SimpleNamespace(
                execution_concurrency=1,
                execution_timeout_seconds=int(timeout_seconds),
            ),
        )

        success, summary, output_text = execute_playbook_job(playbook_request, persist_target_logs=False)
        finished_at = timezone.now()
        duration_seconds = (finished_at - started_at).total_seconds()
        snapshots = _split_monitor_output_snapshots(output_text)
        message_text = str((summary or {}).get('message', '') or '')

        if _is_history_cancelled():
            logger.info('Monitor history cancelled during execution, skip final write-back (history_id=%s)', history_id)
            return

        if success:
            target.install_status = target.InstallStatus.SUCCESS
            target.install_message = '执行成功'
            target.retry_count = 0
            _safe_target_save(['install_status', 'install_message', 'retry_count', 'update_time'])
            history.status = MonitorTargetInstallHistory.Status.SUCCESS
            history.summary_message = target.install_message
        else:
            target.install_status = target.InstallStatus.FAILED
            if target.last_dispatch_manual:
                target.install_message = (
                    f'执行失败：{message_text}，人工重试失败，如需再次尝试请再次点击“重试”'
                )
            else:
                target.install_message = f'执行失败：{message_text}，需人工重试'
            _safe_target_save(['install_status', 'install_message', 'update_time'])
            history.status = MonitorTargetInstallHistory.Status.FAILED
            history.summary_message = target.install_message

        history.stdout_snapshot = snapshots['stdout']
        history.stderr_snapshot = snapshots['stderr']
        history.error_message_snapshot = snapshots['error']
        history.result_summary_snapshot = summary if isinstance(summary, dict) else {}
        history.end_time = finished_at
        history.duration_seconds = duration_seconds
        _safe_history_save([
            'status', 'summary_message', 'stdout_snapshot', 'stderr_snapshot', 'error_message_snapshot',
            'result_summary_snapshot', 'end_time', 'duration_seconds', 'update_time',
        ])
    except Exception as exc:
        finished_at = timezone.now()
        duration_seconds = (finished_at - started_at).total_seconds()
        target.install_status = target.InstallStatus.FAILED
        target.install_message = f'执行失败：{exc}'
        _safe_target_save(['install_status', 'install_message', 'update_time'])
        history.status = MonitorTargetInstallHistory.Status.FAILED
        history.summary_message = target.install_message
        history.error_message_snapshot = str(exc)
        history.result_summary_snapshot = {'message': str(exc)}
        history.end_time = finished_at
        history.duration_seconds = duration_seconds
        _safe_history_save([
            'status', 'summary_message', 'error_message_snapshot', 'result_summary_snapshot',
            'end_time', 'duration_seconds', 'update_time',
        ])
    finally:
        close_old_connections()


def dispatch_exporter_install_job(host, monitor_target, manual=False):
    """下发监控组件（exporter）安装任务；供主机监控设置及自动/人工重试共用。
    安装/卸载直接复用监控软件仓库（monitor.SoftwarePackage）绑定的 install_playbook_template
    （automation.PlaybookTemplate），不再经由 AutomationTask 中转；执行结果只写入
    MonitorTargetInstallHistory，并复用 automation 模块现成的 backend Ansible runner 执行，exporter 名称/版本/校验值/systemd unit
    内容通过 extra_vars 传给 playbook。
    manual=True 表示由用户在前端点击“重试”手动触发：这类任务只尝试 1 次，
    失败后直接终止，需再次人工点击。
    安装在当前请求内同步执行，底层通过 platform SSH 到目标主机。"""
    exporter_name = monitor_target.exporter_type
    agent_id = str(getattr(host, 'agent_id', '') or '').strip()
    if agent_id == '':
        monitor_target.install_status = monitor_target.InstallStatus.FAILED
        monitor_target.install_message = f'主机未绑定 agent 实例，无法下发 {exporter_name} 安装任务'
        monitor_target.save(update_fields=['install_status', 'install_message', 'update_time'])
        return

    from monitor.models import SoftwarePackage
    from monitor.package_selector import PackageSelectionError, select_software_package

    try:
        latest_pkg = select_software_package(
            host, exporter_name, SoftwarePackage.PackageType.EXPORTER,
        )
    except PackageSelectionError as exc:
        monitor_target.install_status = monitor_target.InstallStatus.FAILED
        monitor_target.install_message = str(exc)
        monitor_target.save(update_fields=['install_status', 'install_message', 'update_time'])
        return

    if latest_pkg.install_playbook_template_id is None:  # type: ignore[attr-defined]  # Django FK 动态生成的 _id 属性，Pylance 无法识别
        monitor_target.install_status = monitor_target.InstallStatus.FAILED
        monitor_target.install_message = f'{exporter_name} 未配置安装 Playbook，请在监控软件仓库中选择'
        monitor_target.save(update_fields=['install_status', 'install_message', 'update_time'])
        return

    template = latest_pkg.install_playbook_template
    if template is None:
        monitor_target.install_status = monitor_target.InstallStatus.FAILED
        monitor_target.install_message = f'{exporter_name} 绑定的安装 Playbook 已不存在，请重新在监控软件仓库中选择'
        monitor_target.save(update_fields=['install_status', 'install_message', 'update_time'])
        return

    package_file = getattr(latest_pkg, 'file', None)
    if not package_file or not getattr(package_file, 'name', ''):
        monitor_target.install_status = monitor_target.InstallStatus.FAILED
        monitor_target.install_message = f'{exporter_name} 的软件仓库记录没有可用安装包文件，请先上传并启用对应包'
        monitor_target.save(update_fields=['install_status', 'install_message', 'update_time'])
        return
    try:
        package_local_path = package_file.path
    except (NotImplementedError, ValueError):
        monitor_target.install_status = monitor_target.InstallStatus.FAILED
        monitor_target.install_message = f'{exporter_name} 的安装包存储不支持本地文件传输，无法执行安装'
        monitor_target.save(update_fields=['install_status', 'install_message', 'update_time'])
        return
    if not os.path.isfile(package_local_path):
        monitor_target.install_status = monitor_target.InstallStatus.FAILED
        monitor_target.install_message = f'{exporter_name} 的安装包文件不存在，请重新上传并启用对应包'
        monitor_target.save(update_fields=['install_status', 'install_message', 'update_time'])
        return

    timeout_seconds = _resolve_monitor_timeout_seconds()
    pending_stale_seconds = _resolve_monitor_pending_stale_seconds(timeout_seconds)

    # 同一监控目标若已有排队/运行中的安装任务，则不重复下发；
    # 但超时卡住的 pending 会先转 failed，再允许本次重新下发。
    if monitor_target.install_status == monitor_target.InstallStatus.PENDING:
        expired = _expire_stale_monitor_pending(
            monitor_target,
            action='install',
            stale_seconds=pending_stale_seconds,
        )
        if expired:
            monitor_target.refresh_from_db()
        else:
            monitor_target.install_status = monitor_target.InstallStatus.PENDING
            monitor_target.install_message = '安装任务已存在（pending）'
            monitor_target.save(update_fields=['install_status', 'install_message', 'update_time'])
            return

    checksums = {}
    for pkg in SoftwarePackage.objects.filter(
        package_type=SoftwarePackage.PackageType.EXPORTER,
        name=exporter_name, version=latest_pkg.version, enabled=True,
    ):
        if pkg.sha256:
            checksums[f'{pkg.os}-{pkg.arch}'] = pkg.sha256

    extra_vars = {
        'exporter_name': exporter_name,
        'exporter_version': latest_pkg.version,
        'service_name': latest_pkg.service_unit_name,
        'service_file_content': latest_pkg.service_file_content,
        # 服务常驻运行身份：模型层默认值/必填约束保证 service_run_as_user 一般不为空，
        # 这里仍做 `or 'dj-agent'` 兜底，覆盖迁移前遗留的历史空值记录。
        # 与"安装过程本身"固定使用的 root 身份是完全独立的两件事。
        'service_run_as_user': latest_pkg.service_run_as_user or 'dj-agent',
        'service_run_as_group': latest_pkg.service_run_as_group or 'dj-agent',
        # backend 通过 Ansible copy 将本地仓库文件直接传到目标主机，目标主机不需要访问 backend HTTP。
        'package_local_path': package_local_path,
        'package_file_name': os.path.basename(package_local_path),
        'package_format': latest_pkg.package_format,
        'package_platform_family': latest_pkg.platform_family,
        'package_platform_major': latest_pkg.platform_major,
        'package_sha256': latest_pkg.sha256,
        'checksums': checksums,
    }

    from monitor.models import MonitorTargetInstallHistory

    history = MonitorTargetInstallHistory.objects.create(
        target=monitor_target,
        host=host,
        action=MonitorTargetInstallHistory.Action.INSTALL,
        trigger_type=(
            MonitorTargetInstallHistory.TriggerType.MANUAL
            if manual else MonitorTargetInstallHistory.TriggerType.AUTO
        ),
        status=MonitorTargetInstallHistory.Status.PENDING,
        host_id_snapshot=int(getattr(host, 'id', 0) or 0),
        host_name_snapshot=str(getattr(host, 'instance_name', '') or ''),
        host_ip_snapshot=str(getattr(host, 'ip', '') or ''),
        exporter_type_snapshot=str(exporter_name or ''),
        summary_message='已下发安装任务',
        result_summary_snapshot={},
        requested_user_id_snapshot=None,
        requested_username_snapshot='',
        start_time=timezone.now(),
    )

    # 记录本次下发是否为人工重试触发，用于失败文案区分与历史审计。
    monitor_target.install_status = monitor_target.InstallStatus.PENDING
    monitor_target.install_message = '已下发安装任务'
    monitor_target.save(update_fields=['install_status', 'install_message', 'last_dispatch_manual', 'update_time'])

    _run_monitor_playbook_and_update_history(
        target=monitor_target,
        host=host,
        history=history,
        template_content=template.content or '',
        extra_vars=extra_vars,
        work_directory=latest_pkg.work_directory or '/tmp',
        timeout_seconds=timeout_seconds,
    )


def _dispatch_exporter_service_action_job(host, monitor_target, action, timeout_seconds=15):
    """同步执行 exporter systemd 服务动作（status/start/stop）。

    Agent 通过 gRPC 执行内置 systemctl action，backend 等待最终结果并写回 AgentJob。
    """
    exporter_name = monitor_target.exporter_type
    agent_id = str(getattr(host, 'agent_id', '') or '').strip()
    if agent_id == '':
        raise RuntimeError(f'主机未绑定 agent 实例，无法对 {exporter_name} 执行 {action}')

    from monitor.models import SoftwarePackage

    latest_pkg = SoftwarePackage.objects.filter(
        package_type=SoftwarePackage.PackageType.EXPORTER,
        name=exporter_name, enabled=True,
    ).order_by('-create_time').first()
    service_unit_name = str(getattr(latest_pkg, 'service_unit_name', '') or '').strip()
    if latest_pkg is None or service_unit_name == '':
        raise RuntimeError(f'本地软件仓库缺少 {exporter_name} 的 systemd 服务名配置（service_unit_name），无法执行 {action}')

    job_id = f'{action}-{uuid.uuid4().hex[:16]}'
    job = AgentJob.objects.create(
        job_id=job_id,
        agent_id=agent_id,
        host=host,
        job_type='custom',
        action=action,
        params={'service_name': service_unit_name},
        timeout_seconds=timeout_seconds,
        status=AgentJob.JobStatus.QUEUED,
    )
    _emit_agent_job_event(job, 'new', {
        'jid': job.job_id,
        'tgt': job.agent_id,
        'tgt_type': 'agent_id',
        'fun': job.action,
        'arg': job.params,
        'minions': [job.agent_id],
    })
    _dispatch_agent_job_via_grpc(job)
    return job


def dispatch_check_exporter_status_job(host, monitor_target):
    """下发"查询 exporter 运行状态"作业（sudo systemctl status <name>.service）。"""
    return _dispatch_exporter_service_action_job(host, monitor_target, 'check_exporter_status')


def dispatch_start_exporter_job(host, monitor_target):
    """下发"启动 exporter 服务"作业（sudo systemctl start <name>.service）。"""
    return _dispatch_exporter_service_action_job(host, monitor_target, 'start_exporter')


def dispatch_stop_exporter_job(host, monitor_target):
    """下发"停止 exporter 服务"作业（sudo systemctl stop <name>.service）。"""
    return _dispatch_exporter_service_action_job(host, monitor_target, 'stop_exporter')


def dispatch_exporter_uninstall_job(host, monitor_target, manual=False):
    """下发监控组件（exporter）卸载任务（停止服务并清理安装文件）；供主机监控设置及自动/人工重试共用。
    同 dispatch_exporter_install_job，使用监控软件仓库该 exporter 绑定的 uninstall_playbook_template 执行。
    卸载在当前请求内同步执行，底层通过 platform SSH 到目标主机。"""
    exporter_name = monitor_target.exporter_type
    agent_id = str(getattr(host, 'agent_id', '') or '').strip()
    if agent_id == '':
        monitor_target.install_status = monitor_target.InstallStatus.FAILED
        monitor_target.install_message = f'主机未绑定 agent 实例，无法下发 {exporter_name} 卸载任务'
        monitor_target.save(update_fields=['install_status', 'install_message', 'update_time'])
        return

    from monitor.models import SoftwarePackage

    latest_pkg = SoftwarePackage.objects.filter(
        package_type=SoftwarePackage.PackageType.EXPORTER,
        name=exporter_name, enabled=True,
    ).order_by('-create_time').first()
    if latest_pkg is None:
        monitor_target.install_status = monitor_target.InstallStatus.FAILED
        monitor_target.install_message = f'本地软件仓库缺少 {exporter_name} 的启用安装包，无法下发卸载任务'
        monitor_target.save(update_fields=['install_status', 'install_message', 'update_time'])
        return

    if latest_pkg.uninstall_playbook_template_id is None:  # type: ignore[attr-defined]  # Django FK 动态生成的 _id 属性，Pylance 无法识别
        monitor_target.install_status = monitor_target.InstallStatus.FAILED
        monitor_target.install_message = f'{exporter_name} 未配置卸载 Playbook，请在监控软件仓库中选择'
        monitor_target.save(update_fields=['install_status', 'install_message', 'update_time'])
        return

    template = latest_pkg.uninstall_playbook_template
    if template is None:
        monitor_target.install_status = monitor_target.InstallStatus.FAILED
        monitor_target.install_message = f'{exporter_name} 绑定的卸载 Playbook 已不存在，请重新在监控软件仓库中选择'
        monitor_target.save(update_fields=['install_status', 'install_message', 'update_time'])
        return

    timeout_seconds = _resolve_monitor_timeout_seconds()
    pending_stale_seconds = _resolve_monitor_pending_stale_seconds(timeout_seconds)

    if monitor_target.install_status == monitor_target.InstallStatus.PENDING:
        expired = _expire_stale_monitor_pending(
            monitor_target,
            action='uninstall',
            stale_seconds=pending_stale_seconds,
        )
        if expired:
            monitor_target.refresh_from_db()
        else:
            monitor_target.install_status = monitor_target.InstallStatus.PENDING
            monitor_target.install_message = '卸载任务已存在（pending）'
            monitor_target.save(update_fields=['install_status', 'install_message', 'update_time'])
            return

    extra_vars = {
        'exporter_name': exporter_name,
        'service_name': latest_pkg.service_unit_name,
    }

    from monitor.models import MonitorTargetInstallHistory

    history = MonitorTargetInstallHistory.objects.create(
        target=monitor_target,
        host=host,
        action=MonitorTargetInstallHistory.Action.UNINSTALL,
        trigger_type=(
            MonitorTargetInstallHistory.TriggerType.MANUAL
            if manual else MonitorTargetInstallHistory.TriggerType.AUTO
        ),
        status=MonitorTargetInstallHistory.Status.PENDING,
        host_id_snapshot=int(getattr(host, 'id', 0) or 0),
        host_name_snapshot=str(getattr(host, 'instance_name', '') or ''),
        host_ip_snapshot=str(getattr(host, 'ip', '') or ''),
        exporter_type_snapshot=str(exporter_name or ''),
        summary_message='已下发卸载任务',
        result_summary_snapshot={},
        requested_user_id_snapshot=None,
        requested_username_snapshot='',
        start_time=timezone.now(),
    )

    # 记录本次下发是否为人工重试触发，用于失败文案区分与历史审计。
    monitor_target.last_dispatch_manual = bool(manual)

    monitor_target.install_status = monitor_target.InstallStatus.PENDING
    monitor_target.install_message = '已下发卸载任务'
    monitor_target.save(update_fields=['install_status', 'install_message', 'last_dispatch_manual', 'update_time'])

    _run_monitor_playbook_and_update_history(
        target=monitor_target,
        host=host,
        history=history,
        template_content=template.content or '',
        extra_vars=extra_vars,
        work_directory=latest_pkg.work_directory or '/tmp',
        timeout_seconds=timeout_seconds,
    )


def _run_exporter_job_in_background(action, host_id, target_id, manual=False):
    """在线程内重新加载 ORM 对象，避免把请求线程的数据库连接带入后台任务。"""
    close_old_connections()
    try:
        from monitor.models import MonitorTarget

        host = Host.objects.get(id=host_id)
        target = MonitorTarget.objects.get(id=target_id, host_id=host_id)
        if action == 'install':
            dispatch_exporter_install_job(host, target, manual=manual)
        elif action == 'uninstall':
            dispatch_exporter_uninstall_job(host, target, manual=manual)
        else:
            raise ValueError(f'不支持的监控后台任务动作: {action}')
    except Exception:
        logger.exception(
            '监控后台任务执行异常: action=%s host_id=%s target_id=%s',
            action, host_id, target_id,
        )
    finally:
        close_old_connections()


def _start_exporter_job_thread(action, host_id, target_id, manual=False):
    # 安装/卸载可能等待 SSH/Ansible 数分钟，必须脱离 Host API 请求线程执行。
    thread = threading.Thread(
        target=_run_exporter_job_in_background,
        args=(action, host_id, target_id, manual),
        daemon=True,
        name=f'monitor-{action}-{target_id}',
    )
    thread.start()



class HostManage(WebSSHHostMixin, GenericViewSet,CreateModelMixin,DestroyModelMixin,UpdateModelMixin,RetrieveModelMixin,ListModelMixin):
    queryset = Host.objects.all()
    serializer_class = HostSerializer
    pagination_class = CustomPagination
    filter_backends = (OrderingFilter, DjangoFilterBackend, SearchFilter)
    # 通用 search 对应前端主搜索框（实例名/IP/备注）；ID检索走 host_id 参数。
    search_fields = ['instance_name', 'agent_id', 'ip', 'remark']
    ordering_fields = ['instance_name', 'create_time']
    lookup_field = 'id'
    permission_classes = [CustomMenuPermission]
    action_perms_map = {
        'list': 'assets:hosts:view',
        'retrieve': 'assets:hosts:view',
        'create': 'assets:hosts:create',
        'destroy': 'assets:hosts:delete',
        'partial_update': 'assets:hosts:update',
        'perform_update': 'assets:hosts:update',
        'batch-delete': 'assets:hosts:delete',
        'webssh_sessions': 'assets:hosts:view',
        'webssh_active_count': 'assets:hosts:view',
        'webssh_active_sessions': 'assets:hosts:view',
        'agent_runtime_status': 'assets:hosts:view',
        'refresh_info': 'assets:hosts:view',
        'batch_refresh_info': 'assets:hosts:view',
        'webssh_files': 'assets:hosts:view',
        'webssh_file_download': 'assets:hosts:view',
        'webssh_file_upload_chunk': 'assets:hosts:update',
        'webssh_file_rename': 'assets:hosts:update',
        'webssh_file_delete': 'assets:hosts:delete',
        'webssh_file_create_dir': 'assets:hosts:update',
        'webssh_file_create_file': 'assets:hosts:update',
    }

    def _get_group_and_subgroups(self, group_id):
        """递归获取分组及其所有子分组的ID列表"""
        group_ids = [group_id]
        group = HostGroup.objects.filter(id=group_id).first()
        if group:
            children = HostGroup.objects.filter(parent_id=group_id)
            for child in children:
                group_ids.extend(self._get_group_and_subgroups(child.id))  # type: ignore[attr-defined]
        return group_ids

    def _display_host_name(self, host):
        system = getattr(host, 'system', None)
        hostname = getattr(system, 'hostname', None) if system else None
        return host.instance_name or hostname or f'Host-{host.id}'

    def get_serializer_class(self):
        if self.action == 'list':
            return HostListSerializer
        return HostSerializer

    def get_queryset(self):
        from monitor.models import MonitorTarget

        # gRPC Session 是 Agent 在线状态的唯一来源，连接时间不能作为超时依据。
        connected_agent_ids = REGISTRY.connected_agent_ids()
        Host.objects.filter(agent_online=True).exclude(agent_id__in=connected_agent_ids).update(agent_online=False)
        if connected_agent_ids:
            Host.objects.filter(agent_id__in=connected_agent_ids, agent_online=False).update(agent_online=True)

        queryset = Host.objects.select_related('group').prefetch_related(
            # 一台主机可能纳管多个 exporter（node_exporter/自定义 exporter 等），不再按 node_exporter 过滤
            Prefetch('monitor_targets', queryset=MonitorTarget.objects.order_by('-id')),
            'hardware',
            'system',
            'disks',
            Prefetch(
                'hostcredential_set',
                queryset=HostCredential.objects.select_related('credential').order_by('-is_default', 'id'),
                to_attr='host_credentials',
            ),
        ).order_by('-id')
        group_id = self.request.query_params.get('group_id')  # type: ignore[union-attr]
        host_id = self.request.query_params.get('host_id')  # type: ignore[union-attr]
        instance_name = (self.request.query_params.get('instance_name') or '').strip()  # type: ignore[union-attr]
        collect_status = self.request.query_params.get('collect_status')  # type: ignore[union-attr]
        agent_status = (self.request.query_params.get('agent_status') or '').strip().lower()  # type: ignore[union-attr]
        environment_id = self.request.query_params.get('environment')  # type: ignore[union-attr]
        if host_id not in [None, '', '0', 0]:
            try:
                queryset = queryset.filter(id=int(host_id))
            except (TypeError, ValueError):
                queryset = queryset.none()
        if group_id not in [None, '', '0', 0]:
            # 查询该分组及其所有子分组下的主机
            group_ids = self._get_group_and_subgroups(int(group_id))
            queryset = queryset.filter(group_id__in=group_ids)
        if instance_name:
            queryset = queryset.filter(instance_name__icontains=instance_name)
        if collect_status in {
            Host.CollectStatus.UNKNOWN,
            Host.CollectStatus.SUCCESS,
            Host.CollectStatus.FAILED,
        }:
            queryset = queryset.filter(collect_status=collect_status)
        if agent_status in {'online', 'offline'}:
            queryset = queryset.filter(agent_online=(agent_status == 'online'))
        if environment_id not in [None, '', '0', 0]:
            try:
                queryset = queryset.filter(environment_id=int(environment_id))
            except (TypeError, ValueError):
                queryset = queryset.none()
        return queryset.distinct()

    def create(self, request, *args, **kwargs):
        monitors_payload = self._resolve_monitors_from_request(request)
        serializer = self.get_serializer(data=request.data)
        serializer.is_valid(raise_exception=True)
        host = serializer.save()
        self._sync_monitors_for_host(host, monitors_payload)
        return Response_200(data=serializer.data)

    def list(self, request, *args, **kwargs):
        queryset = self.filter_queryset(self.get_queryset())
        page = self.paginate_queryset(queryset)
        serializer = self.get_serializer(page if page is not None else queryset, many=True)
        data = serializer.data
        if page is not None:
            paginator = self.paginator
            return Response_200(data={
                'count': paginator.page.paginator.count,
                'results': data,
            })
        return Response_200(data=data)

    def retrieve(self, request, *args, **kwargs):
        instance = self.get_object()
        serializer = self.get_serializer(instance)
        return Response_200(data=serializer.data)

    @action(detail=True, methods=['get'], url_path='agent-runtime-status')
    def agent_runtime_status(self, request, id=None):
        host = self.get_object()
        if not str(getattr(host, 'agent_id', '') or '').strip():
            return Response_error_str('主机未绑定 agent 实例，无法获取状态')
        try:
            data = _fetch_agent_runtime_status(host)
        except RuntimeError as exc:
            return Response_error_str(str(exc))
        return Response_200(data=data)

    @staticmethod
    def _refresh_host_worker(host):
        """批量刷新的线程工作单元：每个线程独立执行 gRPC 拉取+落库，结束后释放本线程 DB 连接。"""
        try:
            return refresh_host_info(host)
        finally:
            # ThreadPoolExecutor 复用线程会保留 Django 线程级连接，用完主动关闭避免连接泄漏。
            connections.close_all()

    @action(detail=True, methods=['post'], url_path='refresh-info')
    def refresh_info(self, request, id=None):
        """按需刷新单台主机信息（打开详情页时调用），经 gRPC 让 agent 执行 get_host_info。"""
        host = self.get_object()
        result = refresh_host_info(host)
        host.refresh_from_db()
        data = HostSerializer(host).data
        return Response_200(data={'result': result, 'host': data})

    @action(detail=False, methods=['post'], url_path='refresh-info')
    def batch_refresh_info(self, request):
        """按需批量刷新主机信息（打开列表页/翻页时调用当前页主机）。

        仅对在线 agent 发起拉取，离线主机直接跳过；使用有限并发避免大批量时阻塞。
        返回被成功更新主机的最新序列化数据，供前端就地合并到表格行。
        """
        raw_ids = request.data.get('ids')
        if not isinstance(raw_ids, list) or not raw_ids:
            return Response_error_str('ids 不能为空，且必须是数组')

        ids = []
        for item in raw_ids:
            try:
                ids.append(int(item))
            except (TypeError, ValueError):
                continue
        if not ids:
            return Response_error_str('ids 中没有合法的主机 ID')

        hosts = list(Host.objects.filter(id__in=ids))
        results = []
        updated_ids = []
        if hosts:
            # 并发上限 8：单台拉取以网络等待为主，适度并发即可，过高会挤占 gRPC/DB 资源。
            with ThreadPoolExecutor(max_workers=min(8, len(hosts))) as executor:
                future_map = {executor.submit(self._refresh_host_worker, host): host for host in hosts}
                for future in as_completed(future_map):
                    res = future.result()
                    results.append(res)
                    if res.get('updated'):
                        updated_ids.append(res['host_id'])

        hosts_data = []
        if updated_ids:
            refreshed = Host.objects.filter(id__in=updated_ids).select_related(
                'group', 'hardware', 'system',
            ).prefetch_related('disks')
            hosts_data = HostListSerializer(refreshed, many=True).data
        return Response_200(data={'results': results, 'hosts': hosts_data})


    def update(self, request, *args, **kwargs):
        instance = self.get_object()
        monitors_payload = self._resolve_monitors_from_request(request)
        # 主机网络地址变化后，已有 WebSSH 会话需重建。
        previous_signature = (str(instance.ip or ''),)
        serializer = self.get_serializer(instance, data=request.data, partial=False)
        serializer.is_valid(raise_exception=True)
        self.perform_update(serializer)
        instance.refresh_from_db()
        current_signature = (str(instance.ip or ''),)
        if previous_signature != current_signature:
            # 连接参数变化后，已有 SSH 通道不再可信，需强制重建。
            self._force_close_webssh_sessions_for_hosts([instance.id])
        self._sync_monitors_for_host(instance, monitors_payload)
        return Response_200(data=serializer.data)

    def partial_update(self, request, *args, **kwargs):
        instance = self.get_object()
        monitors_payload = self._resolve_monitors_from_request(request)
        previous_signature = (str(instance.ip or ''),)
        serializer = self.get_serializer(instance, data=request.data, partial=True)
        serializer.is_valid(raise_exception=True)
        self.perform_update(serializer)
        instance.refresh_from_db()
        current_signature = (str(instance.ip or ''),)
        if previous_signature != current_signature:
            self._force_close_webssh_sessions_for_hosts([instance.id])
        self._sync_monitors_for_host(instance, monitors_payload)
        return Response_200(data=serializer.data)

    @staticmethod
    def _parse_bool_flag(raw_value, default_value=None):
        if raw_value is None:
            return default_value
        if isinstance(raw_value, bool):
            return raw_value
        normalized = str(raw_value).strip().lower()
        if normalized in {'1', 'true', 'yes', 'on'}:
            return True
        if normalized in {'0', 'false', 'no', 'off'}:
            return False
        return default_value

    def _resolve_monitors_from_request(self, request):
        """解析请求中的 monitors 数组：[{name, enabled, port}, ...]。
        请求体中不包含 'monitors' 键时返回 None，表示本次请求不涉及监控配置改动（如仅编辑主机基础信息），
        避免每次编辑主机都重新遍历/下发监控任务。"""
        if 'monitors' not in request.data:
            return None
        raw_list = request.data.get('monitors') or []
        if not isinstance(raw_list, list):
            return []
        monitors = []
        for item in raw_list:
            if not isinstance(item, dict):
                continue
            name = str(item.get('name') or '').strip()
            if not name:
                continue
            raw_port = item.get('port')
            port = None
            if raw_port is not None and str(raw_port).strip() != '':
                try:
                    parsed_port = int(raw_port)
                    if 1 <= parsed_port <= 65535:
                        port = parsed_port
                except (TypeError, ValueError):
                    port = None
            monitors.append({
                'name': name,
                'enabled': self._parse_bool_flag(item.get('enabled'), default_value=True),
                'port': port,
            })
        return monitors

    @staticmethod
    def _build_package_default_port_map():
        from monitor.models import SoftwarePackage

        # 同名软件包可能有多条版本记录，这里按 id 倒序取最新一条的默认端口。
        package_map = {}
        queryset = SoftwarePackage.objects.filter(
            package_type=SoftwarePackage.PackageType.EXPORTER,
            enabled=True,
        ).order_by('-id').values('name', 'default_port')
        for row in queryset:
            name = str(row.get('name') or '').strip()
            if name == '' or name in package_map:
                continue
            raw_port = row.get('default_port')
            if raw_port is None:
                continue
            try:
                port = int(raw_port)
            except (TypeError, ValueError):
                continue
            if 1 <= port <= 65535:
                package_map[name] = port
        return package_map

    def _sync_monitors_for_host(self, host, monitors_payload):
        """按 monitors 数组同步该主机纳管的监控目标（每个 name 对应一个 MonitorTarget）：
        安装/卸载所需的 Playbook 任务、systemd unit 内容均直接来自监控软件仓库（SoftwarePackage）
        在下发时实时读取，MonitorTarget 本身不再保留脚本副本；managed_enabled 是唯一的期望状态开关——
        开启（或新建即开启）自动下发安装任务，关闭（从开启切换为关闭）自动下发卸载任务，语义单一、
        不再区分“是否要连带卸载”，避免一次性指令式的隐藏状态；未在本次请求中提交的已有监控目标保持
        原状，不做隐式禁用/删除（多监控项独立维护，编辑其中一个不影响其他）。"""
        if monitors_payload is None:
            return

        from monitor.models import MonitorTarget

        package_default_ports = self._build_package_default_port_map()

        for entry in monitors_payload:
            name = entry['name']
            enabled = bool(entry['enabled'])
            requested_port = entry.get('port')
            resolved_port = requested_port or package_default_ports.get(name) or 9100

            target, created = MonitorTarget.objects.get_or_create(
                host=host,
                exporter_type=name,
                defaults={
                    'managed_enabled': enabled,
                    'scrape_port': resolved_port,
                },
            )

            update_fields = []

            previous_enabled = bool(target.managed_enabled)
            if previous_enabled != enabled:
                target.managed_enabled = enabled
                update_fields.append('managed_enabled')

            if int(getattr(target, 'scrape_port', 9100) or 9100) != int(resolved_port):
                target.scrape_port = int(resolved_port)
                update_fields.append('scrape_port')

            if update_fields:
                target.save(update_fields=update_fields + ['update_time'])

            host_id = int(getattr(host, 'id'))
            target_id = int(getattr(target, 'id'))
            if not enabled:
                if previous_enabled:
                    # 开启->关闭这一次切换，自动下发卸载任务；新一轮卸载周期先重置自动重试计数，
                    # 确保重试上限从 0 开始计算。
                    target.retry_count = 0
                    target.save(update_fields=['retry_count', 'update_time'])
                    _start_exporter_job_thread('uninstall', host_id, target_id)
                else:
                    target.install_message = f'已关闭监控（未卸载 {name}）'
                    target.save(update_fields=['install_message', 'update_time'])
                continue

            # 仅在首次开启或新建监控目标时自动触发安装，避免每次编辑主机重复下发。
            if created or not previous_enabled:
                # 新一轮安装周期：重置自动重试计数
                target.retry_count = 0
                target.save(update_fields=['retry_count', 'update_time'])
                _start_exporter_job_thread('install', host_id, target_id)

    def destroy(self, request, *args, **kwargs):
        instance = self.get_object()
        deleted_id = instance.id
        self._force_close_webssh_sessions_for_hosts([deleted_id])
        self.perform_destroy(instance)
        return Response_200(data={"id": deleted_id})

    @action(detail=False, methods=['delete'], url_path='batch-delete')
    def batchDelete(self, request):
        ids = request.data.get('ids', [])
        if not ids:
            return Response_200(data=[])
        normalized_ids = []
        for item in ids:
            text = str(item).strip()
            if not text.isdigit():
                continue
            value = int(text)
            if value > 0:
                normalized_ids.append(value)
        self._force_close_webssh_sessions_for_hosts(normalized_ids)
        host_queryset = Host.objects.filter(id__in=normalized_ids)
        deleted_count = host_queryset.count()
        host_queryset.delete()
        return Response_200(data={"deleted_count": deleted_count})

@api_view(['POST'])
def agent_create_job(request):
    """创建并下发 agent 任务（支持单机/组播/全量）。"""
    payload = request.data if isinstance(request.data, dict) else {}  # type: ignore[attr-defined]

    agent_id = str(payload.get('agent_id') or '').strip()
    target_type = str(payload.get('target_type') or 'single').strip().lower()
    target_value = str(payload.get('target_value') or '').strip()
    job_type = str(payload.get('type') or '').strip()
    action = str(payload.get('action') or '').strip()
    client_request_id = str(payload.get('client_request_id') or '').strip()
    params = _normalize_agent_job_params(payload)
    timeout_seconds_raw = payload.get('timeout_seconds', 30)

    if job_type == '':
        return Response_error_str('type不能为空')
    if action == '':
        return Response_error_str('action不能为空')

    try:
        timeout_seconds = int(timeout_seconds_raw)
    except (TypeError, ValueError):
        return Response_error_str('timeout_seconds必须是整数')
    if timeout_seconds <= 0:
        return Response_error_str('timeout_seconds必须大于0')

    try:
        agent_ids = _resolve_target_agent_ids(target_type, target_value, agent_id)
    except RuntimeError as exc:
        return Response_error_str(str(exc))

    if len(agent_ids) == 0:
        return Response_error_str('未解析到可下发的agent')

    if target_type == 'single' and len(agent_ids) == 1:
        final_agent_id = agent_ids[0]
        host = Host.objects.filter(agent_id=final_agent_id).first()
        if host is None:
            return Response_error_str('agent_id未绑定主机')

        if client_request_id:
            existed_job = AgentJob.objects.filter(client_request_id=client_request_id).first()
            if existed_job is not None:
                return Response_200(data={
                    'job_id': existed_job.job_id,
                    'agent_id': existed_job.agent_id,
                    'type': existed_job.job_type,
                    'action': existed_job.action,
                    'status': existed_job.status,
                    'reused': True,
                    'target_type': 'single',
                    'target_value': final_agent_id,
                })

        job_id = f'{action}-{uuid.uuid4().hex[:16]}'
        try:
            job = AgentJob.objects.create(
                job_id=job_id,
                client_request_id=client_request_id or None,
                agent_id=final_agent_id,
                host=host,
                job_type=job_type,
                action=action,
                params=params,
                timeout_seconds=timeout_seconds,
                status=AgentJob.JobStatus.QUEUED,
            )
        except IntegrityError:
            if not client_request_id:
                raise
            existed_job = AgentJob.objects.filter(client_request_id=client_request_id).first()
            if existed_job is None:
                raise
            return Response_200(data={
                'job_id': existed_job.job_id,
                'agent_id': existed_job.agent_id,
                'type': existed_job.job_type,
                'action': existed_job.action,
                'status': existed_job.status,
                'reused': True,
                'target_type': 'single',
                'target_value': final_agent_id,
            })

        _emit_agent_job_event(job, 'new', {
            'jid': job.job_id,
            'tgt': job.agent_id,
            'tgt_type': 'agent_id',
            'fun': job.action,
            'arg': job.params,
            'minions': [job.agent_id],
        })
        _dispatch_agent_job_via_grpc(job)

        return Response_200(data={
            'job_id': job.job_id,
            'agent_id': job.agent_id,
            'type': job.job_type,
            'action': job.action,
            'status': job.status,
            'reused': False,
            'target_type': 'single',
            'target_value': final_agent_id,
        })

    # 组播和全量统一展开为逐 agent 作业，保证状态追踪与重试链路一致。
    result_rows = []
    created_count = 0
    failed_count = 0
    for current_agent_id in agent_ids:
        host = Host.objects.filter(agent_id=current_agent_id).first()
        if host is None:
            failed_count += 1
            result_rows.append({
                'agent_id': current_agent_id,
                'ok': False,
                'error': 'agent_id未绑定主机',
            })
            continue

        item_request_id = f'{client_request_id}:{current_agent_id}' if client_request_id else ''
        if item_request_id:
            existed_job = AgentJob.objects.filter(client_request_id=item_request_id).first()
            if existed_job is not None:
                result_rows.append({
                    'agent_id': current_agent_id,
                    'ok': True,
                    'job_id': existed_job.job_id,
                    'status': existed_job.status,
                    'reused': True,
                })
                continue

        current_job_id = f'{action}-{uuid.uuid4().hex[:16]}'
        try:
            job = AgentJob.objects.create(
                job_id=current_job_id,
                client_request_id=item_request_id or None,
                agent_id=current_agent_id,
                host=host,
                job_type=job_type,
                action=action,
                params=params,
                timeout_seconds=timeout_seconds,
                status=AgentJob.JobStatus.QUEUED,
            )
        except IntegrityError:
            if not item_request_id:
                raise
            existed_job = AgentJob.objects.filter(client_request_id=item_request_id).first()
            if existed_job is None:
                raise
            result_rows.append({
                'agent_id': current_agent_id,
                'ok': True,
                'job_id': existed_job.job_id,
                'status': existed_job.status,
                'reused': True,
            })
            continue

        created_count += 1
        result_rows.append({
            'agent_id': current_agent_id,
            'ok': True,
            'job_id': job.job_id,
            'status': job.status,
            'reused': False,
        })

        _emit_agent_job_event(job, 'new', {
            'jid': job.job_id,
            'tgt': job.agent_id,
            'tgt_type': 'agent_id',
            'fun': job.action,
            'arg': job.params,
            'minions': [job.agent_id],
        })
        _dispatch_agent_job_via_grpc(job)
        result_rows[-1]['status'] = job.status

    return Response_200(data={
        'target_type': target_type,
        'target_value': target_value if target_type != 'all' else 'all',
        'created_count': created_count,
        'failed_count': failed_count,
        'results': result_rows,
    })


@api_view(['POST'])
def agent_install(request: Request) -> Response | JsonResponse:
    """Queue Ansible-based Agent installation or update for selected hosts."""
    payload = request.data if isinstance(request.data, dict) else {}  # type: ignore[attr-defined]
    raw_host_ids = payload.get('host_ids')
    operation = str(payload.get('operation') or 'install').strip().lower()
    try:
        credential_id = int(str(payload.get('credential_id') or '').strip())
    except (TypeError, ValueError):
        return Response_error_str('credential_id必须是整数', code=400)
    if operation not in {'install', 'update'}:
        return Response_error_str('operation仅支持install或update', code=400)
    if not isinstance(raw_host_ids, list) or not raw_host_ids:
        return Response_error_str('host_ids不能为空数组', code=400)

    host_ids = list(dict.fromkeys(
        int(item) for item in raw_host_ids
        if str(item).strip().isdigit() and int(item) > 0
    ))
    hosts = list(Host.objects.filter(id__in=host_ids).order_by('id'))
    if len(hosts) != len(host_ids):
        return Response_error_str('部分主机不存在，请刷新后重试', code=400)
    if operation == 'update' and any(not str(host.agent_id or '').strip() for host in hosts):
        return Response_error_str('更新操作仅支持已绑定 Agent 的主机', code=400)
    credential = Credential.objects.filter(id=credential_id).first()
    if credential is None:
        return Response_error_str('SSH 凭证不存在', code=400)
    if str(credential.username or '').strip().lower() != 'root':
        return Response_error_str('当前版本要求 SSH 凭证使用 root 用户', code=400)

    from automation.models import AutomationExecutionJob, AutomationExecutionTargetLog
    operation_label = {'install': '安装', 'update': '更新'}[operation]
    playbook_content = (Path(settings.BASE_DIR) / 'assets' / 'agent_install.yml').read_text(encoding='utf-8')
    with transaction.atomic():
        hosts = list(Host.objects.select_for_update().filter(id__in=host_ids).order_by('id'))
        stale_before = timezone.now() - timedelta(seconds=30)
        stale_jobs = list(AgentJob.objects.filter(
            host_id__in=host_ids,
            action='install_agent',
            status__in=[AgentJob.JobStatus.QUEUED, AgentJob.JobStatus.RUNNING],
            update_time__lt=stale_before,
        ))
        stale_job_ids = [job.job_id for job in stale_jobs]
        stale_execution_ids = set(AutomationExecutionTargetLog.objects.filter(
            agent_job_id__in=stale_job_ids,
        ).values_list('job_id', flat=True))
        if stale_jobs:
            now = timezone.now()
            AgentJob.objects.filter(pk__in=[job.pk for job in stale_jobs]).update(
                status=AgentJob.JobStatus.FAILED,
                error_message='Agent 安装执行进程已失联，请重新提交',
                exit_code=1,
                finished_at=now,
                update_time=now,
            )
            AutomationExecutionTargetLog.objects.filter(agent_job_id__in=stale_job_ids).update(
                status=AutomationExecutionTargetLog.Status.FAILED,
                error_message='Agent 安装执行进程已失联，请重新提交',
                exit_code=1,
            )
            for execution_id in stale_execution_ids:
                _refresh_automation_job(execution_id)

        active_job = AgentJob.objects.filter(
            host_id__in=host_ids,
            action='install_agent',
            status__in=[AgentJob.JobStatus.QUEUED, AgentJob.JobStatus.RUNNING],
        ).order_by('host_id').first()
        if active_job is not None:
            active_host_id = getattr(active_job, 'host_id', None)
            active_execution_id = AutomationExecutionTargetLog.objects.filter(
                agent_job_id=active_job.job_id,
            ).values_list('job_id', flat=True).first()
            return Response_error_str(
                f'主机 {active_host_id} 已有 Agent 任务执行中'
                f'（作业 #{active_execution_id or "-"}）',
                code=400,
            )

        execution = AutomationExecutionJob.objects.create(
            task=None,
            status=AutomationExecutionJob.Status.RUNNING,
            task_name_snapshot=f'Agent {operation_label}',
            template_name_snapshot='dj-agent Ansible Playbook',
            template_content_snapshot=playbook_content,
            inventory_snapshot={
                'hosts': [
                    {'host_id': host.pk, 'name': host.instance_name or '', 'ip': str(host.ip or '')}
                    for host in hosts
                ],
            },
            extra_vars={'operation': operation, 'credential_id': credential_id},
            requested_user_id=getattr(getattr(request, 'user', None), 'id', None),
            requested_username=str(getattr(getattr(request, 'user', None), 'username', '') or ''),
            start_time=timezone.now(),
        )
        jobs = []
        for host in hosts:
            job = AgentJob.objects.create(
                job_id=f'install-agent-{uuid.uuid4().hex[:16]}',
                agent_id=str(host.agent_id or f'host-{host.pk}'),
                host=host,
                job_type='ansible',
                action='install_agent',
                params={'credential_id': credential_id, 'operation': operation},
                timeout_seconds=300,
                status=AgentJob.JobStatus.QUEUED,
            )
            jobs.append(job)
            AutomationExecutionTargetLog.objects.create(
                job=execution,
                host=host,
                host_id_snapshot=host.pk,
                host_name_snapshot=str(host.instance_name or ''),
                host_ip_snapshot=str(host.ip or ''),
                agent_job_id=job.job_id,
                status=AutomationExecutionTargetLog.Status.PENDING,
            )
            transaction.on_commit(lambda job=job, host=host: threading.Thread(
                target=run_agent_install_job,
                args=(job.pk, host.pk, credential_id, execution.pk),
                daemon=True,
                name=f'agent-install-{host.pk}',
            ).start())

    return Response_200(data={
        'operation': operation,
        'automation_job_id': execution.pk,
        'jobs': [
            {'job_id': job.job_id, 'host_id': getattr(job, 'host_id', None), 'status': job.status}
            for job in jobs
        ],
    })


@api_view(['POST'])
def agent_create_jobs_batch(request):
    """批量创建 agent 任务（逐 agent 写入 queued）。"""
    payload = request.data if isinstance(request.data, dict) else {}  # type: ignore[attr-defined]

    raw_agent_ids = payload.get('agent_ids')
    job_type = str(payload.get('type') or '').strip()
    action = str(payload.get('action') or '').strip()
    client_request_id = str(payload.get('client_request_id') or '').strip()
    params = _normalize_agent_job_params(payload)
    timeout_seconds_raw = payload.get('timeout_seconds', 30)

    if not isinstance(raw_agent_ids, list) or len(raw_agent_ids) == 0:
        return Response_error_str('agent_ids不能为空数组')
    if job_type == '':
        return Response_error_str('type不能为空')
    if action == '':
        return Response_error_str('action不能为空')

    try:
        timeout_seconds = int(timeout_seconds_raw)
    except (TypeError, ValueError):
        return Response_error_str('timeout_seconds必须是整数')
    if timeout_seconds <= 0:
        return Response_error_str('timeout_seconds必须大于0')

    # 去重并保持输入顺序，避免重复 agent 被重复下发。
    agent_ids = []
    seen_agent_id = set()
    for item in raw_agent_ids:
        current_agent_id = str(item or '').strip()
        if current_agent_id == '' or current_agent_id in seen_agent_id:
            continue
        seen_agent_id.add(current_agent_id)
        agent_ids.append(current_agent_id)

    if len(agent_ids) == 0:
        return Response_error_str('agent_ids不能为空数组')

    result_rows = []
    created_count = 0
    failed_count = 0

    for agent_id in agent_ids:
        host = Host.objects.filter(agent_id=agent_id).first()
        if host is None:
            failed_count += 1
            result_rows.append({
                'agent_id': agent_id,
                'ok': False,
                'error': 'agent_id未绑定主机',
            })
            continue

        item_request_id = f'{client_request_id}:{agent_id}' if client_request_id else ''
        if item_request_id:
            existed_job = AgentJob.objects.filter(client_request_id=item_request_id).first()
            if existed_job is not None:
                result_rows.append({
                    'agent_id': agent_id,
                    'ok': True,
                    'job_id': existed_job.job_id,
                    'status': existed_job.status,
                    'reused': True,
                })
                continue

        job_id = f'{action}-{uuid.uuid4().hex[:16]}'
        try:
            job = AgentJob.objects.create(
                job_id=job_id,
                client_request_id=item_request_id or None,
                agent_id=agent_id,
                host=host,
                job_type=job_type,
                action=action,
                params=params,
                timeout_seconds=timeout_seconds,
                status=AgentJob.JobStatus.QUEUED,
            )
        except IntegrityError:
            if not item_request_id:
                raise
            existed_job = AgentJob.objects.filter(client_request_id=item_request_id).first()
            if existed_job is None:
                raise
            result_rows.append({
                'agent_id': agent_id,
                'ok': True,
                'job_id': existed_job.job_id,
                'status': existed_job.status,
                'reused': True,
            })
            continue

        created_count += 1
        result_rows.append({
            'agent_id': agent_id,
            'ok': True,
            'job_id': job.job_id,
            'status': job.status,
            'reused': False,
        })

        _emit_agent_job_event(job, 'new', {
            'jid': job.job_id,
            'tgt': job.agent_id,
            'tgt_type': 'agent_id',
            'fun': job.action,
            'arg': job.params,
            'minions': [job.agent_id],
        })
        _dispatch_agent_job_via_grpc(job)
        result_rows[-1]['status'] = job.status

    return Response_200(data={
        'created_count': created_count,
        'failed_count': failed_count,
        'results': result_rows,
    })


@api_view(['POST'])
def agent_retry_job(request):
    """按 job_id 重试任务（仅允许 failed/timeout/canceled）。"""
    payload = request.data if isinstance(request.data, dict) else {}  # type: ignore[attr-defined]
    source_job_id = str(payload.get('job_id') or '').strip()

    if source_job_id == '':
        return Response_error_str('job_id不能为空')

    with transaction.atomic():
        source_job = AgentJob.objects.select_for_update().filter(job_id=source_job_id).first()
        if source_job is None:
            return Response_error_str('任务不存在')

        if source_job.status not in {
            AgentJob.JobStatus.FAILED,
            AgentJob.JobStatus.TIMEOUT,
            AgentJob.JobStatus.CANCELED,
        }:
            return Response_error_str('仅失败/超时/取消任务允许重试')

        if source_job.host is None:
            return Response_error_str('任务未绑定主机，不能重试')

        new_job_id = f'{source_job.action}-{uuid.uuid4().hex[:16]}'
        retried_job = AgentJob.objects.create(
            job_id=new_job_id,
            agent_id=source_job.agent_id,
            host=source_job.host,
            job_type=source_job.job_type,
            action=source_job.action,
            params=source_job.params,
            timeout_seconds=int(source_job.timeout_seconds or 30),
            status=AgentJob.JobStatus.QUEUED,
            remark=f'retry_from:{source_job.job_id}',
        )

    _emit_agent_job_event(retried_job, 'new', {
        'jid': retried_job.job_id,
        'tgt': retried_job.agent_id,
        'tgt_type': 'agent_id',
        'fun': retried_job.action,
        'arg': retried_job.params,
        'minions': [retried_job.agent_id],
        'retry_from': source_job.job_id,
    })
    _dispatch_agent_job_via_grpc(retried_job)

    return Response_200(data={
        'source_job_id': source_job.job_id,
        'job_id': retried_job.job_id,
        'agent_id': retried_job.agent_id,
        'type': retried_job.job_type,
        'action': retried_job.action,
        'status': retried_job.status,
    })


@api_view(['GET'])
def agent_query_jobs(request):
    """查询 agent 任务列表与状态，支持按 job_id/agent_id/status 过滤。"""
    job_id = str(request.query_params.get('job_id') or '').strip()  # type: ignore[attr-defined]
    agent_id = str(request.query_params.get('agent_id') or '').strip()  # type: ignore[attr-defined]
    host_id_raw = request.query_params.get('host_id')  # type: ignore[attr-defined]
    action = str(request.query_params.get('action') or '').strip()  # type: ignore[attr-defined]
    group_by = str(request.query_params.get('group_by') or '').strip().lower()  # type: ignore[attr-defined]
    status = str(request.query_params.get('status') or '').strip().lower()  # type: ignore[attr-defined]
    page_raw = request.query_params.get('page', 1)  # type: ignore[attr-defined]
    size_raw = request.query_params.get('size', 20)  # type: ignore[attr-defined]

    try:
        page = int(page_raw)
    except (TypeError, ValueError):
        return Response_error_str('page必须是整数')
    if page <= 0:
        return Response_error_str('page必须大于0')

    try:
        size = int(size_raw)
    except (TypeError, ValueError):
        return Response_error_str('size必须是整数')
    if size <= 0:
        return Response_error_str('size必须大于0')
    if size > 200:
        size = 200

    queryset = AgentJob.objects.all().order_by('-create_time')

    if job_id:
        queryset = queryset.filter(job_id=job_id)
    if agent_id:
        queryset = queryset.filter(agent_id=agent_id)
    if host_id_raw not in (None, ''):
        try:
            host_id = int(host_id_raw)
        except (TypeError, ValueError):
            return Response_error_str('host_id必须是整数')
        if host_id <= 0:
            return Response_error_str('host_id必须大于0')
        queryset = queryset.filter(host_id=host_id)
    if action:
        queryset = queryset.filter(action=action)

    if group_by:
        if group_by != 'action':
            return Response_error_str('group_by仅支持action')

        grouped = queryset.values('action').annotate(total=Count('id')).order_by('-total', 'action')
        rows = []
        for item in grouped:
            action_name = str(item.get('action') or '').strip() or '-'
            rows.append({
                'action': action_name,
                'count': int(item.get('total') or 0),
            })

        return Response_200(data={
            'count': len(rows),
            'results': rows,
        })

    summary_queryset = queryset

    if status:
        valid_status = {
            AgentJob.JobStatus.QUEUED,
            AgentJob.JobStatus.RUNNING,
            AgentJob.JobStatus.SUCCESS,
            AgentJob.JobStatus.FAILED,
            AgentJob.JobStatus.CANCELED,
            AgentJob.JobStatus.TIMEOUT,
        }
        if status not in valid_status:
            return Response_error_str('status非法')
        queryset = queryset.filter(status=status)

    total_count = queryset.count()
    offset = (page - 1) * size
    jobs = []
    for item in queryset[offset:offset + size]:
        jobs.append({
            'job_id': item.job_id,
            'agent_id': item.agent_id,
            'host_id': int(getattr(item, 'host_id', 0) or 0) or None,
            'type': item.job_type,
            'action': item.action,
            'status': item.status,
            'timeout_seconds': int(item.timeout_seconds or 0),
            'params': item.params,
            'result_data': item.result_data,
            'error_message': item.error_message,
            # gRPC 任务完成后写入 exit_code/stdout/stderr，查询接口需透出原始输出。
            'exit_code': int(item.exit_code) if item.exit_code is not None else -1,
            'stdout': item.stdout,
            'stderr': item.stderr,
            'create_time': item.create_time,
            'picked_at': item.picked_at,
            'finished_at': item.finished_at,
        })

    summary = {
        'total': summary_queryset.count(),
        'queued': summary_queryset.filter(status=AgentJob.JobStatus.QUEUED).count(),
        'running': summary_queryset.filter(status=AgentJob.JobStatus.RUNNING).count(),
        'success': summary_queryset.filter(status=AgentJob.JobStatus.SUCCESS).count(),
        'failed': summary_queryset.filter(status=AgentJob.JobStatus.FAILED).count(),
        'canceled': summary_queryset.filter(status=AgentJob.JobStatus.CANCELED).count(),
        'timeout': summary_queryset.filter(status=AgentJob.JobStatus.TIMEOUT).count(),
    }

    recent_failed_jobs = summary_queryset.filter(
        status__in=[AgentJob.JobStatus.FAILED, AgentJob.JobStatus.TIMEOUT],
    ).order_by('-finished_at', '-update_time')[:5]

    recent_failure_reasons = []
    for failed_item in recent_failed_jobs:
        recent_failure_reasons.append({
            'job_id': failed_item.job_id,
            'status': failed_item.status,
            'action': failed_item.action,
            'error_message': failed_item.error_message,
            'finished_at': failed_item.finished_at,
        })

    return Response_200(data={
        'count': len(jobs),
        'pageNumber': page,
        'pageSize': size,
        'total': total_count,
        'totalPages': (total_count + size - 1) // size,
        'results': jobs,
        'summary': summary,
        'recent_failure_reasons': recent_failure_reasons,
    })


@api_view(['GET'])
def agent_query_job_chain(request):
    """查询任务重试链路，返回节点与父子边关系。"""
    job_id = str(request.query_params.get('job_id') or '').strip()  # type: ignore[attr-defined]
    if job_id == '':
        return Response_error_str('job_id不能为空')

    def _parse_parent_job_id(remark_text):
        text = str(remark_text or '').strip()
        prefix = 'retry_from:'
        if not text.startswith(prefix):
            return ''
        return text[len(prefix):].strip()

    def _serialize_job(item):
        return {
            'job_id': item.job_id,
            'agent_id': item.agent_id,
            'host_id': int(getattr(item, 'host_id', 0) or 0) or None,
            'type': item.job_type,
            'action': item.action,
            'status': item.status,
            'timeout_seconds': int(item.timeout_seconds or 0),
            'error_message': item.error_message,
            'create_time': item.create_time,
            'picked_at': item.picked_at,
            'finished_at': item.finished_at,
            'parent_job_id': _parse_parent_job_id(item.remark),
        }

    seed_job = AgentJob.objects.filter(job_id=job_id).first()
    if seed_job is None:
        return Response_error_str('任务不存在')

    max_depth = 50
    node_map = {}
    edges = []

    # 向上追溯父链，拿到根节点。
    current_job = seed_job
    depth = 0
    while current_job is not None and depth < max_depth:
        node_map[current_job.job_id] = current_job
        parent_job_id = _parse_parent_job_id(current_job.remark)
        if parent_job_id == '':
            break
        parent_job = AgentJob.objects.filter(job_id=parent_job_id).first()
        if parent_job is None:
            break
        edges.append({'from_job_id': parent_job.job_id, 'to_job_id': current_job.job_id})
        current_job = parent_job
        depth += 1

    root_job = current_job if current_job is not None else seed_job

    # 从根节点向下展开所有重试分支。
    queue = [root_job.job_id]
    visited = set(queue)
    while queue and len(visited) < 1000:
        parent_id = queue.pop(0)
        children = AgentJob.objects.filter(remark=f'retry_from:{parent_id}').order_by('create_time')
        for child in children:
            edges.append({'from_job_id': parent_id, 'to_job_id': child.job_id})
            if child.job_id not in node_map:
                node_map[child.job_id] = child
            if child.job_id not in visited:
                visited.add(child.job_id)
                queue.append(child.job_id)

    # 去重边，避免父链和子链阶段重复添加。
    unique_edge_keys = set()
    unique_edges = []
    for edge in edges:
        key = f"{edge['from_job_id']}->{edge['to_job_id']}"
        if key in unique_edge_keys:
            continue
        unique_edge_keys.add(key)
        unique_edges.append(edge)

    nodes = [_serialize_job(item) for item in sorted(node_map.values(), key=lambda x: x.create_time)]

    return Response_200(data={
        'root_job_id': root_job.job_id,
        'focus_job_id': seed_job.job_id,
        'node_count': len(nodes),
        'edge_count': len(unique_edges),
        'nodes': nodes,
        'edges': unique_edges,
    })


@api_view(['GET'])
def agent_query_job_events(request):
    """查询作业事件流（Salt风格 tag：salt/job/<jid>/new|ret/<mid>）。"""
    job_id = str(request.query_params.get('job_id') or '').strip()  # type: ignore[attr-defined]
    agent_id = str(request.query_params.get('agent_id') or '').strip()  # type: ignore[attr-defined]
    tag = str(request.query_params.get('tag') or '').strip()  # type: ignore[attr-defined]
    event_type = str(request.query_params.get('event_type') or '').strip()  # type: ignore[attr-defined]
    limit_raw = request.query_params.get('limit', 100)  # type: ignore[attr-defined]

    try:
        limit = int(limit_raw)
    except (TypeError, ValueError):
        return Response_error_str('limit必须是整数')
    if limit <= 0:
        return Response_error_str('limit必须大于0')
    if limit > 500:
        limit = 500

    queryset = AgentJobEvent.objects.all().order_by('-create_time')
    if job_id:
        queryset = queryset.filter(job_id=job_id)
    if agent_id:
        queryset = queryset.filter(agent_id=agent_id)
    if tag:
        queryset = queryset.filter(tag__icontains=tag)
    if event_type:
        queryset = queryset.filter(event_type=event_type)

    rows = []
    for item in queryset[:limit]:
        rows.append({
            'id': int(getattr(item, 'pk', 0) or 0),
            'tag': item.tag,
            'job_id': item.job_id,
            'agent_id': item.agent_id,
            'event_type': item.event_type,
            'payload': item.payload,
            'create_time': item.create_time,
        })

    return Response_200(data={
        'count': len(rows),
        'results': rows,
    })


@api_view(['POST'])
def agent_cancel_job(request):
    """取消任务（仅允许 queued/running -> canceled）。"""
    payload = request.data if isinstance(request.data, dict) else {}  # type: ignore[attr-defined]
    job_id = str(payload.get('job_id') or '').strip()
    reason = str(payload.get('reason') or '').strip()

    if job_id == '':
        return Response_error_str('job_id不能为空')

    with transaction.atomic():
        job = AgentJob.objects.select_for_update().filter(job_id=job_id).first()
        if job is None:
            return Response_error_str('任务不存在')

        if job.status in {
            AgentJob.JobStatus.SUCCESS,
            AgentJob.JobStatus.FAILED,
            AgentJob.JobStatus.CANCELED,
            AgentJob.JobStatus.TIMEOUT,
        }:
            return Response_error_str('任务已结束，不能取消')

        now = timezone.now()
        job.status = AgentJob.JobStatus.CANCELED
        job.finished_at = now
        if reason:
            job.error_message = reason
        elif job.error_message == '':
            job.error_message = 'job canceled by operator'
        job.save(update_fields=['status', 'finished_at', 'error_message', 'update_time'])

        _emit_agent_job_event(job, 'ret', {
            'id': job.agent_id,
            'jid': job.job_id,
            'retcode': 1,
            'fun': job.action,
            'return': {'status': job.status, 'error': job.error_message},
        })

    return Response_200(data={
        'job_id': job.job_id,
        'status': job.status,
    })
