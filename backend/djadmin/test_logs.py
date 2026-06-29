#!/usr/bin/env python
"""
Test scheduler logs API response
"""
import os
import sys
import django
import json

sys.path.insert(0, os.path.dirname(__file__))
os.environ.setdefault('DJANGO_SETTINGS_MODULE', 'djadmin.settings')
django.setup()

from scheduler.models import ScheduledTask, ScheduledTaskLog
from scheduler.serializer import ScheduledTaskLogSerializer, ScheduledTaskSerializer

print("=" * 60)
print("Scheduler Logs Test")
print("=" * 60)

# Get all tasks
tasks = ScheduledTask.objects.all()
print(f"\nTotal tasks: {tasks.count()}")

for task in tasks:
    print(f"\n▶ Task: {task.name} (ID: {task.id})")
    print(f"  Code: {task.code}")
    print(f"  Enabled: {task.enabled}")
    
    # Get logs for this task
    logs = ScheduledTaskLog.objects.filter(task_id=task.id)
    print(f"  Logs count: {logs.count()}")
    
    if logs.exists():
        # Simulate API response
        serializer = ScheduledTaskLogSerializer(logs, many=True)
        api_response = {
            'code': 200,
            'msg': 'success',
            'data': {
                'count': logs.count(),
                'results': serializer.data
            }
        }
        print("\n  API Response:")
        print(json.dumps(api_response, indent=4, default=str, ensure_ascii=False))
    else:
        print("  No logs found")

print("\n" + "=" * 60)
