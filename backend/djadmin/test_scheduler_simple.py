#!/usr/bin/env python
import os
import django
import json

os.environ.setdefault('DJANGO_SETTINGS_MODULE', 'djadmin.settings')
django.setup()

from scheduler.models import ScheduledTask
from scheduler.serializer import ScheduledTaskSerializer

# Get all tasks
tasks = ScheduledTask.objects.all()
print(f"Total tasks: {tasks.count()}")

if tasks.count() > 0:
    # Serialize first task only
    task = tasks.first()
    serializer = ScheduledTaskSerializer(task)
    print("\nFirst task serialized:")
    print(json.dumps(serializer.data, indent=2, ensure_ascii=False, default=str))
    
    # Serialize all tasks
    serializer_many = ScheduledTaskSerializer(tasks[:1], many=True)
    print("\nMultiple tasks serialized:")
    print(json.dumps(serializer_many.data, indent=2, ensure_ascii=False, default=str))
