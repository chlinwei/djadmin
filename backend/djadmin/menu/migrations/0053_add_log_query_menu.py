from datetime import date

from django.db import migrations


def add_log_query_menu(apps, schema_editor):
    SysMenu = apps.get_model('menu', 'SysMenu')
    SysRole = apps.get_model('role', 'SysRole')
    SysRoleMenu = apps.get_model('menu', 'SysRoleMenu')
    today = date.today()

    monitor_dir = SysMenu.objects.filter(path='/monitor', menu_type='M').order_by('id').first()
    if monitor_dir is None:
        return

    menu, _ = SysMenu.objects.update_or_create(
        path='/monitor/log-query',
        defaults={
            'name': '日志查询',
            'icon': 'fa-search',
            'parent_id': monitor_dir.id,
            'order_num': 21,
            'component': 'monitor/log-query/index',
            'menu_type': 'C',
            'is_expanded': True,
            # 复用 monitor:view：与本文件其余日志相关菜单（存储/解析规则/保留策略）保持同一权限粒度。
            'perms': 'monitor:view',
            'location': 1,
            'create_time': today,
            'update_time': today,
            'remark': 'per-service raw log search menu',
        },
    )

    admin_role = (
        SysRole.objects.filter(code='admin').order_by('id').first()
        or SysRole.objects.filter(name='超级管理员').order_by('id').first()
    )
    if admin_role:
        SysRoleMenu.objects.get_or_create(role_id=admin_role.id, menu_id=menu.id)


def remove_log_query_menu(apps, schema_editor):
    SysMenu = apps.get_model('menu', 'SysMenu')
    SysRoleMenu = apps.get_model('menu', 'SysRoleMenu')
    menu = SysMenu.objects.filter(path='/monitor/log-query').order_by('id').first()
    if menu:
        SysRoleMenu.objects.filter(menu_id=menu.id).delete()
        menu.delete()


class Migration(migrations.Migration):

    dependencies = [
        ('menu', '0052_fix_invalid_menu_icons'),
    ]

    operations = [
        migrations.RunPython(add_log_query_menu, remove_log_query_menu),
    ]
