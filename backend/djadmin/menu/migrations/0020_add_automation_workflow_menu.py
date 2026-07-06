from datetime import date

from django.db import migrations


def add_automation_workflow_menu(apps, schema_editor):
    SysMenu = apps.get_model('menu', 'SysMenu')
    SysRole = apps.get_model('role', 'SysRole')
    SysRoleMenu = apps.get_model('menu', 'SysRoleMenu')

    today = date.today()

    automation_dir = SysMenu.objects.filter(path='/automation', menu_type='M').order_by('id').first()
    if automation_dir is None:
        automation_dir = SysMenu.objects.filter(name='自动化运维', menu_type='M').order_by('id').first()

    if automation_dir is None:
        return

    workflow_menu = SysMenu.objects.filter(path='/sys/automation/workflow', menu_type='C').order_by('id').first()
    if workflow_menu is None:
        workflow_menu = SysMenu.objects.create(
            name='Workflow编排',
            icon='diagram-project',
            parent_id=automation_dir.id,
            order_num=5,
            path='/sys/automation/workflow',
            component='sys/automation/workflow',
            menu_type='C',
            perms='automation:workflow:view',
            location=1,
            create_time=today,
            update_time=today,
            remark='automation workflow menu',
        )

    admin_role = (
        SysRole.objects.filter(code='admin').order_by('id').first()
        or SysRole.objects.filter(name='超级管理员').order_by('id').first()
    )

    if admin_role:
        SysRoleMenu.objects.get_or_create(role_id=admin_role.id, menu_id=workflow_menu.id)


def reverse_add_automation_workflow_menu(apps, schema_editor):
    SysMenu = apps.get_model('menu', 'SysMenu')
    SysRoleMenu = apps.get_model('menu', 'SysRoleMenu')

    menu = SysMenu.objects.filter(path='/sys/automation/workflow', menu_type='C').order_by('id').first()
    if menu:
        SysRoleMenu.objects.filter(menu_id=menu.id).delete()
        menu.delete()


class Migration(migrations.Migration):

    dependencies = [
        ('role', '0001_initial'),
        ('menu', '0019_add_automation_inventory_buttons'),
    ]

    operations = [
        migrations.RunPython(add_automation_workflow_menu, reverse_add_automation_workflow_menu),
    ]
