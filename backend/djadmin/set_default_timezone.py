import os
import django

os.environ.setdefault('DJANGO_SETTINGS_MODULE', 'djadmin.settings')
django.setup()

from user.models import SysUser

# 为所有没有时区的用户设置默认时区
users = SysUser.objects.filter(timezone__isnull=True) | SysUser.objects.filter(timezone='')
updated_count = 0

for user in users:
    user.timezone = 'UTC'
    user.save()
    updated_count += 1

print(f"Updated {updated_count} users with timezone='UTC'")

