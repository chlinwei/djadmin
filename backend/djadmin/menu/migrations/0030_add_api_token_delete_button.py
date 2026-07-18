from datetime import date

from django.db import migrations


DELETE_PERM = 'system:api_token:delete'


def forward(apps, schema_editor):
    SysMenu = apps.get_model('menu', 'SysMenu')
    SysRole = apps.get_model('role', 'SysRole')
    SysRoleMenu = apps.get_model('menu', 'SysRoleMenu')

    today = date.today()

    page = SysMenu.objects.filter(perms='system:api_token:view', menu_type='C').order_by('id').first()
    if page is None:
        return

    button = SysMenu.objects.filter(perms=DELETE_PERM, menu_type='F').order_by('id').first()
    if button is None:
        button = SysMenu.objects.create(
            name='Token删除',
            icon='',
            parent_id=page.id,
            order_num=4,
            path='',
            component='',
            menu_type='F',
            perms=DELETE_PERM,
            location=1,
            create_time=today,
            update_time=today,
            remark='api token delete permission',
        )
    else:
        changed = False
        if button.parent_id != page.id:
            button.parent_id = page.id
            changed = True
        if (button.order_num or 0) != 4:
            button.order_num = 4
            changed = True
        if changed:
            button.update_time = today
            button.save(update_fields=['parent_id', 'order_num', 'update_time'])

    admin_role = (
        SysRole.objects.filter(code='admin').order_by('id').first()
        or SysRole.objects.filter(name='超级管理员').order_by('id').first()
    )
    if admin_role:
        SysRoleMenu.objects.get_or_create(role_id=admin_role.id, menu_id=button.id)


def backward(apps, schema_editor):
    SysMenu = apps.get_model('menu', 'SysMenu')
    SysRoleMenu = apps.get_model('menu', 'SysRoleMenu')

    button = SysMenu.objects.filter(perms=DELETE_PERM, menu_type='F').order_by('id').first()
    if button:
        SysRoleMenu.objects.filter(menu_id=button.id).delete()
        button.delete()


class Migration(migrations.Migration):

    dependencies = [
        ('role', '0001_initial'),
        ('menu', '0029_rename_agent_token_menu_to_api_token'),
    ]

    operations = [
        migrations.RunPython(forward, backward),
    ]
