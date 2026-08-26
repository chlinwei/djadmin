import threading

from django.db import transaction
from django.utils import timezone
from rest_framework.decorators import action
from rest_framework.mixins import CreateModelMixin, DestroyModelMixin, ListModelMixin, RetrieveModelMixin, UpdateModelMixin
from rest_framework.request import Request
from rest_framework.viewsets import GenericViewSet, ReadOnlyModelViewSet

from djadmin.utils import CustomPagination, Response_200, Response_error_str
from assets.models import Host, HostGroup
from assets.grpc_transfer.client import AgentChannelClient

from .executor import run_inspection_execution
from .models import InspectionExecution, InspectionGroup, InspectionTargetExecution, InspectionTask
from .serializers import InspectionExecutionSerializer, InspectionGroupSerializer, InspectionTaskSerializer


def _descendant_group_ids(root_id):
    child_map = {}
    for group_id, parent_id in HostGroup.objects.values_list('id', 'parent_id'):
        child_map.setdefault(parent_id, []).append(group_id)
    result = []
    pending = [root_id]
    while pending:
        group_id = pending.pop()
        result.append(group_id)
        pending.extend(child_map.get(group_id, []))
    return result


class InspectionGroupViewSet(CreateModelMixin, UpdateModelMixin, RetrieveModelMixin, ListModelMixin, DestroyModelMixin, GenericViewSet):
    queryset = InspectionGroup.objects.prefetch_related('checks').all()
    serializer_class = InspectionGroupSerializer
    pagination_class = CustomPagination

    def create(self, request: Request, *args, **kwargs):
        serializer = self.get_serializer(data=request.data)
        serializer.is_valid(raise_exception=True)
        serializer.save()
        return Response_200(data=serializer.data)

    def update(self, request: Request, *args, **kwargs):
        serializer = self.get_serializer(self.get_object(), data=request.data, partial=kwargs.pop('partial', False))
        serializer.is_valid(raise_exception=True)
        serializer.save()
        return Response_200(data=serializer.data)

    def retrieve(self, request: Request, *args, **kwargs):
        return Response_200(data=self.get_serializer(self.get_object()).data)

    def destroy(self, request: Request, *args, **kwargs):
        group = self.get_object()
        if group.tasks.exists():
            return Response_error_str('巡检组已被任务使用，不能删除', code=400)
        group.delete()
        return Response_200()


