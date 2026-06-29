#!/usr/bin/env python
"""Test script to verify is_running field in serializer"""

import os
import django
import json

os.environ.setdefault('DJANGO_SETTINGS_MODULE', 'djadmin.settings')
django.setup()

from scheduler.models import ScheduledTask
from scheduler.serializer import ScheduledTaskSerializer

# Test 1: Check database has is_running field
print("=" * 60)
print("Test 1: Check database has is_running field")
print("=" * 60)

tasks = ScheduledTask.objects.all()
print(f"Total tasks in database: {tasks.count()}")

for task in tasks[:3]:
    print(f"\nTask: {task.name}")
    print(f"  - id: {task.id}")
    print(f"  - is_running: {task.is_running}")
    print(f"  - enabled: {task.enabled}")
    print(f"  - interval_minutes: {task.interval_minutes}")

# Test 2: Check serializer includes is_running
print("\n" + "=" * 60)
print("Test 2: Check serializer includes is_running field")
print("=" * 60)

if tasks.exists():
    task = tasks.first()
    serializer = ScheduledTaskSerializer(task)
    data = serializer.data
    
    print(f"\nSerialized task response:")
    print(f"Fields in response: {list(data.keys())}")
    
    if 'is_running' in data:
        print(f"\n✓ 'is_running' field found in serializer: {data['is_running']}")
    else:
        print(f"\n✗ 'is_running' field NOT found in serializer")
        
    # Print full serialized data for debugging
    print(f"\nFull serialized data (first 1000 chars):")
    print(json.dumps(data, indent=2, default=str)[:1000])

print("\n" + "=" * 60)
print("Test completed")
print("=" * 60)
