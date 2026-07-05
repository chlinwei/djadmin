from datetime import date

from django.db import migrations


def add_automation_logs_menu(apps, schema_editor):
    SysMenu = apps.get_model('menu', 'SysMenu')
    SysRole = apps.get_model('role', 'SysRole')
    SysRoleMenu = apps.get_model('menu', 'SysRoleMenu')

    today = date.today()

    automation_dir = SysMenu.objects.filter(path='/automation', menu_type='M').order_by('id').first()
    if automation_dir is None:
        automation_dir = SysMenu.objects.filter(name='自动化运维', menu_type='M').order_by('id').first()

    if automation_dir is None:
        return

    logs_menu = SysMenu.objects.filter(path='/sys/automation/logs', menu_type='C').order_by('id').first()
    if logs_menu is None:
        logs_menu = SysMenu.objects.create(
            name='任务运行记录',
            icon='list',
            parent_id=automation_dir.id,
            order_num=3,
            path='/sys/automation/logs',
            component='sys/automation/logs',
            menu_type='C',
            perms='automation:jobs:view',
            location=1,
            create_time=today,
            update_time=today,
            remark='automation run logs menu',
        )

    admin_role = (
        SysRole.objects.filter(code='admin').order_by('id').first()
        or SysRole.objects.filter(name='超级管理员').order_by('id').first()
    )

    if admin_role:
        SysRoleMenu.objects.get_or_create(role_id=admin_role.id, menu_id=logs_menu.id)


def reverse_add_automation_logs_menu(apps, schema_editor):
    SysMenu = apps.get_model('menu', 'SysMenu')
    SysRoleMenu = apps.get_model('menu', 'SysRoleMenu')

    menu = SysMenu.objects.filter(path='/sys/automation/logs', menu_type='C').order_by('id').first()
    if menu:
        SysRoleMenu.objects.filter(menu_id=menu.id).delete()
        menu.delete()


class Migration(migrations.Migration):

    dependencies = [
        ('role', '0001_initial'),
        ('menu', '0016_add_playbook_template_menu'),
    ]

    operations = [
        migrations.RunPython(add_automation_logs_menu, reverse_add_automation_logs_menu),
    ]