class InspectionTaskViewSet(CreateModelMixin, UpdateModelMixin, RetrieveModelMixin, ListModelMixin, DestroyModelMixin, GenericViewSet):
    queryset = InspectionTask.objects.select_related('group', 'logical_service', 'host_group').all()
    serializer_class = InspectionTaskSerializer
    pagination_class = CustomPagination

    def create(self, request: Request, *args, **kwargs):
        serializer = self.get_serializer(data=request.data)
        serializer.is_valid(raise_exception=True)
        serializer.save()
        return Response_200(data=serializer.data)

    def update(self, request: Request, *args, **kwargs):
        serializer = self.get_serializer(self.get_object(), data=request.data, partial=kwargs.pop('partial', False))
        serializer.is_valid(raise_exception=True)
        serializer.save()
        return Response_200(data=serializer.data)

    def retrieve(self, request: Request, *args, **kwargs):
        return Response_200(data=self.get_serializer(self.get_object()).data)

    def destroy(self, request: Request, *args, **kwargs):
        self.get_object().delete()
        return Response_200()

    @action(detail=True, methods=['post'], url_path='run')
    def run(self, request: Request, pk=None):
        task = self.get_object()
        if not task.enabled or not task.group.enabled:
            return Response_error_str('巡检任务或巡检组已禁用', code=400)
        checks = list(task.group.checks.filter(enabled=True).values('name', 'executor', 'config', 'order'))
        if not checks:
            return Response_error_str('巡检组没有启用的检查项', code=400)
        service = task.logical_service
        host_group = task.host_group
        deployments = []
        hosts = []
        if task.target_type == InspectionTask.TargetType.HOST_GROUP:
            if host_group is None:
                return Response_error_str('巡检任务未绑定主机组', code=400)
            hosts = list(Host.objects.filter(
                group_id__in=_descendant_group_ids(host_group.pk),
                is_deleted_in_cloud=False,
            ).order_by('instance_name', 'id'))
            if not hosts:
                return Response_error_str('主机组及其子组中没有主机', code=400)
            offline = [str(item.instance_name or item.ip or item.pk) for item in hosts if not item.agent_id or not item.agent_online]
            if offline:
                return Response_error_str(f'以下主机 Agent 离线: {", ".join(offline)}', code=400)
        else:
            if service is None:
                return Response_error_str('巡检任务未绑定逻辑服务', code=400)
            deployments = list(service.deployments.filter(
                enabled=True,
                service_links__service=service,
                service_links__enabled=True,
            ).select_related('host').distinct())
            if task.group.scope == InspectionGroup.Scope.PER_DEPLOYMENT and not deployments:
                return Response_error_str('逻辑服务没有启用的部署实例', code=400)
            offline = [item.instance_name for item in deployments if not item.host.agent_id or not item.host.agent_online]
            if task.group.scope == InspectionGroup.Scope.PER_DEPLOYMENT and offline:
                return Response_error_str(f'以下部署实例 Agent 离线: {", ".join(offline)}', code=400)

        user = getattr(request, 'user', None)
        deployment_snapshot = [
            {
                'deployment_id': item.pk,
                'instance_name': item.instance_name,
                'host_id': item.host_id,
                'host_ip': str(item.host.ip or ''),
                'agent_id': str(item.host.agent_id or ''),
            }
            for item in deployments
        ] if task.group.scope == InspectionGroup.Scope.PER_DEPLOYMENT else [
            {'target_name': service.name, 'access_address': service.access_address},
        ]
        host_snapshot = [{
            'host_id': item.pk,
            'host_name': str(item.instance_name or f'Host-{item.pk}'),
            'host_ip': str(item.ip or ''),
            'agent_id': str(item.agent_id or ''),
        } for item in hosts]
        target_snapshot = host_snapshot if task.target_type == InspectionTask.TargetType.HOST_GROUP else deployment_snapshot
        target_context = (
            {'target_type': task.target_type, 'id': host_group.pk, 'name': host_group.name}
            if host_group else
            {'target_type': task.target_type, 'id': service.pk, 'name': service.name, 'code': service.code, 'access_address': service.access_address}
        )
        with transaction.atomic():
            execution = InspectionExecution.objects.create(
                task=task,
                task_snapshot={'id': task.pk, 'name': task.name, 'target_type': task.target_type, 'concurrency': task.concurrency, 'timeout_seconds': task.timeout_seconds},
                group_snapshot={'id': task.group_id, 'name': task.group.name, 'scope': task.group.scope, 'checks': checks},
                service_snapshot=target_context,
                target_snapshot=target_snapshot,
                requested_user_id=getattr(user, 'id', None),
                requested_username=str(getattr(user, 'username', '') or ''),
            )
            if task.target_type == InspectionTask.TargetType.HOST_GROUP:
                InspectionTargetExecution.objects.bulk_create([
                    InspectionTargetExecution(
                        execution=execution,
                        host=item,
                        target_name=str(item.instance_name or item.ip or f'Host-{item.pk}'),
                        host_id_snapshot=item.pk,
                        host_ip_snapshot=str(item.ip or ''),
                        agent_id_snapshot=str(item.agent_id or ''),
                    )
                    for item in hosts
                ])
            elif task.group.scope == InspectionGroup.Scope.PER_DEPLOYMENT:
                InspectionTargetExecution.objects.bulk_create([
                    InspectionTargetExecution(
                        execution=execution,
                        deployment=item,
                        target_name=item.instance_name,
                        host_id_snapshot=item.host_id,
                        host_ip_snapshot=str(item.host.ip or ''),
                        agent_id_snapshot=str(item.host.agent_id or ''),
                    )
                    for item in deployments
                ])
            else:
                InspectionTargetExecution.objects.create(execution=execution, target_name=service.name)
            transaction.on_commit(lambda: threading.Thread(
                target=run_inspection_execution,
                args=(execution.pk,),
                daemon=True,
                name=f'inspection-{execution.pk}',
            ).start())
        return Response_200(data={'execution_id': execution.pk, 'status': execution.status})


class InspectionExecutionViewSet(ReadOnlyModelViewSet):
    queryset = InspectionExecution.objects.select_related('task').prefetch_related('targets__results').all()
    serializer_class = InspectionExecutionSerializer
    pagination_class = CustomPagination

    def retrieve(self, request: Request, *args, **kwargs):
        return Response_200(data=self.get_serializer(self.get_object()).data)

    @action(detail=True, methods=['post'], url_path='cancel')
    def cancel(self, request: Request, *args, **kwargs):
        execution = self.get_object()
        if execution.status not in (
            InspectionExecution.Status.PENDING,
            InspectionExecution.Status.RUNNING,
        ):
            return Response_error_str('只有等待中或执行中的巡检可以取消', code=400)
        execution.status = InspectionExecution.Status.CANCELED
        execution.end_time = timezone.now()
        execution.summary = {
            **(execution.summary or {}),
            'canceled': True,
        }
        execution.save(update_fields=['status', 'end_time', 'summary', 'update_time'])
        for target in InspectionTargetExecution.objects.filter(execution=execution).only('agent_id_snapshot', 'id'):
            if target.agent_id_snapshot:
                try:
                    AgentChannelClient(target.agent_id_snapshot).cancel_automation(
                        f'inspection-{execution.pk}-{target.pk}',
                    )
                except Exception:
                    pass
        InspectionTargetExecution.objects.filter(
            execution=execution,
            status__in=[
                InspectionTargetExecution.Status.PENDING,
                InspectionTargetExecution.Status.RUNNING,
            ],
        ).update(status=InspectionTargetExecution.Status.CANCELED, end_time=timezone.now())
        return Response_200(data={'id': execution.pk, 'status': execution.status})