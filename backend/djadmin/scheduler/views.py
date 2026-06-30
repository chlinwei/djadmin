from rest_framework import viewsets, status
from rest_framework.decorators import action
from rest_framework.response import Response
from scheduler.models import ScheduledTask, ScheduledTaskLog
from scheduler.serializer import ScheduledTaskSerializer, ScheduledTaskLogSerializer
from django.utils import timezone
from django.db.models import Q
from assets.tasks import collect_all_hosts_info
from djadmin.utils import CustomPagination, Response_200, Response_error_str

try:
    from scheduler_manager import start, shutdown, re_register_task, calculate_next_run_time, run_scheduled_task, scheduler
except ImportError:
    # Fallback if module not found
    start = None
    shutdown = None
    re_register_task = None
    calculate_next_run_time = None
    run_scheduled_task = None
    scheduler = None


class ScheduledTaskViewSet(viewsets.ModelViewSet):
    queryset = ScheduledTask.objects.all()
    serializer_class = ScheduledTaskSerializer
    pagination_class = CustomPagination

    def get_queryset(self):
        """Override to support search functionality"""
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
        """Handle task update, including interval_minutes changes"""
        task = self.get_object()
        old_interval = task.interval_minutes
        
        # Save the updated task
        serializer.save()
        
        # If interval_minutes changed, re-register the task in APScheduler
        new_interval = serializer.instance.interval_minutes
        if old_interval != new_interval and re_register_task:
            re_register_task(task.code)

    def perform_create(self, serializer):
        """Create task and register it to scheduler when applicable."""
        task = serializer.save()
        if re_register_task:
            re_register_task(task.code)

    def perform_destroy(self, instance):
        """Delete task and remove related scheduler job."""
        task_code = instance.code
        super().perform_destroy(instance)

        try:
            job_id = f'task_{task_code}'
            if scheduler and getattr(scheduler, 'running', False) and scheduler.get_job(job_id):
                scheduler.remove_job(job_id)
        except Exception:
            pass

    @action(detail=True, methods=['post'])
    def toggle_enabled(self, request, pk=None):
        """Toggle task enabled status"""
        task = self.get_object()
        task.enabled = not task.enabled
        task.save()
        # Re-register task in APScheduler
        if re_register_task:
            re_register_task(task.code)
        return Response_200(ScheduledTaskSerializer(task).data)

    @action(detail=True, methods=['post'])
    def enable(self, request, pk=None):
        """Enable task"""
        task = self.get_object()
        task.enabled = True
        task.save()
        # Re-register task in APScheduler
        if re_register_task:
            re_register_task(task.code)
        return Response_200(ScheduledTaskSerializer(task).data)

    @action(detail=True, methods=['post'])
    def disable(self, request, pk=None):
        """Disable task"""
        task = self.get_object()
        task.enabled = False
        task.save()
        # Re-register task in APScheduler
        if re_register_task:
            re_register_task(task.code)
        return Response_200(ScheduledTaskSerializer(task).data)

    @action(detail=True, methods=['post'])
    def run_now(self, request, pk=None):
        """Execute task immediately (async mode - returns 202 Accepted)"""
        task = self.get_object()
        
        if not task.enabled:
            return Response_error_str('Task is disabled', code=400)
        
        if task.is_running:
            return Response_error_str('Task is already running', code=400)
        
        try:
            # Mark task as running immediately
            task.is_running = True
            task.save(update_fields=['is_running'])
            
            if task.code == 'collect_all_hosts_info':
                # Execute task in background without waiting
                # The scheduler will handle it and update is_running when done
                import threading
                thread = threading.Thread(
                    target=run_scheduled_task,
                    args=(task.code, collect_all_hosts_info),
                    daemon=True
                )
                thread.start()
                
                # Return 200 OK - task is submitted
                return Response({
                    'code': 200,
                    'msg': 'Task submitted to background execution',
                    'data': {
                        'task_id': task.id,
                        'task_name': task.name,
                        'status': 'submitted'
                    }
                })
            else:
                task.is_running = False
                task.save(update_fields=['is_running'])
                return Response_error_str(f'Unknown task code: {task.code}', code=400)
        except Exception as e:
            task.is_running = False
            task.save(update_fields=['is_running'])
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
                'last_status': task.last_status,
                'last_message': task.last_message,
                'last_run_time': task.last_run_time,
                'next_run_time': task.next_run_time
            }
        })

    @action(detail=False, methods=['post'])
    def start_scheduler(self, request):
        """Start the APScheduler"""
        if not start:
            return Response_error_str('Scheduler manager not available', code=400)
        try:
            scheduler = start()
            return Response_200({'status': 'Scheduler started successfully'})
        except Exception as e:
            return Response_error_str(str(e), code=400)

    @action(detail=False, methods=['post'])
    def stop_scheduler(self, request):
        """Stop the APScheduler"""
        if not shutdown:
            return Response_error_str('Scheduler manager not available', code=400)
        try:
            shutdown()
            return Response_200({'status': 'Scheduler stopped successfully'})
        except Exception as e:
            return Response_error_str(str(e), code=400)


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