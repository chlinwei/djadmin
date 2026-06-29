#!/usr/bin/env python
"""
Test scheduler startup
"""
import os
import sys
import time
import django

sys.path.insert(0, os.path.dirname(__file__))
os.environ.setdefault('DJANGO_SETTINGS_MODULE', 'djadmin.settings')
django.setup()

from scheduler_manager import start, shutdown
from scheduler.models import ScheduledTask

print("=" * 60)
print("Scheduler Startup Test")
print("=" * 60)

# Start scheduler
print("\nStarting scheduler...")
scheduler = start()

print(f"\nScheduler Status:")
print(f"  Running: {scheduler.running}")
print(f"  Jobs count: {len(scheduler.get_jobs())}")
for job in scheduler.get_jobs():
    print(f"    - {job.id}: {job.name}")
    print(f"      Next run time: {job.next_run_time}")

print("\nWaiting 5 seconds to check if scheduler is working...")
time.sleep(5)

print(f"\nScheduler still running: {scheduler.running}")

# Stop scheduler
print("\nStopping scheduler...")
shutdown()

print("\n" + "=" * 60)
