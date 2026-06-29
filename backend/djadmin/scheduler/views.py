from rest_framework import viewsets, status
from rest_framework.decorators import action
from rest_framework.response import Response
from scheduler.models import ScheduledTask, ScheduledTaskLog
from scheduler.serializer import ScheduledTaskSerializer, ScheduledTaskLogSerializer
from django.utils import timezone
from assets.tasks import collect_all_hosts_info
from djadmin.utils import CustomPagination

try:
    from scheduler_manager import start, shutdown, re_register_task, calculate_next_run_time, run_scheduled_task
except ImportError:
    # Fallback if module not found
    start = None
    shutdown = None
    re_register_task = None
    calculate_next_run_time = None
    run_scheduled_task = None


class ScheduledTaskViewSet(viewsets.ModelViewSet):
    queryset = ScheduledTask.objects.all()
    serializer_class = ScheduledTaskSerializer

    def get_queryset(self):
        """Override to support search functionality"""
        queryset = ScheduledTask.objects.all()
        search = self.request.query_params.get('search')
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

    @action(detail=True, methods=['post'])
    def toggle_enabled(self, request, pk=None):
        """Toggle task enabled status"""
        task = self.get_object()
        task.enabled = not task.enabled
        task.save()
        # Re-register task in APScheduler
        if re_register_task:
            re_register_task(task.code)
        return Response(ScheduledTaskSerializer(task).data)

    @action(detail=True, methods=['post'])
    def enable(self, request, pk=None):
        """Enable task"""
        task = self.get_object()
        task.enabled = True
        task.save()
        # Re-register task in APScheduler
        if re_register_task:
            re_register_task(task.code)
        return Response(ScheduledTaskSerializer(task).data)

    @action(detail=True, methods=['post'])
    def disable(self, request, pk=None):
        """Disable task"""
        task = self.get_object()
        task.enabled = False
        task.save()
        # Re-register task in APScheduler
        if re_register_task:
            re_register_task(task.code)
        return Response(ScheduledTaskSerializer(task).data)

    @action(detail=True, methods=['post'])
    def run_now(self, request, pk=None):
        """Execute task immediately (async mode - returns 202 Accepted)"""
        task = self.get_object()
        
        if not task.enabled:
            return Response(
                {'error': 'Task is disabled'},
                status=status.HTTP_400_BAD_REQUEST
            )
        
        if task.is_running:
            return Response(
                {'error': 'Task is already running'},
                status=status.HTTP_400_BAD_REQUEST
            )
        
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
                return Response(
                    {'error': f'Unknown task code: {task.code}'},
                    status=status.HTTP_400_BAD_REQUEST
                )
        except Exception as e:
            task.is_running = False
            task.save(update_fields=['is_running'])
            return Response(
                {'error': f'Task submission failed: {str(e)}'},
                status=status.HTTP_400_BAD_REQUEST
            )

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
            return Response({'error': 'Scheduler manager not available'}, status=status.HTTP_400_BAD_REQUEST)
        try:
            scheduler = start()
            return Response({'status': 'Scheduler started successfully'})
        except Exception as e:
            return Response({'error': str(e)}, status=status.HTTP_400_BAD_REQUEST)

    @action(detail=False, methods=['post'])
    def stop_scheduler(self, request):
        """Stop the APScheduler"""
        if not shutdown:
            return Response({'error': 'Scheduler manager not available'}, status=status.HTTP_400_BAD_REQUEST)
        try:
            shutdown()
            return Response({'status': 'Scheduler stopped successfully'})
        except Exception as e:
            return Response({'error': str(e)}, status=status.HTTP_400_BAD_REQUEST)


class ScheduledTaskLogViewSet(viewsets.ReadOnlyModelViewSet):
    serializer_class = ScheduledTaskLogSerializer
    pagination_class = CustomPagination

    def get_queryset(self):
        queryset = ScheduledTaskLog.objects.all()
        task_id = self.request.query_params.get('task_id')
        if task_id:
            queryset = queryset.filter(task_id=task_id)
        return queryset.order_by('-run_time')