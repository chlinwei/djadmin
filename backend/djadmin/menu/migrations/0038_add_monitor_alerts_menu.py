from datetime import date

from django.db import migrations


def add_monitor_alerts_menu(apps, schema_editor):
    SysMenu = apps.get_model('menu', 'SysMenu')
    SysRole = apps.get_model('role', 'SysRole')
    SysRoleMenu = apps.get_model('menu', 'SysRoleMenu')

    today = date.today()

    monitor_dir = SysMenu.objects.filter(path='/monitor', menu_type='M').order_by('id').first()
    if monitor_dir is None:
        monitor_dir = SysMenu.objects.filter(name='监控中心', menu_type='M').order_by('id').first()
    if monitor_dir is None:
        return

    alerts_menu = SysMenu.objects.filter(path='/monitor/alerts', menu_type='C').order_by('id').first()
    if alerts_menu is None:
        alerts_menu = SysMenu.objects.create(
            name='告警',
            # triangle-exclamation 用于当前活跃告警列表；规则定义页（告警规则）使用 list-check，
            # 两者语义区分，避免菜单图标混淆。
            icon='triangle-exclamation',
            parent_id=monitor_dir.id,
            order_num=3,
            path='/monitor/alerts',
            component='monitor/alerts/index',
            menu_type='C',
            perms='monitor:view',
            location=1,
            create_time=today,
            update_time=today,
            remark='monitor prometheus active alerts menu',
        )
    elif alerts_menu.parent_id != monitor_dir.id:
        alerts_menu.parent_id = monitor_dir.id
        alerts_menu.update_time = today
        alerts_menu.save(update_fields=['parent_id', 'update_time'])

    admin_role = (
        SysRole.objects.filter(code='admin').order_by('id').first()
        or SysRole.objects.filter(name='超级管理员').order_by('id').first()
    )
    if admin_role:
        SysRoleMenu.objects.get_or_create(role_id=admin_role.id, menu_id=alerts_menu.id)


def reverse_add_monitor_alerts_menu(apps, schema_editor):
    SysMenu = apps.get_model('menu', 'SysMenu')
    SysRoleMenu = apps.get_model('menu', 'SysRoleMenu')

    alerts_menu = SysMenu.objects.filter(path='/monitor/alerts', menu_type='C').order_by('id').first()
    if alerts_menu:
        SysRoleMenu.objects.filter(menu_id=alerts_menu.id).delete()
        alerts_menu.delete()


class Migration(migrations.Migration):

    dependencies = [
        ('menu', '0037_alert_rules_menu_icon_list_check'),
        ('role', '0001_initial'),
    ]

    operations = [
        migrations.RunPython(add_monitor_alerts_menu, reverse_add_monitor_alerts_menu),
    ]
