from datetime import date

from django.db import migrations


def add_playbook_template_menu(apps, schema_editor):
    SysMenu = apps.get_model('menu', 'SysMenu')
    SysRole = apps.get_model('role', 'SysRole')
    SysRoleMenu = apps.get_model('menu', 'SysRoleMenu')

    today = date.today()

    automation_dir = SysMenu.objects.filter(path='/automation', menu_type='M').order_by('id').first()
    if automation_dir is None:
        automation_dir = SysMenu.objects.filter(name='自动化运维', menu_type='M').order_by('id').first()

    if automation_dir is None:
        return

    playbook_menu = SysMenu.objects.filter(path='/sys/automation/playbooks', menu_type='C').order_by('id').first()
    if playbook_menu is None:
        playbook_menu = SysMenu.objects.create(
            name='Playbook模板',
            icon='file-lines',
            parent_id=automation_dir.id,
            order_num=2,
            path='/sys/automation/playbooks',
            component='sys/playbookTemplate/index',
            menu_type='C',
            perms='automation:playbooks:view',
            location=1,
            create_time=today,
            update_time=today,
            remark='automation playbook template menu',
        )

    admin_role = (
        SysRole.objects.filter(code='admin').order_by('id').first()
        or SysRole.objects.filter(name='超级管理员').order_by('id').first()
    )

    if admin_role:
        SysRoleMenu.objects.get_or_create(role_id=admin_role.id, menu_id=playbook_menu.id)


def reverse_add_playbook_template_menu(apps, schema_editor):
    SysMenu = apps.get_model('menu', 'SysMenu')
    SysRoleMenu = apps.get_model('menu', 'SysRoleMenu')

    menu = SysMenu.objects.filter(path='/sys/automation/playbooks', menu_type='C').order_by('id').first()
    if menu:
        SysRoleMenu.objects.filter(menu_id=menu.id).delete()
        menu.delete()


class Migration(migrations.Migration):

    dependencies = [
        ('role', '0001_initial'),
        ('menu', '0015_add_automation_menu'),
    ]

    operations = [
        migrations.RunPython(add_playbook_template_menu, reverse_add_playbook_template_menu),
    ]
