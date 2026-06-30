import django, os
os.environ.setdefault('DJANGO_SETTINGS_MODULE', 'djadmin.settings')
django.setup()

from user.models import SysUser, SysUserRole
from django.db.models import Prefetch

# 直接测试查询
print('=== 测试 get_queryset ===')
qs = SysUser.objects.prefetch_related(
    Prefetch('sysuserrole_set', queryset=SysUserRole.objects.select_related('role'))
).order_by('-id')

print(f'Count: {qs.count()}')
for user in qs[:3]:
    print(f'用户: {user.username}')
