from datetime import date

from django.db import migrations


def add_monitor_alert_rules_menu(apps, schema_editor):
    SysMenu = apps.get_model('menu', 'SysMenu')
    SysRole = apps.get_model('role', 'SysRole')
    SysRoleMenu = apps.get_model('menu', 'SysRoleMenu')

    today = date.today()

    monitor_dir = SysMenu.objects.filter(path='/monitor', menu_type='M').order_by('id').first()
    if monitor_dir is None:
        monitor_dir = SysMenu.objects.filter(name='监控中心', menu_type='M').order_by('id').first()
    if monitor_dir is None:
        return

    alert_rules_menu = SysMenu.objects.filter(path='/monitor/alert-rules', menu_type='C').order_by('id').first()
    if alert_rules_menu is None:
        alert_rules_menu = SysMenu.objects.create(
            name='告警规则',
            icon='alert',
            parent_id=monitor_dir.id,
            order_num=2,
            path='/monitor/alert-rules',
            component='monitor/alert-rules/index',
            menu_type='C',
            perms='monitor:view',
            location=1,
            create_time=today,
            update_time=today,
            remark='monitor alert rules menu',
        )
    elif alert_rules_menu.parent_id != monitor_dir.id:
        alert_rules_menu.parent_id = monitor_dir.id
        alert_rules_menu.update_time = today
        alert_rules_menu.save(update_fields=['parent_id', 'update_time'])

    admin_role = (
        SysRole.objects.filter(code='admin').order_by('id').first()
        or SysRole.objects.filter(name='超级管理员').order_by('id').first()
    )
    if admin_role:
        SysRoleMenu.objects.get_or_create(role_id=admin_role.id, menu_id=alert_rules_menu.id)


def reverse_add_monitor_alert_rules_menu(apps, schema_editor):
    SysMenu = apps.get_model('menu', 'SysMenu')
    SysRoleMenu = apps.get_model('menu', 'SysRoleMenu')

    alert_rules_menu = SysMenu.objects.filter(path='/monitor/alert-rules', menu_type='C').order_by('id').first()
    if alert_rules_menu:
        SysRoleMenu.objects.filter(menu_id=alert_rules_menu.id).delete()
        alert_rules_menu.delete()


class Migration(migrations.Migration):

    dependencies = [
        ('menu', '0032_update_monitor_menu_path'),
        ('role', '0001_initial'),
    ]

    operations = [
        migrations.RunPython(add_monitor_alert_rules_menu, reverse_add_monitor_alert_rules_menu),
    ]
