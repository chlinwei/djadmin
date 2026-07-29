from datetime import date

from django.db import migrations


def add_monitor_explore_menu(apps, schema_editor):
    SysMenu = apps.get_model('menu', 'SysMenu')
    SysRole = apps.get_model('role', 'SysRole')
    SysRoleMenu = apps.get_model('menu', 'SysRoleMenu')

    today = date.today()

    monitor_dir = SysMenu.objects.filter(path='/monitor', menu_type='M').order_by('id').first()
    if monitor_dir is None:
        monitor_dir = SysMenu.objects.filter(name='监控中心', menu_type='M').order_by('id').first()
    if monitor_dir is None:
        return

    explore_menu = SysMenu.objects.filter(path='/monitor/explore', menu_type='C').order_by('id').first()
    if explore_menu is None:
        explore_menu = SysMenu.objects.create(
            name='Explore',
            icon='search',
            parent_id=monitor_dir.id,
            order_num=4,
            path='/monitor/explore',
            component='monitor/explore/index',
            menu_type='C',
            perms='monitor:view',
            location=1,
            create_time=today,
            update_time=today,
            remark='monitor explore menu',
        )
    elif explore_menu.parent_id != monitor_dir.id:
        explore_menu.parent_id = monitor_dir.id
        explore_menu.update_time = today
        explore_menu.save(update_fields=['parent_id', 'update_time'])

    admin_role = (
        SysRole.objects.filter(code='admin').order_by('id').first()
        or SysRole.objects.filter(name='超级管理员').order_by('id').first()
    )
    if admin_role:
        SysRoleMenu.objects.get_or_create(role_id=admin_role.id, menu_id=explore_menu.id)


def reverse_add_monitor_explore_menu(apps, schema_editor):
    SysMenu = apps.get_model('menu', 'SysMenu')
    SysRoleMenu = apps.get_model('menu', 'SysRoleMenu')

    explore_menu = SysMenu.objects.filter(path='/monitor/explore', menu_type='C').order_by('id').first()
    if explore_menu:
        SysRoleMenu.objects.filter(menu_id=explore_menu.id).delete()
        explore_menu.delete()


class Migration(migrations.Migration):

    dependencies = [
        ('menu', '0038_add_monitor_alerts_menu'),
        ('role', '0001_initial'),
    ]

    operations = [
        migrations.RunPython(add_monitor_explore_menu, reverse_add_monitor_explore_menu),
    ]
