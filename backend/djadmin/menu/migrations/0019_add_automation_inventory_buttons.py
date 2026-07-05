from datetime import date

from django.db import migrations


INVENTORY_BUTTONS = [
    ('Inventory新增', 'automation:inventory:create', 1),
    ('Inventory编辑', 'automation:inventory:update', 2),
    ('Inventory删除', 'automation:inventory:delete', 3),
]


def add_inventory_buttons(apps, schema_editor):
    SysMenu = apps.get_model('menu', 'SysMenu')
    SysRole = apps.get_model('role', 'SysRole')
    SysRoleMenu = apps.get_model('menu', 'SysRoleMenu')

    today = date.today()

    inventory_menu = SysMenu.objects.filter(path='/sys/automation/inventory', menu_type='C').order_by('id').first()
    if inventory_menu is None:
        return

    menus_to_bind = [inventory_menu]
    for name, perms, order_num in INVENTORY_BUTTONS:
        button = SysMenu.objects.filter(perms=perms, menu_type='F').order_by('id').first()
        if button is None:
            button = SysMenu.objects.create(
                name=name,
                icon='',
                parent_id=inventory_menu.id,
                order_num=order_num,
                path='',
                component='',
                menu_type='F',
                perms=perms,
                location=1,
                create_time=today,
                update_time=today,
                remark='automation inventory permissions',
            )
        menus_to_bind.append(button)

    admin_role = (
        SysRole.objects.filter(code='admin').order_by('id').first()
        or SysRole.objects.filter(name='超级管理员').order_by('id').first()
    )
    if admin_role:
        for menu in menus_to_bind:
            SysRoleMenu.objects.get_or_create(role_id=admin_role.id, menu_id=menu.id)


def reverse_add_inventory_buttons(apps, schema_editor):
    SysMenu = apps.get_model('menu', 'SysMenu')
    SysRoleMenu = apps.get_model('menu', 'SysRoleMenu')

    for _, perms, _ in INVENTORY_BUTTONS:
        button = SysMenu.objects.filter(perms=perms, menu_type='F').order_by('id').first()
        if button:
            SysRoleMenu.objects.filter(menu_id=button.id).delete()
            button.delete()


class Migration(migrations.Migration):

    dependencies = [
        ('role', '0001_initial'),
        ('menu', '0018_add_automation_inventory_menu'),
    ]

    operations = [
        migrations.RunPython(add_inventory_buttons, reverse_add_inventory_buttons),
    ]
