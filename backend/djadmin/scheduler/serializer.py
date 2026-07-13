from rest_framework import serializers
from scheduler.models import ScheduledTask, ScheduledTaskLog
from scheduler_manager import (
    get_task_cron_expression,
    validate_cron_expression,
)


class ScheduledTaskLogSerializer(serializers.ModelSerializer):
    task_name = serializers.CharField(source='task.name', read_only=True)

    class Meta:
        model = ScheduledTaskLog
        fields = ['id', 'task_name', 'run_time', 'status', 'message', 'duration_seconds', 'output']


class ScheduledTaskSerializer(serializers.ModelSerializer):
    logs = ScheduledTaskLogSerializer(many=True, read_only=True)
    menu_name = serializers.CharField(source='menu.name', read_only=True)
    menu_path = serializers.CharField(source='menu.path', read_only=True)
    effective_cron_expression = serializers.SerializerMethodField(read_only=True)

    def get_effective_cron_expression(self, obj):
        return get_task_cron_expression(obj)

    def validate(self, attrs):
        cron_text = str(attrs.get('cron_expression') or '').strip()
        if cron_text:
            ok, error_message = validate_cron_expression(cron_text)
            if not ok:
                raise serializers.ValidationError({'cron_expression': error_message or 'cron 表达式无效'})
            attrs['cron_expression'] = cron_text
            attrs['interval_minutes'] = None

        return attrs

    class Meta:
        model = ScheduledTask
        fields = ['id', 'name', 'code', 'description', 'menu', 'menu_name', 'menu_path', 'enabled', 'is_running', 'cron_expression', 'effective_cron_expression',
                  'last_run_time', 'next_run_time', 'last_status', 'last_message', 'create_time', 'update_time', 'logs']
        read_only_fields = ['id', 'menu', 'is_running', 'next_run_time', 'create_time', 'update_time']