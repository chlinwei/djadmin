#!/usr/bin/env python
"""Check if host info was updated correctly"""

import os
import django

os.environ.setdefault('DJANGO_SETTINGS_MODULE', 'djadmin.settings')
django.setup()

from assets.models import Host, HostSystem

print("=" * 80)
print("Host Information Check")
print("=" * 80)

hosts = Host.objects.filter(ip__isnull=False).order_by('ip')

for host in hosts:
    print(f"\nHost ID {host.id}:")
    print(f"  - IP: {host.ip}")
    print(f"  - Name: {host.name}")
    print(f"  - Port: {host.port}")
    
    # Check system info
    sys_info = HostSystem.objects.filter(host=host).first()
    if sys_info:
        print(f"  - OS Hostname: {sys_info.hostname}")
        print(f"  - OS Type: {sys_info.os_type}")
        print(f"  - OS Version: {sys_info.os_version}")
        print(f"  - Kernel Version: {sys_info.kernel_version}")
        print(f"  - Last Updated: {sys_info.update_time}")
    else:
        print(f"  - OS Info: Not collected yet")

print("\n" + "=" * 80)
print("Check completed")
print("=" * 80)
