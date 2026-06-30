#!/usr/bin/env python
import os
import django
import json

os.environ.setdefault('DJANGO_SETTINGS_MODULE', 'djadmin.settings')
django.setup()

from scheduler.models import ScheduledTask
from scheduler.serializer import ScheduledTaskSerializer
from djadmin.utils import CustomPagination
from rest_framework.test import APIRequestFactory

# Get all tasks
tasks = ScheduledTask.objects.all()
print(f"Total tasks in DB: {tasks.count()}")

if tasks.count() > 0:
    print("\nAll tasks:")
    for task in tasks:
        print(f"  - ID: {task.id}, Name: {task.name}, Code: {task.code}")
    
    # Test serializer
    serializer = ScheduledTaskSerializer(tasks.first())
    print("\nFirst task serialized:")
    print(json.dumps(serializer.data, indent=2, ensure_ascii=False, default=str))
    
    # Test pagination response format
    print("\n--- Testing pagination response format ---")
    factory = APIRequestFactory()
    request = factory.get('/sys/scheduler/tasks/?page=1')
    
    # Mock paginator
    paginator = CustomPagination()
    page = paginator.paginate_queryset(tasks, request)
    response_data = paginator.get_paginated_response(
        ScheduledTaskSerializer(page, many=True).data
    )
    
    print("\nPaginated response:")
    print(json.dumps(response_data.data, indent=2, ensure_ascii=False, default=str))
else:
    print("No tasks found in database!")
