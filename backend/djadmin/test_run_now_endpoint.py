#!/usr/bin/env python
"""Test run_now API endpoint directly"""

import os
import django
import json

os.environ.setdefault('DJANGO_SETTINGS_MODULE', 'djadmin.settings')
django.setup()

from django.test import RequestFactory
from scheduler.views import ScheduledTaskViewSet
from scheduler.models import ScheduledTask

# Get the task
task = ScheduledTask.objects.filter(code='collect_all_hosts_info').first()
if not task:
    print("ERROR: Task not found")
    exit(1)

print("=" * 80)
print("Testing run_now endpoint")
print("=" * 80)

# Create a request factory and request
factory = RequestFactory()
request = factory.post(f'/sys/scheduler/tasks/{task.id}/run_now/')

# Create view
view = ScheduledTaskViewSet.as_view({'post': 'run_now'})

# Call the endpoint
print(f"\nCalling run_now for task: {task.name} (ID: {task.id})")
print(f"Task state before:")
print(f"  - is_running: {task.is_running}")
print(f"  - enabled: {task.enabled}")

response = view(request, pk=task.id)

print(f"\nResponse status: {response.status_code}")
print(f"Response data type: {type(response.data)}")

# Print response
print(f"\nResponse content:")
print(json.dumps(response.data, indent=2, default=str)[:1500])

# Check task state after
task.refresh_from_db()
print(f"\nTask state after:")
print(f"  - is_running: {task.is_running}")
print(f"  - last_run_time: {task.last_run_time}")
print(f"  - last_status: {task.last_status}")

print("\n" + "=" * 80)
print("Test completed")
print("=" * 80)
