from datetime import date

from django.db import migrations


WORKFLOW_BUTTONS = [
    ('Workflow新增', 'automation:workflow:create', 1),
    ('Workflow编辑', 'automation:workflow:update', 2),
    ('Workflow删除', 'automation:workflow:delete', 3),
    ('Workflow启动', 'automation:workflow:launch', 4),
]


def add_workflow_buttons(apps, schema_editor):
    SysMenu = apps.get_model('menu', 'SysMenu')
    SysRole = apps.get_model('role', 'SysRole')
    SysRoleMenu = apps.get_model('menu', 'SysRoleMenu')

    today = date.today()

    workflow_menu = SysMenu.objects.filter(path='/sys/automation/workflow', menu_type='C').order_by('id').first()
    if workflow_menu is None:
        return

    menus_to_bind = [workflow_menu]
    for name, perms, order_num in WORKFLOW_BUTTONS:
        button = SysMenu.objects.filter(perms=perms, menu_type='F').order_by('id').first()
        if button is None:
            button = SysMenu.objects.create(
                name=name,
                icon='',
                parent_id=workflow_menu.id,
                order_num=order_num,
                path='',
                component='',
                menu_type='F',
                perms=perms,
                location=1,
                create_time=today,
                update_time=today,
                remark='automation workflow permissions',
            )
        menus_to_bind.append(button)

    admin_role = (
        SysRole.objects.filter(code='admin').order_by('id').first()
        or SysRole.objects.filter(name='超级管理员').order_by('id').first()
    )
    if admin_role:
        for menu in menus_to_bind:
            SysRoleMenu.objects.get_or_create(role_id=admin_role.id, menu_id=menu.id)


def reverse_add_workflow_buttons(apps, schema_editor):
    SysMenu = apps.get_model('menu', 'SysMenu')
    SysRoleMenu = apps.get_model('menu', 'SysRoleMenu')

    for _, perms, _ in WORKFLOW_BUTTONS:
        button = SysMenu.objects.filter(perms=perms, menu_type='F').order_by('id').first()
        if button:
            SysRoleMenu.objects.filter(menu_id=button.id).delete()
            button.delete()


class Migration(migrations.Migration):

    dependencies = [
        ('role', '0001_initial'),
        ('menu', '0020_add_automation_workflow_menu'),
    ]

    operations = [
        migrations.RunPython(add_workflow_buttons, reverse_add_workflow_buttons),
    ]
