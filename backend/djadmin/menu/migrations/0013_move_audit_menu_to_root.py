from datetime import date

from django.db import migrations


def move_audit_menu_to_root(apps, schema_editor):
    SysMenu = apps.get_model('menu', 'SysMenu')

    today = date.today()

    audit_dir = (
        SysMenu.objects.filter(name='操作审计', menu_type='M').order_by('id').first()
        or SysMenu.objects.filter(path='/audit', menu_type='M').order_by('id').first()
        or SysMenu.objects.filter(path='/sys/audit', menu_type='M').order_by('id').first()
    )

    if not audit_dir:
        return

    changed = False
    if audit_dir.parent_id != 0:
        audit_dir.parent_id = 0
        changed = True
    if audit_dir.path != '/audit':
        audit_dir.path = '/audit'
        changed = True
    if changed:
        audit_dir.update_time = today
        audit_dir.save(update_fields=['parent_id', 'path', 'update_time'])

    webssh_menu = (
        SysMenu.objects.filter(name='Web SSH会话日志', menu_type='C').order_by('id').first()
        or SysMenu.objects.filter(path='/audit/webssh', menu_type='C').order_by('id').first()
        or SysMenu.objects.filter(path='/sys/audit/webssh', menu_type='C').order_by('id').first()
    )
    if webssh_menu:
        changed = False
        if webssh_menu.parent_id != audit_dir.id:
            webssh_menu.parent_id = audit_dir.id
            changed = True
        if webssh_menu.path != '/audit/webssh':
            webssh_menu.path = '/audit/webssh'
            changed = True
        if changed:
            webssh_menu.update_time = today
            webssh_menu.save(update_fields=['parent_id', 'path', 'update_time'])

    login_menu = (
        SysMenu.objects.filter(name='登录日志', menu_type='C').order_by('id').first()
        or SysMenu.objects.filter(path='/audit/login', menu_type='C').order_by('id').first()
        or SysMenu.objects.filter(path='/sys/audit/login', menu_type='C').order_by('id').first()
    )
    if login_menu:
        changed = False
        if login_menu.parent_id != audit_dir.id:
            login_menu.parent_id = audit_dir.id
            changed = True
        if login_menu.path != '/audit/login':
            login_menu.path = '/audit/login'
            changed = True
        if changed:
            login_menu.update_time = today
            login_menu.save(update_fields=['parent_id', 'path', 'update_time'])


def reverse_move_audit_menu_to_root(apps, schema_editor):
    return


class Migration(migrations.Migration):

    dependencies = [
        ('menu', '0012_add_webssh_audit_menu'),
    ]

    operations = [
        migrations.RunPython(move_audit_menu_to_root, reverse_move_audit_menu_to_root),
    ]
