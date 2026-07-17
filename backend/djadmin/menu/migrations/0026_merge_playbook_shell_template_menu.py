from datetime import date

from django.db import migrations


PLAYBOOK_BUTTON_PERMS = [
    'automation:playbooks:create',
    'automation:playbooks:update',
    'automation:playbooks:delete',
]

SHELL_BUTTON_PERMS = [
    'automation:shell_scripts:create',
    'automation:shell_scripts:update',
    'automation:shell_scripts:delete',
]


def _first_menu(SysMenu, *, menu_type='C', path='', perms='', name=''):
    if path:
        menu = SysMenu.objects.filter(path=path, menu_type=menu_type).order_by('id').first()
        if menu:
            return menu
    if perms:
        menu = SysMenu.objects.filter(perms=perms, menu_type=menu_type).order_by('id').first()
        if menu:
            return menu
    if name:
        return SysMenu.objects.filter(name=name, menu_type=menu_type).order_by('id').first()
    return None


def merge_template_menus(apps, schema_editor):
    SysMenu = apps.get_model('menu', 'SysMenu')
    SysRole = apps.get_model('role', 'SysRole')
    SysRoleMenu = apps.get_model('menu', 'SysRoleMenu')

    today = date.today()

    automation_dir = _first_menu(SysMenu, menu_type='M', path='/automation')
    if automation_dir is None:
        automation_dir = _first_menu(SysMenu, menu_type='M', name='自动化运维')
    if automation_dir is None:
        return

    unified_menu = _first_menu(SysMenu, path='/sys/automation/templates')
    if unified_menu is None:
        unified_menu = _first_menu(SysMenu, path='/sys/automation/playbooks', perms='automation:playbooks:view', name='Playbook模板')

    if unified_menu is None:
        unified_menu = SysMenu.objects.create(
            name='模板',
            icon='file-lines',
            parent_id=automation_dir.id,
            order_num=2,
            path='/sys/automation/templates',
            component='automation/templates/index',
            menu_type='C',
            perms='automation:playbooks:view',
            location=1,
            create_time=today,
            update_time=today,
            remark='automation merged template menu',
        )
    else:
        unified_menu.name = '模板'
        unified_menu.icon = 'file-lines'
        unified_menu.parent_id = automation_dir.id
        unified_menu.order_num = 2
        unified_menu.path = '/sys/automation/templates'
        unified_menu.component = 'automation/templates/index'
        if not (unified_menu.perms or '').strip():
            unified_menu.perms = 'automation:playbooks:view'
        unified_menu.update_time = today
        unified_menu.save(update_fields=['name', 'icon', 'parent_id', 'order_num', 'path', 'component', 'perms', 'update_time'])

    shell_menu = _first_menu(SysMenu, path='/sys/automation/shell-scripts', perms='automation:shell_scripts:view', name='Shell脚本模板')

    all_button_perms = PLAYBOOK_BUTTON_PERMS + SHELL_BUTTON_PERMS
    for perms in all_button_perms:
        button_menu = _first_menu(SysMenu, menu_type='F', perms=perms)
        if button_menu is None:
            continue
        if button_menu.parent_id != unified_menu.id:
            button_menu.parent_id = unified_menu.id
            button_menu.update_time = today
            button_menu.save(update_fields=['parent_id', 'update_time'])

    if shell_menu and shell_menu.id != unified_menu.id:
        shell_role_ids = list(
            SysRoleMenu.objects.filter(menu_id=shell_menu.id).values_list('role_id', flat=True)
        )
        for role_id in shell_role_ids:
            SysRoleMenu.objects.get_or_create(role_id=role_id, menu_id=unified_menu.id)

        SysRoleMenu.objects.filter(menu_id=shell_menu.id).delete()
        shell_menu.delete()

    admin_role = (
        SysRole.objects.filter(code='admin').order_by('id').first()
        or SysRole.objects.filter(name='超级管理员').order_by('id').first()
    )
    if admin_role:
        SysRoleMenu.objects.get_or_create(role_id=admin_role.id, menu_id=unified_menu.id)


class Migration(migrations.Migration):

    dependencies = [
        ('role', '0001_initial'),
        ('menu', '0025_add_shell_script_template_menu'),
    ]

    operations = [
        migrations.RunPython(merge_template_menus, migrations.RunPython.noop),
    ]
