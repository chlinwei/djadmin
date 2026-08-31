from datetime import date

from django.db import migrations


SERVICE_TREE_PATH = '/assets/service-tree/index'
OLD_VIEW_PERM = 'assets:applications:view'
NEW_VIEW_PERM = 'assets:service-tree:view'
MANAGE_PERM = 'assets:service-tree:manage'


def add_service_tree_permissions(apps, schema_editor):
    """服务树的业务系统/逻辑服务增删改查现在有了独立入口，给它拆成"查看"/"增删改"两个权限码，
    后端 action_perms_map 已改为只认这两个新码，不再兼容"应用配置"下的旧权限码。"""
    SysMenu = apps.get_model('menu', 'SysMenu')
    SysRole = apps.get_model('role', 'SysRole')
    SysRoleMenu = apps.get_model('menu', 'SysRoleMenu')
    today = date.today()

    service_tree_menu = SysMenu.objects.filter(path=SERVICE_TREE_PATH, menu_type='C').order_by('id').first()
    if service_tree_menu is None:
        return

    if service_tree_menu.perms == OLD_VIEW_PERM:
        service_tree_menu.perms = NEW_VIEW_PERM
        service_tree_menu.update_time = today
        service_tree_menu.save(update_fields=['perms', 'update_time'])

    manage_button = SysMenu.objects.filter(perms=MANAGE_PERM, menu_type='F').order_by('id').first()
    if manage_button is None:
        manage_button = SysMenu.objects.create(
            name='服务树管理操作',
            icon='',
            parent_id=service_tree_menu.id,
            order_num=1,
            path='',
            component='',
            menu_type='F',
            perms=MANAGE_PERM,
            location=1,
            create_time=today,
            update_time=today,
            remark='业务系统/逻辑服务在服务树内的增删改权限',
        )

    admin_role = (
        SysRole.objects.filter(code='admin').order_by('id').first()
        or SysRole.objects.filter(name='超级管理员').order_by('id').first()
    )
    if admin_role:
        SysRoleMenu.objects.get_or_create(role_id=admin_role.id, menu_id=manage_button.id)


def remove_service_tree_permissions(apps, schema_editor):
    SysMenu = apps.get_model('menu', 'SysMenu')
    SysRoleMenu = apps.get_model('menu', 'SysRoleMenu')

    manage_button = SysMenu.objects.filter(perms=MANAGE_PERM, menu_type='F').order_by('id').first()
    if manage_button:
        SysRoleMenu.objects.filter(menu_id=manage_button.id).delete()
        manage_button.delete()

    service_tree_menu = SysMenu.objects.filter(path=SERVICE_TREE_PATH, menu_type='C').order_by('id').first()
    if service_tree_menu is not None and service_tree_menu.perms == NEW_VIEW_PERM:
        service_tree_menu.perms = OLD_VIEW_PERM
        service_tree_menu.save(update_fields=['perms'])


class Migration(migrations.Migration):

    dependencies = [
        ('menu', '0054_remove_log_query_menu'),
    ]

    operations = [
        migrations.RunPython(add_service_tree_permissions, remove_service_tree_permissions),
    ]
