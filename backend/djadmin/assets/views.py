import warnings
import re
import time
import uuid
import asyncio
import json
import importlib
import os
import pika
import logging
from datetime import timedelta
from urllib import error as urllib_error
from urllib import request as urllib_request
from asgiref.sync import async_to_sync

from cryptography.utils import CryptographyDeprecationWarning
from djadmin.utils import Response_200, Response_error_str
from rest_framework.mixins import CreateModelMixin,DestroyModelMixin,UpdateModelMixin,RetrieveModelMixin,ListModelMixin
from rest_framework.viewsets import GenericViewSet
from rest_framework.decorators import action, api_view
from .models import *
from .serializer import *
from djadmin.utils import CustomPagination
from rest_framework.filters import OrderingFilter,SearchFilter
from django_filters.rest_framework  import DjangoFilterBackend
from djadmin.errordict import DjadminException,AssetsError
from io import TextIOWrapper
import csv
from menu.permisssion import CustomMenuPermission
from django.db.models import Prefetch, Count
from django.db import IntegrityError, transaction
from django.conf import settings
from django.utils import timezone
from .credential_crypto import decrypt_secret
from .webssh_runtime import WebSSHRuntimeRegistry
from .webssh_host_mixin import WebSSHHostMixin

# 日志记录器
logger = logging.getLogger(__name__)


# 自动下发主机采集任务的最小间隔（秒）。
AGENT_AUTO_COLLECT_MIN_INTERVAL_SECONDS = 300


def _build_agent_status_url(host):
    scheme = str(getattr(settings, 'AGENT_HTTP_SCHEME', os.getenv('AGENT_HTTP_SCHEME', 'http')) or 'http').strip() or 'http'
    port_text = str(getattr(settings, 'AGENT_HTTP_PORT', os.getenv('AGENT_HTTP_PORT', '19090')) or '19090').strip() or '19090'
    endpoint = str(
        getattr(settings, 'AGENT_HTTP_STATUS_ENDPOINT', os.getenv('AGENT_HTTP_STATUS_ENDPOINT', '/api/v1/agent/status'))
        or '/api/v1/agent/status'
    ).strip() or '/api/v1/agent/status'
    if not endpoint.startswith('/'):
        endpoint = '/' + endpoint
    return f"{scheme}://{host.ip}:{port_text}{endpoint}"


def _fetch_agent_runtime_status(host):
    if not host or not str(getattr(host, 'ip', '') or '').strip():
        raise RuntimeError('主机IP为空，无法获取 agent 状态')

    url = _build_agent_status_url(host)
    req = urllib_request.Request(url=url, method='GET')

    token = str(getattr(settings, 'AGENT_HTTP_TOKEN', os.getenv('DJ_AGENT_HTTP_TOKEN', '')) or '').strip()
    if token != '':
        req.add_header('Authorization', f'Bearer {token}')

    configured_timeout = getattr(settings, 'AGENT_HTTP_REQUEST_TIMEOUT_SECONDS', os.getenv('AGENT_HTTP_REQUEST_TIMEOUT_SECONDS', 15))
    try:
        request_timeout = max(3, min(30, int(str(configured_timeout).strip())))
    except (TypeError, ValueError):
        request_timeout = 15

    try:
        with urllib_request.urlopen(req, timeout=request_timeout) as resp:
            raw_text = resp.read().decode('utf-8', errors='replace')
            if int(resp.status) != 200:
                raise RuntimeError(f'agent http status={resp.status}: {raw_text}')
    except urllib_error.URLError as exc:
        raise RuntimeError(f'agent request failed: {exc}') from exc

    try:
        payload = json.loads(raw_text)
    except json.JSONDecodeError as exc:
        raise RuntimeError(f'agent response is not valid json: {raw_text}') from exc

    if int(payload.get('code') or 0) != 200:
        raise RuntimeError(str(payload.get('msg') or 'agent business error'))

    data = payload.get('data')
    if not isinstance(data, dict):
        raise RuntimeError('agent response missing data')
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
            instance_name__isnull=True,
        ).exclude(instance_name='').values_list('instance_name', flat=True)
        return list(dict.fromkeys([str(item).strip() for item in rows if str(item).strip()]))

    if normalized_target_type == 'all':
        rows = Host.objects.exclude(instance_name__isnull=True).exclude(
            instance_name='',
        ).values_list('instance_name', flat=True)
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

