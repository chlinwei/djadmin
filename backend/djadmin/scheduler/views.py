from rest_framework import mixins, viewsets, status
from rest_framework.decorators import action
from rest_framework.response import Response
from scheduler.models import ScheduledTask, ScheduledTaskLog
from scheduler.serializer import ScheduledTaskSerializer, ScheduledTaskLogSerializer
from scheduler.celery_health import has_active_celery_worker
from django.db.models import Q
from djadmin.utils import CustomPagination, Response_200, Response_error_str
from scheduler.tasks import execute_scheduled_task
from scheduler_manager import (
    calculate_next_run_time,
    ensure_default_tasks,
    is_scheduler_enabled,
    set_scheduler_enabled,
)


class ScheduledTaskViewSet(
    mixins.ListModelMixin,
    mixins.RetrieveModelMixin,
    mixins.UpdateModelMixin,
    viewsets.GenericViewSet,
):
    queryset = ScheduledTask.objects.all()
    serializer_class = ScheduledTaskSerializer
    pagination_class = CustomPagination
    http_method_names = ['get', 'patch', 'head', 'options']

    def get_queryset(self):
        """Override to support search functionality"""
        ensure_default_tasks()
        queryset = ScheduledTask.objects.all()
        search = self.request.query_params.get('search')  # type: ignore[union-attr]
        if search:
            # Search by task name or code (case-insensitive)
            from django.db.models import Q
            queryset = queryset.filter(
                Q(name__icontains=search) | Q(code__icontains=search)
            )
        return queryset.order_by('-id')

    def perform_update(self, serializer):
        task = serializer.save()
        calculate_next_run_time(task)

    @action(detail=True, methods=['post'])
    def toggle_enabled(self, request, pk=None):
        task = self.get_object()
        task.enabled = not task.enabled
        task.save()
        calculate_next_run_time(task)
        return Response_200(ScheduledTaskSerializer(task).data)

    @action(detail=True, methods=['post'])
    def enable(self, request, pk=None):
        task = self.get_object()
        task.enabled = True
        task.save()
        calculate_next_run_time(task)
        return Response_200(ScheduledTaskSerializer(task).data)

    @action(detail=True, methods=['post'])
    def disable(self, request, pk=None):
        task = self.get_object()
        task.enabled = False
        task.save()
        calculate_next_run_time(task)
        return Response_200(ScheduledTaskSerializer(task).data)

    @action(detail=True, methods=['post'])
    def run_now(self, request, pk=None):
        task = self.get_object()

        if not task.enabled:
            return Response_error_str('Task is disabled', code=400)

        if task.is_running:
            return Response_error_str('Task is already running', code=400)

        if not has_active_celery_worker():
            return Response_error_str('Celery worker is not running', code=400)

        try:
            execute_scheduled_task.delay(task.code)
            return Response({
                'code': 200,
                'msg': 'Task submitted to Celery worker',
                'data': {
                    'task_id': task.id,
                    'task_name': task.name,
                    'status': 'submitted',
                },
            })
        except Exception as e:
            return Response_error_str(f'Task submission failed: {str(e)}', code=400)

    @action(detail=True, methods=['get'])
    def status(self, request, pk=None):
        """Get task execution status"""
        task = self.get_object()
        return Response({
            'code': 200,
            'msg': 'success',
            'data': {
                'task_id': task.id,
                'task_name': task.name,
                'is_running': task.is_running,
                'scheduler_enabled': is_scheduler_enabled(),
                'last_status': task.last_status,
                'last_message': task.last_message,
                'last_run_time': task.last_run_time,
                'next_run_time': task.next_run_time
            }
        })

    @action(detail=False, methods=['post'])
    def start_scheduler(self, request):
        set_scheduler_enabled(True)
        return Response_200({'status': 'Celery scheduler enabled'})

    @action(detail=False, methods=['post'])
    def stop_scheduler(self, request):
        set_scheduler_enabled(False)
        return Response_200({'status': 'Celery scheduler disabled'})


class ScheduledTaskLogViewSet(viewsets.ReadOnlyModelViewSet):
    serializer_class = ScheduledTaskLogSerializer
    pagination_class = CustomPagination

    def get_queryset(self):
        queryset = ScheduledTaskLog.objects.all()
        task_id = self.request.query_params.get('task_id')  # type: ignore[union-attr]
        status_value = self.request.query_params.get('status')  # type: ignore[union-attr]
        duration_min = self.request.query_params.get('duration_min')  # type: ignore[union-attr]
        duration_max = self.request.query_params.get('duration_max')  # type: ignore[union-attr]
        content = self.request.query_params.get('content')  # type: ignore[union-attr]
        ordering = self.request.query_params.get('ordering')  # type: ignore[union-attr]

        if task_id:
            queryset = queryset.filter(task_id=task_id)

        if status_value:
            normalized = str(status_value).strip().lower()
            if normalized in ('success', '成功', 'succeeded', 'ok'):
                queryset = queryset.filter(status__in=['success', '成功', 'succeeded', 'ok'])
            elif normalized in ('failed', 'failure', 'error', '失败', '异常'):
                queryset = queryset.filter(status__in=['failed', 'failure', 'error', '失败', '异常'])
            else:
                queryset = queryset.filter(status__iexact=status_value)

        if duration_min not in (None, ''):
            try:
                queryset = queryset.filter(duration_seconds__gte=float(duration_min))
            except (TypeError, ValueError):
                pass

        if duration_max not in (None, ''):
            try:
                queryset = queryset.filter(duration_seconds__lte=float(duration_max))
            except (TypeError, ValueError):
                pass

        if content not in (None, ''):
            keyword = str(content).strip()
            if keyword:
                queryset = queryset.filter(
                    Q(message__icontains=keyword) |
                    Q(output__icontains=keyword)
                )

        allowed_ordering_fields = {'status', 'duration_seconds', 'run_time'}
        if ordering:
            normalized = ordering.strip()
            order_field = normalized.lstrip('-')
            if order_field in allowed_ordering_fields:
                return queryset.order_by(normalized, '-run_time')

        return queryset.order_by('-run_time')