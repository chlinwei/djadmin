from rest_framework import serializers
from scheduler.models import ScheduledTask, ScheduledTaskLog


class ScheduledTaskLogSerializer(serializers.ModelSerializer):
    task_name = serializers.CharField(source='task.name', read_only=True)

    class Meta:
        model = ScheduledTaskLog
        fields = ['id', 'task_name', 'run_time', 'status', 'message', 'duration_seconds', 'output']


class ScheduledTaskSerializer(serializers.ModelSerializer):
    logs = ScheduledTaskLogSerializer(many=True, read_only=True)
    menu_name = serializers.CharField(source='menu.name', read_only=True)
    menu_path = serializers.CharField(source='menu.path', read_only=True)

    class Meta:
        model = ScheduledTask
        fields = ['id', 'name', 'code', 'description', 'menu', 'menu_name', 'menu_path', 'enabled', 'is_running', 'interval_minutes',
                  'last_run_time', 'next_run_time', 'last_status', 'last_message', 'create_time', 'update_time', 'logs']
        read_only_fields = ['id', 'is_running', 'next_run_time', 'create_time', 'update_time']