def _publish_agent_job_to_mq(job):
    """将作业发布到 RabbitMQ，供 agent 实时消费。"""
    if job is None:
        return

    agent_id = str(getattr(job, 'agent_id', '') or '').strip()
    if agent_id == '':
        return

    rabbitmq_url = str(getattr(settings, 'RABBITMQ_URL', 'amqp://127.0.0.1:5672//') or '').strip()
    if rabbitmq_url == '':
        raise RuntimeError('RABBITMQ_URL未配置，无法发布agent任务')

    # 当前任务模型默认按单机下发
    message = {
        'command_id': f'cmd-{uuid.uuid4().hex[:16]}',
        'client_request_id': str(getattr(job, 'client_request_id', '') or ''),
        'job_id': str(getattr(job, 'job_id', '') or ''),
        'target_type': 'single',
        'target_value': agent_id,
        'type': str(getattr(job, 'job_type', '') or ''),
        'action': str(getattr(job, 'action', '') or ''),
        'params': getattr(job, 'params', {}) or {},
        'timeout_seconds': int(getattr(job, 'timeout_seconds', 30) or 30),
        'created_at': timezone.now().isoformat(),
    }

    try:
        connection = pika.BlockingConnection(pika.URLParameters(rabbitmq_url))
        channel = connection.channel()
        queue_name = getattr(settings, 'RABBITMQ_AGENT_TASKS_QUEUE', 'agent.tasks')
        channel.queue_declare(queue=queue_name, durable=True, auto_delete=False)
        channel.basic_publish(
            exchange='',
            routing_key=queue_name,
            body=json.dumps(message, ensure_ascii=False),
            properties=pika.BasicProperties(delivery_mode=2),  # 消息持久化
        )
        connection.close()
    except Exception as exc:
        raise RuntimeError(f'发布RabbitMQ消息失败: {exc}') from exc


