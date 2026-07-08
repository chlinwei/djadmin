from datetime import date

from django.db import migrations


AUTOMATION_BUTTONS = [
    ('模板新增', 'automation:playbooks:create', 1),
    ('模板编辑', 'automation:playbooks:update', 2),
    ('模板删除', 'automation:playbooks:delete', 3),
    ('任务创建', 'automation:jobs:create', 4),
    ('任务查看', 'automation:jobs:view', 5),
    ('任务取消', 'automation:jobs:cancel', 6),
    ('目标查看', 'automation:targets:view', 7),
]


def _ensure_menu(SysMenu, today, *, name, menu_type, perms='', path='', component='', parent_id=0, order_num=0, icon=''):
    menu = None

    if perms:
        menu = SysMenu.objects.filter(perms=perms, menu_type=menu_type).order_by('id').first()

    if menu is None and path:
        menu = SysMenu.objects.filter(path=path, menu_type=menu_type).order_by('id').first()

    if menu is None:
        menu = SysMenu.objects.filter(name=name, menu_type=menu_type).order_by('id').first()

    if menu is None:
        return SysMenu.objects.create(
            name=name,
            icon=icon,
            parent_id=parent_id,
            order_num=order_num,
            path=path,
            component=component,
            menu_type=menu_type,
            perms=perms,
            location=1,
            create_time=today,
            update_time=today,
            remark='automation permissions',
        )

    changed = False
    if menu.parent_id != parent_id:
        menu.parent_id = parent_id
        changed = True
    if (menu.order_num or 0) != order_num:
        menu.order_num = order_num
        changed = True
    if (menu.path or '') != path:
        menu.path = path
        changed = True
    if (menu.component or '') != component:
        menu.component = component
        changed = True
    if (menu.perms or '') != perms:
        menu.perms = perms
        changed = True
    if (menu.icon or '') != icon:
        menu.icon = icon
        changed = True

    if changed:
        menu.update_time = today
        menu.save(update_fields=['parent_id', 'order_num', 'path', 'component', 'perms', 'icon', 'update_time'])

    return menu


def add_automation_menu(apps, schema_editor):
    SysMenu = apps.get_model('menu', 'SysMenu')
    SysRole = apps.get_model('role', 'SysRole')
    SysRoleMenu = apps.get_model('menu', 'SysRoleMenu')

    today = date.today()

    automation_dir = _ensure_menu(
        SysMenu,
        today,
        name='自动化运维',
        menu_type='M',
        path='/automation',
        component='',
        parent_id=0,
        order_num=50,
        icon='robot',
    )

    automation_page = _ensure_menu(
        SysMenu,
        today,
        name='自动化任务',
        menu_type='C',
        perms='automation:playbooks:view',
        path='/sys/automation',
        component='sys/automation/index',
        parent_id=automation_dir.id,
        order_num=1,
        icon='sliders',
    )

    menus_to_bind = [automation_dir, automation_page]

    for button_name, button_perms, button_order in AUTOMATION_BUTTONS:
        button_menu = _ensure_menu(
            SysMenu,
            today,
            name=button_name,
            menu_type='F',
            perms=button_perms,
            path='',
            component='',
            parent_id=automation_page.id,
            order_num=button_order,
            icon='',
        )
        menus_to_bind.append(button_menu)

    admin_role = (
        SysRole.objects.filter(code='admin').order_by('id').first()
        or SysRole.objects.filter(name='超级管理员').order_by('id').first()
    )

    if admin_role:
        for menu in menus_to_bind:
            SysRoleMenu.objects.get_or_create(role_id=admin_role.id, menu_id=menu.id)


def reverse_add_automation_menu(apps, schema_editor):
    return


class Migration(migrations.Migration):

    dependencies = [
        ('role', '0001_initial'),
        ('menu', '0014_add_operation_audit_menu'),
    ]

    operations = [
        migrations.RunPython(add_automation_menu, reverse_add_automation_menu),
    ]
