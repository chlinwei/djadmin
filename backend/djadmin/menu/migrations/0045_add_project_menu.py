from datetime import date

from django.db import migrations


PROJECT_PATH = '/assets/projects/index'


def add_project_menu(apps, schema_editor):
    SysMenu = apps.get_model('menu', 'SysMenu')
    SysRoleMenu = apps.get_model('menu', 'SysRoleMenu')
    today = date.today()

    parent = SysMenu.objects.filter(path='/assets/application-management', menu_type='M').order_by('id').first()
    if parent is None:
        return

    project_menu, _ = SysMenu.objects.update_or_create(
        path=PROJECT_PATH,
        defaults={
            'name': '项目',
            'icon': 'fa-diagram-project',
            'parent_id': parent.id,
            'order_num': 3,
            'component': 'assets/projects/index',
            'menu_type': 'C',
            'perms': 'assets:projects:view',
            'location': 1,
            'create_time': today,
            'update_time': today,
            'remark': 'project management menu',
        },
    )
    role_ids = SysRoleMenu.objects.filter(
        menu_id__in=SysMenu.objects.filter(parent_id=parent.id).values_list('id', flat=True),
    ).values_list('role_id', flat=True).distinct()
    for role_id in role_ids:
        SysRoleMenu.objects.get_or_create(role_id=role_id, menu_id=project_menu.id)


def remove_project_menu(apps, schema_editor):
    SysMenu = apps.get_model('menu', 'SysMenu')
    SysRoleMenu = apps.get_model('menu', 'SysRoleMenu')
    menu = SysMenu.objects.filter(path=PROJECT_PATH).first()
    if menu is not None:
        SysRoleMenu.objects.filter(menu_id=menu.id).delete()
        menu.delete()


class Migration(migrations.Migration):
    dependencies = [
        ('menu', '0044_add_inspection_menu'),
        ('assets', '0091_project'),
    ]

    operations = [migrations.RunPython(add_project_menu, remove_project_menu)]
