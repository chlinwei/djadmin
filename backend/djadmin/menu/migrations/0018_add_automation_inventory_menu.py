from datetime import date

from django.db import migrations


def add_automation_inventory_menu(apps, schema_editor):
    SysMenu = apps.get_model('menu', 'SysMenu')
    SysRole = apps.get_model('role', 'SysRole')
    SysRoleMenu = apps.get_model('menu', 'SysRoleMenu')

    today = date.today()

    automation_dir = SysMenu.objects.filter(path='/automation', menu_type='M').order_by('id').first()
    if automation_dir is None:
        automation_dir = SysMenu.objects.filter(name='自动化运维', menu_type='M').order_by('id').first()

    if automation_dir is None:
        return

    inventory_menu = SysMenu.objects.filter(path='/sys/automation/inventory', menu_type='C').order_by('id').first()
    if inventory_menu is None:
        inventory_menu = SysMenu.objects.create(
            name='Inventory管理',
            icon='boxes-stacked',
            parent_id=automation_dir.id,
            order_num=4,
            path='/sys/automation/inventory',
            component='sys/automation/inventory',
            menu_type='C',
            perms='automation:inventory:view',
            location=1,
            create_time=today,
            update_time=today,
            remark='automation inventory menu',
        )

    admin_role = (
        SysRole.objects.filter(code='admin').order_by('id').first()
        or SysRole.objects.filter(name='超级管理员').order_by('id').first()
    )

    if admin_role:
        SysRoleMenu.objects.get_or_create(role_id=admin_role.id, menu_id=inventory_menu.id)


def reverse_add_automation_inventory_menu(apps, schema_editor):
    SysMenu = apps.get_model('menu', 'SysMenu')
    SysRoleMenu = apps.get_model('menu', 'SysRoleMenu')

    menu = SysMenu.objects.filter(path='/sys/automation/inventory', menu_type='C').order_by('id').first()
    if menu:
        SysRoleMenu.objects.filter(menu_id=menu.id).delete()
        menu.delete()


class Migration(migrations.Migration):

    dependencies = [
        ('role', '0001_initial'),
        ('menu', '0017_add_automation_logs_menu'),
    ]

    operations = [
        migrations.RunPython(add_automation_inventory_menu, reverse_add_automation_inventory_menu),
    ]
