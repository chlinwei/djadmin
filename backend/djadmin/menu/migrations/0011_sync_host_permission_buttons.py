from datetime import date

from django.db import migrations


def sync_host_permission_buttons(apps, schema_editor):
    SysMenu = apps.get_model('menu', 'SysMenu')
    SysRole = apps.get_model('role', 'SysRole')
    SysRoleMenu = apps.get_model('menu', 'SysRoleMenu')

    today = date.today()

    # Locate the host management menu first; create a fallback if missing.
    host_menu = (
        SysMenu.objects.filter(menu_type='C', perms='assets:hosts:view').order_by('id').first()
        or SysMenu.objects.filter(name='主机管理').order_by('id').first()
    )

    if host_menu is None:
        assets_root = (
            SysMenu.objects.filter(path='/assets').order_by('id').first()
            or SysMenu.objects.filter(name='资产管理').order_by('id').first()
            or SysMenu.objects.filter(parent_id=0).order_by('id').first()
        )
        parent_id = assets_root.id if assets_root else 0
        host_menu = SysMenu.objects.create(
            name='主机管理',
            icon='desktop',
            parent_id=parent_id,
            order_num=1,
            path='/assets/host',
            component='assets/host/index',
            menu_type='C',
            perms='assets:hosts:view',
            location=1,
            create_time=today,
            update_time=today,
            remark='主机管理菜单',
        )

    buttons = [
        ('新增主机', 'assets:hosts:create', 10),
        ('编辑主机', 'assets:hosts:update', 11),
        ('删除主机', 'assets:hosts:delete', 12),
        ('查看主机分组', 'assets:hostgroups:view', 20),
        ('新增主机分组', 'assets:hostgroups:create', 21),
        ('编辑主机分组', 'assets:hostgroups:update', 22),
        ('删除主机分组', 'assets:hostgroups:delete', 23),
    ]

    target_menu_ids = [host_menu.id]
    for name, perm, order_num in buttons:
        menu = (
            SysMenu.objects.filter(parent_id=host_menu.id, perms=perm).order_by('id').first()
            or SysMenu.objects.filter(name=name).order_by('id').first()
        )
        if menu is None:
            menu = SysMenu.objects.create(
                name=name,
                icon='',
                parent_id=host_menu.id,
                order_num=order_num,
                path='',
                component='',
                menu_type='F',
                perms=perm,
                location=1,
                create_time=today,
                update_time=today,
                remark='主机管理按钮权限',
            )
        else:
            changed = False
            if menu.parent_id != host_menu.id:
                menu.parent_id = host_menu.id
                changed = True
            if menu.menu_type != 'F':
                menu.menu_type = 'F'
                changed = True
            if menu.perms != perm:
                menu.perms = perm
                changed = True
            if changed:
                menu.update_time = today
                menu.save(update_fields=['parent_id', 'menu_type', 'perms', 'update_time'])

        target_menu_ids.append(menu.id)

    admin_role = (
        SysRole.objects.filter(code='admin').order_by('id').first()
        or SysRole.objects.filter(name='超级管理员').order_by('id').first()
    )
    if admin_role:
        for menu_id in target_menu_ids:
            SysRoleMenu.objects.get_or_create(role_id=admin_role.id, menu_id=menu_id)


def reverse_sync_host_permission_buttons(apps, schema_editor):
    # Keep data on reverse migrations to avoid accidental permission loss.
    return


class Migration(migrations.Migration):

    dependencies = [
        ('role', '0001_initial'),
        ('menu', '0010_alter_sysmenu_location'),
    ]

    operations = [
        migrations.RunPython(sync_host_permission_buttons, reverse_sync_host_permission_buttons),
    ]