def dispatch_host_report_interval_update(interval_seconds, client_request_id=''):
    """将主机上报间隔更新任务下发到全部 agent。"""
    try:
        normalized_interval_seconds = int(interval_seconds)
    except (TypeError, ValueError) as exc:
        raise RuntimeError('interval_seconds必须是整数') from exc

    agent_ids = _resolve_target_agent_ids('all', '', '')
    if len(agent_ids) == 0:
        return {
            'target_type': 'all',
            'target_value': 'all',
            'created_count': 0,
            'failed_count': 0,
            'results': [],
        }

    params = {
        'interval_seconds': normalized_interval_seconds,
    }
    timeout_seconds = 10
    result_rows = []
    created_count = 0
    failed_count = 0

    for current_agent_id in agent_ids:
        host = Host.objects.filter(instance_name=current_agent_id).first()
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

        current_job_id = f'set_host_report_interval-{uuid.uuid4().hex[:16]}'
        try:
            job = AgentJob.objects.create(
                job_id=current_job_id,
                client_request_id=item_request_id or None,
                agent_id=current_agent_id,
                host=host,
                job_type='custom',
                action='set_host_report_interval',
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
        _publish_agent_job_to_mq(job)

    return {
        'target_type': 'all',
        'target_value': 'all',
        'created_count': created_count,
        'failed_count': failed_count,
        'results': result_rows,
    }

class CredentialManage(GenericViewSet,CreateModelMixin,UpdateModelMixin,RetrieveModelMixin,ListModelMixin,DestroyModelMixin):
    queryset =  Credential.objects.all()
    serializer_class = CredentialSerializer
    pagination_class = CustomPagination
    filter_backends = (OrderingFilter,DjangoFilterBackend,SearchFilter)
    search_fields = ['name', 'remark'] 
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
        serializer = self.get_serializer(data=request.data)
        serializer.is_valid(raise_exception=True)
        serializer.save()
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

    def destroy(self, request, *args, **kwargs):
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
        

class ApplicationManage(GenericViewSet,CreateModelMixin,UpdateModelMixin,RetrieveModelMixin,ListModelMixin):
    queryset =  Application.objects.all()
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

    # 批量删除Application
    @action(detail=False,methods=['delete'],url_path='batch-delete')
    def batchDelete(self,request):
        # 获取ID数组参数
        ids = request.data.get('ids', [])
        # 先查用户角色列表
        Application.objects.filter(id__in=ids).delete()
        return Response_200()


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


class HostManage(WebSSHHostMixin, GenericViewSet,CreateModelMixin,DestroyModelMixin,UpdateModelMixin,RetrieveModelMixin,ListModelMixin):
    queryset = Host.objects.all()
    serializer_class = HostSerializer
    pagination_class = CustomPagination
    filter_backends = (OrderingFilter, DjangoFilterBackend, SearchFilter)
    # 通用 search 对应前端主搜索框（实例名/IP/备注）；ID检索走 host_id 参数。
    search_fields = ['instance_name', 'ip', 'remark']
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
        queryset = Host.objects.select_related('group').prefetch_related(
            Prefetch('hostcredential_set', queryset=HostCredential.objects.select_related('credential').filter(is_default=True)),
            'hardware',
            'system',
            'disks',
        ).order_by('-id')
        group_id = self.request.query_params.get('group_id')  # type: ignore[union-attr]
        host_id = self.request.query_params.get('host_id')  # type: ignore[union-attr]
        instance_name = (self.request.query_params.get('instance_name') or '').strip()  # type: ignore[union-attr]
        collect_status = self.request.query_params.get('collect_status')  # type: ignore[union-attr]
        agent_status = (self.request.query_params.get('agent_status') or '').strip().lower()  # type: ignore[union-attr]
        has_default_credential = (self.request.query_params.get('has_default_credential') or '').strip().lower()  # type: ignore[union-attr]
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
        if has_default_credential in {'true', '1', 'yes'}:
            queryset = queryset.filter(hostcredential__is_default=True)
        elif has_default_credential in {'false', '0', 'no'}:
            queryset = queryset.exclude(hostcredential__is_default=True)
        if collect_status in {
            Host.CollectStatus.UNKNOWN,
            Host.CollectStatus.SUCCESS,
            Host.CollectStatus.FAILED,
        }:
            queryset = queryset.filter(collect_status=collect_status)
        if agent_status in {'online', 'offline'}:
            # dj-agent 每 10 秒发一次心跳，取 3 倍间隔（30 秒）作为超时阈值
            # 可容忍 2 次连续丢包，避免网络抖动导致误报离线
            heartbeat_timeout = timezone.now() - timedelta(seconds=30)
            # 每次列表查询都同步超时状态到 DB，保证序列化器直接读 DB 字段准确
            Host.objects.filter(
                agent_online=True,
                agent_online_time__lt=heartbeat_timeout,
            ).update(agent_online=False)
            # 基于 agent_online 标志判断在线状态
            queryset = queryset.filter(agent_online=(agent_status == 'online'))
        else:
            # 即使不按 agent_status 过滤，也需同步超时状态，确保详情/WebSSH 判断准确
            heartbeat_timeout = timezone.now() - timedelta(seconds=30)
            Host.objects.filter(
                agent_online=True,
                agent_online_time__lt=heartbeat_timeout,
            ).update(agent_online=False)
        return queryset.distinct()

    def create(self, request, *args, **kwargs):
        serializer = self.get_serializer(data=request.data)
        serializer.is_valid(raise_exception=True)
        serializer.save()
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
        if not str(getattr(host, 'instance_name', '') or '').strip():
            return Response_error_str('主机未绑定 agent 实例，无法获取状态')
        try:
            data = _fetch_agent_runtime_status(host)
        except RuntimeError as exc:
            return Response_error_str(str(exc))
        return Response_200(data=data)

    def update(self, request, *args, **kwargs):
        instance = self.get_object()
        # 主机连接签名：IP + SSH 端口 + 默认凭证。
        previous_signature = (
            str(instance.ip or ''),
            int(instance.port or 22),
            self._get_default_credential_id_for_host(instance),
        )
        serializer = self.get_serializer(instance, data=request.data, partial=False)
        serializer.is_valid(raise_exception=True)
        self.perform_update(serializer)
        instance.refresh_from_db()
        current_signature = (
            str(instance.ip or ''),
            int(instance.port or 22),
            self._get_default_credential_id_for_host(instance),
        )
        if previous_signature != current_signature:
            # 连接参数变化后，已有 SSH 通道不再可信，需强制重建。
            self._force_close_webssh_sessions_for_hosts([instance.id])
        return Response_200(data=serializer.data)

    def partial_update(self, request, *args, **kwargs):
        instance = self.get_object()
        previous_signature = (
            str(instance.ip or ''),
            int(instance.port or 22),
            self._get_default_credential_id_for_host(instance),
        )
        serializer = self.get_serializer(instance, data=request.data, partial=True)
        serializer.is_valid(raise_exception=True)
        self.perform_update(serializer)
        instance.refresh_from_db()
        current_signature = (
            str(instance.ip or ''),
            int(instance.port or 22),
            self._get_default_credential_id_for_host(instance),
        )
        if previous_signature != current_signature:
            self._force_close_webssh_sessions_for_hosts([instance.id])
        return Response_200(data=serializer.data)

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
    raw_params = payload.get('params')
    params = raw_params if isinstance(raw_params, dict) else {}
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
        host = Host.objects.filter(instance_name=final_agent_id).first()
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
        _publish_agent_job_to_mq(job)

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
        host = Host.objects.filter(instance_name=current_agent_id).first()
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
        _publish_agent_job_to_mq(job)

    return Response_200(data={
        'target_type': target_type,
        'target_value': target_value if target_type != 'all' else 'all',
        'created_count': created_count,
        'failed_count': failed_count,
        'results': result_rows,
    })


@api_view(['POST'])
def agent_create_jobs_batch(request):
    """批量创建 agent 任务（逐 agent 写入 queued）。"""
    payload = request.data if isinstance(request.data, dict) else {}  # type: ignore[attr-defined]

    raw_agent_ids = payload.get('agent_ids')
    job_type = str(payload.get('type') or '').strip()
    action = str(payload.get('action') or '').strip()
    client_request_id = str(payload.get('client_request_id') or '').strip()
    raw_params = payload.get('params')
    params = raw_params if isinstance(raw_params, dict) else {}
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
        host = Host.objects.filter(instance_name=agent_id).first()
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
        _publish_agent_job_to_mq(job)

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
    _publish_agent_job_to_mq(retried_job)

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
