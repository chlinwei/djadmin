from datetime import date

from django.db import migrations


def add_monitor_alert_rule_groups_menu(apps, schema_editor):
    SysMenu = apps.get_model('menu', 'SysMenu')
    SysRole = apps.get_model('role', 'SysRole')
    SysRoleMenu = apps.get_model('menu', 'SysRoleMenu')

    today = date.today()

    monitor_dir = SysMenu.objects.filter(path='/monitor', menu_type='M').order_by('id').first()
    if monitor_dir is None:
        monitor_dir = SysMenu.objects.filter(name='监控中心', menu_type='M').order_by('id').first()
    if monitor_dir is None:
        return

    groups_menu = SysMenu.objects.filter(path='/monitor/alert-rule-groups', menu_type='C').order_by('id').first()
    if groups_menu is None:
        groups_menu = SysMenu.objects.create(
            name='规则组管理',
            icon='cluster',
            parent_id=monitor_dir.id,
            order_num=3,
            path='/monitor/alert-rule-groups',
            component='monitor/alert-rule-groups/index',
            menu_type='C',
            perms='monitor:view',
            location=1,
            create_time=today,
            update_time=today,
            remark='monitor alert rule groups menu',
        )
    elif groups_menu.parent_id != monitor_dir.id:
        groups_menu.parent_id = monitor_dir.id
        groups_menu.update_time = today
        groups_menu.save(update_fields=['parent_id', 'update_time'])

    admin_role = (
        SysRole.objects.filter(code='admin').order_by('id').first()
        or SysRole.objects.filter(name='超级管理员').order_by('id').first()
    )
    if admin_role:
        SysRoleMenu.objects.get_or_create(role_id=admin_role.id, menu_id=groups_menu.id)


def reverse_add_monitor_alert_rule_groups_menu(apps, schema_editor):
    SysMenu = apps.get_model('menu', 'SysMenu')
    SysRoleMenu = apps.get_model('menu', 'SysRoleMenu')

    groups_menu = SysMenu.objects.filter(path='/monitor/alert-rule-groups', menu_type='C').order_by('id').first()
    if groups_menu:
        SysRoleMenu.objects.filter(menu_id=groups_menu.id).delete()
        groups_menu.delete()


class Migration(migrations.Migration):

    dependencies = [
        ('menu', '0033_add_monitor_alert_rules_menu'),
        ('role', '0001_initial'),
    ]

    operations = [
        migrations.RunPython(add_monitor_alert_rule_groups_menu, reverse_add_monitor_alert_rule_groups_menu),
    ]
