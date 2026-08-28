from django.utils import timezone
from django.utils.dateparse import parse_datetime
from django_filters.rest_framework import DjangoFilterBackend
from rest_framework.decorators import action
from rest_framework.filters import OrderingFilter, SearchFilter
from rest_framework.mixins import CreateModelMixin, DestroyModelMixin, ListModelMixin, RetrieveModelMixin, UpdateModelMixin
from rest_framework.request import Request
from rest_framework.viewsets import GenericViewSet, ReadOnlyModelViewSet

from djadmin.utils import CustomPagination, Response_200, Response_error_str
from assets.grpc_transfer.client import AgentChannelClient
from menu.permisssion import CustomMenuPermission

from .models import InspectionExecution, InspectionGroup, InspectionTargetExecution, InspectionTask
from .serializers import (
    InspectionExecutionListSerializer,
    InspectionExecutionSerializer,
    InspectionGroupSerializer,
    InspectionTaskSerializer,
)
from .service import InspectionRequestError, create_execution
from .scheduling import calculate_next_run_time


def _parse_range_boundary(value):
    parsed = parse_datetime(str(value or ''))
    if parsed is None:
        return None
    return timezone.make_aware(parsed) if timezone.is_naive(parsed) else parsed


class InspectionGroupViewSet(CreateModelMixin, UpdateModelMixin, RetrieveModelMixin, ListModelMixin, DestroyModelMixin, GenericViewSet):
    queryset = InspectionGroup.objects.prefetch_related('checks').all()
    serializer_class = InspectionGroupSerializer
    pagination_class = CustomPagination
    filter_backends = (OrderingFilter, DjangoFilterBackend, SearchFilter)
    search_fields = ['name', 'description']
    ordering_fields = ['name', 'scope', 'enabled', 'create_time', 'update_time']
    permission_classes = [CustomMenuPermission]
    action_perms_map = {
        'list': 'inspection:view',
        'retrieve': 'inspection:view',
        'create': 'inspection:groups:create',
        'update': 'inspection:groups:update',
        'partial_update': 'inspection:groups:update',
        'destroy': 'inspection:groups:delete',
    }

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
    filter_backends = (OrderingFilter, DjangoFilterBackend, SearchFilter)
    search_fields = ['name', 'group__name', 'logical_service__name', 'host_group__name']
    ordering_fields = ['name', 'enabled', 'next_run_time', 'last_run_time', 'create_time', 'update_time']
    permission_classes = [CustomMenuPermission]
    action_perms_map = {
        'list': 'inspection:view',
        'retrieve': 'inspection:view',
        'create': 'inspection:tasks:create',
        'update': 'inspection:tasks:update',
        'partial_update': 'inspection:tasks:update',
        'destroy': 'inspection:tasks:delete',
        'run': 'inspection:tasks:run',
    }

    def create(self, request: Request, *args, **kwargs):
        serializer = self.get_serializer(data=request.data)
        serializer.is_valid(raise_exception=True)
        task = serializer.save()
        calculate_next_run_time(task)
        return Response_200(data=self.get_serializer(task).data)

    def update(self, request: Request, *args, **kwargs):
        serializer = self.get_serializer(self.get_object(), data=request.data, partial=kwargs.pop('partial', False))
        serializer.is_valid(raise_exception=True)
        task = serializer.save()
        # cron 或启用状态变更后必须立刻重算，否则分发器仍按旧计划触发。
        calculate_next_run_time(task)
        return Response_200(data=self.get_serializer(task).data)

    def retrieve(self, request: Request, *args, **kwargs):
        return Response_200(data=self.get_serializer(self.get_object()).data)

    def destroy(self, request: Request, *args, **kwargs):
        self.get_object().delete()
        return Response_200()

    @action(detail=True, methods=['post'], url_path='run')
    def run(self, request: Request, pk=None):
        user = getattr(request, 'user', None)
        try:
            execution = create_execution(
                self.get_object(),
                requested_user_id=getattr(user, 'id', None),
                requested_username=str(getattr(user, 'username', '') or ''),
            )
        except InspectionRequestError as exc:
            return Response_error_str(str(exc), code=400)
        return Response_200(data={'execution_id': execution.pk, 'status': execution.status})


class InspectionExecutionViewSet(ReadOnlyModelViewSet):
    queryset = InspectionExecution.objects.select_related('task').all()
    serializer_class = InspectionExecutionSerializer
    pagination_class = CustomPagination
    filter_backends = (OrderingFilter, DjangoFilterBackend, SearchFilter)
    search_fields = ['task__name', 'requested_username']
    ordering_fields = ['create_time', 'start_time', 'end_time', 'status']
    permission_classes = [CustomMenuPermission]
    action_perms_map = {
        'list': 'inspection:view',
        'retrieve': 'inspection:view',
        'cancel': 'inspection:executions:cancel',
    }

    def get_serializer_class(self):
        # 列表页不展开 targets/results，否则一页 30 条执行记录会带出上万行检查明细。
        if self.action == 'list':
            return InspectionExecutionListSerializer
        return InspectionExecutionSerializer

    def get_queryset(self):
        queryset = super().get_queryset()
        if self.action != 'list':
            return queryset.prefetch_related('targets__results')

        params = self.request.query_params  # type: ignore[union-attr]
        task_id = params.get('task')
        status = params.get('status')
        trigger_type = params.get('trigger_type')
        # 前端已把日期范围归一化为自然日闭区间（00:00:00 ~ 23:59:59），这里按原值过滤即可。
        start_time = _parse_range_boundary(params.get('start_time'))
        end_time = _parse_range_boundary(params.get('end_time'))

        if task_id:
            queryset = queryset.filter(task_id=task_id)
        if status:
            queryset = queryset.filter(status=status)
        if trigger_type:
            queryset = queryset.filter(trigger_type=trigger_type)
        if start_time:
            queryset = queryset.filter(create_time__gte=start_time)
        if end_time:
            queryset = queryset.filter(create_time__lte=end_time)
        return queryset

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