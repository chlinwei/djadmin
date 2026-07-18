from datetime import date

from django.db import migrations


AGENT_TOKEN_BUTTONS = [
    ('Token新增', 'system:agent_token:create', 1),
    ('Token轮换', 'system:agent_token:rotate', 2),
    ('Token禁用', 'system:agent_token:disable', 3),
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
            remark='agent token permissions',
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


def add_agent_token_menu(apps, schema_editor):
    SysMenu = apps.get_model('menu', 'SysMenu')
    SysRole = apps.get_model('role', 'SysRole')
    SysRoleMenu = apps.get_model('menu', 'SysRoleMenu')

    today = date.today()

    system_dir = (
        SysMenu.objects.filter(path='/sys', menu_type='M').order_by('id').first()
        or SysMenu.objects.filter(name='系统管理', menu_type='M').order_by('id').first()
    )
    if system_dir is None:
        return

    agent_token_page = _ensure_menu(
        SysMenu,
        today,
        name='Agent Token管理',
        menu_type='C',
        perms='system:agent_token:view',
        path='/sys/agentToken',
        component='sys/agentToken/index',
        parent_id=system_dir.id,
        order_num=4,
        icon='safety-certificate',
    )

    menus_to_bind = [system_dir, agent_token_page]

    for button_name, button_perms, button_order in AGENT_TOKEN_BUTTONS:
        button_menu = _ensure_menu(
            SysMenu,
            today,
            name=button_name,
            menu_type='F',
            perms=button_perms,
            path='',
            component='',
            parent_id=agent_token_page.id,
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


def reverse_add_agent_token_menu(apps, schema_editor):
    SysMenu = apps.get_model('menu', 'SysMenu')
    SysRoleMenu = apps.get_model('menu', 'SysRoleMenu')

    for _, perms, _ in AGENT_TOKEN_BUTTONS:
        button = SysMenu.objects.filter(perms=perms, menu_type='F').order_by('id').first()
        if button:
            SysRoleMenu.objects.filter(menu_id=button.id).delete()
            button.delete()

    menu = SysMenu.objects.filter(perms='system:agent_token:view', menu_type='C').order_by('id').first()
    if menu:
        SysRoleMenu.objects.filter(menu_id=menu.id).delete()
        menu.delete()


class Migration(migrations.Migration):

    dependencies = [
        ('role', '0001_initial'),
        ('menu', '0027_add_sysmenu_is_expanded'),
    ]

    operations = [
        migrations.RunPython(add_agent_token_menu, reverse_add_agent_token_menu),
    ]
