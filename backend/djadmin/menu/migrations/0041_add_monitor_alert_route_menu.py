from datetime import date

from django.db import migrations


def add_monitor_alert_route_menu(apps, schema_editor):
    SysMenu = apps.get_model('menu', 'SysMenu')
    SysRole = apps.get_model('role', 'SysRole')
    SysRoleMenu = apps.get_model('menu', 'SysRoleMenu')

    monitor_dir = SysMenu.objects.filter(path='/monitor', menu_type='M').order_by('id').first()
    if monitor_dir is None:
        monitor_dir = SysMenu.objects.filter(name='监控中心', menu_type='M').order_by('id').first()
    if monitor_dir is None:
        return

    today = date.today()
    route_menu, _ = SysMenu.objects.get_or_create(
        path='/monitor/alert-routes',
        menu_type='C',
        defaults={
            'name': '告警路由',
            'icon': 'route',
            'parent_id': monitor_dir.id,
            'order_num': 6,
            'component': 'monitor/alert-routes/index',
            'perms': 'monitor:view',
            'location': 1,
            'create_time': today,
            'update_time': today,
            'remark': 'monitor alert route menu',
        },
    )
    if route_menu.parent_id != monitor_dir.id:
        route_menu.parent_id = monitor_dir.id
        route_menu.save(update_fields=['parent_id'])

    admin_role = (
        SysRole.objects.filter(code='admin').order_by('id').first()
        or SysRole.objects.filter(name='超级管理员').order_by('id').first()
    )
    if admin_role:
        SysRoleMenu.objects.get_or_create(role_id=admin_role.id, menu_id=route_menu.id)


def remove_monitor_alert_route_menu(apps, schema_editor):
    SysMenu = apps.get_model('menu', 'SysMenu')
    SysRoleMenu = apps.get_model('menu', 'SysRoleMenu')
    route_menu = SysMenu.objects.filter(path='/monitor/alert-routes', menu_type='C').first()
    if route_menu:
        SysRoleMenu.objects.filter(menu_id=route_menu.id).delete()
        route_menu.delete()


class Migration(migrations.Migration):

    dependencies = [
        ('menu', '0040_add_monitor_media_menu'),
        ('role', '0001_initial'),
    ]

    operations = [
        migrations.RunPython(add_monitor_alert_route_menu, remove_monitor_alert_route_menu),
    ]