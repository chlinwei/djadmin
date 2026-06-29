#!/usr/bin/env python
"""Test the full run_now flow with proper response rendering"""

import os
import sys
import django
import json
from io import StringIO

os.environ.setdefault('DJANGO_SETTINGS_MODULE', 'djadmin.settings')
django.setup()

from django.test import Client
from scheduler.models import ScheduledTask

# Get the task
task = ScheduledTask.objects.filter(code='collect_all_hosts_info').first()
if not task:
    print("ERROR: Task not found")
    exit(1)

print("=" * 80)
print("Complete run_now Test")
print("=" * 80)

# Reset task state
task.is_running = False
task.save()

print(f"\nTest task: {task.name} (ID: {task.id})")
print(f"Initial state - is_running: {task.is_running}, enabled: {task.enabled}")

# Use Django's test client with proper SERVER_NAME
client = Client(SERVER_NAME='testserver')

print("\n" + "-" * 80)
print("Step 1: Call run_now endpoint")
print("-" * 80)

try:
    response = client.post(f'/sys/scheduler/tasks/{task.id}/run_now/')
    
    print(f"\n✓ Request succeeded")
    print(f"  - Status code: {response.status_code}")
    print(f"  - Content-Type: {response.get('Content-Type', 'N/A')}")
    
    # Parse response
    try:
        response_data = json.loads(response.content.decode('utf-8'))
        
        print(f"\n✓ Response parsed as JSON")
        print(f"  - Has 'code': {('code' in response_data)}")
        print(f"  - Has 'msg': {('msg' in response_data)}")
        print(f"  - Has 'data': {('data' in response_data)}")
        print(f"  - Code value: {response_data.get('code', 'N/A')}")
        
        if response_data.get('code') == 200:
            print(f"\n✓ Response has correct code (200)")
        else:
            print(f"\n✗ Response code is not 200: {response_data.get('code')}")
            
        # Show sample of response data
        data_preview = json.dumps(response_data, indent=2, default=str)[:800]
        print(f"\n Response preview:")
        print(data_preview)
        
    except json.JSONDecodeError as e:
        print(f"\n✗ Failed to parse JSON: {e}")
        print(f"  Raw content: {response.content[:200]}")
        
except Exception as e:
    print(f"\n✗ Request failed: {e}")
    import traceback
    traceback.print_exc()

# Check task state after
task.refresh_from_db()
print(f"\n" + "-" * 80)
print("Step 2: Check task state after run_now")
print("-" * 80)
print(f"\nTask state after:")
print(f"  - is_running: {task.is_running} {'✓' if not task.is_running else '✗'}")
print(f"  - last_run_time: {task.last_run_time}")
print(f"  - last_status: {task.last_status}")

# Check logs
from scheduler.models import ScheduledTaskLog
log = ScheduledTaskLog.objects.filter(task=task).last()
if log:
    print(f"\nLatest log:")
    print(f"  - Status: {log.status}")
    print(f"  - Duration: {log.duration_seconds}s")
    print(f"  - Message: {log.message}")

print("\n" + "=" * 80)
if task.is_running:
    print("✗ TEST FAILED: is_running is still True")
    sys.exit(1)
elif response_data.get('code') != 200:
    print("✗ TEST FAILED: Response code is not 200")
    sys.exit(1)
else:
    print("✓ TEST PASSED: All checks successful")
    sys.exit(0)
print("=" * 80)
