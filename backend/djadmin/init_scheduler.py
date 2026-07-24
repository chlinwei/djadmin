#!/usr/bin/env python
"""
Initialize scheduler tasks and verify setup
"""
import os
import sys
import django

sys.path.insert(0, os.path.dirname(__file__))
os.environ.setdefault('DJANGO_SETTINGS_MODULE', 'djadmin.settings')
django.setup()

from scheduler_manager import ensure_default_tasks
from scheduler.models import ScheduledTask

ensure_default_tasks()

print(f"Total tasks in database: {ScheduledTask.objects.count()}")
print("\nAll tasks:")
for t in ScheduledTask.objects.all().order_by('id'):
    print(f"  - {t.name} ({t.code}) - Enabled: {t.enabled}")
