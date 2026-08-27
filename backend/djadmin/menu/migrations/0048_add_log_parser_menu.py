from datetime import date

from django.db import migrations


def add_log_parser_menu(apps, schema_editor):
    SysMenu = apps.get_model('menu', 'SysMenu')
    SysRole = apps.get_model('role', 'SysRole')
    SysRoleMenu = apps.get_model('menu', 'SysRoleMenu')
    monitor_dir = SysMenu.objects.filter(path='/monitor', menu_type='M').order_by('id').first()
    if monitor_dir is None:
        return

    today = date.today()
    menu, _ = SysMenu.objects.update_or_create(
        path='/monitor/log-parsers',
        defaults={
            'name': '日志解析规则',
            'icon': 'fa-code',
            'parent_id': monitor_dir.id,
            'order_num': 21,
            'component': 'monitor/log-parsers/index',
            'menu_type': 'C',
            'is_expanded': True,
            'perms': 'monitor:view',
            'location': 1,
            'create_time': today,
            'update_time': today,
            'remark': 'OpenSearch ingest pipeline management menu',
        },
    )

    admin_role = (
        SysRole.objects.filter(code='admin').order_by('id').first()
        or SysRole.objects.filter(name='超级管理员').order_by('id').first()
    )
    if admin_role:
        SysRoleMenu.objects.get_or_create(role_id=admin_role.id, menu_id=menu.id)


def remove_log_parser_menu(apps, schema_editor):
    SysMenu = apps.get_model('menu', 'SysMenu')
    SysRoleMenu = apps.get_model('menu', 'SysRoleMenu')
    menu = SysMenu.objects.filter(path='/monitor/log-parsers').order_by('id').first()
    if menu:
        SysRoleMenu.objects.filter(menu_id=menu.id).delete()
        menu.delete()


class Migration(migrations.Migration):

    dependencies = [
        ('menu', '0047_add_log_storage_menu'),
    ]

    operations = [
        migrations.RunPython(add_log_parser_menu, remove_log_parser_menu),
    ]