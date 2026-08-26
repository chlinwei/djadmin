from datetime import date

from django.db import migrations


ENVIRONMENT_PATH = '/assets/environments/index'
APPLICATION_PATH = '/assets/application-management'


def move_environment_menu(apps, schema_editor):
    SysMenu = apps.get_model('menu', 'SysMenu')
    SysRoleMenu = apps.get_model('menu', 'SysRoleMenu')
    today = date.today()

    assets_root = (
        SysMenu.objects.filter(path='/assets').order_by('id').first()
        or SysMenu.objects.filter(name='资产管理').order_by('id').first()
    )
    environment_menu = SysMenu.objects.filter(path=ENVIRONMENT_PATH).order_by('id').first()
    if environment_menu is None:
        environment_menu = SysMenu.objects.filter(name='环境').order_by('id').first()
    if environment_menu is None:
        environment_menu = SysMenu.objects.create(
            name='环境', icon='fa-layer-group', parent_id=assets_root.id if assets_root else 0,
            order_num=3, path=ENVIRONMENT_PATH, component='assets/environments/index',
            menu_type='C', perms='assets:applications:view', location=1,
            create_time=today, update_time=today, remark='environment management menu',
        )
    else:
        environment_menu.parent_id = assets_root.id if assets_root else 0
    role_ids = SysRoleMenu.objects.filter(
        menu_id__in=SysMenu.objects.filter(parent_id=assets_root.id if assets_root else 0).values_list('id', flat=True),
    ).values_list('role_id', flat=True).distinct()
    environment_menu.path = ENVIRONMENT_PATH
    environment_menu.component = 'assets/environments/index'
    environment_menu.menu_type = 'C'
    environment_menu.perms = 'assets:applications:view'
    environment_menu.order_num = 3
    environment_menu.update_time = today
    environment_menu.save(update_fields=['parent_id', 'path', 'component', 'menu_type', 'perms', 'order_num', 'update_time'])

    for role_id in role_ids:
        SysRoleMenu.objects.get_or_create(role_id=role_id, menu_id=environment_menu.id)


def reverse_move_environment_menu(apps, schema_editor):
    SysMenu = apps.get_model('menu', 'SysMenu')
    parent = SysMenu.objects.filter(path=APPLICATION_PATH, menu_type='M').order_by('id').first()
    environment_menu = SysMenu.objects.filter(path=ENVIRONMENT_PATH).order_by('id').first()
    if environment_menu is None or parent is None:
        return
    environment_menu.parent_id = parent.id
    environment_menu.path = '/assets/applications/index'
    environment_menu.component = 'assets/application/index'
    environment_menu.menu_type = 'C'
    environment_menu.perms = 'assets:applications:view'
    environment_menu.save(update_fields=['parent_id', 'path', 'component', 'menu_type', 'perms'])


class Migration(migrations.Migration):
    dependencies = [
        ('menu', '0045_add_project_menu'),
    ]
    operations = [migrations.RunPython(move_environment_menu, reverse_move_environment_menu)]
