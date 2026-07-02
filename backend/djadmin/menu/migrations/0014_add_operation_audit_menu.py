from datetime import date

from django.db import migrations


def add_operation_audit_menu(apps, schema_editor):
    SysMenu = apps.get_model('menu', 'SysMenu')
    SysRole = apps.get_model('role', 'SysRole')
    SysRoleMenu = apps.get_model('menu', 'SysRoleMenu')

    today = date.today()

    audit_dir = (
        SysMenu.objects.filter(path='/audit', menu_type='M').order_by('id').first()
        or SysMenu.objects.filter(path='/sys/audit', menu_type='M').order_by('id').first()
        or SysMenu.objects.filter(name='操作审计', menu_type='M').order_by('id').first()
    )
    if audit_dir is None:
        audit_dir = SysMenu.objects.create(
            name='操作审计',
            icon='shield-halved',
            parent_id=0,
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

    operation_menu = (
        SysMenu.objects.filter(path='/audit/operation-log', menu_type='C').order_by('id').first()
        or SysMenu.objects.filter(path='/sys/audit/operation-log', menu_type='C').order_by('id').first()
        or SysMenu.objects.filter(name='操作日志', menu_type='C').order_by('id').first()
    )

    if operation_menu is None:
        operation_menu = SysMenu.objects.create(
            name='操作日志',
            icon='file-lines',
            parent_id=audit_dir.id,
            order_num=3,
            path='/audit/operation-log',
            component='sys/audit/operationLog/index',
            menu_type='C',
            perms='audit:operation_logs:view',
            location=1,
            create_time=today,
            update_time=today,
            remark='系统操作审计日志',
        )
    else:
        changed = False
        if operation_menu.parent_id != audit_dir.id:
            operation_menu.parent_id = audit_dir.id
            changed = True
        if operation_menu.path != '/audit/operation-log':
            operation_menu.path = '/audit/operation-log'
            changed = True
        if operation_menu.component != 'sys/audit/operationLog/index':
            operation_menu.component = 'sys/audit/operationLog/index'
            changed = True
        if operation_menu.menu_type != 'C':
            operation_menu.menu_type = 'C'
            changed = True
        if operation_menu.perms != 'audit:operation_logs:view':
            operation_menu.perms = 'audit:operation_logs:view'
            changed = True
        if changed:
            operation_menu.update_time = today
            operation_menu.save(update_fields=['parent_id', 'path', 'component', 'menu_type', 'perms', 'update_time'])

    admin_role = (
        SysRole.objects.filter(code='admin').order_by('id').first()
        or SysRole.objects.filter(name='超级管理员').order_by('id').first()
    )
    if admin_role:
        webssh_menu = (
            SysMenu.objects.filter(path='/audit/webssh', menu_type='C').order_by('id').first()
            or SysMenu.objects.filter(path='/sys/audit/webssh', menu_type='C').order_by('id').first()
            or SysMenu.objects.filter(name='Web SSH会话日志', menu_type='C').order_by('id').first()
        )
        login_menu = (
            SysMenu.objects.filter(path='/audit/login', menu_type='C').order_by('id').first()
            or SysMenu.objects.filter(path='/sys/audit/login', menu_type='C').order_by('id').first()
            or SysMenu.objects.filter(name='登录日志', menu_type='C').order_by('id').first()
        )
        SysRoleMenu.objects.get_or_create(role_id=admin_role.id, menu_id=audit_dir.id)
        if webssh_menu:
            SysRoleMenu.objects.get_or_create(role_id=admin_role.id, menu_id=webssh_menu.id)
        if login_menu:
            SysRoleMenu.objects.get_or_create(role_id=admin_role.id, menu_id=login_menu.id)
        SysRoleMenu.objects.get_or_create(role_id=admin_role.id, menu_id=operation_menu.id)


def reverse_add_operation_audit_menu(apps, schema_editor):
    return


class Migration(migrations.Migration):

    dependencies = [
        ('role', '0001_initial'),
        ('menu', '0013_move_audit_menu_to_root'),
    ]

    operations = [
        migrations.RunPython(add_operation_audit_menu, reverse_add_operation_audit_menu),
    ]