from datetime import date

from django.db import migrations


def merge_automation_run_menus(apps, schema_editor):
    SysMenu = apps.get_model('menu', 'SysMenu')
    SysRoleMenu = apps.get_model('menu', 'SysRoleMenu')

    today = date.today()

    logs_menu = SysMenu.objects.filter(path='/sys/automation/logs', menu_type='C').order_by('id').first()
    if logs_menu is None:
        return

    logs_menu.name = '运行记录中心'
    logs_menu.remark = 'automation run center menu'
    logs_menu.update_time = today
    logs_menu.save(update_fields=['name', 'remark', 'update_time'])

    workflow_menu = SysMenu.objects.filter(path='/sys/automation/workflow', menu_type='C').order_by('id').first()
    if workflow_menu is None:
        return

    SysRoleMenu.objects.filter(menu_id=workflow_menu.id).delete()
    workflow_menu.delete()


def reverse_merge_automation_run_menus(apps, schema_editor):
    SysMenu = apps.get_model('menu', 'SysMenu')
    SysRole = apps.get_model('role', 'SysRole')
    SysRoleMenu = apps.get_model('menu', 'SysRoleMenu')

    today = date.today()

    logs_menu = SysMenu.objects.filter(path='/sys/automation/logs', menu_type='C').order_by('id').first()
    if logs_menu:
        logs_menu.name = '任务运行记录'
        logs_menu.remark = 'automation run logs menu'
        logs_menu.update_time = today
        logs_menu.save(update_fields=['name', 'remark', 'update_time'])

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


class Migration(migrations.Migration):

    dependencies = [
        ('role', '0001_initial'),
        ('menu', '0021_add_automation_workflow_buttons'),
    ]

    operations = [
        migrations.RunPython(merge_automation_run_menus, reverse_merge_automation_run_menus),
    ]
