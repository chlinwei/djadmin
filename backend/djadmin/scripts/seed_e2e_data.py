#!/usr/bin/env python3
"""Seed minimal data for frontend Playwright E2E login.

Creates:
- admin role (code=admin)
- admin user (username/password = admin/admin)
- user-role relation
- minimal menu and role-menu relation
"""

from __future__ import annotations

import os
import sys
from pathlib import Path
from datetime import date

PROJECT_DIR = Path(__file__).resolve().parents[1]
if str(PROJECT_DIR) not in sys.path:
    sys.path.insert(0, str(PROJECT_DIR))

os.environ.setdefault('DJANGO_SETTINGS_MODULE', 'djadmin.e2e_settings')

import django


def main() -> int:
    django.setup()

    from user.models import SysUser, SysUserRole
    from role.models import SysRole
    from menu.models import SysMenu, SysRoleMenu

    today = date.today()

    role, _ = SysRole.objects.get_or_create(
        code='admin',
        defaults={
            'name': '管理员',
            'create_time': today,
            'update_time': today,
            'remark': 'playwright-e2e seed role',
        },
    )
    if not role.name:
        role.name = '管理员'
        role.update_time = today
        role.save(update_fields=['name', 'update_time'])

    user, created = SysUser.objects.get_or_create(
        username='admin',
        defaults={
            'status': 1,
            'timezone': 'Asia/Shanghai',
            'create_time': today,
            'update_time': today,
            'remark': 'playwright-e2e seed user',
        },
    )

    user.status = 1
    user.timezone = user.timezone or 'Asia/Shanghai'
    user.create_time = user.create_time or today
    user.update_time = today
    user.set_password('admin')
    # Keep field type compatibility for Date/DateTime migrations.
    user.login_date = None
    user.save()

    SysUserRole.objects.get_or_create(user=user, role=role)

    menu, _ = SysMenu.objects.get_or_create(
        path='/index',
        defaults={
            'name': '首页',
            'icon': 'House',
            'parent_id': 0,
            'order_num': 1,
            'component': 'index/index',
            'menu_type': 'C',
            'perms': 'system:index:view',
            'location': 1,
            'create_time': today,
            'update_time': today,
            'remark': 'playwright-e2e seed menu',
        },
    )

    SysRoleMenu.objects.get_or_create(role=role, menu=menu)

    print('[OK] e2e seed ready: user=admin/admin, role=admin, menu=/index')
    return 0


if __name__ == '__main__':
    raise SystemExit(main())
