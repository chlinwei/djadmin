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

from scheduler.models import ScheduledTask, ScheduledTaskLog

# Initialize default task
defaults = {
    'name': '主机信息采集',
    'description': '定时采集所有主机信息',
    'enabled': True,
    'interval_minutes': 15,
}
task, created = ScheduledTask.objects.get_or_create(
    code='collect_all_hosts_info',
    defaults=defaults,
)

print(f"{'✓ Created' if created else '✓ Found'} task: {task.name}")
print(f"Total tasks in database: {ScheduledTask.objects.count()}")
print("\nAll tasks:")
for t in ScheduledTask.objects.all():
    print(f"  - {t.name} ({t.code}) - Enabled: {t.enabled}")
