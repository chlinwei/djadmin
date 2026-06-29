#!/usr/bin/env python
"""Simulate frontend API call to test run_now response"""

import os
import sys
import json
import time
from subprocess import Popen, PIPE
from threading import Thread

# 首先启动Django服务器（如果还没启动的话）
print("=" * 80)
print("Testing run_now endpoint response")
print("=" * 80)

# 等待一下让请求能连上
time.sleep(2)

# 使用 Django 的 test client
os.environ.setdefault('DJANGO_SETTINGS_MODULE', 'djadmin.settings')
import django
django.setup()

from django.test import Client
from scheduler.models import ScheduledTask
import json

# Get the task
task = ScheduledTask.objects.filter(code='collect_all_hosts_info').first()
if not task:
    print("ERROR: Task not found")
    exit(1)

print(f"\nTest task: {task.name} (ID: {task.id})")
print(f"Initial state - is_running: {task.is_running}, enabled: {task.enabled}")

# Create Django test client
client = Client()

print("\n" + "-" * 80)
print("Step 1: Call run_now endpoint")
print("-" * 80)

# Make POST request to run_now
response = client.post(f'/sys/scheduler/tasks/{task.id}/run_now/')

print(f"\nResponse status code: {response.status_code}")
print(f"Response content-type: {response.get('Content-Type', 'N/A')}")

# Parse response
try:
    data = json.loads(response.content.decode('utf-8'))
    print(f"\nResponse data (parsed as JSON):")
    print(json.dumps(data, indent=2, default=str)[:1500])
    
    # Check if response has the expected structure
    if isinstance(data, dict):
        print(f"\nResponse structure check:")
        print(f"  - Has 'code': {'code' in data}")
        print(f"  - Has 'msg': {'msg' in data}")
        print(f"  - Has 'data': {'data' in data}")
        print(f"  - Code value: {data.get('code', 'N/A')}")
        print(f"  - Msg value: {data.get('msg', 'N/A')}")
except json.JSONDecodeError as e:
    print(f"\nERROR: Failed to parse response as JSON: {e}")
    print(f"Raw response: {response.content[:500]}")

# Check task state after
task.refresh_from_db()
print(f"\nTask state after run_now:")
print(f"  - is_running: {task.is_running}")
print(f"  - last_run_time: {task.last_run_time}")
print(f"  - last_status: {task.last_status}")

print("\n" + "=" * 80)
print("Test completed")
print("=" * 80)
