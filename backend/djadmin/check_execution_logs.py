#!/usr/bin/env python
"""Check execution logs and task status"""

import os
import django
import json

os.environ.setdefault('DJANGO_SETTINGS_MODULE', 'djadmin.settings')
django.setup()

from scheduler.models import ScheduledTask, ScheduledTaskLog
from django.utils import timezone

print("=" * 80)
print("Task Execution Log Check")
print("=" * 80)

# Get the task
task = ScheduledTask.objects.filter(code='collect_all_hosts_info').first()
if not task:
    print("\nERROR: Task not found")
    exit(1)

print(f"\nTask: {task.name}")
print(f"Status: is_running={task.is_running}, enabled={task.enabled}")
print(f"Last run: {task.last_run_time}")
print(f"Last status: {task.last_status}")
print(f"Last message: {task.last_message}")

# Get recent logs
print("\n" + "=" * 80)
print("Recent Execution Logs")
print("=" * 80)

logs = ScheduledTaskLog.objects.filter(task=task).order_by('-run_time')[:5]

if not logs.exists():
    print("\nNo logs found!")
else:
    print(f"\nFound {logs.count()} recent log entries:\n")
    
    for i, log in enumerate(logs, 1):
        print(f"--- Log #{i} ---")
        print(f"Run time: {log.run_time}")
        print(f"Status: {log.status}")
        print(f"Message: {log.message}")
        print(f"Duration: {log.duration_seconds}s")
        print(f"Output length: {len(log.output) if log.output else 0} chars")
        
        if log.output:
            print(f"\nOutput (first 500 chars):")
            print(log.output[:500])
            if len(log.output) > 500:
                print(f"\n... ({len(log.output) - 500} more characters)")
        else:
            print(f"\nOutput: [EMPTY]")
        
        print()

print("=" * 80)
print("Check completed")
print("=" * 80)
