#!/usr/bin/env python
"""
Manual task execution for testing
"""
import os
import sys
import django

sys.path.insert(0, os.path.dirname(__file__))
os.environ.setdefault('DJANGO_SETTINGS_MODULE', 'djadmin.settings')
django.setup()

from scheduler_manager import run_scheduled_task
from assets.tasks import collect_all_hosts_info
from django.utils import timezone

print("=" * 60)
print("Manual Task Execution")
print("=" * 60)

# Execute the task manually
print(f"\nExecuting task at {timezone.now()}...")
run_scheduled_task('collect_all_hosts_info', collect_all_hosts_info)

# Verify the update
from scheduler.models import ScheduledTask
task = ScheduledTask.objects.filter(code='collect_all_hosts_info').first()
if task:
    print(f"\nTask Status After Execution:")
    print(f"  Name: {task.name}")
    print(f"  Enabled: {task.enabled}")
    print(f"  Last Run Time: {task.last_run_time}")
    print(f"  Last Status: {task.last_status}")
    print(f"  Last Message: {task.last_message}")
else:
    print("ERROR: Task not found!")

print("\n" + "=" * 60)
