#!/usr/bin/env python
"""Check if hosts exist in the database"""

import os
import django

os.environ.setdefault('DJANGO_SETTINGS_MODULE', 'djadmin.settings')
django.setup()

from assets.models import Host

print("=" * 80)
print("Host Database Check")
print("=" * 80)

# Count all hosts
all_hosts = Host.objects.all()
print(f"\nTotal hosts in database: {all_hosts.count()}")

if all_hosts.count() == 0:
    print("No hosts found in database!")
else:
    print(f"\nAll hosts:")
    for host in all_hosts:
        print(f"  - ID: {host.id}, IP: {host.ip}, Name: {host.name}, Port: {host.port}")

# Check hosts with valid IP (used by the collection task)
print("\n" + "-" * 80)
print("Hosts with valid IP (what collection task looks for):")
print("-" * 80)

valid_hosts = Host.objects.filter(ip__isnull=False).exclude(ip='')
print(f"\nHosts with non-empty IP: {valid_hosts.count()}")

if valid_hosts.count() == 0:
    print("ERROR: No hosts with valid IP found!")
    print("\nThis is why collection task finds 0 hosts.")
else:
    for host in valid_hosts:
        print(f"  - ID: {host.id}, IP: {host.ip}, Name: {host.name}")

print("\n" + "=" * 80)
print("Check completed")
print("=" * 80)
