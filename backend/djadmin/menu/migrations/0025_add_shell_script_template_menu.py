from datetime import date

from django.db import migrations


SHELL_SCRIPT_BUTTONS = [
    ('脚本模板新增', 'automation:shell_scripts:create', 1),
    ('脚本模板编辑', 'automation:shell_scripts:update', 2),
    ('脚本模板删除', 'automation:shell_scripts:delete', 3),
]


def add_shell_script_template_menu(apps, schema_editor):
    SysMenu = apps.get_model('menu', 'SysMenu')
    SysRole = apps.get_model('role', 'SysRole')
    SysRoleMenu = apps.get_model('menu', 'SysRoleMenu')

    today = date.today()

    automation_dir = SysMenu.objects.filter(path='/automation', menu_type='M').order_by('id').first()
    if automation_dir is None:
        automation_dir = SysMenu.objects.filter(name='自动化运维', menu_type='M').order_by('id').first()

    if automation_dir is None:
        return

    shell_menu = SysMenu.objects.filter(path='/sys/automation/shell-scripts', menu_type='C').order_by('id').first()
    if shell_menu is None:
        shell_menu = SysMenu.objects.create(
            name='Shell脚本模板',
            icon='terminal',
            parent_id=automation_dir.id,
            order_num=6,
            path='/sys/automation/shell-scripts',
            component='sys/automation/shellScripts',
            menu_type='C',
            perms='automation:shell_scripts:view',
            location=1,
            create_time=today,
            update_time=today,
            remark='automation shell script template menu',
        )

    menus_to_bind = [shell_menu]
    for name, perms, order_num in SHELL_SCRIPT_BUTTONS:
        button = SysMenu.objects.filter(perms=perms, menu_type='F').order_by('id').first()
        if button is None:
            button = SysMenu.objects.create(
                name=name,
                icon='',
                parent_id=shell_menu.id,
                order_num=order_num,
                path='',
                component='',
                menu_type='F',
                perms=perms,
                location=1,
                create_time=today,
                update_time=today,
                remark='automation shell script template permissions',
            )
        menus_to_bind.append(button)

    admin_role = (
        SysRole.objects.filter(code='admin').order_by('id').first()
        or SysRole.objects.filter(name='超级管理员').order_by('id').first()
    )

    if admin_role:
        for menu in menus_to_bind:
            SysRoleMenu.objects.get_or_create(role_id=admin_role.id, menu_id=menu.id)


def reverse_add_shell_script_template_menu(apps, schema_editor):
    SysMenu = apps.get_model('menu', 'SysMenu')
    SysRoleMenu = apps.get_model('menu', 'SysRoleMenu')

    for _, perms, _ in SHELL_SCRIPT_BUTTONS:
        button = SysMenu.objects.filter(perms=perms, menu_type='F').order_by('id').first()
        if button:
            SysRoleMenu.objects.filter(menu_id=button.id).delete()
            button.delete()

    shell_menu = SysMenu.objects.filter(path='/sys/automation/shell-scripts', menu_type='C').order_by('id').first()
    if shell_menu:
        SysRoleMenu.objects.filter(menu_id=shell_menu.id).delete()
        shell_menu.delete()


class Migration(migrations.Migration):

    dependencies = [
        ('role', '0001_initial'),
        ('menu', '0024_normalize_audit_component_paths'),
    ]

    operations = [
        migrations.RunPython(add_shell_script_template_menu, reverse_add_shell_script_template_menu),
    ]
