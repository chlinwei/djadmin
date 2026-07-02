from datetime import date

from django.db import migrations


def add_webssh_audit_menu(apps, schema_editor):
    SysMenu = apps.get_model('menu', 'SysMenu')
    SysRole = apps.get_model('role', 'SysRole')
    SysRoleMenu = apps.get_model('menu', 'SysRoleMenu')

    today = date.today()

    parent_id = 0

    audit_dir = (
        SysMenu.objects.filter(path='/audit', menu_type='M').order_by('id').first()
        or SysMenu.objects.filter(path='/sys/audit', menu_type='M').order_by('id').first()
        or SysMenu.objects.filter(name='操作审计', menu_type='M').order_by('id').first()
    )

    if audit_dir is None:
        audit_dir = SysMenu.objects.create(
            name='操作审计',
            icon='shield-halved',
            parent_id=parent_id,
            order_num=40,
            path='/audit',
            component='',
            menu_type='M',
            perms='',
            location=1,
            create_time=today,
            update_time=today,
            remark='操作审计目录',
        )
    else:
        changed = False
        if audit_dir.parent_id != parent_id:
            audit_dir.parent_id = parent_id
            changed = True
        if audit_dir.path != '/audit':
            audit_dir.path = '/audit'
            changed = True
        if audit_dir.menu_type != 'M':
            audit_dir.menu_type = 'M'
            changed = True
        if changed:
            audit_dir.update_time = today
            audit_dir.save(update_fields=['parent_id', 'path', 'menu_type', 'update_time'])

    webssh_menu = (
        SysMenu.objects.filter(path='/audit/webssh', menu_type='C').order_by('id').first()
        or SysMenu.objects.filter(path='/sys/audit/webssh', menu_type='C').order_by('id').first()
        or SysMenu.objects.filter(name='Web SSH会话日志', menu_type='C').order_by('id').first()
    )

    if webssh_menu is None:
        webssh_menu = SysMenu.objects.create(
            name='Web SSH会话日志',
            icon='terminal',
            parent_id=audit_dir.id,
            order_num=1,
            path='/audit/webssh',
            component='sys/audit/webssh/index',
            menu_type='C',
            perms='audit:webssh_sessions:view',
            location=1,
            create_time=today,
            update_time=today,
            remark='WebSSH操作会话审计',
        )
    else:
        changed = False
        if webssh_menu.parent_id != audit_dir.id:
            webssh_menu.parent_id = audit_dir.id
            changed = True
        if webssh_menu.path != '/audit/webssh':
            webssh_menu.path = '/audit/webssh'
            changed = True
        if webssh_menu.component != 'sys/audit/webssh/index':
            webssh_menu.component = 'sys/audit/webssh/index'
            changed = True
        if webssh_menu.menu_type != 'C':
            webssh_menu.menu_type = 'C'
            changed = True
        if webssh_menu.perms != 'audit:webssh_sessions:view':
            webssh_menu.perms = 'audit:webssh_sessions:view'
            changed = True
        if changed:
            webssh_menu.update_time = today
            webssh_menu.save(update_fields=['parent_id', 'path', 'component', 'menu_type', 'perms', 'update_time'])

    login_menu = (
        SysMenu.objects.filter(path='/audit/login', menu_type='C').order_by('id').first()
        or SysMenu.objects.filter(path='/sys/audit/login', menu_type='C').order_by('id').first()
        or SysMenu.objects.filter(name='登录日志', menu_type='C').order_by('id').first()
    )

    if login_menu is None:
        login_menu = SysMenu.objects.create(
            name='登录日志',
            icon='right-to-bracket',
            parent_id=audit_dir.id,
            order_num=2,
            path='/audit/login',
            component='sys/audit/login/index',
            menu_type='C',
            perms='audit:login_logs:view',
            location=1,
            create_time=today,
            update_time=today,
            remark='系统登录审计日志',
        )
    else:
        changed = False
        if login_menu.parent_id != audit_dir.id:
            login_menu.parent_id = audit_dir.id
            changed = True
        if login_menu.path != '/audit/login':
            login_menu.path = '/audit/login'
            changed = True
        if login_menu.component != 'sys/audit/login/index':
            login_menu.component = 'sys/audit/login/index'
            changed = True
        if login_menu.menu_type != 'C':
            login_menu.menu_type = 'C'
            changed = True
        if login_menu.perms != 'audit:login_logs:view':
            login_menu.perms = 'audit:login_logs:view'
            changed = True
        if changed:
            login_menu.update_time = today
            login_menu.save(update_fields=['parent_id', 'path', 'component', 'menu_type', 'perms', 'update_time'])

    admin_role = (
        SysRole.objects.filter(code='admin').order_by('id').first()
        or SysRole.objects.filter(name='超级管理员').order_by('id').first()
    )
    if admin_role:
        SysRoleMenu.objects.get_or_create(role_id=admin_role.id, menu_id=audit_dir.id)
        SysRoleMenu.objects.get_or_create(role_id=admin_role.id, menu_id=webssh_menu.id)
        SysRoleMenu.objects.get_or_create(role_id=admin_role.id, menu_id=login_menu.id)


def reverse_add_webssh_audit_menu(apps, schema_editor):
    return


class Migration(migrations.Migration):

    dependencies = [
        ('role', '0001_initial'),
        ('menu', '0011_sync_host_permission_buttons'),
    ]

    operations = [
        migrations.RunPython(add_webssh_audit_menu, reverse_add_webssh_audit_menu),
    ]
