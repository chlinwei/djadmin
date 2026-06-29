#!/usr/bin/env python
"""Detailed host field analysis"""

import os
import django

os.environ.setdefault('DJANGO_SETTINGS_MODULE', 'djadmin.settings')
django.setup()

from assets.models import Host

print("=" * 80)
print("Detailed Host Field Analysis")
print("=" * 80)

all_hosts = Host.objects.all()

for host in all_hosts:
    print(f"\nHost ID {host.id}:")
    print(f"  - ip: {repr(host.ip)}")
    print(f"  - ip type: {type(host.ip)}")
    print(f"  - ip is None: {host.ip is None}")
    print(f"  - ip == '': {host.ip == ''}")
    print(f"  - bool(ip): {bool(host.ip)}")
    print(f"  - name: {repr(host.name)}")
    print(f"  - port: {host.port}")

print("\n" + "=" * 80)
print("Manually testing filter condition:")
print("=" * 80)

# Test each filter separately
print("\nip__isnull=False:")
hosts1 = Host.objects.filter(ip__isnull=False)
print(f"  Count: {hosts1.count()}")
for host in hosts1:
    print(f"    - {host.id}: {repr(host.ip)}")

print("\nip != '':")
hosts2 = Host.objects.exclude(ip='')
print(f"  Count: {hosts2.count()}")
for host in hosts2:
    print(f"    - {host.id}: {repr(host.ip)}")

print("\nBoth conditions (ip__isnull=False AND exclude(ip='')):")
hosts3 = Host.objects.filter(ip__isnull=False).exclude(ip='')
print(f"  Count: {hosts3.count()}")
for host in hosts3:
    print(f"    - {host.id}: {repr(host.ip)}")

print("\n" + "=" * 80)
print("Analysis completed")
print("=" * 80)
