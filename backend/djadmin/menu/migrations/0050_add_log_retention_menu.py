from datetime import date

from django.db import migrations

MENU_PATH = '/monitor/log-retention'


def add_log_retention_menu(apps, schema_editor):
    SysMenu = apps.get_model('menu', 'SysMenu')
    SysRole = apps.get_model('role', 'SysRole')
    SysRoleMenu = apps.get_model('menu', 'SysRoleMenu')
    today = date.today()

    monitor_dir = SysMenu.objects.filter(path='/monitor', menu_type='M').order_by('id').first()
    if monitor_dir is None:
        return

    menu, _ = SysMenu.objects.update_or_create(
        path=MENU_PATH,
        defaults={
            'name': '日志保留档位',
            'icon': 'fa-clock-rotate-left',
            'parent_id': monitor_dir.id,
            'order_num': 21,
            'component': 'monitor/log-retention/index',
            'menu_type': 'C',
            'is_expanded': True,
            'perms': 'monitor:view',
            'location': 1,
            'create_time': today,
            'update_time': today,
            'remark': 'log retention tier (data stream + ISM policy) menu',
        },
    )

    admin_role = (
        SysRole.objects.filter(code='admin').order_by('id').first()
        or SysRole.objects.filter(name='超级管理员').order_by('id').first()
    )
    if admin_role:
        SysRoleMenu.objects.get_or_create(role_id=admin_role.id, menu_id=menu.id)


def remove_log_retention_menu(apps, schema_editor):
    SysMenu = apps.get_model('menu', 'SysMenu')
    SysRoleMenu = apps.get_model('menu', 'SysRoleMenu')
    menu = SysMenu.objects.filter(path=MENU_PATH).order_by('id').first()
    if menu:
        SysRoleMenu.objects.filter(menu_id=menu.id).delete()
        menu.delete()


class Migration(migrations.Migration):

    dependencies = [
        ('menu', '0049_rename_log_parser_menu'),
    ]

    operations = [
        migrations.RunPython(add_log_retention_menu, remove_log_retention_menu),
    ]
