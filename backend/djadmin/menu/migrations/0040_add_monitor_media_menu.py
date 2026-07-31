from datetime import date

from django.db import migrations


def add_monitor_media_menu(apps, schema_editor):
    SysMenu = apps.get_model('menu', 'SysMenu')
    SysRole = apps.get_model('role', 'SysRole')
    SysRoleMenu = apps.get_model('menu', 'SysRoleMenu')

    today = date.today()

    monitor_dir = SysMenu.objects.filter(path='/monitor', menu_type='M').order_by('id').first()
    if monitor_dir is None:
        monitor_dir = SysMenu.objects.filter(name='监控中心', menu_type='M').order_by('id').first()
    if monitor_dir is None:
        return

    media_menu = SysMenu.objects.filter(path='/monitor/media', menu_type='C').order_by('id').first()
    if media_menu is None:
        media_menu = SysMenu.objects.create(
            name='媒介',
            icon='bell',
            parent_id=monitor_dir.id,
            order_num=5,
            path='/monitor/media',
            component='monitor/media/index',
            menu_type='C',
            perms='monitor:view',
            location=1,
            create_time=today,
            update_time=today,
            remark='monitor media menu',
        )
    elif media_menu.parent_id != monitor_dir.id:
        media_menu.parent_id = monitor_dir.id
        media_menu.update_time = today
        media_menu.save(update_fields=['parent_id', 'update_time'])

    admin_role = (
        SysRole.objects.filter(code='admin').order_by('id').first()
        or SysRole.objects.filter(name='超级管理员').order_by('id').first()
    )
    if admin_role:
        SysRoleMenu.objects.get_or_create(role_id=admin_role.id, menu_id=media_menu.id)


def reverse_add_monitor_media_menu(apps, schema_editor):
    SysMenu = apps.get_model('menu', 'SysMenu')
    SysRoleMenu = apps.get_model('menu', 'SysRoleMenu')

    media_menu = SysMenu.objects.filter(path='/monitor/media', menu_type='C').order_by('id').first()
    if media_menu:
        SysRoleMenu.objects.filter(menu_id=media_menu.id).delete()
        media_menu.delete()


class Migration(migrations.Migration):

    dependencies = [
        ('menu', '0039_add_monitor_explore_menu'),
        ('role', '0001_initial'),
    ]

    operations = [
        migrations.RunPython(add_monitor_media_menu, reverse_add_monitor_media_menu),
    ]
