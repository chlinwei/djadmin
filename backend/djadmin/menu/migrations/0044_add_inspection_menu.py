from datetime import date

from django.db import migrations


def add_inspection_menu(apps, schema_editor):
    SysMenu = apps.get_model('menu', 'SysMenu')
    SysRole = apps.get_model('role', 'SysRole')
    SysRoleMenu = apps.get_model('menu', 'SysRoleMenu')
    today = date.today()

    automation_dir = SysMenu.objects.filter(path='/automation', menu_type='M').order_by('id').first()
    if automation_dir is None:
        return

    inspection_menu, _ = SysMenu.objects.update_or_create(
        path='/sys/inspection',
        defaults={
            'name': '巡检中心',
            'icon': 'fa-list-check',
            'parent_id': automation_dir.id,
            'order_num': 7,
            'component': 'inspection/index',
            'menu_type': 'C',
            'is_expanded': True,
            'perms': 'inspection:view',
            'location': 1,
            'create_time': today,
            'update_time': today,
            'remark': 'inspection center menu',
        },
    )

    admin_role = (
        SysRole.objects.filter(code='admin').order_by('id').first()
        or SysRole.objects.filter(name='超级管理员').order_by('id').first()
    )
    if admin_role:
        SysRoleMenu.objects.get_or_create(role_id=admin_role.id, menu_id=inspection_menu.id)


def remove_inspection_menu(apps, schema_editor):
    SysMenu = apps.get_model('menu', 'SysMenu')
    SysRoleMenu = apps.get_model('menu', 'SysRoleMenu')
    inspection_menu = SysMenu.objects.filter(path='/sys/inspection').order_by('id').first()
    if inspection_menu:
        SysRoleMenu.objects.filter(menu_id=inspection_menu.id).delete()
        inspection_menu.delete()


class Migration(migrations.Migration):

    dependencies = [
        ('menu', '0043_split_application_management_menu'),
    ]

    operations = [
        migrations.RunPython(add_inspection_menu, remove_inspection_menu),
    ]