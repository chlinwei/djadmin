from datetime import date

from django.db import migrations


APPLICATION_DIRECTORY_PATH = '/assets/application-management'
APPLICATION_PAGE_PATH = '/assets/applications/index'
SERVICE_TREE_PATH = '/assets/service-tree/index'


def split_application_management_menu(apps, schema_editor):
    SysMenu = apps.get_model('menu', 'SysMenu')
    SysRoleMenu = apps.get_model('menu', 'SysRoleMenu')
    today = date.today()

    application_menu = (
        SysMenu.objects.filter(path=APPLICATION_PAGE_PATH).order_by('id').first()
        or SysMenu.objects.filter(name='应用管理').order_by('id').first()
    )
    if application_menu is None:
        return

    role_ids = list(
        SysRoleMenu.objects.filter(menu_id=application_menu.id).values_list('role_id', flat=True)
    )
    application_menu.path = APPLICATION_DIRECTORY_PATH
    application_menu.component = ''
    application_menu.menu_type = 'M'
    application_menu.perms = ''
    application_menu.is_expanded = True
    application_menu.update_time = today
    application_menu.save(update_fields=[
        'path', 'component', 'menu_type', 'perms', 'is_expanded', 'update_time',
    ])

    child_definitions = [
        {
            'name': '应用配置',
            'path': APPLICATION_PAGE_PATH,
            'component': 'assets/application/index',
            'icon': 'fa-cubes',
            'order_num': 1,
        },
        {
            'name': '服务树',
            'path': SERVICE_TREE_PATH,
            'component': 'assets/service-tree/index',
            'icon': 'fa-sitemap',
            'order_num': 2,
        },
    ]
    for definition in child_definitions:
        child, _ = SysMenu.objects.update_or_create(
            path=definition['path'],
            defaults={
                **definition,
                'parent_id': application_menu.id,
                'menu_type': 'C',
                'perms': 'assets:applications:view',
                'location': 1,
                'create_time': today,
                'update_time': today,
                'remark': 'application management child menu',
            },
        )
        for role_id in role_ids:
            SysRoleMenu.objects.get_or_create(role_id=role_id, menu_id=child.id)


def restore_application_management_menu(apps, schema_editor):
    SysMenu = apps.get_model('menu', 'SysMenu')
    SysRoleMenu = apps.get_model('menu', 'SysRoleMenu')

    application_menu = SysMenu.objects.filter(path=APPLICATION_DIRECTORY_PATH).order_by('id').first()
    children = SysMenu.objects.filter(path__in=[APPLICATION_PAGE_PATH, SERVICE_TREE_PATH])
    child_ids = list(children.values_list('id', flat=True))
    SysRoleMenu.objects.filter(menu_id__in=child_ids).delete()
    children.delete()

    if application_menu is not None:
        application_menu.path = APPLICATION_PAGE_PATH
        application_menu.component = 'assets/application/index'
        application_menu.menu_type = 'C'
        application_menu.perms = 'assets:applications:view'
        application_menu.save(update_fields=['path', 'component', 'menu_type', 'perms'])


class Migration(migrations.Migration):

    dependencies = [
        ('menu', '0042_remove_shell_template_menus'),
    ]

    operations = [
        migrations.RunPython(split_application_management_menu, restore_application_management_menu),
    ]
