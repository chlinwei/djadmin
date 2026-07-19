from datetime import date

from django.db import migrations


def add_monitor_menu(apps, schema_editor):
    SysMenu = apps.get_model('menu', 'SysMenu')
    SysRole = apps.get_model('role', 'SysRole')
    SysRoleMenu = apps.get_model('menu', 'SysRoleMenu')

    today = date.today()

    monitor_dir = SysMenu.objects.filter(path='/monitor', menu_type='M').order_by('id').first()
    if monitor_dir is None:
        monitor_dir = SysMenu.objects.filter(name='监控中心', menu_type='M').order_by('id').first()

    if monitor_dir is None:
        monitor_dir = SysMenu.objects.create(
            name='监控中心',
            icon='desktop',
            parent_id=0,
            order_num=60,
            path='/monitor',
            component='',
            menu_type='M',
            perms='',
            location=1,
            create_time=today,
            update_time=today,
            remark='monitor root menu',
        )

    monitor_menu = SysMenu.objects.filter(path='/sys/monitor', menu_type='C').order_by('id').first()
    if monitor_menu is None:
        monitor_menu = SysMenu.objects.create(
            name='智能监控',
            icon='dashboard',
            parent_id=monitor_dir.id,
            order_num=1,
            path='/sys/monitor',
            component='monitor/index',
            menu_type='C',
            perms='monitor:view',
            location=1,
            create_time=today,
            update_time=today,
            remark='smart monitor menu',
        )
    elif monitor_menu.parent_id != monitor_dir.id:
        monitor_menu.parent_id = monitor_dir.id
        monitor_menu.update_time = today
        monitor_menu.save(update_fields=['parent_id', 'update_time'])

    admin_role = (
        SysRole.objects.filter(code='admin').order_by('id').first()
        or SysRole.objects.filter(name='超级管理员').order_by('id').first()
    )

    if admin_role:
        SysRoleMenu.objects.get_or_create(role_id=admin_role.id, menu_id=monitor_dir.id)
        SysRoleMenu.objects.get_or_create(role_id=admin_role.id, menu_id=monitor_menu.id)


def reverse_add_monitor_menu(apps, schema_editor):
    SysMenu = apps.get_model('menu', 'SysMenu')
    SysRoleMenu = apps.get_model('menu', 'SysRoleMenu')

    monitor_menu = SysMenu.objects.filter(path='/sys/monitor', menu_type='C').order_by('id').first()
    if monitor_menu:
        SysRoleMenu.objects.filter(menu_id=monitor_menu.id).delete()
        monitor_menu.delete()

    monitor_dir = SysMenu.objects.filter(path='/monitor', menu_type='M').order_by('id').first()
    if monitor_dir:
        has_children = SysMenu.objects.filter(parent_id=monitor_dir.id).exists()
        if not has_children:
            SysRoleMenu.objects.filter(menu_id=monitor_dir.id).delete()
            monitor_dir.delete()


class Migration(migrations.Migration):

    dependencies = [
        ('role', '0001_initial'),
        ('menu', '0030_add_api_token_delete_button'),
    ]

    operations = [
        migrations.RunPython(add_monitor_menu, reverse_add_monitor_menu),
    ]
